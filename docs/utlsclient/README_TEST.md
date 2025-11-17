# utlsclient 测试文件说明

## 测试文件位置

根据 Go 语言的标准实践，**测试文件（`*_test.go`）必须和源代码文件放在同一个包目录下**。

因此，所有测试文件都位于 `utlsclient/` 目录中，与源代码文件在一起。

## 测试文件列表

### 核心测试
- `utlshotconnpool_test.go` - 连接池基础功能测试
- `utlshotconnpool_extended_test.go` - 连接池扩展功能测试
- `utlsclient_test.go` - HTTP客户端测试

### 模块测试
- `connection_manager_test.go` - 连接管理器测试
- `ip_access_controller_test.go` - IP访问控制器测试
- `logger_test.go` - 日志系统测试
- `utlsfingerprint_test.go` - TLS指纹库测试
- `constants_test.go` - 常量定义测试

## 运行测试

### 运行所有测试
```bash
go test ./utlsclient -v
```

### 运行特定测试
```bash
go test ./utlsclient -v -run TestNewUTLSHotConnPool
```

### 查看测试覆盖率
```bash
go test ./utlsclient -cover
```

### 生成覆盖率报告
```bash
go test ./utlsclient -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 为什么测试文件不能移动？

Go 语言的测试机制要求：
1. 测试文件必须和源代码在同一个包目录
2. 测试文件必须使用相同的包名（`package utlsclient`）
3. 测试文件可以访问包内的未导出函数和变量（在同一包内）

如果移动测试文件到其他目录，将无法：
- 访问包内的未导出函数
- 使用 `go test` 命令运行测试
- 正确导入和测试包的功能

## 相关文件位置

- **示例代码**: `examples/utlsclient/` - 使用示例
- **测试程序**: `test/utlsclient/` - 手动测试程序
- **单元测试**: `utlsclient/*_test.go` - 自动化测试（必须在此目录）

## 测试文档

详细测试说明请参考 `TEST_SUMMARY.md`。

