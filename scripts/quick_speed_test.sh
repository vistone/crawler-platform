#!/bin/bash
# TUIC 快速速度测试

cd "$(dirname "$0")"

# 清理
pkill -f "utlsProxy.*18444" 2>/dev/null || true
sleep 1

# 启动服务器
echo "启动代理服务器..."
./utlsProxy -listen 127.0.0.1:18444 -token test-token -cert server.crt -key server.key -log error > /dev/null 2>&1 &
PROXY_PID=$!
sleep 2

PROXY="127.0.0.1:18444"
TOKEN="test-token"

echo "=========================================="
echo "TUIC 高速传输测试"
echo "=========================================="
echo ""

# 测试连接复用效果
echo "【测试1】连接建立速度"
echo "第一次连接（建立新连接）:"
time ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1

echo ""
echo "第二次连接（复用连接）:"
time ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1

echo ""
echo "【测试2】并发传输 (5并发 x 3请求)"
start=$(date +%s.%N)
for i in {1..5}; do
    for j in {1..3}; do
        ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1 &
    done
done
wait
end=$(date +%s.%N)
duration=$(echo "$end - $start" | bc)
rps=$(echo "scale=2; 15 / $duration" | bc)
echo "总耗时: ${duration}秒"
echo "吞吐量: ${rps} 请求/秒"

echo ""
echo "【测试3】连续快速请求 (10个)"
start=$(date +%s.%N)
for i in {1..10}; do
    ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1 && echo -n "✓" || echo -n "✗"
done
end=$(date +%s.%N)
duration=$(echo "$end - $start" | bc)
rps=$(echo "scale=2; 10 / $duration" | bc)
echo ""
echo "总耗时: ${duration}秒"
echo "速度: ${rps} 请求/秒"

# 清理
kill $PROXY_PID 2>/dev/null || true
echo ""
echo "测试完成！"

