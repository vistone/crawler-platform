#!/bin/bash

# gRPC 服务器和客户端测试脚本

set -e

echo "=== gRPC 功能测试脚本 ==="
echo ""

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 检查可执行文件
SERVER_BIN="/tmp/grpcserver"
CLIENT_BIN="/tmp/grpcclient"
TEST_BIN="/tmp/grpc_test"

echo -e "${YELLOW}1. 检查可执行文件...${NC}"
if [ ! -f "$SERVER_BIN" ]; then
    echo -e "${RED}错误: 服务器可执行文件不存在: $SERVER_BIN${NC}"
    echo "请先运行: go build -o $SERVER_BIN ./cmd/grpcserver"
    exit 1
fi

if [ ! -f "$CLIENT_BIN" ]; then
    echo -e "${RED}错误: 客户端可执行文件不存在: $CLIENT_BIN${NC}"
    echo "请先运行: go build -o $CLIENT_BIN ./cmd/grpcclient"
    exit 1
fi

if [ ! -f "$TEST_BIN" ]; then
    echo -e "${YELLOW}警告: 测试可执行文件不存在，将编译...${NC}"
    cd /home/stone/crawler-platform/test
    go build -o "$TEST_BIN" grpc_test.go
    echo -e "${GREEN}测试程序编译完成${NC}"
fi

echo -e "${GREEN}所有可执行文件检查通过${NC}"
echo ""

# 启动服务器
echo -e "${YELLOW}2. 启动 gRPC 服务器...${NC}"
$SERVER_BIN -address 0.0.0.0 -port 50051 &
SERVER_PID=$!
echo "服务器 PID: $SERVER_PID"

# 等待服务器启动
sleep 2

# 检查服务器是否运行
if ! kill -0 $SERVER_PID 2>/dev/null; then
    echo -e "${RED}错误: 服务器启动失败${NC}"
    exit 1
fi

echo -e "${GREEN}服务器启动成功${NC}"
echo ""

# 运行测试
echo -e "${YELLOW}3. 运行功能测试...${NC}"
if [ -f "$TEST_BIN" ]; then
    $TEST_BIN
    TEST_RESULT=$?
    
    if [ $TEST_RESULT -eq 0 ]; then
        echo -e "${GREEN}所有测试通过${NC}"
    else
        echo -e "${RED}测试失败，退出码: $TEST_RESULT${NC}"
    fi
else
    echo -e "${YELLOW}测试程序不存在，跳过自动化测试${NC}"
fi

echo ""

# 测试客户端连接（后台运行）
echo -e "${YELLOW}4. 启动客户端进行连接测试...${NC}"
$CLIENT_BIN -server localhost:50051 -name "test-client-1" &
CLIENT_PID=$!
echo "客户端 PID: $CLIENT_PID"

# 运行一段时间
sleep 5

echo ""
echo -e "${YELLOW}5. 清理进程...${NC}"
kill $CLIENT_PID 2>/dev/null || true
kill $SERVER_PID 2>/dev/null || true

echo -e "${GREEN}测试完成${NC}"

