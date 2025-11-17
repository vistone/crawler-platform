# utlsclient 测试程序

本目录包含 `utlsclient` 包的测试程序（非单元测试）。

## 测试程序

### 1. test_simple.go
简单测试程序，用于快速验证连接池和客户端的基本功能。

运行方式：
```bash
go run test/utlsclient/test_simple.go
```

### 2. test_structure.go
结构测试程序，用于验证连接池的结构和接口。

运行方式：
```bash
go run test/utlsclient/test_structure.go
```

## 与单元测试的区别

- **单元测试** (`utlsclient/*_test.go`): 使用 Go 的 testing 包，通过 `go test` 运行
- **测试程序** (`test/utlsclient/*.go`): 独立的可执行程序，用于手动测试和调试

## 注意事项

1. 测试程序需要真实的网络连接
2. 可能需要配置依赖模块才能正常运行
3. 用于开发和调试阶段的功能验证

