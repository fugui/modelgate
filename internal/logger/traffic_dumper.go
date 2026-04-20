package logger

import (
	"os"
	"path/filepath"
	"time"
)

// TrafficDumper 负责记录底层的原始流量，用于协议转换的调试与测试桩提取
type TrafficDumper struct {
	basePath string
	enabled  bool
}

// NewTrafficDumper 创建一个新的 TrafficDumper
func NewTrafficDumper(basePath string, enabled bool) *TrafficDumper {
	return &TrafficDumper{
		basePath: filepath.Join(basePath, "raw_dumps"),
		enabled:  enabled,
	}
}

// IsEnabled 检查是否开启了 Dump
func (d *TrafficDumper) IsEnabled() bool {
	return d.enabled
}

func (d *TrafficDumper) getDir(traceID string) string {
	dateStr := time.Now().Format("2006-01-02")
	return filepath.Join(d.basePath, dateStr, traceID)
}

// Dump 将数据写入对应 traceID 的目录中
func (d *TrafficDumper) Dump(traceID, filename string, payload []byte, isAppend bool) {
	if !d.enabled || traceID == "" || len(payload) == 0 {
		return
	}

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

// 预定义的一些常用的 Dump 阶段
const (
	Stage1ClientRequest     = "1_client_request.json"
	Stage2ConvertedRequest  = "2_converted_request.json"
	Stage3BackendResponse   = "3_backend_response.txt"
	Stage4ConvertedResponse = "4_converted_response.txt"
)
