package utlsclient_test

import (
	"os"
	"testing"

	"crawler-platform/logger"
)

func TestGlobalLogger(t *testing.T) {
	// 测试默认日志记录器
	lg := logger.GetGlobalLogger()
	if lg == nil {
		t.Fatal("GetGlobalLogger should not return nil")
	}

	// 测试设置全局日志记录器
	newLogger := &logger.NopLogger{}
	logger.SetGlobalLogger(newLogger)

	currentLogger := logger.GetGlobalLogger()
	if currentLogger != newLogger {
		t.Error("SetGlobalLogger should update the global logger")
	}

	// 恢复默认
	logger.SetGlobalLogger(&logger.DefaultLogger{})
}

func TestInitGlobalLogger(t *testing.T) {
	// 测试初始化全局日志记录器
	lg := &logger.DefaultLogger{}
	logger.InitGlobalLogger(lg)

	currentLogger := logger.GetGlobalLogger()
	if currentLogger != lg {
		t.Error("InitGlobalLogger should set the global logger")
	}
}

func TestGlobalLogFunctions(t *testing.T) {
	// 设置NopLogger以避免输出
	logger.SetGlobalLogger(&logger.NopLogger{})

	// 这些调用不应该panic
	logger.Debug("测试调试信息: %s", "test")
	logger.Info("测试信息: %s", "test")
	logger.Warn("测试警告: %s", "test")
	logger.Error("测试错误: %s", "test")
}

func TestNopLogger(t *testing.T) {
	lg := &logger.NopLogger{}

	// 这些调用不应该panic或输出任何内容
	lg.Debug("debug")
	lg.Info("info")
	lg.Warn("warn")
	lg.Error("error")
}

func TestDefaultLogger(t *testing.T) {
	lg := &logger.DefaultLogger{}

	// 这些调用不应该panic
	lg.Debug("测试调试: %s", "test")
	lg.Info("测试信息: %s", "test")
	lg.Warn("测试警告: %s", "test")
	lg.Error("测试错误: %s", "test")
}

func TestConsoleLogger(t *testing.T) {
	lg := logger.NewConsoleLogger(true, true, true, true)

	if lg == nil {
		t.Fatal("NewConsoleLogger should not return nil")
	}

	// 测试各级别日志
	lg.Debug("debug message")
	lg.Info("info message")
	lg.Warn("warn message")
	lg.Error("error message")

	// 测试禁用某些级别
	logger2 := logger.NewConsoleLogger(false, true, false, true)
	logger2.Debug("should not appear")
	logger2.Info("should appear")
	logger2.Warn("should not appear")
	logger2.Error("should appear")
}

func TestFileLogger(t *testing.T) {
	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "test_log_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// 创建文件日志记录器
	lg, err := logger.NewFileLogger(tmpFile.Name(), true, true, true, true)
	if err != nil {
		t.Fatalf("Failed to create FileLogger: %v", err)
	}
	defer lg.Close()

	if lg == nil {
		t.Fatal("NewFileLogger should not return nil")
	}

	// 测试各级别日志
	lg.Debug("debug message")
	lg.Info("info message")
	lg.Warn("warn message")
	lg.Error("error message")

	// 验证文件被创建且有内容
	info, err := os.Stat(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Log file should not be empty")
	}
}

func TestMultiLogger(t *testing.T) {
	logger1 := &logger.NopLogger{}
	logger2 := &logger.NopLogger{}

	multiLogger := logger.NewMultiLogger(logger1, logger2)

	if multiLogger == nil {
		t.Fatal("NewMultiLogger should not return nil")
	}

	// 这些调用不应该panic
	multiLogger.Debug("debug")
	multiLogger.Info("info")
	multiLogger.Warn("warn")
	multiLogger.Error("error")
}

func TestFileLoggerClose(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_log_*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	lg, err := logger.NewFileLogger(tmpFile.Name(), true, true, true, true)
	if err != nil {
		t.Fatalf("Failed to create FileLogger: %v", err)
	}

	// 关闭应该成功
	if err := lg.Close(); err != nil {
		t.Errorf("Close should not return error: %v", err)
	}

	// 多次关闭会返回错误（文件已关闭），但这是预期的行为
	// 我们可以检查错误或者忽略它
	err = lg.Close()
	if err != nil {
		// 文件已关闭的错误是预期的
		t.Logf("Second Close returned error (expected): %v", err)
	}
}

