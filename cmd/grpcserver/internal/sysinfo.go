package grpcserver

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

// SystemInfo 系统信息结构
type SystemInfo struct {
	CPUUsagePercent    float64
	MemoryUsedBytes    int64
	MemoryTotalBytes   int64
	NetworkRxBytes     int64
	NetworkTxBytes     int64
	NetworkRxPerSec    float64
	NetworkTxPerSec    float64
	DiskUsedBytes      int64
	DiskTotalBytes     int64
	Hostname           string
	SystemInfo         string
	CPUInfo            string
	MemoryInfo         string
}

var (
	lastNetStats *net.IOCountersStat
	lastNetTime  time.Time
)

// GetRealHostname 获取真实的主机名
func GetRealHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown", err
	}
	return hostname, nil
}

// GetRealSystemInfo 获取真实的操作系统信息
func GetRealSystemInfo() string {
	hostInfo, err := host.Info()
	if err != nil {
		return runtime.GOOS
	}
	return fmt.Sprintf("%s %s", hostInfo.Platform, hostInfo.PlatformVersion)
}

// GetRealCPUInfo 获取真实的 CPU 信息
func GetRealCPUInfo() string {
	cpuInfo, err := cpu.Info()
	if err != nil || len(cpuInfo) == 0 {
		return fmt.Sprintf("%s %d cores", runtime.GOARCH, runtime.NumCPU())
	}
	
	var infoParts []string
	for _, info := range cpuInfo {
		if info.ModelName != "" {
			infoParts = append(infoParts, fmt.Sprintf("%s x%d", info.ModelName, info.Cores))
			break
		}
	}
	if len(infoParts) == 0 {
		return fmt.Sprintf("%s %d cores", runtime.GOARCH, runtime.NumCPU())
	}
	return strings.Join(infoParts, ", ")
}

// GetRealMemoryInfo 获取真实的内存信息
func GetRealMemoryInfo() string {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return "Unknown Memory"
	}
	totalGB := float64(vmStat.Total) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.2f GB", totalGB)
}

// GetCPUUsage 获取 CPU 使用率（百分比）
func GetCPUUsage() (float64, error) {
	percentages, err := cpu.Percent(time.Second, false)
	if err != nil {
		return 0, err
	}
	if len(percentages) == 0 {
		return 0, fmt.Errorf("无法获取 CPU 使用率")
	}
	return percentages[0], nil
}

// GetMemoryUsage 获取内存使用情况
func GetMemoryUsage() (usedBytes int64, totalBytes int64, err error) {
	vmStat, err := mem.VirtualMemory()
	if err != nil {
		return 0, 0, err
	}
	return int64(vmStat.Used), int64(vmStat.Total), nil
}

// GetNetworkUsage 获取网络使用情况（字节/秒）
func GetNetworkUsage() (rxPerSec float64, txPerSec float64, err error) {
	ioCounters, err := net.IOCounters(false) // false 表示获取所有网络接口的聚合数据
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
		time.Sleep(1 * time.Second) // 等待1秒再次获取
		return GetNetworkUsage() // 递归调用
	}
	
	// 计算时间差
	timeDiff := now.Sub(lastNetTime).Seconds()
	if timeDiff < 0.1 {
		// 时间差太小，返回0
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

// GetDiskUsage 获取磁盘使用情况（根目录）
func GetDiskUsage() (usedBytes int64, totalBytes int64, err error) {
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

// GetAllSystemInfo 获取所有系统信息
func GetAllSystemInfo() (*SystemInfo, error) {
	info := &SystemInfo{}
	
	// 获取主机名
	hostname, err := GetRealHostname()
	if err == nil {
		info.Hostname = hostname
	}
	
	// 获取系统信息
	info.SystemInfo = GetRealSystemInfo()
	
	// 获取 CPU 信息
	info.CPUInfo = GetRealCPUInfo()
	
	// 获取内存信息
	info.MemoryInfo = GetRealMemoryInfo()
	
	// 获取 CPU 使用率
	cpuUsage, err := GetCPUUsage()
	if err == nil {
		info.CPUUsagePercent = cpuUsage
	}
	
	// 获取内存使用情况
	memUsed, memTotal, err := GetMemoryUsage()
	if err == nil {
		info.MemoryUsedBytes = memUsed
		info.MemoryTotalBytes = memTotal
	}
	
	// 获取网络使用情况
	rxPerSec, txPerSec, err := GetNetworkUsage()
	if err == nil {
		info.NetworkRxPerSec = rxPerSec
		info.NetworkTxPerSec = txPerSec
	}
	
	// 获取磁盘使用情况
	diskUsed, diskTotal, err := GetDiskUsage()
	if err == nil {
		info.DiskUsedBytes = diskUsed
		info.DiskTotalBytes = diskTotal
	}
	
	return info, nil
}

