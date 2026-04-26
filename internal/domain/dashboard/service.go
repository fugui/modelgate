package dashboard

import (
	"container/ring"
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
	// date -> userID -> hour(0-23) -> modelID -> HourlyData
	counts map[string]map[string]map[int]map[string]*HourlyData
	mu     sync.RWMutex
}

func NewHourlyCounter() *HourlyCounter {
	return &HourlyCounter{
		counts: make(map[string]map[string]map[int]map[string]*HourlyData),
	}
}

// Increment 增加指定用户和模型当前小时的计数和 Token
func (hc *HourlyCounter) Increment(userID string, modelID string, inputTokens, outputTokens int) {
	hc.mu.Lock()
	defer hc.mu.Unlock()

	// 确保 modelID 不为空，防止前端显示异常
	if modelID == "" {
		modelID = "unknown"
	}

	today := time.Now().Format("2006-01-02")
	hour := time.Now().Hour()

	if hc.counts[today] == nil {
		hc.counts[today] = make(map[string]map[int]map[string]*HourlyData)
	}
	if hc.counts[today][userID] == nil {
		hc.counts[today][userID] = make(map[int]map[string]*HourlyData)
	}
	if hc.counts[today][userID][hour] == nil {
		hc.counts[today][userID][hour] = make(map[string]*HourlyData)
	}
	if hc.counts[today][userID][hour][modelID] == nil {
		hc.counts[today][userID][hour][modelID] = &HourlyData{}
	}
	hc.counts[today][userID][hour][modelID].Count++
	hc.counts[today][userID][hour][modelID].InputTokens += int64(inputTokens)
	hc.counts[today][userID][hour][modelID].OutputTokens += int64(outputTokens)
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
			Models:       make(map[string]ModelHourlyStat),
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
				if hourData, ok := userData[hour]; ok {
					for modelID, data := range hourData {
						stats[i].Requests += data.Count
						stats[i].InputTokens += data.InputTokens
						stats[i].OutputTokens += data.OutputTokens

						modelStat := stats[i].Models[modelID]
						modelStat.Requests += data.Count
						modelStat.InputTokens += data.InputTokens
						modelStat.OutputTokens += data.OutputTokens
						stats[i].Models[modelID] = modelStat
					}
				}
			}
		}
	}

	return stats
}

// ConcurrencyStatsProvider 并发统计接口
type ConcurrencyStatsProvider interface {
	GetStats() map[string]interface{}
	GetAndResetIntervalPeak() int // 获取当前采样窗口内的最高并发数并重置
}

// MetricsSnapshot 5分钟级指标快照
type MetricsSnapshot struct {
	Timestamp    time.Time `json:"timestamp"`
	TimeLabel    string    `json:"time_label"`     // "HH:MM" 格式
	Concurrency  int       `json:"concurrency"`    // 瞬时并发数
	AvgLatencyMs float64   `json:"avg_latency_ms"` // 平均响应时延
	RequestCount int       `json:"request_count"`  // 该区间请求数
}

// metricsSlot 内部使用的可变槽位
type metricsSlot struct {
	timestamp     time.Time
	concurrency   int
	totalDuration int64 // 总耗时(ms)
	requestCount  int   // 请求数
}

// MetricsCollector 5分钟级指标采集器
type MetricsCollector struct {
	slots *ring.Ring // ring of *metricsSlot
	mu    sync.RWMutex
	// 当前正在累积的槽位（尚未被快照确认）
	currentSlot *metricsSlot
}

const metricsSlotCount = 288 // 24h × 12 (每5分钟一个)

// NewMetricsCollector 创建指标采集器
func NewMetricsCollector() *MetricsCollector {
	r := ring.New(metricsSlotCount)
	return &MetricsCollector{
		slots:       r,
		currentSlot: &metricsSlot{timestamp: time.Now()},
	}
}

// RecordDuration 记录一次请求的耗时（在请求完成时调用）
func (mc *MetricsCollector) RecordDuration(durationMs int64) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.currentSlot.totalDuration += durationMs
	mc.currentSlot.requestCount++
}

