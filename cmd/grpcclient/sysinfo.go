package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
)

var (
	lastNetStats *net.IOCountersStat
	lastNetTime  time.Time
)

// getRealSystemInfo 获取真实的系统信息
func getRealSystemInfo() (hostname string, systemInfo string, cpuInfo string, memoryInfo string, err error) {
	// 获取主机名
	hostname, err = os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// 获取系统信息
	hInfo, err := host.Info()
	if err != nil {
		systemInfo = runtime.GOOS
	} else {
		systemInfo = fmt.Sprintf("%s %s", hInfo.Platform, hInfo.PlatformVersion)
	}

	// 获取 CPU 信息
	cpuInfoList, err := cpu.Info()
	if err != nil || len(cpuInfoList) == 0 {
		cpuInfo = fmt.Sprintf("%s %d cores", runtime.GOARCH, runtime.NumCPU())
	} else {
		var infoParts []string
		for _, info := range cpuInfoList {
			if info.ModelName != "" {
				infoParts = append(infoParts, fmt.Sprintf("%s x%d", info.ModelName, info.Cores))
				break
			}
		}
		if len(infoParts) == 0 {
			cpuInfo = fmt.Sprintf("%s %d cores", runtime.GOARCH, runtime.NumCPU())
		} else {
			cpuInfo = strings.Join(infoParts, ", ")
		}
	}

	// 获取内存信息
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		memoryInfo = "Unknown Memory"
	} else {
		totalGB := float64(vmStat.Total) / (1024 * 1024 * 1024)
		memoryInfo = fmt.Sprintf("%.2f GB", totalGB)
	}

	return hostname, systemInfo, cpuInfo, memoryInfo, nil
}

// getCPUUsage 获取 CPU 使用率
func getCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}
	if len(percentages) == 0 {
		return 0, fmt.Errorf("无法获取 CPU 使用率")
	}
	return percentages[0], nil
}

// getMemoryUsage 获取内存使用情况
func getMemoryUsage() (usedBytes int64, totalBytes int64, err error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	return int64(vmStat.Used), int64(vmStat.Total), nil
}

// getNetworkUsage 获取网络使用情况（字节/秒）
func getNetworkUsage() (rxPerSec float64, txPerSec float64, err error) {
	ioCounters, err := net.IOCounters(false)
	if err != nil {
		return 0, 0, err
	}

	if len(ioCounters) == 0 {
		return 0, 0, fmt.Errorf("无法获取网络统计信息")
	}

	currentStats := ioCounters[0]
	now := time.Now()

	// 首次调用，记录初始值
	if lastNetStats == nil {
		lastNetStats = &currentStats
		lastNetTime = now
		time.Sleep(1 * time.Second)
		return getNetworkUsage() // 递归调用获取真实速度
	}

	// 计算时间差
	timeDiff := now.Sub(lastNetTime).Seconds()
	if timeDiff < 0.1 {
		return 0, 0, nil
	}

	// 计算每秒的字节数
	rxPerSec = float64(currentStats.BytesRecv-lastNetStats.BytesRecv) / timeDiff
	txPerSec = float64(currentStats.BytesSent-lastNetStats.BytesSent) / timeDiff

	// 更新上次的统计值
	lastNetStats = &currentStats
	lastNetTime = now

	return rxPerSec, txPerSec, nil
}

// getDiskUsage 获取磁盘使用情况
func getDiskUsage() (usedBytes int64, totalBytes int64, err error) {
	usage, err := disk.Usage("/")
	if err != nil {
		// 如果根目录失败，尝试当前工作目录
		usage, err = disk.Usage(".")
		if err != nil {
			return 0, 0, err
		}
	}
	return int64(usage.Used), int64(usage.Total), nil
}

