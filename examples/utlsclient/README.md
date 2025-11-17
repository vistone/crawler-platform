# utlsclient 使用示例

本目录包含 `utlsclient` 包的各种使用示例。

## 示例文件

### 1. example_basic_usage.go
基础使用示例，展示如何：
- 创建连接池配置
- 初始化热连接池
- 获取连接池统计信息
- 监控连接池状态

运行方式：
```bash
go run examples/utlsclient/example_basic_usage.go
```

### 2. example_utlsclient_usage.go
UTLS客户端使用示例，展示如何：
- 从配置文件加载连接池配置
- 创建热连接池
- 使用 UTLSClient 进行 HTTP 请求
- 处理响应和错误

运行方式：
```bash
go run examples/utlsclient/example_utlsclient_usage.go
```

### 3. example_hotconnpool_usage.go
热连接池完整使用示例，展示如何：
- 从TOML配置文件加载配置
- 设置依赖模块（指纹库、IP池、访问控制）
- 获取和归还连接
- 使用带验证的连接获取
- 监控和统计功能

运行方式：
```bash
go run examples/utlsclient/example_hotconnpool_usage.go
```

## 配置文件

示例中使用的配置文件 `config.toml` 应该放在项目根目录。

## 注意事项

1. 运行示例前确保已正确配置依赖
2. 某些示例需要真实的网络连接
3. 示例中的域名和URL可能需要根据实际情况调整

