#!/bin/bash
# 简单的TUIC速度测试

set -e

cd "$(dirname "$0")"

echo "=========================================="
echo "TUIC 高速传输测试"
echo "=========================================="

# 清理
pkill -f "utlsProxy.*18444" 2>/dev/null || true
sleep 1

# 启动服务器
echo "启动代理服务器..."
./utlsProxy -listen 127.0.0.1:18444 -token test-token -cert server.crt -key server.key -log error > /dev/null 2>&1 &
PROXY_PID=$!
sleep 3

if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "错误: 服务器启动失败"
    exit 1
fi

PROXY="127.0.0.1:18444"
TOKEN="test-token"

echo "服务器已启动 (PID: $PROXY_PID)"
echo ""

# 测试1: 单次请求
echo "【测试1】单次请求测试"
echo "发送请求到 https://www.google.com ..."
if ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 15s 2>&1 | grep -E "(状态|状态码|响应体长度|错误|失败)" | head -3; then
    echo "✅ 测试1: 成功"
else
    echo "❌ 测试1: 失败"
fi

echo ""
echo "【测试2】连接复用测试"
echo "第一次请求:"
time ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 15s > /dev/null 2>&1

echo ""
echo "第二次请求（复用连接）:"
time ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 15s > /dev/null 2>&1

echo ""
echo "【测试3】连续请求 (3个)"
for i in 1 2 3; do
    echo -n "请求 $i: "
    if timeout 20 ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 15s > /dev/null 2>&1; then
        echo "✓ 成功"
    else
        echo "✗ 失败"
    fi
done

# 清理
kill $PROXY_PID 2>/dev/null || true
echo ""
echo "=========================================="
echo "测试完成"
echo "=========================================="

