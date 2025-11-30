#!/bin/bash
# TUIC 高速传输性能测试

cd "$(dirname "$0")"

echo "=========================================="
echo "TUIC 高速传输性能测试"
echo "=========================================="
echo ""

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

# 测试1: 连接建立速度
echo "【测试1】连接建立速度"
echo "第一次连接（建立新连接）:"
time1_start=$(date +%s%N)
./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1
time1_end=$(date +%s%N)
time1=$(( (time1_end - time1_start) / 1000000 ))
echo "耗时: ${time1}ms"

echo ""
echo "第二次连接（复用连接）:"
time2_start=$(date +%s%N)
./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1
time2_end=$(date +%s%N)
time2=$(( (time2_end - time2_start) / 1000000 ))
echo "耗时: ${time2}ms"

if [ $time1 -gt 0 ] && [ $time2 -gt 0 ]; then
    speedup=$(echo "scale=2; $time1 / $time2" | bc)
    echo "连接复用加速: ${speedup}x"
fi

# 测试2: 并发性能
echo ""
echo "【测试2】并发传输测试 (10并发 x 3请求 = 30请求)"
start=$(date +%s%N)
success=0
for i in {1..10}; do
    for j in {1..3}; do
        if ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1; then
            success=$((success + 1))
        fi
    done &
done
wait
end=$(date +%s%N)
duration=$(( (end - start) / 1000000 ))
rps=$(echo "scale=2; $success * 1000 / $duration" | bc)
echo "总耗时: ${duration}ms"
echo "成功: $success/30"
echo "吞吐量: ${rps} 请求/秒"

# 测试3: 连续快速请求
echo ""
echo "【测试3】连续快速请求 (10个)"
start=$(date +%s%N)
success=0
for i in {1..10}; do
    if ./utlsclient-cli -proxy $PROXY -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1; then
        success=$((success + 1))
        echo -n "✓"
    else
        echo -n "✗"
    fi
done
end=$(date +%s%N)
duration=$(( (end - start) / 1000000 ))
rps=$(echo "scale=2; $success * 1000 / $duration" | bc)
echo ""
echo "总耗时: ${duration}ms"
echo "成功: $success/10"
echo "速度: ${rps} 请求/秒"

# 清理
kill $PROXY_PID 2>/dev/null || true
echo ""
echo "=========================================="
echo "测试完成！"
echo "=========================================="

