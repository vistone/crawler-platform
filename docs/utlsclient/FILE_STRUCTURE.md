# utlsclient 文件结构说明

## 目录结构

```
utlsclient/                          # 源代码和测试文件目录
├── *.go                             # 源代码文件
├── *_test.go                        # 单元测试文件（必须在此目录）
├── *.md                             # 文档文件
└── *.go.bak                         # 备份文件（可删除）

examples/utlsclient/                 # 使用示例目录
├── example_basic_usage.go           # 基础使用示例
├── example_utlsclient_usage.go      # UTLS客户端使用示例
├── example_hotconnpool_usage.go     # 热连接池完整示例
└── README.md                        # 示例说明文档
```

## 文件分类

### 源代码文件 (`utlsclient/*.go`)
- `utlsclient.go` - HTTP客户端实现
- `utlshotconnpool.go` - 热连接池实现
- `utlsfingerprint.go` - TLS指纹库
- `connection_manager.go` - 连接管理器
- `connection_validator.go` - 连接验证器
- `health_checker.go` - 健康检查器
- `ip_access_controller.go` - IP访问控制器
- `logger.go` - 日志系统
- `interfaces.go` - 接口定义
- `constants.go` - 常量定义
- `connection_helpers.go` - 连接辅助函数

### 单元测试文件 (`utlsclient/*_test.go`)
**注意**: 测试文件必须和源代码在同一目录（Go语言要求）

- `utlshotconnpool_test.go` - 连接池基础测试
- `utlshotconnpool_extended_test.go` - 连接池扩展测试
- `utlsclient_test.go` - HTTP客户端测试
- `connection_manager_test.go` - 连接管理器测试
- `ip_access_controller_test.go` - IP访问控制器测试
- `logger_test.go` - 日志系统测试
- `utlsfingerprint_test.go` - TLS指纹库测试
- `constants_test.go` - 常量测试

### 示例文件 (`examples/utlsclient/*.go`)
- `example_basic_usage.go` - 基础使用示例
- `example_utlsclient_usage.go` - UTLS客户端示例
- `example_hotconnpool_usage.go` - 热连接池完整示例

### 测试位置
- 所有测试位于 `utlsclient/*_test.go`，使用 `go test ./utlsclient -v` 运行

### 文档文件
- `README.md` - 主要文档
- `LOGGING.md` - 日志系统使用指南
- `TEST_SUMMARY.md` - 测试总结
- `DESIGN_ISSUES.md` - 设计问题分析
- `README_TEST.md` - 测试文件说明
- `FILE_STRUCTURE.md` - 本文件

## 为什么测试文件不能移动？

Go 语言的测试机制要求：
1. **测试文件必须和源代码在同一个包目录**
2. **测试文件必须使用相同的包名**（`package utlsclient`）
3. **测试文件可以访问包内的未导出函数**（在同一包内）

如果移动测试文件到其他目录，将无法：
- 访问包内的未导出函数（如 `acquireIP`, `validateIPAccess` 等）
- 使用 `go test` 命令运行测试
- 正确导入和测试包的功能

## 文件组织原则

1. **源代码和测试**: 同一包目录（`utlsclient/`）
2. **使用示例**: `examples/utlsclient/`
3. **测试程序**: `test/utlsclient/`
4. **文档**: 各目录下的 `README.md` 或相关文档

## 运行方式

### 运行单元测试
```bash
# 在项目根目录
go test ./utlsclient -v
```

### 运行示例
```bash
# 基础示例
go run examples/utlsclient/example_basic_usage.go

# UTLS客户端示例
go run examples/utlsclient/example_utlsclient_usage.go

# 热连接池示例
go run examples/utlsclient/example_hotconnpool_usage.go
```


## 清理建议

可以删除的文件：
- `utlshotconnpool.go.bak` - 备份文件

保留的文件：
- 所有 `*_test.go` - 单元测试（必须保留）
- 所有源代码文件
- 所有文档文件

