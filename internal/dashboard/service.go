package dashboard

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// HourlyData 每小时统计数据
type HourlyData struct {
	Count        int
	InputTokens  int64
	OutputTokens int64
}

// HourlyCounter 内存小时级计数器
// 保留今天和昨天的数据以支持跨天的 24 小时查询
type HourlyCounter struct {
	// date -> userID -> hour(0-23) -> HourlyData
	counts map[string]map[string]map[int]*HourlyData
	mu     sync.RWMutex
}

func NewHourlyCounter() *HourlyCounter {
	return &HourlyCounter{
		counts: make(map[string]map[string]map[int]*HourlyData),
	}
}

// Increment 增加指定用户当前小时的计数和 Token
func (hc *HourlyCounter) Increment(userID string, inputTokens, outputTokens int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	hour := time.Now().Hour()

	if hc.counts[today] == nil {
		hc.counts[today] = make(map[string]map[int]*HourlyData)
	}
	if hc.counts[today][userID] == nil {
		hc.counts[today][userID] = make(map[int]*HourlyData)
	}
	if hc.counts[today][userID][hour] == nil {
		hc.counts[today][userID][hour] = &HourlyData{}
	}
	hc.counts[today][userID][hour].Count++
	hc.counts[today][userID][hour].InputTokens += int64(inputTokens)
	hc.counts[today][userID][hour].OutputTokens += int64(outputTokens)
}

// GetLast24Hours 获取最近24小时的总请求数（按小时汇总）
// 正确跨天：例如当前 8:00，返回昨天 9:00 到今天 8:00 的数据
func (hc *HourlyCounter) GetLast24Hours() []HourlyStat {
	hc.mu.RLock()
	defer hc.mu.RUnlock()

	now := time.Now()
	currentHour := now.Hour()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")

	// 初始化24小时的数据（从24小时前到现在）
	stats := make([]HourlyStat, 24)
	for i := 0; i < 24; i++ {
		hour := (currentHour - 23 + i + 24) % 24
		timeStr := fmt.Sprintf("%02d:00", hour)
		stats[i] = HourlyStat{
			Hour:         timeStr,
			Requests:     0,
			InputTokens:  0,
			OutputTokens: 0,
		}
	}

	// 确定哪些小时属于昨天、哪些属于今天
	// 例如当前 08:00，那么 stats[0]=09:00(昨天), stats[1]=10:00(昨天), ..., stats[14]=23:00(昨天), stats[15]=00:00(今天), ..., stats[23]=08:00(今天)
	for i := 0; i < 24; i++ {
		hour := (currentHour - 23 + i + 24) % 24
		// 判断这个小时属于昨天还是今天
		var date string
		if i < (23 - currentHour) {
			// 属于昨天
			date = yesterday
		} else {
			// 属于今天
			date = today
		}

		// 汇总该日期所有用户在此小时的请求数和 Token
		if dayData, ok := hc.counts[date]; ok {
			for _, userData := range dayData {
				if data, ok := userData[hour]; ok {
					stats[i].Requests += data.Count
					stats[i].InputTokens += data.InputTokens
					stats[i].OutputTokens += data.OutputTokens
				}
			}
		}
	}

	return stats
}

// Service 仪表板服务
type Service struct {
	db            *sql.DB
	hourlyCounter *HourlyCounter
}

// NewService 创建仪表板服务
func NewService(db *sql.DB) *Service {
	s := &Service{
		db:            db,
		hourlyCounter: NewHourlyCounter(),
	}
	// 启动清理任务
	go s.dateCheckLoop()
	return s
}

// dateCheckLoop 每小时清理过期数据（只保留今天和昨天）
func (s *Service) dateCheckLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.hourlyCounter.mu.Lock()
		today := time.Now().Format("2006-01-02")
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		for date := range s.hourlyCounter.counts {
			if date != today && date != yesterday {
				delete(s.hourlyCounter.counts, date)
			}
		}
		s.hourlyCounter.mu.Unlock()
	}
}

// RecordHourlyStat 记录小时级统计
func (s *Service) RecordHourlyStat(userID string, inputTokens, outputTokens int) {
	s.hourlyCounter.Increment(userID, inputTokens, outputTokens)
}

// DashboardStats 系统概览
type DashboardStats struct {
	TodayTotalRequests int64   `json:"today_total_requests"`  // 今日总请求
	TodayInputTokens   int64   `json:"today_input_tokens"`    // 今日输入Token
	TodayOutputTokens  int64   `json:"today_output_tokens"`   // 今日输出Token
	ActiveUsers        int     `json:"active_users"`          // 今日活跃用户
	TotalUsers         int     `json:"total_users"`           // 总用户数
	DepartmentCount    int     `json:"department_count"`      // 部门数量
	AvgRequestsPerUser float64 `json:"avg_requests_per_user"` // 人均请求数
}

// TopUser TOP用户
type TopUser struct {
	UserID        string `json:"user_id"`
	Name          string `json:"name"`
	Department    string `json:"department"`
	RequestCount  int    `json:"request_count"`
	InputTokens   int64  `json:"input_tokens"`
	OutputTokens  int64  `json:"output_tokens"`
}

