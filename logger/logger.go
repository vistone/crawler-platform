package logger

import (
	"fmt"
	"log"
	"os"
	"sync"
)

var (
	globalLogger Logger
	loggerOnce   sync.Once
	loggerMu     sync.RWMutex
)

func SetGlobalLogger(l Logger) {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if l == nil {
		globalLogger = &DefaultLogger{}
	} else {
		globalLogger = l
	}
}

func GetGlobalLogger() Logger {
	loggerOnce.Do(func() {
		if globalLogger == nil {
			globalLogger = &DefaultLogger{}
		}
	})
	loggerMu.RLock()
	defer loggerMu.RUnlock()
	return globalLogger
}

func InitGlobalLogger(l Logger) { SetGlobalLogger(l) }

func Debug(format string, args ...interface{}) { GetGlobalLogger().Debug(format, args...) }
func Info(format string, args ...interface{})  { GetGlobalLogger().Info(format, args...) }
func Warn(format string, args ...interface{})  { GetGlobalLogger().Warn(format, args...) }
func Error(format string, args ...interface{}) { GetGlobalLogger().Error(format, args...) }

type ConsoleLogger struct {
	debug bool
	info  bool
	warn  bool
	error bool
}

func NewConsoleLogger(debug, info, warn, error bool) *ConsoleLogger {
	return &ConsoleLogger{debug: debug, info: info, warn: warn, error: error}
}

func (l *ConsoleLogger) Debug(format string, args ...interface{}) {
	if l.debug {
		log.Printf("[DEBUG] "+format, args...)
	}
}
func (l *ConsoleLogger) Info(format string, args ...interface{}) {
	if l.info {
		log.Printf("[INFO] "+format, args...)
	}
}
func (l *ConsoleLogger) Warn(format string, args ...interface{}) {
	if l.warn {
		log.Printf("[WARN] "+format, args...)
	}
}
func (l *ConsoleLogger) Error(format string, args ...interface{}) {
	if l.error {
		log.Printf("[ERROR] "+format, args...)
	}
}

type FileLogger struct {
	file   *os.File
	logger *log.Logger
	debug  bool
	info   bool
	warn   bool
	error  bool
}

func NewFileLogger(filename string, debug, info, warn, error bool) (*FileLogger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, fmt.Errorf("无法打开日志文件 %s: %v", filename, err)
	}
	return &FileLogger{
		file:   file,
		logger: log.New(file, "", log.LstdFlags),
		debug:  debug, info: info, warn: warn, error: error,
	}, nil
}

func (l *FileLogger) Debug(format string, args ...interface{}) { if l.debug { l.logger.Printf("[DEBUG] "+format, args...) } }
func (l *FileLogger) Info(format string, args ...interface{})  { if l.info { l.logger.Printf("[INFO] "+format, args...) } }
func (l *FileLogger) Warn(format string, args ...interface{})  { if l.warn { l.logger.Printf("[WARN] "+format, args...) } }
func (l *FileLogger) Error(format string, args ...interface{}) { if l.error { l.logger.Printf("[ERROR] "+format, args...) } }
func (l *FileLogger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

type MultiLogger struct{ loggers []Logger }

func NewMultiLogger(loggers ...Logger) *MultiLogger { return &MultiLogger{loggers: loggers} }
func (l *MultiLogger) Debug(format string, args ...interface{}) {
	for _, lg := range l.loggers {
		lg.Debug(format, args...)
	}
}
func (l *MultiLogger) Info(format string, args ...interface{}) {
	for _, lg := range l.loggers {
		lg.Info(format, args...)
	}
}
func (l *MultiLogger) Warn(format string, args ...interface{}) {
	for _, lg := range l.loggers {
		lg.Warn(format, args...)
	}
}
func (l *MultiLogger) Error(format string, args ...interface{}) {
	for _, lg := range l.loggers {
		lg.Error(format, args...)
	}
}


