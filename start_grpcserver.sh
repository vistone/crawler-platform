#!/bin/bash

# gRPC/TUIC 服务器启动脚本
# 自动使用 with_quic 构建标签，支持真正的 TUIC 协议

cd "$(dirname "$0")"

echo "=========================================="
echo "启动 gRPC/TUIC 服务器"
echo "使用 QUIC 构建标签（支持真正的 TUIC 协议）"
echo "=========================================="
echo ""

# 使用 with_quic 构建标签运行
go run -tags with_quic ./cmd/grpcserver
