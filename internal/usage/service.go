package usage

import (
	"time"

	"github.com/google/uuid"
	"llmgate/internal/logger"
)

// Service 使用记录服务
type Service struct {
	logger *logger.UserLogger
}

// Record 使用记录
type Record struct {
	UserID     uuid.UUID
	ModelID    string
	LatencyMs  int
	ClientIP   string
	UserAgent  string
	StatusCode int
	Error      string
	BackendID  string
}

// NewService 创建使用记录服务
func NewService(logger *logger.UserLogger) *Service {
	return &Service{
		logger: logger,
	}
}

// RecordUsageDetailed 记录详细的使用信息
func (s *Service) RecordUsageDetailed(record *Record) {
	s.logger.LogUsageWithDetails(record.UserID.String(), logger.UsageLogEntry{
		Time:       time.Now().Format(time.RFC3339),
		Model:      record.ModelID,
		LatencyMs:  record.LatencyMs,
		ClientIP:   record.ClientIP,
		UserAgent:  record.UserAgent,
		StatusCode: record.StatusCode,
		Error:      record.Error,
		BackendID:  record.BackendID,
	})
}

// CleanupOldRecords 清理旧记录（由 logger 自动处理）
func (s *Service) CleanupOldRecords() error {
	return s.logger.CleanupOldLogs()
}

// GetUsageStats 获取使用统计（简化版本）
func (s *Service) GetUsageStats(userID string, startDate, endDate time.Time) (map[string]interface{}, error) {
	// 简化处理，返回空统计
	return map[string]interface{}{
		"user_id":    userID,
		"start_date": startDate.Format("2006-01-02"),
		"end_date":   endDate.Format("2006-01-02"),
		"note":       "Stats from file logs not yet implemented",
	}, nil
}

// Flush 刷新日志（立即关闭并重开文件，确保数据写入磁盘）
func (s *Service) Flush() {
	// 在 SQLite 版本中，日志是实时写入的，不需要批量 flush
	// 但保留此方法以兼容旧代码
}
