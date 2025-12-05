package utlsclient

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"

	projlogger "crawler-platform/logger"
)

// RemoteIPPool 定义了远程IP池的接口。
// 在本项目中，推荐的实现是基于 ./data/domain_ips/ 目录下的域名-IP 池：
// - 由 DomainMonitor 周期性写入各个域名对应的 IP 列表到该目录
// - RemoteIPPool 实现只需要按域名聚合这些 IP，并在内存中返回快照
type RemoteIPPool interface {
	GetAllDomainIPs() map[string][]string
}

// PoolManager 是主动连接池管理器。
type PoolManager struct {
	connManager *ConnectionManager
	blacklist   *Blacklist
	validator   Validator
	config      *PoolConfig
	remotePool  RemoteIPPool

	stopChan    chan struct{}
	wg          sync.WaitGroup
	stopOnce    sync.Once // 确保Stop只执行一次
	initialized bool      // 标记是否已完成初始化
	initOnce    sync.Once // 确保初始化只执行一次
}

// NewPoolManager 创建一个新的池管理器。
func NewPoolManager(
	remotePool RemoteIPPool,
	connManager *ConnectionManager,
	blacklist *Blacklist,
	validator Validator,
	config *PoolConfig,
) *PoolManager {
	return &PoolManager{
		remotePool:  remotePool,
		connManager: connManager,
		blacklist:   blacklist,
		validator:   validator,
		config:      config,
		stopChan:    make(chan struct{}),
	}
}

// Start 启动池管理器的后台维护循环。
func (pm *PoolManager) Start() {
	pm.wg.Add(1)
	go pm.maintenanceLoop()
	projlogger.Info("连接池管理器已启动")
}

// Stop 停止后台维护循环。
func (pm *PoolManager) Stop() {
	pm.stopOnce.Do(func() {
		close(pm.stopChan)
		pm.wg.Wait()
		projlogger.Info("连接池管理器已停止")
	})
}

func (pm *PoolManager) maintenanceLoop() {
	defer pm.wg.Done()

	// 初始化：从 RemoteIPPool 获取所有 IP 进行预热
	pm.initOnce.Do(func() {
		pm.maintainPoolFromRemoteIPs(true) // true 表示初始化模式
		pm.initialized = true
	})

	// 预热定时器
	preWarmTicker := time.NewTicker(pm.config.PreWarmInterval)
	defer preWarmTicker.Stop()

	// 黑名单恢复检查定时器（如果配置了检查间隔）
	var blacklistTicker *time.Ticker
	var blacklistTickerChan <-chan time.Time
	if pm.config.BlacklistCheckInterval > 0 {
		blacklistTicker = time.NewTicker(pm.config.BlacklistCheckInterval)
		defer blacklistTicker.Stop()
		blacklistTickerChan = blacklistTicker.C
		// 启动后立即检查一次
		pm.checkBlacklistRecovery()
	}

	for {
		select {
		case <-preWarmTicker.C:
			// 定时维护：只维护白名单中已有的连接，不再从 RemoteIPPool 获取新 IP
			pm.maintainPoolFromWhitelist()
		case <-blacklistTickerChan:
			pm.checkBlacklistRecovery()
		case <-pm.stopChan:
			return
		}
	}
}

