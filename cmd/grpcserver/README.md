# gRPC 任务管理器服务

## 概述

这是一个基于 gRPC 的任务管理器服务，支持去中心化架构，每个节点既是服务器也是客户端，可以相互通信和协作。

## 功能特性

### 1. 节点管理
- ✅ 节点注册：新节点可以加入网络
- ✅ 节点心跳：定期发送心跳保持在线状态
- ✅ 节点发现：自动发现网络中的其他节点
- ✅ 节点列表同步：维护网络中所有节点的最新状态

### 2. 客户端管理
- ✅ 客户端信息列表：获取所有已连接的客户端
- ✅ 客户端状态跟踪：实时跟踪客户端连接状态

### 3. 任务管理
- ✅ 任务提交：向客户端提交 HTTP 任务请求
- ✅ 任务类型支持：支持多种 Google Earth 数据类型任务
- ✅ 任务状态跟踪：跟踪任务执行状态

### 4. 去中心化通信
- ✅ 节点间消息传递：支持点对点和广播消息
- ✅ 消息队列：消息存储和转发
- ✅ 节点状态同步：实时同步节点状态

### 5. 实时监控
- ✅ 资源使用监控：CPU、内存、网络、硬盘使用情况
- ✅ 健康状况监控：实时上报节点健康状况
- ✅ 性能指标跟踪：跟踪各项性能指标

## 项目结构

```
cmd/grpcserver/
├── main.go                    # 服务器主程序入口
├── internal/
│   └── server.go             # 服务器实现
├── tasksmanager/
│   ├── TasksManager.pb.go    # 生成的 protobuf 消息定义
│   └── TasksManager_grpc.pb.go  # 生成的 gRPC 服务代码
└── TasksManager.proto        # protobuf 定义文件

cmd/grpcclient/
└── main.go                   # 客户端主程序

cmd/grpc_test/
└── main.go                   # 功能测试程序
```

## 使用方法

### 启动服务器

```bash
# 编译
go build -o grpcserver ./cmd/grpcserver

# 运行（默认监听 0.0.0.0:50051）
./grpcserver -address 0.0.0.0 -port 50051

# 指定地址和端口
./grpcserver -address 127.0.0.1 -port 50052
```

### 启动客户端

```bash
# 编译
go build -o grpcclient ./cmd/grpcclient

# 运行（连接到 localhost:50051）
./grpcclient -server localhost:50051 -name "my-client"
```

### 运行测试

```bash
# 编译测试程序
go build -o grpc_test ./cmd/grpc_test

# 运行测试（确保服务器正在运行）
./grpc_test

# 或使用测试脚本
./scripts/test_grpc.sh
```

## API 接口

### 1. GetTaskClientInfoList
获取所有已连接的客户端列表。

### 2. GetGrpcServerNodeInfoList
获取所有 gRPC 服务器节点列表。

### 3. SubmitTask
提交新的任务请求。

### 4. RegisterNode
新节点注册到网络。

### 5. NodeHeartbeat
发送节点心跳，更新节点状态和资源使用情况。

### 6. SendNodeMessage
节点间发送消息（支持点对点和广播）。

### 7. SyncNodeList
同步节点列表，获取需要添加、移除或更新的节点。

## 测试结果

所有功能测试均已通过：

- ✅ 基础连接测试
- ✅ 节点注册测试
- ✅ 节点心跳测试
- ✅ 节点列表同步测试
- ✅ 节点消息传递测试
- ✅ 任务提交测试
- ✅ 实时监控测试

## 配置说明

### 服务器配置

- `address`: 服务器监听地址（默认: 0.0.0.0）
- `port`: 服务器监听端口（默认: 50051）

### 客户端配置

- `server`: gRPC 服务器地址（默认: localhost:50051）
- `name`: 客户端名称（默认: client-1）

## 开发说明

### 重新生成 protobuf 代码

```bash
cd cmd/grpcserver
export PATH=$PATH:$(go env GOPATH)/bin
protoc --go_out=. --go_opt=paths=source_relative \
       --go-grpc_out=. --go-grpc_opt=paths=source_relative \
       TasksManager.proto
```

### 依赖安装

```bash
go get google.golang.org/grpc@latest
go get google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
```

## 注意事项

1. 服务器会自动将自己注册为节点
2. 节点心跳超时时间为 60 秒
3. 所有节点都是对等的，支持去中心化架构
4. 消息支持 TTL（生存时间）控制广播范围