// SnapshotConcurrency 快照当前并发数并推进到下一个区间
func (mc *MetricsCollector) SnapshotConcurrency(concurrency int) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// 完成当前槽位
	mc.currentSlot.concurrency = concurrency

	// 存入 ring buffer
	mc.slots.Value = mc.currentSlot
	mc.slots = mc.slots.Next()

	// 开始新的槽位
	mc.currentSlot = &metricsSlot{timestamp: time.Now()}
}

// GetHistory 获取所有有效快照
func (mc *MetricsCollector) GetHistory() []MetricsSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var result []MetricsSnapshot
	mc.slots.Do(func(v interface{}) {
		if v == nil {
			return
		}
		slot := v.(*metricsSlot)
		if slot.timestamp.IsZero() {
			return
		}
		avgLatency := float64(0)
		if slot.requestCount > 0 {
			avgLatency = float64(slot.totalDuration) / float64(slot.requestCount)
		}
		result = append(result, MetricsSnapshot{
			Timestamp:    slot.timestamp,
			TimeLabel:    slot.timestamp.Format("15:04"),
			Concurrency:  slot.concurrency,
			AvgLatencyMs: avgLatency,
			RequestCount: slot.requestCount,
		})
	})

	return result
}

// Service 仪表板服务
type Service struct {
	db                 *sql.DB
	hourlyCounter      *HourlyCounter
	metricsCollector   *MetricsCollector
	concurrencyLimiter ConcurrencyStatsProvider
}

// NewService 创建仪表板服务
func NewService(db *sql.DB) *Service {
	s := &Service{
		db:               db,
		hourlyCounter:    NewHourlyCounter(),
		metricsCollector: NewMetricsCollector(),
	}
	// 启动清理任务
	go s.dateCheckLoop()
	return s
}

// SetConcurrencyLimiter 设置并发限制器引用，启动定时采样
func (s *Service) SetConcurrencyLimiter(limiter ConcurrencyStatsProvider) {
	s.concurrencyLimiter = limiter
	go s.metricsLoop()
}

// metricsLoop 每5分钟采样一次并发数
func (s *Service) metricsLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		concurrency := 0
		if s.concurrencyLimiter != nil {
			// 使用窗口峰值而非瞬时值，确保短命请求不会被遗漏
			concurrency = s.concurrencyLimiter.GetAndResetIntervalPeak()
		}
		s.metricsCollector.SnapshotConcurrency(concurrency)
	}
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
func (s *Service) RecordHourlyStat(userID string, modelID string, inputTokens, outputTokens int, durationMs int64) {
	s.hourlyCounter.Increment(userID, modelID, inputTokens, outputTokens)
	// 同时记录到5分钟级指标采集器
	if durationMs > 0 {
		s.metricsCollector.RecordDuration(durationMs)
	}
}

// GetMetricsHistory 获取最近24小时的5分钟级指标历史
func (s *Service) GetMetricsHistory() []MetricsSnapshot {
	return s.metricsCollector.GetHistory()
}

// DashboardStats 系统概览
type DashboardStats struct {
	TodayTotalRequests int64   `json:"today_total_requests"`  // 今日总请求
	TodayInputTokens   int64   `json:"today_input_tokens"`    // 今日输入Token
	TodayOutputTokens  int64   `json:"today_output_tokens"`   // 今日输出Token
	ActiveUsers        int     `json:"active_users"`          // 今日活跃用户
	TotalUsers         int     `json:"total_users"`           // 总用户数
	PeakConcurrency    int     `json:"peak_concurrency"`      // 今日最高并发
	AvgRequestsPerUser float64 `json:"avg_requests_per_user"` // 人均请求数
}

