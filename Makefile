# Makefile for crawler-platform

.PHONY: build-server build-client run-server run-client help

# 构建服务器（带 QUIC 支持）
build-server:
	@echo "构建 gRPC/TUIC 服务器（带 QUIC 支持）..."
	go build -tags with_quic -o bin/grpcserver ./cmd/grpcserver

# 构建客户端（带 QUIC 支持）
build-client:
	@echo "构建 gRPC/TUIC 客户端（带 QUIC 支持）..."
	go build -tags with_quic -o bin/grpcclient ./cmd/grpcclient

# 运行服务器（带 QUIC 支持）
run-server:
	@echo "启动 gRPC/TUIC 服务器（带 QUIC 支持）..."
	go run -tags with_quic ./cmd/grpcserver

# 运行客户端（带 QUIC 支持）
run-client:
	@echo "启动 gRPC/TUIC 客户端（带 QUIC 支持）..."
	go run -tags with_quic ./cmd/grpcclient

# 帮助信息
help:
	@echo "可用的 make 命令:"
	@echo "  make build-server  - 构建服务器（带 QUIC 支持）"
	@echo "  make build-client  - 构建客户端（带 QUIC 支持）"
	@echo "  make run-server    - 运行服务器（带 QUIC 支持）"
	@echo "  make run-client    - 运行客户端（带 QUIC 支持）"
	@echo ""
	@echo "或者使用启动脚本:"
	@echo "  ./start_grpcserver.sh  - 启动服务器"
	@echo "  ./start_grpcclient.sh  - 启动客户端"
