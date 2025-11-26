package Store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// StorageBackend 存储后端类型
type StorageBackend string

const (
	BackendBBolt  StorageBackend = "bbolt"
	BackendSQLite StorageBackend = "sqlite"
)

// TileStorageConfig 瓦片存储配置
type TileStorageConfig struct {
	// 持久化后端选择（bbolt 或 sqlite）
	Backend StorageBackend
	// 持久化数据库目录
	DBDir string
	// Redis 地址（为空则禁用缓存）
	RedisAddr string
	// Redis 缓存过期时间（0 表示永不过期）
	CacheExpiration time.Duration
	// 是否启用 Redis 缓存
	EnableCache bool
	// 是否启用异步持久化（Redis 作为写缓冲区）
	EnableAsyncPersist bool
	// 异步持久化批次大小（默认 100）
	PersistBatchSize int
	// 异步持久化间隔（默认 5 秒）
	PersistInterval time.Duration
	// 持久化成功后是否清理 Redis（默认 true）
	// 使用指针区分“未设置”和“设置为false”
	ClearRedisAfterPersist *bool
}

// TileStorage 瓦片存储管理器（缓存 + 持久化）
type TileStorage struct {
	config TileStorageConfig

	// 异步持久化相关
	persistQueue chan *persistTask
	persistWg    sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
}

// persistTask 持久化任务
type persistTask struct {
	dataType string
	tilekey  string
	value    []byte
}