// maintainPoolFromRemoteIPs 从 RemoteIPPool 获取 IP 进行预热（用于初始化）
// isInitial 为 true 表示是初始化阶段，false 表示是定时维护（但当前已改为只维护白名单）
func (pm *PoolManager) maintainPoolFromRemoteIPs(isInitial bool) {
	allDomainIPs := pm.remotePool.GetAllDomainIPs()
	if len(allDomainIPs) == 0 {
		projlogger.Debug("远程IP池为空，跳过本次维护")
		return
	}

	if isInitial {
		projlogger.Info("开始初始化连接池，从 RemoteIPPool 获取所有 IP，目标数量: %d 个域", len(allDomainIPs))
	} else {
		projlogger.Info("开始维护连接池，从 RemoteIPPool 获取 IP，目标数量: %d 个域", len(allDomainIPs))
	}

	var wg sync.WaitGroup
	var successCount int64 // 成功加入热连接池的连接数
	concurrencyLimit := make(chan struct{}, pm.config.MaxConcurrentPreWarms)

	for domain, ips := range allDomainIPs {
		// 检查该主机当前已有的连接数
		existingConnections := pm.connManager.GetConnectionsForHost(domain)
		// 计算当前已有的连接数
		currentConnCount := len(existingConnections)
		projlogger.Debug("主机 %s 当前已有的连接数: %d", domain, currentConnCount)

		// 收集需要预热的IP列表
		var targetIPs []string
		for _, ip := range ips {
			// 核心逻辑：如果一个IP既不在白名单(connManager)中，也不在黑名单中，
			// 那么它就是一个需要被预热的目标。
			if pm.connManager.GetConnection(ip) != nil {
				continue // 已在白名单中，跳过
			}
			if pm.blacklist.IsBlocked(ip) {
				continue // 在黑名单中，跳过
			}

			// 检查是否超过每个主机的最大连接数限制
			// 注意：max_conns_per_host 限制的是每个主机（域名）的连接数
			// 如果设置为 0 或负数，表示不限制
			if pm.config.MaxConnsPerHost > 0 && currentConnCount >= pm.config.MaxConnsPerHost {
				// 已达到该主机的最大连接数限制，跳过此 IP
				projlogger.Debug("主机 %s 已达到最大连接数限制 (%d)，跳过 IP %s", domain, pm.config.MaxConnsPerHost, ip)
				continue
			}

			targetIPs = append(targetIPs, ip)
			currentConnCount++
		}

		if len(targetIPs) == 0 {
			projlogger.Info("主机 %s 没有需要预热的IP", domain)
			continue
		}

		// 批量预热：先建立所有连接，然后只验证一次获取sessionid
		wg.Add(1)
		go func(d string, targetIPs []string) {
			defer wg.Done()
			successCountForDomain := pm.preWarmConnectionsBatch(d, targetIPs, concurrencyLimit)
			atomic.AddInt64(&successCount, int64(successCountForDomain))
			projlogger.Info("主机 %s 预热完成，成功加入热连接数: %d", d, successCountForDomain)
		}(domain, targetIPs)
	}
	wg.Wait()
	successTotal := atomic.LoadInt64(&successCount)
	if isInitial {
		projlogger.Info("连接池初始化完成，成功加入热连接池的连接数: %d", successTotal)
	} else {
		projlogger.Info("热连接池维护完成，成功加入热连接池的连接数: %d", successTotal)
	}
}

// maintainPoolFromWhitelist 只维护白名单中已有的连接（定时维护模式）
// 不再从 RemoteIPPool 获取新 IP，只对白名单中已有的 IP 进行重新建立连接和验证
func (pm *PoolManager) maintainPoolFromWhitelist() {
	// 获取所有白名单中的连接
	allConnections := pm.connManager.GetAllConnections()
	if len(allConnections) == 0 {
		projlogger.Debug("白名单为空，跳过维护")
		return
	}

	projlogger.Info("开始维护白名单连接，连接数量: %d", len(allConnections))

	// 按域名分组，收集需要维护的 IP
	domainToIPs := make(map[string][]string)
	for _, conn := range allConnections {
		conn.mu.Lock()
		domain := conn.targetHost
		ip := conn.targetIP
		conn.mu.Unlock()

		// 只维护不在黑名单中的 IP
		if !pm.blacklist.IsBlocked(ip) {
			domainToIPs[domain] = append(domainToIPs[domain], ip)
		}
	}

	if len(domainToIPs) == 0 {
		projlogger.Debug("白名单中所有IP都在黑名单中，跳过维护")
		return
	}

	var wg sync.WaitGroup
	var successCount int64
	concurrencyLimit := make(chan struct{}, pm.config.MaxConcurrentPreWarms)

	for domain, ips := range domainToIPs {
		wg.Add(1)
		go func(d string, ipList []string) {
			defer wg.Done()
			// 对白名单中的 IP 进行重新预热
			// 注意：preWarmConnectionsBatch 会跳过已在白名单中的 IP，所以我们需要先移除旧连接
			// 或者修改逻辑，允许对白名单中的 IP 重新建立连接
			successCountForDomain := pm.reWarmWhitelistIPs(d, ipList, concurrencyLimit)
			atomic.AddInt64(&successCount, int64(successCountForDomain))
			projlogger.Info("主机 %s 白名单连接维护完成，成功维护连接数: %d", d, successCountForDomain)
		}(domain, ips)
	}

	wg.Wait()
	successTotal := atomic.LoadInt64(&successCount)
	projlogger.Info("白名单连接维护完成，成功维护连接数: %d", successTotal)
}

