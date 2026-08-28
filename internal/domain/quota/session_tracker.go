package quota

import (
	"sync"
	"time"
)

// UserSessionStats 单个用户当日的会话统计快照
type UserSessionStats struct {
	Date              string              // "YYYY-MM-DD"
	DistinctSessions  map[string]struct{} // Set 去重集合，存储当日已见过的 session_key
	SessionRequests   int                 // 属于会话的请求次数
	NoSessionRequests int                 // 无会话的请求次数
}

// DailySessionTracker 全局内存会话统计管理器（线程安全、O(1) 复杂度）
type DailySessionTracker struct {
	// key: userID (string) -> *UserSessionStats
	stats map[string]*UserSessionStats
	mu    sync.RWMutex
}

// NewDailySessionTracker 创建每日会话跟踪器
func NewDailySessionTracker() *DailySessionTracker {
	return &DailySessionTracker{
		stats: make(map[string]*UserSessionStats),
	}
}

// RecordRequest 记录一次请求并按 session_key 进行分类与去重统计
func (t *DailySessionTracker) RecordRequest(userID string, sessionKey string, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	todayStr := now.Format("2006-01-02")
	userStat, exists := t.stats[userID]

	// 跨天或首次访问初始化
	if !exists || userStat.Date != todayStr {
		userStat = &UserSessionStats{
			Date:              todayStr,
			DistinctSessions:  make(map[string]struct{}),
			SessionRequests:   0,
			NoSessionRequests: 0,
		}
		t.stats[userID] = userStat
	}

	// 1. 无会话请求（未携带 X-Session-Id 等）
	if sessionKey == "" {
		userStat.NoSessionRequests++
		return
	}

	// 2. 属于多轮会话的请求
	userStat.SessionRequests++
	userStat.DistinctSessions[sessionKey] = struct{}{}
}

// GetUserStats 获取指定用户今日的会话统计
func (t *DailySessionTracker) GetUserStats(userID string, now time.Time) (distinctSessions, sessionReqs, noSessionReqs int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	todayStr := now.Format("2006-01-02")
	if userStat, ok := t.stats[userID]; ok && userStat.Date == todayStr {
		return len(userStat.DistinctSessions), userStat.SessionRequests, userStat.NoSessionRequests
	}
	return 0, 0, 0
}

// GetGlobalStats 获取全站今日的会话统计汇总（供数据看板调用）
func (t *DailySessionTracker) GetGlobalStats(now time.Time) (totalDistinctSessions, totalSessionReqs, totalNoSessionReqs int) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	todayStr := now.Format("2006-01-02")
	for _, userStat := range t.stats {
		if userStat.Date == todayStr {
			totalDistinctSessions += len(userStat.DistinctSessions)
			totalSessionReqs += userStat.SessionRequests
			totalNoSessionReqs += userStat.NoSessionRequests
		}
	}
	return totalDistinctSessions, totalSessionReqs, totalNoSessionReqs
}

// CleanupExpired 清理非今日的历史统计内存
func (t *DailySessionTracker) CleanupExpired(now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	todayStr := now.Format("2006-01-02")
	for uid, userStat := range t.stats {
		if userStat.Date != todayStr {
			delete(t.stats, uid)
		}
	}
}
