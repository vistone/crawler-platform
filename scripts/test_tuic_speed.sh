#!/bin/bash
# TUIC 高速传输性能测试脚本

set -e

cd "$(dirname "$0")"

echo "=========================================="
echo "TUIC 高速传输性能测试"
echo "=========================================="

# 清理旧进程
pkill -f "utlsProxy.*18444" 2>/dev/null || true
sleep 1

# 生成测试证书
if [ ! -f server.crt ] || [ ! -f server.key ]; then
    echo "生成测试证书..."
    openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes \
        -subj "/CN=localhost" 2>/dev/null
fi

# 启动代理服务器
echo ""
echo "启动代理服务器 (端口: 18444)..."
./utlsProxy -listen 127.0.0.1:18444 -token test-token-perf -cert server.crt -key server.key -log error > /tmp/proxy_perf.log 2>&1 &
PROXY_PID=$!
sleep 3

# 检查服务器是否启动
if ! kill -0 $PROXY_PID 2>/dev/null; then
    echo "错误: 代理服务器启动失败"
    cat /tmp/proxy_perf.log
    exit 1
fi

PROXY_ADDR="127.0.0.1:18444"
TOKEN="test-token-perf"

echo "代理服务器已启动 (PID: $PROXY_PID)"
echo ""

# 测试1: 连接建立速度（使用连接复用）
echo "=========================================="
echo "【测试1】连接建立速度测试（连接复用）"
echo "=========================================="

echo "第一次连接（建立连接）..."
time1_start=$(date +%s%N)
./utlsclient-cli -proxy $PROXY_ADDR -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1
time1_end=$(date +%s%N)
time1=$(( (time1_end - time1_start) / 1000000 ))
echo "第一次连接耗时: ${time1}ms"

echo ""
echo "第二次连接（复用连接）..."
time2_start=$(date +%s%N)
./utlsclient-cli -proxy $PROXY_ADDR -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1
time2_end=$(date +%s%N)
time2=$(( (time2_end - time2_start) / 1000000 ))
echo "第二次连接耗时: ${time2}ms"

speedup=$(echo "scale=2; $time1 / $time2" | bc)
echo ""
echo "连接复用加速比: ${speedup}x"
echo ""

# 测试2: 数据传输速度
echo "=========================================="
echo "【测试2】数据传输速度测试"
echo "=========================================="

echo "测试大文件传输（通过代理下载）..."
transfer_start=$(date +%s%N)
./utlsclient-cli -proxy $PROXY_ADDR -token $TOKEN -url https://www.google.com -timeout 30s > /tmp/transfer_test.out 2>&1
transfer_end=$(date +%s%N)
transfer_time=$(( (transfer_end - transfer_start) / 1000000 ))

if [ -f /tmp/transfer_test.out ]; then
    file_size=$(stat -c%s /tmp/transfer_test.out 2>/dev/null || stat -f%z /tmp/transfer_test.out 2>/dev/null || echo 0)
    if [ "$file_size" -gt 0 ]; then
        speed_mbps=$(echo "scale=2; $file_size * 8 / $transfer_time / 1000" | bc)
        echo "传输时间: ${transfer_time}ms"
        echo "数据大小: $file_size 字节"
        echo "传输速度: ${speed_mbps} Mbps"
    fi
    rm -f /tmp/transfer_test.out
fi
echo ""

# 测试3: 并发性能
echo "=========================================="
echo "【测试3】并发性能测试"
echo "=========================================="

CONCURRENCY=10
REQUESTS_PER_WORKER=5
TOTAL_REQUESTS=$((CONCURRENCY * REQUESTS_PER_WORKER))

echo "并发数: $CONCURRENCY"
echo "每并发请求数: $REQUESTS_PER_WORKER"
echo "总请求数: $TOTAL_REQUESTS"
echo ""
echo "开始测试..."

concurrent_start=$(date +%s%N)
success_count=0
fail_count=0

# 并发执行请求
for i in $(seq 1 $CONCURRENCY); do
    (
        for j in $(seq 1 $REQUESTS_PER_WORKER); do
            if ./utlsclient-cli -proxy $PROXY_ADDR -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1; then
                echo "1" >> /tmp/success_$$.txt
            else
                echo "1" >> /tmp/fail_$$.txt
            fi
        done
    ) &
done

# 等待所有后台任务完成
wait

concurrent_end=$(date +%s%N)
concurrent_time=$(( (concurrent_end - concurrent_start) / 1000000 ))

if [ -f /tmp/success_$$.txt ]; then
    success_count=$(wc -l < /tmp/success_$$.txt)
    rm -f /tmp/success_$$.txt
fi

if [ -f /tmp/fail_$$.txt ]; then
    fail_count=$(wc -l < /tmp/fail_$$.txt)
    rm -f /tmp/fail_$$.txt
fi

rps=$(echo "scale=2; $success_count * 1000 / $concurrent_time" | bc)
success_rate=$(echo "scale=2; $success_count * 100 / $TOTAL_REQUESTS" | bc)

echo ""
echo "测试完成！"
echo "总耗时: ${concurrent_time}ms"
echo "成功请求: $success_count"
echo "失败请求: $fail_count"
echo "成功率: ${success_rate}%"
echo "请求/秒: ${rps}"
echo ""

# 测试4: 持续传输测试
echo "=========================================="
echo "【测试4】持续传输测试（10个连续请求）"
echo "=========================================="

sustained_start=$(date +%s%N)
sustained_success=0

for i in $(seq 1 10); do
    if ./utlsclient-cli -proxy $PROXY_ADDR -token $TOKEN -url https://www.google.com -timeout 10s > /dev/null 2>&1; then
        sustained_success=$((sustained_success + 1))
        echo -n "✓"
    else
        echo -n "✗"
    fi
done

sustained_end=$(date +%s%N)
sustained_time=$(( (sustained_end - sustained_start) / 1000000 ))
sustained_rps=$(echo "scale=2; $sustained_success * 1000 / $sustained_time" | bc)

echo ""
echo ""
echo "持续传输测试完成"
echo "总耗时: ${sustained_time}ms"
echo "成功: $sustained_success/10"
echo "平均速度: ${sustained_rps} 请求/秒"
echo ""

# 汇总报告
echo "=========================================="
echo "性能测试汇总报告"
echo "=========================================="
echo "首次连接时间: ${time1}ms"
echo "复用连接时间: ${time2}ms"
echo "连接复用加速: ${speedup}x"
echo "并发测试 (${CONCURRENCY}x${REQUESTS_PER_WORKER}): ${rps} 请求/秒, 成功率 ${success_rate}%"
echo "持续传输: ${sustained_rps} 请求/秒"
echo ""

# 清理
echo "停止代理服务器..."
kill $PROXY_PID 2>/dev/null || true
wait $PROXY_PID 2>/dev/null || true

echo ""
echo "测试完成！"

