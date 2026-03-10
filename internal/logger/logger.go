package logger

import (
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// defaultLogger is the package-level logger instance
	defaultLogger *zap.Logger
	// sugar is the sugared logger for easier usage
	sugar *zap.SugaredLogger
)

func init() {
	// Initialize with a basic development logger
	config := zap.NewDevelopmentConfig()
	config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	config.EncoderConfig.TimeKey = "timestamp"
	config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	config.DisableCaller = true

	logger, err := config.Build()
	if err != nil {
		// Fallback to basic logger if configuration fails
		logger = zap.New(zapcore.NewCore(
			zapcore.NewJSONEncoder(zapcore.EncoderConfig{
				TimeKey:        "timestamp",
				LevelKey:       "level",
				NameKey:        "logger",
				FunctionKey:    zapcore.OmitKey,
				MessageKey:     "msg",
				StacktraceKey:  "stacktrace",
				LineEnding:     zapcore.DefaultLineEnding,
				EncodeLevel:    zapcore.LowercaseLevelEncoder,
				EncodeTime:     zapcore.ISO8601TimeEncoder,
				EncodeDuration: zapcore.SecondsDurationEncoder,
			}),
			zapcore.AddSync(os.Stdout),
			zapcore.InfoLevel,
		))
	}

	defaultLogger = logger
	sugar = logger.Sugar()
}

// InitLogger initializes the global logger with custom configuration
func InitLogger(development bool) error {
	var config zap.Config
	if development {
		config = zap.NewDevelopmentConfig()
		config.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		config = zap.NewProductionConfig()
		config.EncoderConfig.TimeKey = "timestamp"
		config.EncoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.Format("2006-01-02T15:04:05.000Z0700"))
		}
	}
	config.DisableCaller = true

	logger, err := config.Build()
	if err != nil {
		return err
	}

	defaultLogger = logger
	sugar = logger.Sugar()
	return nil
}

// Logger returns the global logger instance
func Logger() *zap.Logger {
	return defaultLogger
}

// Sugar returns the sugared logger instance
func Sugar() *zap.SugaredLogger {
	return sugar
}

// Debug logs a debug message
func Debug(msg string, fields ...zap.Field) {
	defaultLogger.Debug(msg, fields...)
}

// Info logs an info message
func Info(msg string, fields ...zap.Field) {
	defaultLogger.Info(msg, fields...)
}

// Warn logs a warning message
func Warn(msg string, fields ...zap.Field) {
	defaultLogger.Warn(msg, fields...)
}

// Error logs an error message
func Error(msg string, fields ...zap.Field) {
	defaultLogger.Error(msg, fields...)
}

// Fatal logs a fatal message and exits
func Fatal(msg string, fields ...zap.Field) {
	defaultLogger.Fatal(msg, fields...)
}

// Debugf logs a formatted debug message
func Debugf(template string, args ...interface{}) {
	sugar.Debugf(template, args...)
}

// Infof logs a formatted info message
func Infof(template string, args ...interface{}) {
	sugar.Infof(template, args...)
}

// Warnf logs a formatted warning message
func Warnf(template string, args ...interface{}) {
	sugar.Warnf(template, args...)
}

// Errorf logs a formatted error message
func Errorf(template string, args ...interface{}) {
	sugar.Errorf(template, args...)
}

// Warnw logs a message with some additional context (key-value pairs)
func Warnw(msg string, keysAndValues ...interface{}) {
	sugar.Warnw(msg, keysAndValues...)
}

// Errorw logs a message with some additional context (key-value pairs)
func Errorw(msg string, keysAndValues ...interface{}) {
	sugar.Errorw(msg, keysAndValues...)
}

// Infow logs a message with some additional context (key-value pairs)
func Infow(msg string, keysAndValues ...interface{}) {
	sugar.Infow(msg, keysAndValues...)
}

// Fatalf logs a formatted fatal message and exits
func Fatalf(template string, args ...interface{}) {
	sugar.Fatalf(template, args...)
}

// With creates a child logger with the provided fields
func With(fields ...zap.Field) *zap.Logger {
	return defaultLogger.With(fields...)
}

// WithOptions creates a child logger with the provided options
func WithOptions(opts ...zap.Option) *zap.Logger {
	return defaultLogger.WithOptions(opts...)
}

// Sync flushes any buffered log entries
func Sync() error {
	return defaultLogger.Sync()
}

// RequestLogFields returns standard fields for request logging
func RequestLogFields(requestID, userID, modelID, backendID string) []zap.Field {
	return []zap.Field{
		zap.String("request_id", requestID),
		zap.String("user_id", userID),
		zap.String("model_id", modelID),
		zap.String("backend_id", backendID),
	}
}

// PerformanceLogFields returns standard fields for performance logging
func PerformanceLogFields(duration int64, statusCode int, requestBytes, responseBytes int64) []zap.Field {
	return []zap.Field{
		zap.Int64("duration_ms", duration),
		zap.Int("status_code", statusCode),
		zap.Int64("request_bytes", requestBytes),
		zap.Int64("response_bytes", responseBytes),
	}
}
