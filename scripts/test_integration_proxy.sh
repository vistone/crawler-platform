#!/bin/bash
# 集成测试脚本：测试utlsProxy和utlsclient的配合

set -e

cd "$(dirname "$0")"

# 生成测试证书
if [ ! -f server.crt ] || [ ! -f server.key ]; then
    echo "生成测试证书..."
    openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes \
        -subj "/CN=localhost" 2>/dev/null || {
        echo "错误: 无法生成证书，需要安装openssl"
        exit 1
    }
fi

# 启动代理服务器（后台运行）
echo "启动utlsProxy服务器..."
./utlsProxy -listen 127.0.0.1:8443 -token test-token-12345 -cert server.crt -key server.key -log info &
PROXY_PID=$!

# 等待服务器启动
sleep 2

# 检查服务器是否在运行
if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "错误: 代理服务器启动失败"
    exit 1
fi

echo "代理服务器已启动 (PID: $PROXY_PID)"

# 运行测试
echo ""
echo "=== 开始测试 ==="

# 测试1: 单个请求
echo "测试1: 单个HTTP请求"
./utlsclient-cli -proxy 127.0.0.1:8443 -token test-token-12345 -url https://www.google.com -timeout 10s || {
    echo "测试1失败"
    kill $PROXY_PID 2>/dev/null || true
    exit 1
}

# 测试2: 并发测试（少量）
echo ""
echo "测试2: 并发测试（10个请求，并发2）"
./utlsclient-cli -concurrent-test -proxy 127.0.0.1:8443 -token test-token-12345 \
    -url https://www.google.com -concurrency 2 -requests 10 -timeout 30s || {
    echo "测试2失败"
    kill $PROXY_PID 2>/dev/null || true
    exit 1
}

# 清理
echo ""
echo "停止代理服务器..."
kill $PROXY_PID 2>/dev/null || true
wait $PROXY_PID 2>/dev/null || true

echo ""
echo "=== 测试完成 ==="
