package utlsclient

import (
	"sync"
	"sync/atomic"
	"time"

	projlogger "crawler-platform/logger"
)

// DNSUpdater DNS更新器接口
// 负责定期更新域名的IP地址列表
type DNSUpdater interface {
	// Start 启动DNS更新器
	Start()

	// Stop 停止DNS更新器
	Stop()

	// Update 更新指定域名的IP列表
	// 输入: domain - 域名
	// 输出: error - 错误信息
	Update(domain string) error
}

// DefaultDNSUpdater 默认DNS更新器实现
type DefaultDNSUpdater struct {
	ipPool         IPPoolProvider
	connManager    *ConnectionManager
	pool           *UTLSHotConnPool // 用于创建新连接
	updateInterval time.Duration
	done           chan struct{}
	mu             sync.Mutex
	running        bool
	wg             sync.WaitGroup
}

// NewDefaultDNSUpdater 创建默认DNS更新器
// 输入: ipPool - IP池提供者, connManager - 连接管理器, pool - 连接池, updateInterval - 更新间隔
// 输出: *DefaultDNSUpdater - DNS更新器实例
func NewDefaultDNSUpdater(
	ipPool IPPoolProvider,
	connManager *ConnectionManager,
	pool *UTLSHotConnPool,
	updateInterval time.Duration,
) *DefaultDNSUpdater {
	return &DefaultDNSUpdater{
		ipPool:         ipPool,
		connManager:    connManager,
		pool:           pool,
		updateInterval: updateInterval,
		done:           make(chan struct{}),
	}
}

// Start 启动DNS更新器
func (d *DefaultDNSUpdater) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return
	}

	d.running = true
	d.done = make(chan struct{})
	d.wg.Add(1)
	go d.dnsUpdateLoop()
}

// Stop 停止DNS更新器
func (d *DefaultDNSUpdater) Stop() {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return
	}
	d.running = false
	close(d.done)
	d.mu.Unlock()

	d.wg.Wait()
}

// Update 更新指定域名的IP列表
func (d *DefaultDNSUpdater) Update(domain string) error {
	if d.ipPool == nil {
		return nil
	}

	// 获取新的IP列表
	newIPs := d.ipPool.GetIPsForDomain(domain)
	if len(newIPs) == 0 {
		return nil
	}

	// 检查哪些IP是新的
	var newConnections []*UTLSConnection
	for _, ip := range newIPs {
		if d.isNewIPForDomain(ip, domain) {
			// 创建新连接
			conn, err := d.pool.createNewHotConnectionWithHost(ip, domain)
			if err != nil {
				projlogger.Debug("DNS更新：创建新连接失败 %s -> %s: %v", domain, ip, err)
				continue
			}

			newConnections = append(newConnections, conn)
		}
	}

	// 将新连接加入连接池并更新统计
	for _, conn := range newConnections {
		d.pool.PutConnection(conn)
		atomic.AddInt64(&d.pool.stats.NewConnectionsFromDNS, 1)
		projlogger.Debug("DNS更新：添加新连接 %s -> %s", domain, conn.targetIP)
	}

	return nil
}

// dnsUpdateLoop DNS更新循环
func (d *DefaultDNSUpdater) dnsUpdateLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.updateInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 立即检查done通道，确保快速响应停止信号
			select {
			case <-d.done:
				return
			default:
			}
			d.performDNSUpdate()
		case <-d.done:
			return
		}
	}
}

// performDNSUpdate 执行DNS更新
func (d *DefaultDNSUpdater) performDNSUpdate() {
	if d.ipPool == nil {
		return
	}

	// 获取所有已知的域名
	hostMapping := d.connManager.GetHostMapping()
	var domains []string
	for host := range hostMapping {
		domains = append(domains, host)
	}

	if len(domains) == 0 {
		return
	}

	// 并发更新每个域名的IP
	var wg sync.WaitGroup
	for _, domain := range domains {
		wg.Add(1)
		go func(domainName string) {
			defer wg.Done()
			if err := d.Update(domainName); err != nil {
				projlogger.Debug("DNS更新失败 %s: %v", domainName, err)
			}
		}(domain)
	}
	wg.Wait()
}

// isNewIPForDomain 检查IP是否是该域名的新IP
func (d *DefaultDNSUpdater) isNewIPForDomain(ip, domain string) bool {
	// 检查连接是否已存在
	if d.connManager.GetConnection(ip) != nil {
		return false
	}

	// 检查域名映射中是否已包含该IP
	if ipList, exists := d.connManager.GetHostMapping()[domain]; exists {
		for _, existingIP := range ipList {
			if existingIP == ip {
				return false
			}
		}
	}

	return true
}
