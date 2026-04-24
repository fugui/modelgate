package logger

import (
	"os"
	"path/filepath"
	"sync"
	"time"
)

type dumpSession struct {
	mu    sync.Mutex
	files map[string][]byte
}

// TrafficDumper 负责记录底层的原始流量，用于协议转换的调试与测试桩提取
type TrafficDumper struct {
	basePath string
	mode     string // "none", "error", "full"
	sessions sync.Map
}

// NewTrafficDumper 创建一个新的 TrafficDumper
func NewTrafficDumper(basePath string, mode string) *TrafficDumper {
	if mode == "" {
		mode = "none"
	}
	return &TrafficDumper{
		basePath: filepath.Join(basePath, "raw_dumps"),
		mode:     mode,
	}
}

// IsEnabled 检查是否开启了 Dump
func (d *TrafficDumper) IsEnabled() bool {
	return d.mode == "full" || d.mode == "error"
}

func (d *TrafficDumper) getDir(traceID string) string {
	dateStr := time.Now().Format("2006-01-02")
	return filepath.Join(d.basePath, dateStr, traceID)
}

func (d *TrafficDumper) writeToDisk(traceID, filename string, payload []byte, isAppend bool) {
	dir := d.getDir(traceID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	filePath := filepath.Join(dir, filename)
	flags := os.O_WRONLY | os.O_CREATE
	if isAppend {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	f, err := os.OpenFile(filePath, flags, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	_, _ = f.Write(payload)
}

// Dump 将数据写入对应 traceID 的目录中
func (d *TrafficDumper) Dump(traceID, filename string, payload []byte, isAppend bool) {
	if traceID == "" || len(payload) == 0 {
		return
	}

	if d.mode == "full" {
		d.writeToDisk(traceID, filename, payload, isAppend)
		return
	}

	if d.mode == "error" {
		v, _ := d.sessions.LoadOrStore(traceID, &dumpSession{
			files: make(map[string][]byte),
		})
		session := v.(*dumpSession)

		session.mu.Lock()
		defer session.mu.Unlock()

		if isAppend {
			session.files[filename] = append(session.files[filename], payload...)
		} else {
			copied := make([]byte, len(payload))
			copy(copied, payload)
			session.files[filename] = copied
		}
	}
}

// FlushOrDiscard 结束一个会话，决定是落盘还是丢弃
func (d *TrafficDumper) FlushOrDiscard(traceID string, hasError bool) {
	if d.mode == "error" {
		v, loaded := d.sessions.LoadAndDelete(traceID)
		if !loaded {
			return
		}

		if hasError {
			session := v.(*dumpSession)
			session.mu.Lock()
			defer session.mu.Unlock()

			for filename, data := range session.files {
				d.writeToDisk(traceID, filename, data, false) // Buffer is complete, no append needed
			}
		}
	}
}

// 预定义的一些常用的 Dump 阶段
const (
	Stage1ClientRequest     = "1_client_request.json"
	Stage2ConvertedRequest  = "2_converted_request.json"
)
