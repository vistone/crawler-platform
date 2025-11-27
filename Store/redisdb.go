package Store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisManager 管理 Redis 连接池（每个数据类型使用独立的数据库）
type RedisManager struct {
	mu      sync.Mutex
	clients map[string]*redis.Client // dataType -> client
	addr    string
	dbMap   map[string]int // dataType -> DB编号
}

// 全局默认 Redis 管理器
var defaultRedisManager = &RedisManager{
	clients: make(map[string]*redis.Client),
	dbMap:   make(map[string]int),
}

// findSafeRedisDBs 为多个数据类型分配独立的安全数据库
// 返回 dataType -> DB编号 的映射
func findSafeRedisDBs(client *redis.Client, dataTypes []string) (map[string]int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. 扫描所有数据库，获取 key 数量
	type dbInfo struct {
		db   int
		size int64
	}
	var dbList []dbInfo

	for db := 0; db < 16; db++ {
		testClient := redis.NewClient(&redis.Options{
			Addr: client.Options().Addr,
			DB:   db,
		})

		size, err := testClient.DBSize(ctx).Result()
		testClient.Close()

		if err != nil {
			continue
		}

		dbList = append(dbList, dbInfo{db: db, size: size})
	}

	if len(dbList) == 0 {
		return nil, errors.New("无法扫描 Redis 数据库")
	}

	// 2. 按 key 数量排序（从少到多）
	for i := 0; i < len(dbList)-1; i++ {
		for j := i + 1; j < len(dbList); j++ {
			if dbList[j].size < dbList[i].size {
				dbList[i], dbList[j] = dbList[j], dbList[i]
			}
		}
	}

	// 3. 为每个数据类型分配一个数据库
	allocation := make(map[string]int)
	for i, dataType := range dataTypes {
		if i >= len(dbList) {
			// 数据类型数量超过16个，复用数据库
			allocation[dataType] = dbList[i%len(dbList)].db
		} else {
			allocation[dataType] = dbList[i].db
		}
	}

	return allocation, nil
}

// findSafeRedisDB 查找一个安全的 Redis 数据库编号（0-15）
// 选择 key 数量最少的数据库，避免影响其他程序
func findSafeRedisDB(client *redis.Client) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	minKeys := int64(-1)
	safeDB := 0

	// 遍历 0-15 号数据库
	for db := 0; db < 16; db++ {
		// 临时连接到该数据库
		testClient := redis.NewClient(&redis.Options{
			Addr: client.Options().Addr,
			DB:   db,
		})

		// 获取该数据库的 key 数量
		size, err := testClient.DBSize(ctx).Result()
		testClient.Close()

		if err != nil {
			continue // 跳过出错的数据库
		}

		// 找到 key 数量最少的数据库
		if minKeys == -1 || size < minKeys {
			minKeys = size
			safeDB = db
		}

		// 如果找到空数据库，直接使用
		if size == 0 {
			break
		}
	}

	return safeDB, nil
}

// InitRedis 初始化全局 Redis 连接（可选，使用默认配置则无需调用）
// addr 格式: "localhost:6379"
// opts: 自定义配置（传 nil 自动选择最安全的库）
// 已废弃：新的实现会为每个数据类型自动分配数据库
func InitRedis(addr string, opts *redis.Options) error {
	// 仅用于测试连接
	defaultRedisManager.mu.Lock()
	defer defaultRedisManager.mu.Unlock()

	if addr == "" {
		addr = "localhost:6379"
	}

	defaultRedisManager.addr = addr

	// 测试连接
	testClient := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := testClient.Ping(ctx).Err(); err != nil {
		testClient.Close()
		return fmt.Errorf("redis 连接失败: %w", err)
	}

	testClient.Close()
	return nil
}

// getOrInitClient 获取或初始化 Redis 客户端（使用默认配置）
// 已废弃：请使用 getOrInitClientForDataType
func (rm *RedisManager) getOrInitClient(addr string) (*redis.Client, error) {
	// 为向后兼容，使用默认数据类型 "default"
	return rm.getOrInitClientForDataType(addr, "default")
}

