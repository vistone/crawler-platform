#!/bin/bash
# 测试脚本：客户端向服务器发送1000次请求到指定URL

set -e

cd "$(dirname "$0")"

TARGET_URL="https://kh.google.com/rt/earth/PlanetoidMetadata"
PROXY_PORT="18443"
TOKEN="test-token-12345"
CONCURRENCY=50
TOTAL_REQUESTS=1000

echo "=========================================="
echo "高并发测试: 发送1000次请求"
echo "目标URL: $TARGET_URL"
echo "=========================================="
echo ""

# 检查可执行文件
if [ ! -f "./utlsProxy" ]; then
    echo "错误: utlsProxy 不存在，正在编译..."
    go build -o utlsProxy ./cmd/utlsProxy
fi

if [ ! -f "./utlsclient-cli" ]; then
    echo "错误: utlsclient-cli 不存在，正在编译..."
    go build -o utlsclient-cli ./cmd/utlsclient
fi

# 生成测试证书
if [ ! -f server.crt ] || [ ! -f server.key ]; then
    echo "生成测试证书..."
    openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes \
        -subj "/CN=localhost" 2>/dev/null || {
        echo "错误: 无法生成证书，需要安装openssl"
        exit 1
    }
    echo "证书已生成"
fi

# 清理函数
cleanup() {
    echo ""
    echo "清理资源..."
    if [ ! -z "$PROXY_PID" ]; then
        kill $PROXY_PID 2>/dev/null || true
        wait $PROXY_PID 2>/dev/null || true
    fi
    rm -f proxy.log
}
trap cleanup EXIT INT TERM

# 启动代理服务器
echo "启动utlsProxy服务器 (端口: $PROXY_PORT)..."
./utlsProxy -listen 127.0.0.1:$PROXY_PORT -token $TOKEN -cert server.crt -key server.key -log info > proxy.log 2>&1 &
PROXY_PID=$!

# 等待服务器启动
echo "等待服务器启动（3秒）..."
sleep 3

# 检查服务器是否在运行
if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "错误: 代理服务器启动失败"
    echo "服务器日志:"
    cat proxy.log
    exit 1
fi

echo "代理服务器已启动 (PID: $PROXY_PID)"
echo ""

# 运行并发测试
echo "=========================================="
echo "开始测试: 发送 $TOTAL_REQUESTS 个请求"
echo "并发数: $CONCURRENCY"
echo "目标URL: $TARGET_URL"
echo "=========================================="
echo ""

START_TIME=$(date +%s)

# 运行并发测试
if ./utlsclient-cli \
    -concurrent-test \
    -proxy 127.0.0.1:$PROXY_PORT \
    -token $TOKEN \
    -url "$TARGET_URL" \
    -concurrency $CONCURRENCY \
    -requests $TOTAL_REQUESTS \
    -timeout 60s 2>&1 | tee test_output.log; then
    TEST_RESULT="成功"
else
    TEST_RESULT="失败"
    TEST_FAILED=1
fi

END_TIME=$(date +%s)
ELAPSED=$((END_TIME - START_TIME))

echo ""
echo "=========================================="
echo "测试完成"
echo "=========================================="
echo "测试结果: $TEST_RESULT"
echo "耗时: ${ELAPSED}秒"
echo ""

# 显示统计信息
if [ -f test_output.log ]; then
    echo "测试统计:"
    grep -E "(总请求数|成功请求|失败请求|平均延迟|QPS)" test_output.log || echo "未找到统计信息"
    echo ""
fi

# 显示服务器日志摘要
echo "服务器日志摘要（最后30行）:"
tail -30 proxy.log || echo "无法读取服务器日志"

echo ""

# 退出状态
if [ -z "$TEST_FAILED" ]; then
    echo "✓ 测试通过"
    exit 0
else
    echo "✗ 测试失败"
    exit 1
fi
