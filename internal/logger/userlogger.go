package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UsageLogEntry 使用日志条目
type UsageLogEntry struct {
	Time            string                 `json:"time"`
	UserName        string                 `json:"user_name,omitempty"`
	UserEmail       string                 `json:"user_email,omitempty"`
	Model           string                 `json:"model"`
	LatencyMs       int                    `json:"latency_ms"`
	ClientIP        string                 `json:"client_ip,omitempty"`
	ClientType      string                 `json:"client_type,omitempty"`
	StatusCode      int                    `json:"status_code,omitempty"`
	Error           string                 `json:"error,omitempty"`
	BackendID       string                 `json:"backend_id,omitempty"`
	InputTokens     int                    `json:"input_tokens,omitempty"`
	OutputTokens    int                    `json:"output_tokens,omitempty"`
	TraceID         string                 `json:"trace_id,omitempty"`
	RequestPayload  map[string]interface{} `json:"request_payload,omitempty"`
	ResponsePayload string                 `json:"response_payload,omitempty"`
	OriginalTTFTMs  int64                  `json:"original_ttft_ms,omitempty"`
}

// UserLogger 按用户分文件的日志记录器
type UserLogger struct {
	basePath      string
	retentionDays int
	writers       map[string]*os.File
	mu            sync.RWMutex
}

// NewUserLogger 创建用户日志记录器
func NewUserLogger(basePath string, retentionDays int) *UserLogger {
	logger := &UserLogger{
		basePath:      basePath,
		retentionDays: retentionDays,
		writers:       make(map[string]*os.File),
	}

	// 确保日志目录存在
	os.MkdirAll(basePath, 0755)

	// 启动清理任务
	go logger.cleanupLoop()

	return logger
}

// getLogPath 获取用户的日志文件路径
func (l *UserLogger) getLogPath(userID string, date time.Time) string {
	dateStr := date.Format("2006-01-02")
	return filepath.Join(l.basePath, dateStr, userID+".jsonl")
}

// getWriter 获取或创建用户的日志写入器
func (l *UserLogger) getWriter(userID string, date time.Time) (*os.File, error) {
	dateStr := date.Format("2006-01-02")
	key := dateStr + "/" + userID

	l.mu.RLock()
	writer, exists := l.writers[key]
	l.mu.RUnlock()

	if exists {
		return writer, nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// 双重检查
	if writer, exists := l.writers[key]; exists {
		return writer, nil
	}

	// 创建日期目录
	dateDir := filepath.Join(l.basePath, dateStr)
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// 打开日志文件（追加模式）
	logPath := filepath.Join(dateDir, userID+".jsonl")
	writer, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	l.writers[key] = writer
	return writer, nil
}

// LogUsageWithDetails 记录详细的使用日志
func (l *UserLogger) LogUsageWithDetails(userID string, entry UsageLogEntry) error {
	if entry.Time == "" {
		entry.Time = time.Now().Format(time.RFC3339)
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}

	writer, err := l.getWriter(userID, time.Now())
	if err != nil {
		return err
	}

	_, err = writer.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("failed to write log entry: %w", err)
	}

	return nil
}

// Close 关闭所有日志文件
func (l *UserLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, writer := range l.writers {
		writer.Close()
	}
	l.writers = make(map[string]*os.File)
}

// cleanupLoop 定期清理旧日志
func (l *UserLogger) cleanupLoop() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	// 立即执行一次清理
	l.CleanupOldLogs()

	for range ticker.C {
		l.CleanupOldLogs()
	}
}

// CleanupOldLogs 清理超过 retentionDays 的旧日志
func (l *UserLogger) CleanupOldLogs() error {
	entries, err := os.ReadDir(l.basePath)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}

	cutoffDate := time.Now().AddDate(0, 0, -l.retentionDays)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dateStr := entry.Name()
		date, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue // 跳过非日期格式的目录
		}

		if date.Before(cutoffDate) {
			// 关闭该日期的所有 writer
			l.closeWritersForDate(dateStr)

			// 删除目录
			dateDir := filepath.Join(l.basePath, dateStr)
			if err := os.RemoveAll(dateDir); err != nil {
				// Error is logged to stderr since this is the logger package itself
				fmt.Fprintf(os.Stderr, "Failed to remove old log directory %s: %v\n", dateDir, err)
			}
		}
	}

	return nil
}

// closeWritersForDate 关闭指定日期的所有 writer
func (l *UserLogger) closeWritersForDate(dateStr string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	prefix := dateStr + "/"
	for key, writer := range l.writers {
		if len(key) > len(prefix) && key[:len(prefix)] == prefix {
			writer.Close()
			delete(l.writers, key)
		}
	}
}
