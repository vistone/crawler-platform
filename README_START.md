# 快速启动指南

## 为什么需要 `-tags with_quic`？

`-tags with_quic` 是 Go 的**构建标签**（build tags），用于在编译时决定是否包含 QUIC 相关的代码。这不是运行时配置，而是编译时配置。

- **构建标签**：在编译时决定包含哪些代码
- **配置文件**：在运行时决定程序的行为

两者是不同的概念，所以需要在编译/运行时指定构建标签。

## 简化启动方式

为了避免每次都要输入 `-tags with_quic`，我们提供了几种方式：

### 方式 1：使用启动脚本（推荐）

```bash
# 启动服务器
./start_grpcserver.sh

# 启动客户端
./start_grpcclient.sh
```

### 方式 2：使用 Makefile

```bash
# 启动服务器
make run-server

# 启动客户端
make run-client

# 构建服务器
make build-server

# 构建客户端
make build-client
```

### 方式 3：编译后运行（推荐用于生产环境）

```bash
# 编译服务器
go build -tags with_quic -o bin/grpcserver ./cmd/grpcserver

# 运行服务器
./bin/grpcserver

# 编译客户端
go build -tags with_quic -o bin/grpcclient ./cmd/grpcclient

# 运行客户端
./bin/grpcclient
```

### 方式 4：直接使用 go run（需要每次都加参数）

```bash
# 启动服务器
go run -tags with_quic ./cmd/grpcserver

# 启动客户端
go run -tags with_quic ./cmd/grpcclient
```

## 推荐使用方式

**开发环境**：使用启动脚本 `./start_grpcserver.sh` 或 `make run-server`

**生产环境**：编译后运行 `./bin/grpcserver`

## 配置文件

所有运行时配置都在 `config.toml` 中：

- 协议选择（grpc/tuic/both）
- 服务器地址和端口
- TLS 证书路径
- UUID 和密码（如果为空会自动生成）

构建标签（`-tags with_quic`）是编译时配置，不能放在配置文件中。
