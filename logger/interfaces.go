package logger

import "log"

// Logger 定义日志记录器的接口
type Logger interface {
	Debug(format string, args ...interface{})
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Error(format string, args ...interface{})
}

// NopLogger 空日志记录器
type NopLogger struct{}

func (l *NopLogger) Debug(format string, args ...interface{}) {}
func (l *NopLogger) Info(format string, args ...interface{})  {}
func (l *NopLogger) Warn(format string, args ...interface{})  {}
func (l *NopLogger) Error(format string, args ...interface{}) {}

// DefaultLogger 默认日志记录器，使用 fmt.Printf
type DefaultLogger struct{}

func (l *DefaultLogger) Debug(format string, args ...interface{}) {
	log.Printf("[DEBUG] "+format, args...)
}
func (l *DefaultLogger) Info(format string, args ...interface{})  { log.Printf("[INFO] "+format, args...) }
func (l *DefaultLogger) Warn(format string, args ...interface{})  { log.Printf("[WARN] "+format, args...) }
func (l *DefaultLogger) Error(format string, args ...interface{}) { log.Printf("[ERROR] "+format, args...) }