// reWarmWhitelistIPs 重新预热白名单中的 IP（用于定时维护）
// 先移除旧连接，然后重新建立连接和验证
func (pm *PoolManager) reWarmWhitelistIPs(domain string, ips []string, concurrencyLimit chan struct{}) int {
	// 先移除旧连接，以便重新建立
	for _, ip := range ips {
		pm.connManager.RemoveConnection(ip)
	}

	// 然后使用 preWarmConnectionsBatch 重新建立连接
	// 由于已经移除了旧连接，preWarmConnectionsBatch 会重新建立这些 IP 的连接
	return pm.preWarmConnectionsBatch(domain, ips, concurrencyLimit)
}

// preWarmConnectionsBatch 批量预热连接：先建立所有连接，然后只验证一次获取sessionid，应用到所有连接
// 返回成功加入热连接池的连接数
func (pm *PoolManager) preWarmConnectionsBatch(domain string, ips []string, concurrencyLimit chan struct{}) int {
	type connResult struct {
		conn *UTLSConnection
		ip   string
		err  error
	}

	// 第一步：过滤掉已加入黑名单的IP，避免不必要的握手
	var validIPs []string
	for _, ip := range ips {
		if pm.blacklist.IsBlocked(ip) {
			projlogger.Debug("跳过预热已加入黑名单的IP: %s", ip)
			continue
		}
		validIPs = append(validIPs, ip)
	}

	if len(validIPs) == 0 {
		projlogger.Debug("主机 %s 所有IP都在黑名单中，跳过预热", domain)
		return 0
	}

	// 第二步：并发建立所有连接（不验证）
	connChan := make(chan connResult, len(validIPs))
	var establishWg sync.WaitGroup

	for _, ip := range validIPs {
		establishWg.Add(1)
		concurrencyLimit <- struct{}{}
		go func(ipAddr string) {
			defer func() {
				<-concurrencyLimit
				establishWg.Done()
			}()
			// 在握手前再次检查黑名单（可能在goroutine启动期间被加入黑名单）
			if pm.blacklist.IsBlocked(ipAddr) {
				projlogger.Debug("跳过握手已加入黑名单的IP: %s", ipAddr)
				return
			}
			// 设置403回调，将IP加入黑名单
			conn, err := establishConnection(ipAddr, domain, pm.config, pm.blacklist.Add)
			connChan <- connResult{conn: conn, ip: ipAddr, err: err}
		}(ip)
	}

	// 等待所有连接建立完成
	establishWg.Wait()
	close(connChan)

	// 收集成功建立的连接，同时过滤掉已加入黑名单的IP
	var establishedConns []connResult
	for result := range connChan {
		if result.err != nil {
			projlogger.Warn("预热失败(建立连接): %s, 原因: %v", result.ip, result.err)
			continue
		}
		// 检查是否在黑名单中，如果在则关闭连接并跳过
		if pm.blacklist.IsBlocked(result.ip) {
			projlogger.Debug("连接建立后发现IP已在黑名单，关闭连接: %s", result.ip)
			if result.conn != nil {
				result.conn.Close()
			}
			continue
		}
		establishedConns = append(establishedConns, result)
	}

	if len(establishedConns) == 0 {
		projlogger.Warn("主机 %s 没有成功建立的连接", domain)
		return 0
	}

	// 第三步：遍历所有连接，直到找到一个验证成功并返回有效sessionid的连接
	// 注意：不关闭任何连接（除了403错误），即使验证失败也保留连接
	var sessionID string
	var blockedIPs []string // 记录403错误的IP，需要关闭

	for _, result := range establishedConns {
		// 在验证之前再次检查黑名单，避免验证已经被拉黑的IP
		if pm.blacklist.IsBlocked(result.ip) {
			projlogger.Debug("跳过验证已加入黑名单的IP: %s", result.ip)
			// 关闭连接并跳过
			result.conn.Close()
			continue
		}

		validationResult, err := pm.validator.Validate(result.conn)
		if err != nil {
			if errors.Is(err, ErrIPBlockedBy403) {
				projlogger.Warn("预热失败(403 Forbidden)，将IP加入黑名单: %s", result.ip)
				pm.blacklist.Add(result.ip)
				// 403错误，记录这个IP，稍后关闭
				blockedIPs = append(blockedIPs, result.ip)
				continue
			} else {
				// 其他错误（如404、500等）：这个IP无法获取sessionid，但连接本身是好的
				// 继续尝试其他IP，直到找到一个能成功获取sessionid的IP
				projlogger.Debug("该IP无法获取SessionID: %s, 原因: %v，继续尝试下一个连接", result.ip, err)
				// 验证失败但连接成功，保留连接继续尝试
				continue
			}
		}

		// 验证成功，检查是否有有效的sessionid
		if validationResult != nil && validationResult.SessionID != "" {
			sessionID = validationResult.SessionID
			projlogger.Info("验证成功，已获取SessionID: %s (来自IP: %s)，停止验证", sessionID, result.ip)
			// 找到有效的sessionid，停止验证
			break
		} else {
			// 验证成功但没有sessionid（可能是GET请求），继续尝试下一个
			projlogger.Debug("验证成功但未获取到SessionID (来自IP: %s)，继续尝试下一个连接", result.ip)
			continue
		}
	}

	// 如果没有找到验证成功并获取到sessionid的连接，关闭所有连接
	if sessionID == "" {
		projlogger.Warn("主机 %s 所有连接验证都失败或未获取到SessionID，关闭所有 %d 个连接", domain, len(establishedConns))
		for _, result := range establishedConns {
			result.conn.Close()
		}
		return 0
	}

	// 只关闭403错误的连接，其他验证失败的连接保留
	for _, result := range establishedConns {
		for _, blockedIP := range blockedIPs {
			if result.ip == blockedIP {
				result.conn.Close()
				break
			}
		}
	}

	// 过滤出未被403阻止的连接（包括验证失败但连接成功的连接）
	var activeConns []connResult
	for _, result := range establishedConns {
		isBlocked := false
		for _, blockedIP := range blockedIPs {
			if result.ip == blockedIP {
				isBlocked = true
				break
			}
		}
		if !isBlocked {
			activeConns = append(activeConns, result)
		}
	}
	establishedConns = activeConns

	// 第三步：将sessionid应用到所有成功建立的连接，并加入白名单
	successCount := 0
	for _, result := range establishedConns {
		if sessionID != "" {
			result.conn.SetSessionID(sessionID)
		}
		pm.connManager.AddConnection(result.conn)
		successCount++
	}

	projlogger.Info("主机 %s 批量预热完成，成功加入 %d 个连接，使用SessionID: %s", domain, successCount, sessionID)
	return successCount
}