// HourlyStat 小时统计
type HourlyStat struct {
	Hour         string `json:"hour"`          // 格式: "14:00"
	Requests     int    `json:"requests"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// DepartmentStat 部门统计
type DepartmentStat struct {
	Department   string `json:"department"`
	UserCount    int    `json:"user_count"`    // 该部门用户数
	RequestCount int    `json:"request_count"` // 该部门总请求
	InputTokens  int64  `json:"input_tokens"`  // 该部门输入Token
	OutputTokens int64  `json:"output_tokens"` // 该部门输出Token
}

// ModelStat 模型统计
type ModelStat struct {
	ModelID      string `json:"model_id"`
	RequestCount int    `json:"request_count"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// GetDashboardStats 获取系统概览数据
func (s *Service) GetDashboardStats() (*DashboardStats, error) {
	today := time.Now().Format("2006-01-02")

	stats := &DashboardStats{}

	// 今日总请求数 + Token 合计
	query := `SELECT COALESCE(SUM(request_count), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0)
	          FROM quota_usage_daily WHERE date = ?`
	err := s.db.QueryRow(query, today).Scan(&stats.TodayTotalRequests, &stats.TodayInputTokens, &stats.TodayOutputTokens)
	if err != nil {
		return nil, fmt.Errorf("failed to get today total requests: %w", err)
	}

	// 今日活跃用户数（有请求记录的用户）
	query = `SELECT COUNT(DISTINCT user_id) FROM quota_usage_daily WHERE date = ?`
	err = s.db.QueryRow(query, today).Scan(&stats.ActiveUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get active users: %w", err)
	}

	// 总用户数
	query = `SELECT COUNT(*) FROM users`
	err = s.db.QueryRow(query).Scan(&stats.TotalUsers)
	if err != nil {
		return nil, fmt.Errorf("failed to get total users: %w", err)
	}

	// 部门数量（排除空部门）
	query = `SELECT COUNT(DISTINCT department) FROM users WHERE department != '' AND department IS NOT NULL`
	err = s.db.QueryRow(query).Scan(&stats.DepartmentCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get department count: %w", err)
	}

	// 计算人均请求数
	if stats.TotalUsers > 0 {
		stats.AvgRequestsPerUser = float64(stats.TodayTotalRequests) / float64(stats.TotalUsers)
	}

	return stats, nil
}

// GetTopUsers 获取今日TOP10用户
func (s *Service) GetTopUsers(limit int) ([]TopUser, error) {
	if limit <= 0 {
		limit = 10
	}

	today := time.Now().Format("2006-01-02")

	query := `
		SELECT u.id, u.name, u.department,
		       COALESCE(SUM(q.request_count), 0) as request_count,
		       COALESCE(SUM(q.input_tokens), 0) as input_tokens,
		       COALESCE(SUM(q.output_tokens), 0) as output_tokens
		FROM users u
		LEFT JOIN quota_usage_daily q ON u.id = q.user_id AND q.date = ?
		GROUP BY u.id, u.name, u.department
		ORDER BY request_count DESC
		LIMIT ?`

	rows, err := s.db.Query(query, today, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top users: %w", err)
	}
	defer rows.Close()

	var users []TopUser
	for rows.Next() {
		var user TopUser
		var userID uuid.UUID
		err := rows.Scan(&userID, &user.Name, &user.Department, &user.RequestCount, &user.InputTokens, &user.OutputTokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan top user: %w", err)
		}
		user.UserID = userID.String()
		users = append(users, user)
	}

	return users, rows.Err()
}

// GetHourlyStats 获取最近24小时每小时请求数
func (s *Service) GetHourlyStats() []HourlyStat {
	return s.hourlyCounter.GetLast24Hours()
}

// GetDepartmentStats 获取部门使用统计
func (s *Service) GetDepartmentStats() ([]DepartmentStat, error) {
	today := time.Now().Format("2006-01-02")

	query := `
		SELECT
			COALESCE(u.department, '未设置') as department,
			COUNT(DISTINCT u.id) as user_count,
			COALESCE(SUM(q.request_count), 0) as request_count,
			COALESCE(SUM(q.input_tokens), 0) as input_tokens,
			COALESCE(SUM(q.output_tokens), 0) as output_tokens
		FROM users u
		LEFT JOIN quota_usage_daily q ON u.id = q.user_id AND q.date = ?
		GROUP BY u.department
		ORDER BY request_count DESC`

	rows, err := s.db.Query(query, today)
	if err != nil {
		return nil, fmt.Errorf("failed to query department stats: %w", err)
	}
	defer rows.Close()

	var stats []DepartmentStat
	for rows.Next() {
		var stat DepartmentStat
		err := rows.Scan(&stat.Department, &stat.UserCount, &stat.RequestCount, &stat.InputTokens, &stat.OutputTokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan department stat: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetModelStats 获取模型使用分布
func (s *Service) GetModelStats() ([]ModelStat, error) {
	today := time.Now().Format("2006-01-02")

	query := `
		SELECT model_id,
		       COALESCE(SUM(request_count), 0) as request_count,
		       COALESCE(SUM(input_tokens), 0) as input_tokens,
		       COALESCE(SUM(output_tokens), 0) as output_tokens
		FROM quota_usage_daily
		WHERE date = ?
		GROUP BY model_id
		ORDER BY request_count DESC`

	rows, err := s.db.Query(query, today)
	if err != nil {
		return nil, fmt.Errorf("failed to query model stats: %w", err)
	}
	defer rows.Close()

	var stats []ModelStat
	for rows.Next() {
		var stat ModelStat
		err := rows.Scan(&stat.ModelID, &stat.RequestCount, &stat.InputTokens, &stat.OutputTokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan model stat: %w", err)
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}
