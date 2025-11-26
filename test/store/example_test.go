package Store_test

import (
	"fmt"
	"log"
	"time"

	"crawler-platform/Store"
)

// Example_basicUsage 基础使用示例
func Example_basicUsage() {
	// 创建存储管理器（BBolt + Redis 缓存）
	config := Store.TileStorageConfig{
		Backend:         Store.BackendBBolt,
		DBDir:           "/data/tiles",
		RedisAddr:       "localhost:6379",
		CacheExpiration: 1 * time.Hour,
		EnableCache:     true,
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 写入瓦片
	tileData := []byte("tile_image_data")
	err = storage.Put("imagery", "01230123", tileData)
	if err != nil {
		log.Fatal(err)
	}

	// 读取瓦片（优先从缓存）
	data, err := storage.Get("imagery", "01230123")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("读取到 %d 字节数据\n", len(data))
}

// Example_batchWrite 批量写入示例
func Example_batchWrite() {
	config := Store.TileStorageConfig{
		Backend:         Store.BackendBBolt,
		DBDir:           "/data/tiles",
		RedisAddr:       "localhost:6379",
		CacheExpiration: 30 * time.Minute,
		EnableCache:     true,
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 批量写入瓦片
	tiles := map[string][]byte{
		"0000": []byte("tile_0"),
		"0001": []byte("tile_1"),
		"0010": []byte("tile_2"),
		"0011": []byte("tile_3"),
	}

	err = storage.PutBatch("imagery", tiles)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("批量写入 %d 条瓦片数据\n", len(tiles))
}

// Example_sqliteBackend SQLite 后端示例
func Example_sqliteBackend() {
	// 使用 SQLite 作为持久化后端
	config := Store.TileStorageConfig{
		Backend:         Store.BackendSQLite,
		DBDir:           "/data/tiles_sqlite",
		RedisAddr:       "localhost:6379",
		CacheExpiration: 2 * time.Hour,
		EnableCache:     true,
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 正常使用
	storage.Put("terrain", "32103210", []byte("terrain_data"))
	data, _ := storage.Get("terrain", "32103210")
	fmt.Printf("SQLite 后端读取到 %d 字节数据\n", len(data))
}

// Example_noCacheMode 纯持久化模式（不使用缓存）
func Example_noCacheMode() {
	// 禁用 Redis 缓存，仅使用持久化存储
	config := Store.TileStorageConfig{
		Backend:     Store.BackendBBolt,
		DBDir:       "/data/tiles_persist",
		EnableCache: false, // 禁用缓存
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 所有数据直接写入持久化层
	storage.Put("vector", "01010101", []byte("vector_data"))
	fmt.Println("纯持久化模式，无缓存")
}

// Example_cacheWarmup 缓存预热示例
func Example_cacheWarmup() {
	config := Store.TileStorageConfig{
		Backend:         Store.BackendBBolt,
		DBDir:           "/data/tiles",
		RedisAddr:       "localhost:6379",
		CacheExpiration: 1 * time.Hour,
		EnableCache:     true,
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	// 预热常用瓦片到缓存
	hotTiles := []string{
		"0000", "0001", "0010", "0011",
		"0100", "0101", "0110", "0111",
	}

	err = storage.WarmupCache("imagery", hotTiles)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("预热 %d 条瓦片到 Redis 缓存\n", len(hotTiles))
}

// Example_cacheInvalidation 缓存失效示例
func Example_cacheInvalidation() {
	config := Store.TileStorageConfig{
		Backend:         Store.BackendBBolt,
		DBDir:           "/data/tiles",
		RedisAddr:       "localhost:6379",
		CacheExpiration: 1 * time.Hour,
		EnableCache:     true,
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close()

	tileKey := "01230123"

	// 写入数据
	storage.Put("imagery", tileKey, []byte("old_data"))

	// 清除缓存（强制下次从持久化重新加载）
	err = storage.InvalidateCache("imagery", tileKey)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("缓存已清除，下次读取将从持久化层加载")
}

// Example_asyncPersist 异步持久化示例（Redis写缓冲 + 后台持久化）
func Example_asyncPersist() {
	// 启用异步持久化模式
	config := Store.TileStorageConfig{
		Backend:            Store.BackendBBolt,
		DBDir:              "/data/tiles_async",
		RedisAddr:          "localhost:6379",
		CacheExpiration:    24 * time.Hour,     // 缓存24小时
		EnableCache:        true,               // 必须启用Redis
		EnableAsyncPersist: true,               // 启用异步持久化
		PersistBatchSize:   100,                // 每100条批量持久化
		PersistInterval:    10 * time.Second, // 或每10秒自动刷新
	}

	storage, err := Store.NewTileStorage(config)
	if err != nil {
		log.Fatal(err)
	}
	defer storage.Close() // 关闭时会刷新剩余数据

	// 下载的数据立即写入Redis（极速响应）
	for i := 0; i < 1000; i++ {
		tilekey := fmt.Sprintf("%08d", i) // 转换为合法tilekey
		tileData := []byte(fmt.Sprintf("downloaded_tile_%d", i))

		// 写入Redis（毫秒级），后台异步持久化到BBolt
		err = storage.Put("imagery", tilekey, tileData)
		if err != nil {
			log.Printf("写入失败: %v", err)
		}
	}

	// 检查待持久化任务数
	pending := storage.GetPendingPersistCount()
	fmt.Printf("待持久化任务数: %d\n", pending)

	// 关闭时会自动将剩余数据持久化
}

// Example_redisSafeDB 演示 Redis 自动选择安全数据库
func Example_redisSafeDB() {
	addr := "localhost:6379"

	// 1. 扫描所有数据库
	dbSizes, _ := Store.ScanAllRedisDBs(addr)
	fmt.Println("当前 Redis 数据库使用情况:")
	for db := 0; db < 3; db++ {
		fmt.Printf("  DB %d: %d keys\n", db, dbSizes[db])
	}

	_ = Store.InitRedis(addr, nil)
	defer Store.CloseRedis()

	// 写入测试数据
	_ = Store.PutTileRedis("", "imagery", "0000", []byte("test"), 0)

	// 获取所有数据类型的数据库分配
	allocation := Store.GetAllRedisDBs()
	fmt.Printf("数据库分配: %+v\n", allocation)

	fmt.Println("安全数据库选择成功")

	// 清理
	_ = Store.DeleteTileRedis("", "imagery", "0000")
}

// Example_redisEpoch 演示 Redis 元数据管理（epoch）
func Example_redisEpoch() {
	addr := "localhost:6379"

	// 为不同数据类型设置 epoch
	dataTypes := []string{"imagery", "terrain", "vector", "q2", "qp"}

	for i, dataType := range dataTypes {
		epoch := int64(1000 + i*100)
		_ = Store.SetEpoch(addr, dataType, epoch)
		fmt.Printf("%s epoch: %d\n", dataType, epoch)
	}

	// 读取 epoch
	epoch, _ := Store.GetEpoch(addr, "imagery")
	fmt.Printf("\nimagery 当前 epoch: %d\n", epoch)

	// 设置完整元数据
	metadata := map[string]interface{}{
		"epoch":       int64(12345),
		"version":     "v2.1.0",
		"update_time": "2024-01-15T10:30:00Z",
	}
	_ = Store.SetMetadata(addr, "imagery", metadata)

	// 读取所有元数据
	allMeta, _ := Store.GetMetadata(addr, "imagery")
	fmt.Println("\nimagery 完整元数据:")
	for k, v := range allMeta {
		fmt.Printf("  %s: %s\n", k, v)
	}

	Store.CloseRedis()
}
