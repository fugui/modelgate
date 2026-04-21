package usage

import (
	"container/ring"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"modelgate/internal/infra/constants"
	"modelgate/internal/infra/logger"
	"modelgate/internal/infra/utils"
)

// AccessLog 访问日志结构
type AccessLog struct {
	UserID          uuid.UUID         `json:"user_id"`
	Method          string            `json:"method"`           // GET/POST/PUT/DELETE
	Path            string            `json:"path"`             // 访问路径
	ClientIP        string            `json:"client_ip"`        // 客户端IP
	UserAgent       string            `json:"user_agent"`       // 用户代理
	Timestamp       time.Time         `json:"timestamp"`        // 访问时间
	StatusCode      int               `json:"status_code"`      // HTTP状态码
	RequestBytes    int64             `json:"request_bytes"`    // 请求字节数
	ResponseBytes   int64             `json:"response_bytes"`   // 响应字节数
	RequestHeaders  map[string]string `json:"request_headers"`  // 请求头
	RequestBody     string            `json:"request_body"`     // 请求体（限制大小）
	ResponseHeaders map[string]string `json:"response_headers"` // 响应头
	ResponseBody    string            `json:"response_body"`    // 响应体（限制大小）
	InputTokens     int               `json:"input_tokens"`     // 请求Tokens
	OutputTokens    int               `json:"output_tokens"`    // 响应Tokens
	DurationMs      int64             `json:"duration_ms"`      // 请求持续时间(毫秒)
}

// Service 使用记录服务
type Service struct {
	logger     *logger.UserLogger
	accessLogs map[uuid.UUID]*ring.Ring // 每个用户的访问日志循环缓冲区
	logsMutex  sync.RWMutex             // 保护 accessLogs 的并发访问
	maxLogs    int                      // 每个用户最大日志条数
}

// Record 使用记录
type Record struct {
	UserID          uuid.UUID
	UserName        string
	UserEmail       string
	ModelID         string
	LatencyMs       int
	ClientIP        string
	UserAgent       string
	StatusCode      int
	Error           string
	BackendID       string
	InputTokens     int
	OutputTokens    int
	TraceID         string
	RequestPayload  map[string]interface{}
	ResponsePayload string
	TTFTMs          int64
}

// NewService 创建使用记录服务
func NewService(logger *logger.UserLogger) *Service {
	return &Service{
		logger:     logger,
		accessLogs: make(map[uuid.UUID]*ring.Ring),
		maxLogs:    20, // 每个用户最多保存20条访问记录
	}
}

// RecordUsageDetailed 记录详细的使用信息（写文件日志 + ring buffer）
func (s *Service) RecordUsageDetailed(record *Record) {
	s.logger.LogUsageWithDetails(record.UserID.String(), logger.UsageLogEntry{
		Time:            time.Now().Format(time.RFC3339),
		UserName:        record.UserName,
		UserEmail:       record.UserEmail,
		Model:           record.ModelID,
		LatencyMs:       record.LatencyMs,
		ClientIP:        record.ClientIP,
		ClientType:      utils.ParseClientType(record.UserAgent),
		StatusCode:      record.StatusCode,
		Error:           record.Error,
		BackendID:       record.BackendID,
		InputTokens:     record.InputTokens,
		OutputTokens:    record.OutputTokens,
		TraceID:         record.TraceID,
		RequestPayload:  record.RequestPayload,
		ResponsePayload: record.ResponsePayload,
		OriginalTTFTMs:  record.TTFTMs,
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

// RecordAccess 记录用户访问日志
func (s *Service) RecordAccess(userID uuid.UUID, method, path, clientIP, userAgent string, statusCode int, requestBytes, responseBytes int64, durationMs int64) {
	s.RecordAccessDetailed(userID, method, path, clientIP, userAgent, statusCode, requestBytes, responseBytes, nil, "", nil, "", 0, 0, durationMs)
}

// RecordAccessDetailed 记录用户访问日志（包含详细信息）
func (s *Service) RecordAccessDetailed(
	userID uuid.UUID,
	method, path, clientIP, userAgent string,
	statusCode int,
	requestBytes, responseBytes int64,
	requestHeaders map[string]string,
	requestBody string,
	responseHeaders map[string]string,
	responseBody string,
	inputTokens int,
	outputTokens int,
	durationMs int64,
) {
	s.logsMutex.Lock()
	defer s.logsMutex.Unlock()

	// 获取或创建用户的 ring buffer
	r, exists := s.accessLogs[userID]
	if !exists {
		r = ring.New(s.maxLogs)
		s.accessLogs[userID] = r
	}

	// 创建访问日志条目（截断大内容）
	log := AccessLog{
		UserID:          userID,
		Method:          method,
		Path:            path,
		ClientIP:        clientIP,
		UserAgent:       userAgent,
		Timestamp:       time.Now(),
		StatusCode:      statusCode,
		RequestBytes:    requestBytes,
		ResponseBytes:   responseBytes,
		RequestHeaders:  requestHeaders,
		RequestBody:     truncateString(requestBody, constants.MaxLogRequestBodySize),
		ResponseHeaders: responseHeaders,
		ResponseBody:    truncateString(responseBody, constants.MaxLogResponseBodySize),
		InputTokens:     inputTokens,
		OutputTokens:    outputTokens,
		DurationMs:      durationMs,
	}

	// 存入 ring buffer
	r.Value = log
	s.accessLogs[userID] = r.Next()
}

// truncateString 截断字符串到指定长度
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n[truncated...]"
}

// GetRecentAccess 获取用户最近访问记录（按时间倒序）
func (s *Service) GetRecentAccess(userID uuid.UUID, limit int) []AccessLog {
	s.logsMutex.RLock()
	defer s.logsMutex.RUnlock()

	r, exists := s.accessLogs[userID]
	if !exists {
		return []AccessLog{}
	}

	// 限制最大条数
	if limit > s.maxLogs {
		limit = s.maxLogs
	}
	if limit <= 0 {
		limit = s.maxLogs
	}

	var logs []AccessLog
	// 从当前位置开始遍历，收集所有非空条目
	r.Do(func(p interface{}) {
		if p != nil {
			log := p.(AccessLog)
			logs = append(logs, log)
		}
	})

	// 按时间倒序排序（最新的在前）
	for i, j := 0, len(logs)-1; i < j; i, j = i+1, j-1 {
		logs[i], logs[j] = logs[j], logs[i]
	}

	// 限制返回条数
	if len(logs) > limit {
		logs = logs[:limit]
	}

	return logs
}

// GetAllRecentAccess 获取所有用户最近的访问记录（按时间倒序）
func (s *Service) GetAllRecentAccess(limit int) []AccessLog {
	s.logsMutex.RLock()
	defer s.logsMutex.RUnlock()

	var allLogs []AccessLog

	for _, r := range s.accessLogs {
		if r == nil {
			continue
		}
		r.Do(func(p interface{}) {
			if p != nil {
				log := p.(AccessLog)
				allLogs = append(allLogs, log)
			}
		})
	}

	// 按时间倒序排序（最新的在前）
	sort.SliceStable(allLogs, func(i, j int) bool {
		return allLogs[i].Timestamp.After(allLogs[j].Timestamp)
	})

	// 限制返回条数
	if limit > 0 && len(allLogs) > limit {
		allLogs = allLogs[:limit]
	}

	return allLogs
}
