package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"crawler-platform/logger"
	"crawler-platform/utlsclient"
)

type IPPool struct {
	IPv4 []string `json:"ipv4"`
	IPv6 []string `json:"ipv6"`
}

func main() {
	// 关闭DEBUG日志
	logger.SetGlobalLogger(&logger.NopLogger{})

	// 测试的4个URL
	urls := []string{
		"https://kh.google.com/rt/earth/PlanetoidMetadata",
		"https://kh.google.com/rt/earth/BulkMetadata/pb=!1m2!1s!2u1003",
		"https://kh.google.com/rt/earth/NodeData/pb=!1m2!1s21!2u1002!2e1!3u1028!4b0",
		"https://kh.google.com/rt/earth/NodeData/pb=!1m2!1s12!2u1002!2e1!3u1028!4b0",
	}

	fmt.Println("=== IP池性能测试（轮询每个IP访问4个URL）===\n")

	// 1. 读取IP池
	fmt.Println("步骤1：读取IP池...")
	data, err := os.ReadFile("cmd/utlsclient/kh_google_com.json")
	if err != nil {
		fmt.Printf("❌ 失败: %v\n", err)
		return
	}

	var ipPool IPPool
	json.Unmarshal(data, &ipPool)

	// 测试IPv6（先测试少量验证连接）
	var testIPs []string
	testIPs = append(testIPs, ipPool.IPv4...)
	testIPs = append(testIPs, ipPool.IPv6...)

	fmt.Printf("✅ IP池统计: IPv4=%d, IPv6=%d\n", len(ipPool.IPv4), len(ipPool.IPv6))
	fmt.Printf("✅ 总共 %d 个IP × %d 个URL = %d 次请求\n", len(testIPs), len(urls), len(testIPs)*len(urls))
	fmt.Printf("✅ 执行方式: 先所有IP访问URL1 → 再所有IP访问URL2 → ...\n\n")

	// 2. 创建连接池
	pool := utlsclient.NewUTLSHotConnPool(nil)
	defer pool.Close()

	// 3. 预热阶段：为所有IP建立热连接
	fmt.Println("步骤2：预热阶段 - 为所有IP建立热连接...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	warmupURL := urls[0] // 使用第一个URL进行预热
	var wgWarmup sync.WaitGroup
	warmupStart := time.Now()
	warmupSuccess := 0
	warmupFail := 0
	var muWarmup sync.Mutex

	// 统计指纹和语言的多样性
	fingerprintStats := make(map[string]int)
	languageStats := make(map[string]int)
	type ConnInfo struct {
		IP          string
		Fingerprint string
		Language    string
	}
	connInfos := make([]ConnInfo, 0, len(testIPs))

	for ipIdx, targetIP := range testIPs {
		wgWarmup.Add(1)
		go func(idx int, ip string) {
			defer wgWarmup.Done()

			// 获取连接（这会建立新连接）
			conn, err := pool.GetConnectionToIP(warmupURL, ip)
			if err != nil {
				muWarmup.Lock()
				warmupFail++
				muWarmup.Unlock()
				fmt.Printf("  [预热 IP%4d] ❌ %s 连接失败: %v\n", idx+1, ip, err)
				return
			}

			// 获取指纹和语言信息
			fp := conn.Fingerprint()
			lang := conn.AcceptLanguage()

			// 发送一个简单请求验证连接
			client := utlsclient.NewUTLSClient(conn)
			client.SetTimeout(15 * time.Second)

			req, _ := http.NewRequest("GET", warmupURL, nil)
			resp, err := client.Do(req)

			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()

				muWarmup.Lock()
				warmupSuccess++

				// 记录连接信息
				connInfos = append(connInfos, ConnInfo{
					IP:          ip,
					Fingerprint: fp.Name,
					Language:    lang,
				})
				fingerprintStats[fp.Name]++
				languageStats[lang]++

				// 打印详细信息（前20个和每10个倍数打印）
				if warmupSuccess <= 20 || warmupSuccess%10 == 0 {
					fmt.Printf("  [预热 IP%4d] ✅ %-45s | 指纹: %-35s | 语言: %s\n",
						warmupSuccess, ip, fp.Name, lang)
				}

				if warmupSuccess%100 == 0 {
					fmt.Printf("  ⏳ 预热进度: %d/%d (%.1f%%)\n", warmupSuccess, len(testIPs), float64(warmupSuccess)*100/float64(len(testIPs)))
				}
				muWarmup.Unlock()
			} else {
				muWarmup.Lock()
				warmupFail++
				muWarmup.Unlock()
				fmt.Printf("  [预热 IP%4d] ❌ %s 请求失败: %v\n", idx+1, ip, err)
			}

			// 归还连接到池子
			pool.PutConnection(conn)
		}(ipIdx, targetIP)

		// 控制并发数
		if (ipIdx+1)%100 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	wgWarmup.Wait()
	warmupElapsed := time.Since(warmupStart)

	fmt.Printf("\n✅ 预热完成: 成功 %d/%d, 失败 %d, 耗时 %.1f秒\n\n",
		warmupSuccess, len(testIPs), warmupFail, warmupElapsed.Seconds())

	// 打印指纹和语言统计
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("📊 预热阶段 - TLS指纹统计（共 %d 种）：\n", len(fingerprintStats))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	for fp, count := range fingerprintStats {
		fmt.Printf("  %-45s: %4d 次 (%5.2f%%)\n", fp, count, float64(count)*100/float64(warmupSuccess))
	}

	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("🌍 预热阶段 - Accept-Language统计（共 %d 种）：\n", len(languageStats))
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// 只显示出现2次以上的语言组合
	multiUseLangs := 0
	singleUseLangs := 0
	for lang, count := range languageStats {
		if count >= 2 {
			fmt.Printf("  %s: %d次\n", lang, count)
			multiUseLangs++
		} else {
			singleUseLangs++
		}
	}
	fmt.Printf("  ... 其他单次出现的语言组合: %d种\n", singleUseLangs)
	fmt.Printf("\n✨ 语言多样性：%.1f%% 的连接使用了独特的语言组合\n", float64(singleUseLangs)*100/float64(len(languageStats)))
	fmt.Println()

	// 短暂等待，确保所有连接稳定
	time.Sleep(1 * time.Second)

	// 4. 业务请求阶段：使用后3个URL测试热连接性能
	fmt.Println("步骤3：业务请求阶段 - 测试热连接池性能...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("注意：所有IP的连接已预热，现在测试的是纯粹的热连接复用性能！\n")

	// 使用后3个URL进行测试
	businessURLs := urls[1:]
	fmt.Println("步骤2：按URL轮询所有IP...")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	type Result struct {
		IP       string
		URL      string
		Duration time.Duration
		Success  bool
		Error    string
	}

	var results []Result
	var mu sync.Mutex

	startTime := time.Now()

	// 外层循环：遍历每个业务URL（跳过预热时使用的第1个URL）
	for urlIdx, testURL := range businessURLs {
		fmt.Printf("\n════════════════════════════════════════════════════════\n")
		fmt.Printf("第 %d 轮（热连接）：所有IP访问 %s\n", urlIdx+1, testURL)
		fmt.Printf("════════════════════════════════════════════════════════\n")

		var wg sync.WaitGroup
		roundStart := time.Now()

		// 内层循环：所有IP并发访问当前URL
		for ipIdx, targetIP := range testIPs {
			wg.Add(1)
			go func(idx int, ip string, url string, round int) {
				defer wg.Done()

				// 1. 从池中获取到指定IP的连接
				reqStart := time.Now()
				conn, err := pool.GetConnectionToIP(url, ip)
				if err != nil {
					mu.Lock()
					results = append(results, Result{
						IP:      ip,
						URL:     url,
						Success: false,
						Error:   err.Error(),
					})
					mu.Unlock()
					fmt.Printf("  [轮%d IP%4d] ❌ %s 连接失败: %v\n", round, idx+1, ip, err)
					return
				}

				actualIP := conn.TargetIP()

				// 2. 使用连接发送请求
				client := utlsclient.NewUTLSClient(conn)
				client.SetTimeout(10 * time.Second)

				req, _ := http.NewRequest("GET", url, nil)
				resp, err := client.Do(req)

				elapsed := time.Since(reqStart)

				if err == nil {
					bodyLen := int64(0)
					bodyLen, _ = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()

					mu.Lock()
					results = append(results, Result{
						IP:       actualIP,
						URL:      url,
						Duration: elapsed,
						Success:  true,
					})
					mu.Unlock()

					fmt.Printf("  [轮%d IP%4d] ✅ %s | %dms | %d字节\n",
						round, idx+1, actualIP, elapsed.Milliseconds(), bodyLen)
				} else {
					mu.Lock()
					results = append(results, Result{
						IP:       actualIP,
						URL:      url,
						Duration: elapsed,
						Success:  false,
						Error:    err.Error(),
					})
					mu.Unlock()

					fmt.Printf("  [轮%d IP%4d] ❌ %s | %dms | 失败: %v\n",
						round, idx+1, actualIP, elapsed.Milliseconds(), err)
				}

				// 3. 归还连接到池子
				pool.PutConnection(conn)
			}(ipIdx, targetIP, testURL, urlIdx+1)

			// 控制并发数，避免过快
			if (ipIdx+1)%50 == 0 {
				time.Sleep(50 * time.Millisecond)
			}
		}

		// 等待当前轮次所有请求完成
		wg.Wait()
		roundElapsed := time.Since(roundStart)

		// 统计当前轮次结果
		mu.Lock()
		roundSuccess := 0
		roundFail := 0
		for i := len(results) - len(testIPs); i < len(results); i++ {
			if results[i].Success {
				roundSuccess++
			} else {
				roundFail++
			}
		}
		mu.Unlock()

		fmt.Printf("\n第 %d 轮完成: 成功 %d/%d, 失败 %d, 耗时 %.1f秒\n",
			urlIdx+1, roundSuccess, len(testIPs), roundFail, roundElapsed.Seconds())

		// 每轮之间稍作停顿
		if urlIdx < len(urls)-1 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	totalElapsed := time.Since(startTime)

	// 4. 统计结果
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("测试结果统计:")

	successCount := 0
	failCount := 0
	var totalDuration time.Duration
	minDuration := time.Hour
	maxDuration := time.Duration(0)

	// 按IP统计
	ipStats := make(map[string]struct {
		success int
		fail    int
		avgTime time.Duration
	})

	for _, r := range results {
		if r.Success {
			successCount++
			totalDuration += r.Duration
			if r.Duration < minDuration {
				minDuration = r.Duration
			}
			if r.Duration > maxDuration {
				maxDuration = r.Duration
			}

			stat := ipStats[r.IP]
			stat.success++
			stat.avgTime += r.Duration
			ipStats[r.IP] = stat
		} else {
			failCount++
			stat := ipStats[r.IP]
			stat.fail++
			ipStats[r.IP] = stat
		}
	}

	fmt.Printf("\n总体统计:\n")
	fmt.Printf("  总请求数: %d\n", len(results))
	fmt.Printf("  成功: %d (%.1f%%)\n", successCount, float64(successCount)*100/float64(len(results)))
	fmt.Printf("  失败: %d (%.1f%%)\n", failCount, float64(failCount)*100/float64(len(results)))
	fmt.Printf("  总耗时: %.1f秒\n", totalElapsed.Seconds())

	if successCount > 0 {
		avgDuration := totalDuration / time.Duration(successCount)
		fmt.Printf("\n时间统计:\n")
		fmt.Printf("  平均响应时间: %dms\n", avgDuration.Milliseconds())
		fmt.Printf("  最快响应: %dms\n", minDuration.Milliseconds())
		fmt.Printf("  最慢响应: %dms\n", maxDuration.Milliseconds())
	}

	// 显示每个IP的统计
	fmt.Printf("\n各IP统计:\n")
	ipCount := 0
	for ip, stat := range ipStats {
		ipCount++
		avgTime := time.Duration(0)
		if stat.success > 0 {
			avgTime = stat.avgTime / time.Duration(stat.success)
		}
		fmt.Printf("  %s: 成功%d 失败%d 平均%dms\n",
			ip, stat.success, stat.fail, avgTime.Milliseconds())
	}

	// 连接池状态
	stats := pool.GetStats()
	fmt.Printf("\n连接池状态:\n")
	fmt.Printf("  总连接数: %d\n", stats.TotalConnections)
	fmt.Printf("  白名单IP数: %d\n", stats.WhitelistIPs)
	fmt.Printf("  总请求数: %d\n", stats.TotalRequests)

	fmt.Println("\n✅ 测试完成！")
}