// NewTileStorage 创建瓦片存储管理器
func NewTileStorage(config TileStorageConfig) (*TileStorage, error) {
	// 验证配置
	if config.Backend != BackendBBolt && config.Backend != BackendSQLite {
		return nil, fmt.Errorf("不支持的后端类型: %s", config.Backend)
	}
	if config.DBDir == "" {
		return nil, errors.New("DBDir 不能为空")
	}

	// 如果启用缓存但未提供 Redis 地址，使用默认地址
	if config.EnableCache && config.RedisAddr == "" {
		config.RedisAddr = "localhost:6379"
	}

	// 如果启用缓存，测试 Redis 连接
	if config.EnableCache {
		if err := InitRedis(config.RedisAddr, nil); err != nil {
			return nil, fmt.Errorf("Redis 连接失败: %w", err)
		}
	}

	// 异步持久化默认配置
	if config.EnableAsyncPersist {
		if config.PersistBatchSize <= 0 {
			config.PersistBatchSize = 100
		}
		if config.PersistInterval <= 0 {
			config.PersistInterval = 5 * time.Second
		}
		// 默认持久化后清理 Redis
		if config.ClearRedisAfterPersist == nil {
			// 未设置，默认启用
			trueVal := true
			config.ClearRedisAfterPersist = &trueVal
		}
	}

	ctx, cancel := context.WithCancel(context.Background())

	ts := &TileStorage{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	// 启动异步持久化 worker
	if config.EnableAsyncPersist {
		ts.persistQueue = make(chan *persistTask, config.PersistBatchSize*10) // 队列容量是批次的10倍
		ts.startPersistWorker()
	}

	return ts, nil
}

// startPersistWorker 启动异步持久化 worker
func (ts *TileStorage) startPersistWorker() {
	ts.persistWg.Add(1)
	go func() {
		defer ts.persistWg.Done()

		ticker := time.NewTicker(ts.config.PersistInterval)
		defer ticker.Stop()

		batch := make(map[string][]byte)
		dataTypeMap := make(map[string]string) // tilekey -> dataType

		flushBatch := func() {
			if len(batch) == 0 {
				return
			}

			// 按 dataType 分组
			grouped := make(map[string]map[string][]byte)
			for tilekey, value := range batch {
				dataType := dataTypeMap[tilekey]
				if grouped[dataType] == nil {
					grouped[dataType] = make(map[string][]byte)
				}
				grouped[dataType][tilekey] = value
			}

			// 批量持久化
			for dataType, records := range grouped {
				var persistErr error
				switch ts.config.Backend {
				case BackendBBolt:
					persistErr = PutTilesBBoltBatch(ts.config.DBDir, dataType, records)
				case BackendSQLite:
					// SQLite 逐条写入（批量优化不明显）
					for tilekey, value := range records {
						if err := PutTileSQLite(ts.config.DBDir, dataType, tilekey, value); err != nil {
							persistErr = err
							break
						}
					}
				}

				// 持久化成功后清理 Redis
				if persistErr == nil && ts.config.ClearRedisAfterPersist != nil && *ts.config.ClearRedisAfterPersist {
					for tilekey := range records {
						// 异步删除 Redis，失败不影响整体
						go func(dt, tk string) {
							_ = DeleteTileRedis(ts.config.RedisAddr, dt, tk)
						}(dataType, tilekey)
					}
				}
			}

			// 清空批次
			batch = make(map[string][]byte)
			dataTypeMap = make(map[string]string)
		}

		for {
			select {
			case task, ok := <-ts.persistQueue:
				if !ok {
					// 队列已关闭，刷新剩余数据后退出
					flushBatch()
					return
				}

				// 收集任务到批次
				batch[task.tilekey] = task.value
				dataTypeMap[task.tilekey] = task.dataType

				// 达到批次大小，立即刷新
				if len(batch) >= ts.config.PersistBatchSize {
					flushBatch()
				}

			case <-ticker.C:
				// 定时刷新
				flushBatch()
			}
		}
	}()
}

// Put 写入瓦片数据（异步模式：先写 Redis，后台异步持久化）
func (ts *TileStorage) Put(dataType, tilekey string, value []byte) error {
	if ts.config.EnableAsyncPersist {
		// 异步模式：先写 Redis，再异步持久化
		if !ts.config.EnableCache {
			return errors.New("异步持久化模式必须启用 Redis 缓存")
		}

		// 1. 立即写入 Redis（极速响应）
		if err := PutTileRedis(ts.config.RedisAddr, dataType, tilekey, value, ts.config.CacheExpiration); err != nil {
			return fmt.Errorf("Redis 写入失败: %w", err)
		}

		// 2. 异步提交持久化任务（非阻塞）
		select {
		case ts.persistQueue <- &persistTask{
			dataType: dataType,
			tilekey:  tilekey,
			value:    value,
		}:
			// 任务提交成功
		default:
			// 队列满，记录警告但不阻塞（Redis 已写入）
			// 可选：集成日志系统
		}

		return nil
	}

	// 同步模式：先持久化，再缓存
	var persistErr error
	switch ts.config.Backend {
	case BackendBBolt:
		persistErr = PutTileBBolt(ts.config.DBDir, dataType, tilekey, value)
	case BackendSQLite:
		persistErr = PutTileSQLite(ts.config.DBDir, dataType, tilekey, value)
	}

	if persistErr != nil {
		return fmt.Errorf("持久化写入失败: %w", persistErr)
	}

	// 写入 Redis 缓存（失败不影响整体）
	if ts.config.EnableCache {
		if err := PutTileRedis(ts.config.RedisAddr, dataType, tilekey, value, ts.config.CacheExpiration); err != nil {
			// 缓存写入失败，仅记录日志，不返回错误
		}
	}

	return nil
}

// PutBatch 批量写入瓦片数据（先写持久化，再写缓存）
func (ts *TileStorage) PutBatch(dataType string, records map[string][]byte) error {
	// 1. 批量写入持久化存储
	var persistErr error
	switch ts.config.Backend {
	case BackendBBolt:
		persistErr = PutTilesBBoltBatch(ts.config.DBDir, dataType, records)
	case BackendSQLite:
		// SQLite 批量优化效果不明显，使用单条写入
		for tilekey, value := range records {
			if err := PutTileSQLite(ts.config.DBDir, dataType, tilekey, value); err != nil {
				return fmt.Errorf("SQLite 批量写入失败: %w", err)
			}
		}
	}

	if persistErr != nil {
		return fmt.Errorf("持久化批量写入失败: %w", persistErr)
	}

	// 2. 批量写入 Redis 缓存（失败不影响整体）
	if ts.config.EnableCache {
		if err := PutTilesRedisBatch(ts.config.RedisAddr, dataType, records, ts.config.CacheExpiration); err != nil {
			// 缓存写入失败，仅记录日志
		}
	}

	return nil
}

// Get 读取瓦片数据（先查缓存，缓存未命中则查持久化并回填缓存）
func (ts *TileStorage) Get(dataType, tilekey string) ([]byte, error) {
	// 1. 先查 Redis 缓存
	if ts.config.EnableCache {
		if data, err := GetTileRedis(ts.config.RedisAddr, dataType, tilekey); err == nil {
			// 缓存命中
			return data, nil
		}
		// 缓存未命中，继续查持久化
	}

	// 2. 查询持久化存储
	var data []byte
	var persistErr error

	switch ts.config.Backend {
	case BackendBBolt:
		data, persistErr = GetTileBBolt(ts.config.DBDir, dataType, tilekey)
	case BackendSQLite:
		data, persistErr = GetTileSQLite(ts.config.DBDir, dataType, tilekey)
	}

	if persistErr != nil {
		return nil, fmt.Errorf("持久化读取失败: %w", persistErr)
	}

	// 3. 回填缓存（异步，失败不影响返回）
	if ts.config.EnableCache && len(data) > 0 {
		go func() {
			_ = PutTileRedis(ts.config.RedisAddr, dataType, tilekey, data, ts.config.CacheExpiration)
		}()
	}

	return data, nil
}

// Delete 删除瓦片数据（同时删除缓存和持久化）
func (ts *TileStorage) Delete(dataType, tilekey string) error {
	// 1. 删除缓存
	if ts.config.EnableCache {
		_ = DeleteTileRedis(ts.config.RedisAddr, dataType, tilekey)
	}

	// 2. 删除持久化数据
	var persistErr error
	switch ts.config.Backend {
	case BackendBBolt:
		persistErr = DeleteTileBBolt(ts.config.DBDir, dataType, tilekey)
	case BackendSQLite:
		persistErr = DeleteTileSQLite(ts.config.DBDir, dataType, tilekey)
	}

	if persistErr != nil {
		return fmt.Errorf("持久化删除失败: %w", persistErr)
	}

	return nil
}

// Exists 检查瓦片是否存在（先查缓存，再查持久化）
func (ts *TileStorage) Exists(dataType, tilekey string) (bool, error) {
	// 1. 先查 Redis 缓存
	if ts.config.EnableCache {
		if exists, err := ExistsTileRedis(ts.config.RedisAddr, dataType, tilekey); err == nil && exists {
			return true, nil
		}
	}

	// 2. 查询持久化存储（尝试读取）
	_, err := ts.Get(dataType, tilekey)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// InvalidateCache 清除指定 key 的缓存（强制从持久化重新加载）
func (ts *TileStorage) InvalidateCache(dataType, tilekey string) error {
	if !ts.config.EnableCache {
		return nil
	}
	return DeleteTileRedis(ts.config.RedisAddr, dataType, tilekey)
}

// WarmupCache 预热缓存（从持久化加载数据到 Redis）
func (ts *TileStorage) WarmupCache(dataType string, tilekeys []string) error {
	if !ts.config.EnableCache {
		return errors.New("缓存未启用")
	}

	for _, tilekey := range tilekeys {
		// 从持久化读取
		var data []byte
		var err error

		switch ts.config.Backend {
		case BackendBBolt:
			data, err = GetTileBBolt(ts.config.DBDir, dataType, tilekey)
		case BackendSQLite:
			data, err = GetTileSQLite(ts.config.DBDir, dataType, tilekey)
		}

		if err != nil {
			continue // 跳过不存在的 key
		}

		// 写入缓存
		_ = PutTileRedis(ts.config.RedisAddr, dataType, tilekey, data, ts.config.CacheExpiration)
	}

	return nil
}

// Close 关闭所有连接
func (ts *TileStorage) Close() error {
	// 停止异步持久化 worker
	if ts.config.EnableAsyncPersist && ts.persistQueue != nil {
		close(ts.persistQueue) // 关闭队列，触发 worker 退出
		ts.persistWg.Wait()     // 等待 worker 完成剩余任务
	}

	var errs []error

	// 关闭 Redis 连接
	if ts.config.EnableCache {
		if err := CloseRedis(); err != nil {
			errs = append(errs, err)
		}
	}

	// 关闭持久化连接
	switch ts.config.Backend {
	case BackendBBolt:
		if err := CloseAllBBolt(); err != nil {
			errs = append(errs, err)
		}
	case BackendSQLite:
		if err := CloseAllSQLite(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("关闭连接时出现错误: %v", errs)
	}

	return nil
}

// GetBackend 获取当前持久化后端类型
func (ts *TileStorage) GetBackend() StorageBackend {
	return ts.config.Backend
}

// IsCacheEnabled 检查缓存是否启用
func (ts *TileStorage) IsCacheEnabled() bool {
	return ts.config.EnableCache
}

// IsAsyncPersistEnabled 检查是否启用异步持久化
func (ts *TileStorage) IsAsyncPersistEnabled() bool {
	return ts.config.EnableAsyncPersist
}

// GetPendingPersistCount 获取待持久化任务数量
func (ts *TileStorage) GetPendingPersistCount() int {
	if !ts.config.EnableAsyncPersist || ts.persistQueue == nil {
		return 0
	}
	return len(ts.persistQueue)
}
