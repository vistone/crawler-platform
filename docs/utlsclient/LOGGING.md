# 全局日志系统使用指南

## 概述

`utlsclient` 包使用统一的全局日志系统，所有代码都通过全局日志函数输出日志，确保日志输出的一致性和可配置性。

## 核心特性

- **全局统一**: 所有模块使用相同的日志系统
- **线程安全**: 支持并发环境下的日志记录
- **可配置**: 支持多种日志实现（控制台、文件、多重输出等）
- **简单易用**: 提供全局函数直接调用

## 快速开始

### 基本使用

```go
package main

import (
    "crawler-platform/utlsclient"
)

func main() {
    // 使用默认日志记录器（输出到控制台）
    utlsclient.Debug("这是一条调试信息")
    utlsclient.Info("这是一条信息")
    utlsclient.Warn("这是一条警告")
    utlsclient.Error("这是一条错误")
}
```

### 自定义日志记录器

```go
package main

import (
    "crawler-platform/utlsclient"
)

func main() {
    // 方式1: 使用控制台日志记录器
    consoleLogger := utlsclient.NewConsoleLogger(
        true,  // debug
        true,  // info
        true,  // warn
        true,  // error
    )
    utlsclient.InitGlobalLogger(consoleLogger)
    
    // 方式2: 使用文件日志记录器
    fileLogger, err := utlsclient.NewFileLogger(
        "app.log",
        true,  // debug
        true,  // info
        true,  // warn
        true,  // error
    )
    if err != nil {
        panic(err)
    }
    defer fileLogger.Close()
    utlsclient.InitGlobalLogger(fileLogger)
    
    // 方式3: 使用多重日志记录器（同时输出到控制台和文件）
    consoleLogger := utlsclient.NewConsoleLogger(true, true, true, true)
    fileLogger, _ := utlsclient.NewFileLogger("app.log", true, true, true, true)
    multiLogger := utlsclient.NewMultiLogger(consoleLogger, fileLogger)
    utlsclient.InitGlobalLogger(multiLogger)
    
    // 现在所有代码都会使用这个日志记录器
    utlsclient.Info("应用启动")
}
```

## API 参考

### 全局日志函数

所有模块都可以直接调用这些全局函数：

```go
// Debug 输出调试信息
utlsclient.Debug(format string, args ...interface{})

// Info 输出信息
utlsclient.Info(format string, args ...interface{})

// Warn 输出警告
utlsclient.Warn(format string, args ...interface{})

// Error 输出错误
utlsclient.Error(format string, args ...interface{})
```

### 全局日志管理器

```go
// InitGlobalLogger 初始化全局日志记录器（推荐在程序启动时调用）
utlsclient.InitGlobalLogger(logger Logger)

// SetGlobalLogger 设置全局日志记录器（线程安全）
utlsclient.SetGlobalLogger(logger Logger)

// GetGlobalLogger 获取全局日志记录器（线程安全）
logger := utlsclient.GetGlobalLogger()
```

### 日志记录器实现

#### DefaultLogger

默认日志记录器，使用 `fmt.Printf` 输出到标准输出：

```go
logger := &utlsclient.DefaultLogger{}
utlsclient.SetGlobalLogger(logger)
```

#### NopLogger

空日志记录器，不输出任何日志（用于禁用日志）：

```go
logger := &utlsclient.NopLogger{}
utlsclient.SetGlobalLogger(logger)
```

#### ConsoleLogger

控制台日志记录器，可以控制各级别日志的输出：

```go
logger := utlsclient.NewConsoleLogger(
    true,  // 是否输出 Debug
    true,  // 是否输出 Info
    true,  // 是否输出 Warn
    true,  // 是否输出 Error
)
utlsclient.SetGlobalLogger(logger)
```

#### FileLogger

文件日志记录器，将日志输出到文件：

```go
logger, err := utlsclient.NewFileLogger(
    "app.log",  // 日志文件路径
    true,       // 是否输出 Debug
    true,       // 是否输出 Info
    true,       // 是否输出 Warn
    true,       // 是否输出 Error
)
if err != nil {
    // 处理错误
}
defer logger.Close()
utlsclient.SetGlobalLogger(logger)
```

#### MultiLogger

多重日志记录器，可以同时输出到多个地方：

```go
consoleLogger := utlsclient.NewConsoleLogger(true, true, true, true)
fileLogger, _ := utlsclient.NewFileLogger("app.log", true, true, true, true)
multiLogger := utlsclient.NewMultiLogger(consoleLogger, fileLogger)
utlsclient.SetGlobalLogger(multiLogger)
```

## 最佳实践

### 1. 在程序启动时初始化日志

```go
func main() {
    // 在程序启动时立即初始化日志系统
    logger := utlsclient.NewConsoleLogger(true, true, true, true)
    utlsclient.InitGlobalLogger(logger)
    
    // 后续所有代码都会使用这个日志记录器
    utlsclient.Info("应用启动")
    // ...
}
```

### 2. 根据环境配置日志

```go
func initLogger(env string) {
    var logger utlsclient.Logger
    
    switch env {
    case "production":
        // 生产环境：只输出 Info、Warn、Error
        fileLogger, _ := utlsclient.NewFileLogger("app.log", false, true, true, true)
        logger = fileLogger
    case "development":
        // 开发环境：输出所有级别到控制台
        logger = utlsclient.NewConsoleLogger(true, true, true, true)
    default:
        // 测试环境：禁用日志
        logger = &utlsclient.NopLogger{}
    }
    
    utlsclient.InitGlobalLogger(logger)
}
```

### 3. 使用结构化日志格式

```go
// 推荐：使用格式化的日志
utlsclient.Info("连接已建立: host=%s, ip=%s, fingerprint=%s", 
    host, ip, fingerprint)

// 不推荐：字符串拼接
utlsclient.Info("连接已建立: " + host + ", " + ip)
```

### 4. 错误日志包含上下文

```go
if err != nil {
    utlsclient.Error("连接失败: host=%s, ip=%s, error=%v", 
        host, ip, err)
    return err
}
```

## 线程安全

全局日志系统是线程安全的，可以在多个 goroutine 中并发调用：

```go
// 多个 goroutine 可以安全地并发调用
go func() {
    utlsclient.Info("Goroutine 1")
}()

go func() {
    utlsclient.Info("Goroutine 2")
}()

go func() {
    utlsclient.Info("Goroutine 3")
}()
```

## 迁移指南

如果你之前使用实例 logger，现在需要迁移到全局日志：

### 之前（使用实例 logger）

```go
type MyStruct struct {
    logger Logger
}

func (m *MyStruct) DoSomething() {
    m.logger.Info("做某事")
}
```

### 现在（使用全局日志）

```go
type MyStruct struct {
    // 不再需要 logger 字段
}

func (m *MyStruct) DoSomething() {
    utlsclient.Info("做某事")  // 直接使用全局函数
}
```

## 注意事项

1. **初始化时机**: 建议在程序启动时立即初始化全局日志系统
2. **文件日志**: 使用 `FileLogger` 时，记得在程序退出前调用 `Close()` 方法
3. **性能考虑**: 日志输出是同步的，在高并发场景下可能影响性能，可以考虑使用异步日志实现
4. **日志级别**: 根据实际需求选择合适的日志级别，避免输出过多日志

## 示例

完整示例请参考 `example_utlsclient_usage.go` 和 `example_hotconnpool_usage.go`。

