package logger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TrafficEntry matches the required JSONL format
type TrafficEntry struct {
	Timestamp      int64                  `json:"timestamp"`
	TraceID        string                 `json:"trace_id"`
	UserID         string                 `json:"user_id"`
	RequestPayload map[string]interface{} `json:"request_payload"`
	ResponsePayload string                `json:"response_payload,omitempty"`
	Metrics        TrafficMetrics         `json:"metrics"`
}

type TrafficMetrics struct {
	PromptTokens     int   `json:"prompt_tokens"`
	CompletionTokens int   `json:"completion_tokens"`
	OriginalTTFTMs   int64 `json:"original_ttft_ms"`
	OriginalE2EMs    int64 `json:"original_e2e_ms"`
}

type TrafficLogger struct {
	filePath string
	mu       sync.Mutex
}

var (
	defaultTrafficLogger *TrafficLogger
	trafficOnce         sync.Once
)

// GetTrafficLogger returns a global TrafficLogger instance
func GetTrafficLogger() *TrafficLogger {
	trafficOnce.Do(func() {
		defaultTrafficLogger = NewTrafficLogger("logs/traffic.jsonl")
	})
	return defaultTrafficLogger
}

func NewTrafficLogger(filePath string) *TrafficLogger {
	// Ensure directory exists
	dir := filepath.Dir(filePath)
	_ = os.MkdirAll(dir, 0755)
	return &TrafficLogger{filePath: filePath}
}

func (l *TrafficLogger) Log(entry TrafficEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal traffic entry: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Append to file
	f, err := os.OpenFile(l.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open traffic log file: %w", err)
	}
	defer f.Close()

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write traffic entry: %w", err)
	}

	return nil
}
