#!/bin/bash
# 真实测试脚本：测试utlsProxy和utlsclient的配合

set -e

cd "$(dirname "$0")"

echo "=========================================="
echo "真实集成测试：utlsProxy + utlsclient"
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
}
trap cleanup EXIT

# 启动代理服务器
echo "启动utlsProxy服务器..."
./utlsProxy -listen 127.0.0.1:18443 -token test-token-12345 -cert server.crt -key server.key -log info > proxy.log 2>&1 &
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

# 测试1: 单个请求
echo "=========================================="
echo "测试1: 单个HTTP请求"
echo "=========================================="
if ./utlsclient-cli -proxy 127.0.0.1:18443 -token test-token-12345 \
    -url https://www.google.com -timeout 15s 2>&1; then
    echo "✓ 测试1通过"
else
    echo "✗ 测试1失败"
    TEST1_FAILED=1
fi
echo ""

# 测试2: 并发测试（少量）
echo "=========================================="
echo "测试2: 并发测试（10个请求，并发2）"
echo "=========================================="
if ./utlsclient-cli -concurrent-test -proxy 127.0.0.1:18443 -token test-token-12345 \
    -url https://www.google.com -concurrency 2 -requests 10 -timeout 30s 2>&1; then
    echo "✓ 测试2通过"
else
    echo "✗ 测试2失败"
    TEST2_FAILED=1
fi
echo ""

# 测试3: 更多并发
echo "=========================================="
echo "测试3: 并发测试（50个请求，并发10）"
echo "=========================================="
if ./utlsclient-cli -concurrent-test -proxy 127.0.0.1:18443 -token test-token-12345 \
    -url https://www.google.com -concurrency 10 -requests 50 -timeout 30s 2>&1; then
    echo "✓ 测试3通过"
else
    echo "✗ 测试3失败"
    TEST3_FAILED=1
fi
echo ""

# 输出服务器日志
echo "=========================================="
echo "服务器日志（最后50行）:"
echo "=========================================="
tail -50 proxy.log || echo "无法读取服务器日志"

echo ""
echo "=========================================="
if [ -z "$TEST1_FAILED" ] && [ -z "$TEST2_FAILED" ] && [ -z "$TEST3_FAILED" ]; then
    echo "✓ 所有测试通过"
    exit 0
else
    echo "✗ 部分测试失败"
    exit 1
fi
