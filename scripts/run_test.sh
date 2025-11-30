#!/bin/bash

# 清理之前的日志和进程
rm -f proxy_test.log client_test.log

# 清理可能存在的旧代理服务器进程
echo "清理旧进程..."
pkill -f "utlsProxy.*18443" 2>/dev/null
sleep 1

# 确保端口未被占用
if lsof -i :18443 >/dev/null 2>&1; then
    echo "警告: 端口18443仍被占用，尝试强制清理..."
    lsof -ti :18443 | xargs kill -9 2>/dev/null
    sleep 1
fi

# 生成测试证书
TEMP_DIR=$(mktemp -d)
CERT_FILE="$TEMP_DIR/test.crt"
KEY_FILE="$TEMP_DIR/test.key"

# 使用openssl生成自签名证书
openssl req -x509 -newkey rsa:2048 -keyout "$KEY_FILE" -out "$CERT_FILE" -days 365 -nodes \
    -subj "/CN=localhost" \
    -addext "subjectAltName=IP:127.0.0.1,DNS:localhost" 2>/dev/null

if [ ! -f "$CERT_FILE" ] || [ ! -f "$KEY_FILE" ]; then
    echo "错误: 生成证书失败"
    exit 1
fi

# 启动代理服务器
echo "启动代理服务器..."
./utlsProxy \
    -listen 127.0.0.1:18443 \
    -token test-token-123 \
    -cert "$CERT_FILE" \
    -key "$KEY_FILE" \
    -log info \
    > proxy_test.log 2>&1 &

PROXY_PID=$!
echo "代理服务器已启动 (PID: $PROXY_PID)"

# 等待服务器启动
sleep 3

# 检查服务器是否在运行
if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "错误: 代理服务器启动失败"
    cat proxy_test.log
    rm -rf "$TEMP_DIR"
    exit 1
fi

# 运行客户端测试
echo "运行客户端测试..."
./utlsclient-cli \
    -proxy 127.0.0.1:18443 \
    -token test-token-123 \
    -url https://www.google.com \
    -timeout 10s \
    > client_test.log 2>&1

CLIENT_EXIT=$?

# 停止代理服务器
echo "停止代理服务器..."
kill $PROXY_PID 2>/dev/null
wait $PROXY_PID 2>/dev/null

# 清理临时文件
rm -rf "$TEMP_DIR"

# 显示结果
echo ""
echo "=== 代理服务器日志 ==="
cat proxy_test.log
echo ""
echo "=== 客户端日志 ==="
cat client_test.log
echo ""

if [ $CLIENT_EXIT -eq 0 ]; then
    echo "✅ 测试成功"
    exit 0
else
    echo "❌ 测试失败 (退出码: $CLIENT_EXIT)"
    exit 1
fi