// TopUser TOP用户
type TopUser struct {
	UserID       string `json:"user_id"`
	Name         string `json:"name"`
	Department   string `json:"department"`
	RequestCount int    `json:"request_count"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
}

// ModelHourlyStat 模型级别小时统计
type ModelHourlyStat struct {
	Requests     int   `json:"requests"`
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// HourlyStat 小时统计
type HourlyStat struct {
	Hour         string                     `json:"hour"` // 格式: "14:00"
	Requests     int                        `json:"requests"`
	InputTokens  int64                      `json:"input_tokens"`
	OutputTokens int64                      `json:"output_tokens"`
	Models       map[string]ModelHourlyStat `json:"models"` // 按模型分组的统计
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

// TopUser7Days 最近7天TOP用户
type TopUser7Days struct {
	UserID        string      `json:"user_id"`
	Name          string      `json:"name"`
	Department    string      `json:"department"`
	TotalRequests int         `json:"total_requests"`
	TotalTokens   int64       `json:"total_tokens"`
	DailyStats    []DailyStat `json:"daily_stats"`
}

// DailyStat 每日明细
type DailyStat struct {
	Date         string `json:"date"`
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

	// 今日最高并发数
	if s.concurrencyLimiter != nil {
		limiterStats := s.concurrencyLimiter.GetStats()
		if peak, ok := limiterStats["peak_today"]; ok {
			if p, ok := peak.(int); ok {
				stats.PeakConcurrency = p
			}
		}
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

// GetTopUsers7Days 获取最近7天TOP20用户及其明细
func (s *Service) GetTopUsers7Days(limit int) ([]TopUser7Days, error) {
	if limit <= 0 {
		limit = 20
	}

	// 计算7天前的日期
	startDate := time.Now().AddDate(0, 0, -6).Format("2006-01-02")

	// 1. 获取最近7天 Token 总量排名前20的用户
	queryTop := `
		SELECT u.id, u.name, u.department,
		       COALESCE(SUM(q.request_count), 0) as total_requests,
		       COALESCE(SUM(q.input_tokens + q.output_tokens), 0) as total_tokens
		FROM users u
		JOIN quota_usage_daily q ON u.id = q.user_id
		WHERE q.date >= ?
		GROUP BY u.id, u.name, u.department
		ORDER BY total_tokens DESC
		LIMIT ?`

	rows, err := s.db.Query(queryTop, startDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top users 7d: %w", err)
	}
	defer rows.Close()

	var users []TopUser7Days
	userIDs := make([]string, 0)

	for rows.Next() {
		var user TopUser7Days
		var userID uuid.UUID
		err := rows.Scan(&userID, &user.Name, &user.Department, &user.TotalRequests, &user.TotalTokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan top user 7d: %w", err)
		}
		user.UserID = userID.String()
		user.DailyStats = make([]DailyStat, 0)
		users = append(users, user)
		userIDs = append(userIDs, user.UserID)
	}

	// 必须在 append 循环结束后再构建 map，否则 slice 扩容会导致指针失效
	userMap := make(map[string]*TopUser7Days)
	for i := range users {
		userMap[users[i].UserID] = &users[i]
	}

	if len(userIDs) == 0 {
		return users, nil
	}

	// 2. 获取这些用户最近7天的每日明细
	queryDaily := `
		SELECT user_id, date, SUM(request_count), SUM(input_tokens), SUM(output_tokens)
		FROM quota_usage_daily
		WHERE date >= ?
		GROUP BY user_id, date
		ORDER BY date ASC`

	rowsDaily, err := s.db.Query(queryDaily, startDate)
	if err != nil {
		return nil, fmt.Errorf("failed to query daily stats 7d: %w", err)
	}
	defer rowsDaily.Close()

	for rowsDaily.Next() {
		var userID uuid.UUID
		var stat DailyStat
		err := rowsDaily.Scan(&userID, &stat.Date, &stat.RequestCount, &stat.InputTokens, &stat.OutputTokens)
		if err != nil {
			return nil, fmt.Errorf("failed to scan daily stat 7d: %w", err)
		}

		uID := userID.String()
		if user, ok := userMap[uID]; ok {
			user.DailyStats = append(user.DailyStats, stat)
		}
	}

	return users, nil
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