// getOrInitClientForDataType 获取或初始化指定数据类型的 Redis 客户端
func (rm *RedisManager) getOrInitClientForDataType(addr, dataType string) (*redis.Client, error) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 如果已有该数据类型的客户端，直接返回
	if client, exists := rm.clients[dataType]; exists && rm.addr == addr {
		return client, nil
	}

	// 地址为空，使用默认地址
	if addr == "" {
		addr = "localhost:6379"
	}

	// 如果地址变更，关闭所有旧连接
	if rm.addr != "" && rm.addr != addr {
		for _, client := range rm.clients {
			_ = client.Close()
		}
		rm.clients = make(map[string]*redis.Client)
		rm.dbMap = make(map[string]int)
	}

	rm.addr = addr

	// 如果还没有分配数据库，需要分配
	if _, exists := rm.dbMap[dataType]; !exists {
		// 收集所有已知的数据类型
		dataTypes := []string{dataType}
		for dt := range rm.dbMap {
			if dt != dataType {
				dataTypes = append(dataTypes, dt)
			}
		}

		// 为所有数据类型分配数据库
		tempClient := redis.NewClient(&redis.Options{
			Addr: addr,
			DB:   0,
		})

		allocation, err := findSafeRedisDBs(tempClient, dataTypes)
		tempClient.Close()

		if err != nil {
			return nil, err
		}

		// 更新映射
		for dt, db := range allocation {
			rm.dbMap[dt] = db
		}
	}

	// 创建新客户端
	db := rm.dbMap[dataType]
	opts := &redis.Options{
		Addr:         addr,
		DB:           db,
		PoolSize:     100,
		MinIdleConns: 10,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	}

	client := redis.NewClient(opts)

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis 连接失败: %w", err)
	}

	// 关闭旧连接（如果存在）
	if oldClient, exists := rm.clients[dataType]; exists {
		_ = oldClient.Close()
	}

	rm.clients[dataType] = client

	return client, nil
}

// buildRedisKey 构建 Redis key（使用原始 tilekey；已按数据类型分库，无需前缀）
func buildRedisKey(dataType, tilekey string) string {
	return tilekey
}

// buildMetaKey 构建元数据 key
func buildMetaKey(dataType string) string {
	return fmt.Sprintf("%s:_meta", dataType)
}

// SetEpoch 设置数据类型的 epoch
func SetEpoch(addr, dataType string, epoch int64) error {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	key := buildMetaKey(dataType)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.HSet(ctx, key, "epoch", epoch).Err()
}

// GetEpoch 获取数据类型的 epoch
func GetEpoch(addr, dataType string) (int64, error) {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return 0, err
	}

	key := buildMetaKey(dataType)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	val, err := client.HGet(ctx, key, "epoch").Int64()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return 0, nil // epoch 未设置，返回 0
		}
		return 0, err
	}

	return val, nil
}

// SetMetadata 设置数据类型的多个元数据字段
func SetMetadata(addr, dataType string, metadata map[string]interface{}) error {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	key := buildMetaKey(dataType)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.HSet(ctx, key, metadata).Err()
}

// GetMetadata 获取数据类型的所有元数据
func GetMetadata(addr, dataType string) (map[string]string, error) {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return nil, err
	}

	key := buildMetaKey(dataType)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.HGetAll(ctx, key).Result()
}

// PutTileRedisWithMetadata 写入单条数据到 Redis（带元数据）
// addr: Redis 地址（如 "localhost:6379"），为空则使用默认
// dataType: 数据类型（imagery/terrain/vector）
// tilekey: 唯一标识
// value: 负载数据
// metadata: 元数据
// expiration: 过期时间，0 表示永不过期
func PutTileRedisWithMetadata(addr, dataType, tilekey string, value []byte, metadata *TileMetadata, expiration time.Duration) error {
	// 编码数据和元数据
	encodedData, err := encodeTileDataWithMetadata(value, metadata)
	if err != nil {
		return err
	}

	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	// 构建 Redis key
	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.Set(ctx, key, encodedData, expiration).Err()
}

// PutTileRedis 写入单条数据到 Redis（使用压缩 tilekey）
// addr: Redis 地址（如 "localhost:6379"），为空则使用默认
// dataType: 数据类型（imagery/terrain/vector）
// tilekey: 唯一标识
// value: 负载数据
// expiration: 过期时间，0 表示永不过期
func PutTileRedis(addr, dataType, tilekey string, value []byte, expiration time.Duration) error {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	// 构建 Redis key
	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.Set(ctx, key, value, expiration).Err()
}

