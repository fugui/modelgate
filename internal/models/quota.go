package models

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type QuotaPolicy struct {
	Name              string      `json:"name"`
	RateLimit         int         `json:"rate_limit"`
	RateLimitWindow   int         `json:"rate_limit_window"`
	RequestQuotaDaily int         `json:"request_quota_daily"`
	Models            StringArray `json:"models"`
	Description       string      `json:"description"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

type QuotaUsageDaily struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Date          time.Time `json:"date"`
	ModelID       string    `json:"model_id"`
	RequestCount  int       `json:"request_count"`
}

type QuotaCheckResult struct {
	Allowed           bool   `json:"allowed"`
	Reason            string `json:"reason,omitempty"`
	DailyRequests     int    `json:"daily_requests"`
	DailyRequestLimit int    `json:"daily_request_limit"`
	RateRemaining     int    `json:"rate_remaining"`
	RateLimit         int    `json:"rate_limit"`
}

type UsageStats struct {
	TotalRequests int `json:"total_requests"`
	AvgLatencyMs  int `json:"avg_latency_ms"`
	ErrorCount    int `json:"error_count"`
}

// QuotaStore 配额数据访问层
type QuotaStore struct {
	db *sql.DB
}

func NewQuotaStore(db *sql.DB) *QuotaStore {
	return &QuotaStore{db: db}
}

func (s *QuotaStore) GetPolicy(name string) (*QuotaPolicy, error) {
	policy := &QuotaPolicy{}
	query := `
		SELECT name, rate_limit, rate_limit_window, request_quota_daily, models, description, created_at, updated_at
		FROM quota_policies WHERE name = ?`

	var modelsJSON string
	err := s.db.QueryRow(query, name).Scan(
		&policy.Name, &policy.RateLimit, &policy.RateLimitWindow,
		&policy.RequestQuotaDaily, &modelsJSON, &policy.Description,
		&policy.CreatedAt, &policy.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal([]byte(modelsJSON), &policy.Models)
	return policy, nil
}

func (s *QuotaStore) ListPolicies() ([]*QuotaPolicy, error) {
	query := `
		SELECT name, rate_limit, rate_limit_window, request_quota_daily, models, description, created_at, updated_at
		FROM quota_policies ORDER BY name`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*QuotaPolicy
	for rows.Next() {
		policy := &QuotaPolicy{}
		var modelsJSON string
		err := rows.Scan(
			&policy.Name, &policy.RateLimit, &policy.RateLimitWindow,
			&policy.RequestQuotaDaily, &modelsJSON, &policy.Description,
			&policy.CreatedAt, &policy.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(modelsJSON), &policy.Models)
		policies = append(policies, policy)
	}
	return policies, rows.Err()
}

func (s *QuotaStore) CreateOrUpdatePolicy(policy *QuotaPolicy) error {
	query := `
		INSERT INTO quota_policies (name, rate_limit, rate_limit_window, request_quota_daily, models, description)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET
			rate_limit = excluded.rate_limit,
			rate_limit_window = excluded.rate_limit_window,
			request_quota_daily = excluded.request_quota_daily,
			models = excluded.models,
			description = excluded.description,
			updated_at = CURRENT_TIMESTAMP
		RETURNING created_at, updated_at`

	modelsJSON, _ := json.Marshal(policy.Models)
	return s.db.QueryRow(query,
		policy.Name, policy.RateLimit, policy.RateLimitWindow,
		policy.RequestQuotaDaily, string(modelsJSON), policy.Description,
	).Scan(&policy.CreatedAt, &policy.UpdatedAt)
}

func (s *QuotaStore) DeletePolicy(name string) error {
	_, err := s.db.Exec("DELETE FROM quota_policies WHERE name = ?", name)
	return err
}

// GetDailyRequestCount 获取用户当天的请求次数
func (s *QuotaStore) GetDailyRequestCount(userID uuid.UUID, date time.Time) (int, error) {
	var total int
	query := `
		SELECT COALESCE(SUM(request_count), 0)
		FROM quota_usage_daily
		WHERE user_id = ? AND date = ?`

	err := s.db.QueryRow(query, userID.String(), date.Format("2006-01-02")).Scan(&total)
	return total, err
}

// IncrementRequestCount 增加请求计数
func (s *QuotaStore) IncrementRequestCount(userID uuid.UUID, modelID string) error {
	query := `
		INSERT INTO quota_usage_daily (id, user_id, date, model_id, request_count)
		VALUES (?, ?, ?, ?, 1)
		ON CONFLICT(user_id, date, model_id) DO UPDATE SET
			request_count = request_count + 1`

	id := uuid.New().String()
	_, err := s.db.Exec(query, id, userID.String(), time.Now().Format("2006-01-02"), modelID)
	return err
}

// GetUsageStats 获取使用统计
func (s *QuotaStore) GetUsageStats(userID uuid.UUID, startDate, endDate time.Time) (*UsageStats, error) {
	stats := &UsageStats{}
	query := `
		SELECT
			COALESCE(SUM(request_count), 0)
		FROM quota_usage_daily
		WHERE user_id = ? AND date BETWEEN ? AND ?`

	err := s.db.QueryRow(query, userID.String(), startDate.Format("2006-01-02"), endDate.Format("2006-01-02")).Scan(
		&stats.TotalRequests,
	)
	return stats, err
}

// GetDailyUsageList 获取用户指定日期范围内的每日使用列表
func (s *QuotaStore) GetDailyUsageList(userID uuid.UUID, startDate, endDate time.Time) ([]*QuotaUsageDaily, error) {
	query := `
		SELECT id, user_id, date, model_id, request_count
		FROM quota_usage_daily
		WHERE user_id = ? AND date BETWEEN ? AND ?
		ORDER BY date DESC, model_id`

	rows, err := s.db.Query(query, userID.String(), startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var usages []*QuotaUsageDaily
	for rows.Next() {
		usage := &QuotaUsageDaily{}
		var idStr, userIDStr string
		err := rows.Scan(
			&idStr, &userIDStr, &usage.Date, &usage.ModelID,
			&usage.RequestCount,
		)
		if err != nil {
			return nil, err
		}
		usage.ID = uuid.MustParse(idStr)
		usage.UserID = uuid.MustParse(userIDStr)
		usages = append(usages, usage)
	}
	return usages, rows.Err()
}

// GetRecentUsageRecords 获取最近的使用记录（按天汇总）
func (s *QuotaStore) GetRecentUsageRecords(userID uuid.UUID, days int) ([]map[string]interface{}, error) {
	endDate := time.Now()
	startDate := endDate.AddDate(0, 0, -days+1)

	query := `
		SELECT 
			date,
			COALESCE(SUM(request_count), 0) as requests
		FROM quota_usage_daily
		WHERE user_id = ? AND date >= ?
		GROUP BY date
		ORDER BY date DESC`

	rows, err := s.db.Query(query, userID.String(), startDate.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []map[string]interface{}
	for rows.Next() {
		var date time.Time
		var requests int
		err := rows.Scan(&date, &requests)
		if err != nil {
			return nil, err
		}
		records = append(records, map[string]interface{}{
			"date":     date.Format("2006-01-02"),
			"requests": requests,
		})
	}
	return records, rows.Err()
}