// checkBlacklistRecovery 检查黑名单中的IP是否恢复，如果恢复则从黑名单移除并加入白名单
func (pm *PoolManager) checkBlacklistRecovery() {
	blockedIPs := pm.blacklist.GetBlockedIPs()
	if len(blockedIPs) == 0 {
		projlogger.Debug("黑名单为空，跳过恢复检查")
		return
	}

	projlogger.Info("开始检查黑名单恢复，待检查IP数量: %d", len(blockedIPs))

	// 获取所有域名的IP映射，用于查找IP对应的域名
	allDomainIPs := pm.remotePool.GetAllDomainIPs()

	// 构建IP到域名的反向映射
	ipToDomain := make(map[string]string)
	for domain, ips := range allDomainIPs {
		for _, ip := range ips {
			// 如果IP已经在映射中，跳过（一个IP可能对应多个域名，取第一个）
			if _, exists := ipToDomain[ip]; !exists {
				ipToDomain[ip] = domain
			}
		}
	}

	// 并发检查黑名单中的IP
	concurrencyLimit := make(chan struct{}, pm.config.MaxConcurrentPreWarms)
	var wg sync.WaitGroup
	var recoveredCount int64

	for _, blockedIP := range blockedIPs {
		// 检查IP是否还在黑名单中（可能在并发检查时已被移除）
		if !pm.blacklist.IsBlocked(blockedIP) {
			continue
		}

		// 检查IP是否已经在白名单中（已有连接）
		if pm.connManager.GetConnection(blockedIP) != nil {
			// IP已经在白名单中，从黑名单移除
			pm.blacklist.Remove(blockedIP)
			projlogger.Info("IP %s 已在白名单中，从黑名单移除", blockedIP)
			continue
		}

		// 查找IP对应的域名
		domain, exists := ipToDomain[blockedIP]
		if !exists {
			projlogger.Debug("无法找到IP %s 对应的域名，跳过恢复检查", blockedIP)
			continue
		}

		wg.Add(1)
		concurrencyLimit <- struct{}{}
		go func(ipAddr, domainName string) {
			defer func() {
				<-concurrencyLimit
				wg.Done()
			}()

			// 再次检查IP是否还在黑名单中
			if !pm.blacklist.IsBlocked(ipAddr) {
				return
			}

			// 尝试建立连接，设置403回调将IP加入黑名单
			conn, err := establishConnection(ipAddr, domainName, pm.config, pm.blacklist.Add)
			if err != nil {
				projlogger.Debug("黑名单IP %s 恢复检查：连接建立失败: %v", ipAddr, err)
				return
			}
			defer func() {
				// 如果验证失败，关闭连接
				if conn != nil {
					conn.Close()
				}
			}()

			// 使用 CheckRecovery 检查连接是否恢复（只检查是否返回200，不要求SessionID）
			recovered, err := pm.validator.CheckRecovery(conn)
			if err != nil {
				if errors.Is(err, ErrIPBlockedBy403) {
					projlogger.Debug("黑名单IP %s 恢复检查：仍返回403，保持在黑名单", ipAddr)
				} else {
					projlogger.Debug("黑名单IP %s 恢复检查：检查失败: %v", ipAddr, err)
				}
				return
			}

			// 如果恢复成功（返回200），从黑名单移除并加入白名单
			if recovered {
				// 从黑名单移除
				pm.blacklist.Remove(ipAddr)

				// 尝试获取SessionID（可选，不影响恢复判断）
				validationResult, err := pm.validator.Validate(conn)
				if err == nil && validationResult != nil && validationResult.SessionID != "" {
					conn.SetSessionID(validationResult.SessionID)
				}

				// 加入白名单
				pm.connManager.AddConnection(conn)

				// 防止defer关闭连接
				conn = nil

				atomic.AddInt64(&recoveredCount, 1)
				projlogger.Info("✅ 黑名单IP %s 已恢复（返回200），已从黑名单移除并加入白名单", ipAddr)
			}
		}(blockedIP, domain)
	}

	wg.Wait()
	recovered := atomic.LoadInt64(&recoveredCount)
	if recovered > 0 {
		projlogger.Info("黑名单恢复检查完成，成功恢复 %d 个IP", recovered)
	} else {
		projlogger.Debug("黑名单恢复检查完成，没有IP恢复")
	}
}