// GetTileRedis 读取数据
func GetTileRedis(addr, dataType, tilekey string) ([]byte, error) {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return nil, err
	}

	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	val, err := client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, fmt.Errorf("key 不存在: %s", key)
		}
		return nil, err
	}

	return val, nil
}

// DeleteTileRedis 删除数据
func DeleteTileRedis(addr, dataType, tilekey string) error {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.Del(ctx, key).Err()
}

// PutTilesRedisBatch 批量写入多条数据（使用 Pipeline 优化，分批提交）
// 适用于批量导入场景，性能比逐条调用 PutTileRedis 提升数倍
// 自动分批处理，避免单次 Pipeline 过大导致超时
func PutTilesRedisBatch(addr, dataType string, records map[string][]byte, expiration time.Duration) error {
	if len(records) == 0 {
		return nil
	}

	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return err
	}

	const batchSize = 1000 // 每批最多 1000 条

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 分批执行
	var batch []struct {
		key   string
		value []byte
	}

	for tilekey, value := range records {
		batch = append(batch, struct {
			key   string
			value []byte
		}{key: buildRedisKey(dataType, tilekey), value: value})

		// 达到批次大小，执行 Pipeline
		if len(batch) >= batchSize {
			if err := executePipeline(ctx, client, batch, expiration); err != nil {
				return err
			}
			batch = batch[:0] // 清空批次
		}
	}

	// 处理剩余数据
	if len(batch) > 0 {
		if err := executePipeline(ctx, client, batch, expiration); err != nil {
			return err
		}
	}

	return nil
}

// executePipeline 执行单个 Pipeline 批次
func executePipeline(ctx context.Context, client *redis.Client, batch []struct {
	key   string
	value []byte
}, expiration time.Duration) error {
	pipe := client.Pipeline()

	for _, item := range batch {
		pipe.Set(ctx, item.key, item.value, expiration)
	}

	_, err := pipe.Exec(ctx)
	return err
}

// ExistsTileRedis 检查 key 是否存在
func ExistsTileRedis(addr, dataType, tilekey string) (bool, error) {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return false, err
	}

	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	count, err := client.Exists(ctx, key).Result()
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// GetTileTTLRedis 获取 key 的剩余过期时间
func GetTileTTLRedis(addr, dataType, tilekey string) (time.Duration, error) {
	client, err := defaultRedisManager.getOrInitClientForDataType(addr, dataType)
	if err != nil {
		return 0, err
	}

	key := buildRedisKey(dataType, tilekey)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	return client.TTL(ctx, key).Result()
}

// CloseRedis 关闭所有 Redis 连接
func CloseRedis() error {
	defaultRedisManager.mu.Lock()
	defer defaultRedisManager.mu.Unlock()

	var lastErr error
	for _, client := range defaultRedisManager.clients {
		if err := client.Close(); err != nil {
			lastErr = err
		}
	}

	defaultRedisManager.clients = make(map[string]*redis.Client)
	defaultRedisManager.dbMap = make(map[string]int)

	return lastErr
}

// GetRedisDB 获取指定数据类型使用的 Redis 数据库编号
func GetRedisDB(dataType string) int {
	defaultRedisManager.mu.Lock()
	defer defaultRedisManager.mu.Unlock()

	if db, exists := defaultRedisManager.dbMap[dataType]; exists {
		return db
	}
	return -1 // 未初始化
}

// GetAllRedisDBs 获取所有数据类型的数据库分配情况
func GetAllRedisDBs() map[string]int {
	defaultRedisManager.mu.Lock()
	defer defaultRedisManager.mu.Unlock()

	result := make(map[string]int)
	for dt, db := range defaultRedisManager.dbMap {
		result[dt] = db
	}
	return result
}

// ScanAllRedisDBs 扫描所有 Redis 数据库的 key 数量（调试用）
func ScanAllRedisDBs(addr string) (map[int]int64, error) {
	result := make(map[int]int64)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for db := 0; db < 16; db++ {
		client := redis.NewClient(&redis.Options{
			Addr: addr,
			DB:   db,
		})

		size, err := client.DBSize(ctx).Result()
		client.Close()

		if err != nil {
			return nil, err
		}

		result[db] = size
	}

	return result, nil
}
