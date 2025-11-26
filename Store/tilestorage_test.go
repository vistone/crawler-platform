package Store

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// TestTileStorageBBoltWithCache 测试 BBolt + Redis 缓存
func TestTileStorageBBoltWithCache(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:         BackendBBolt,
		DBDir:           tmpDir,
		RedisAddr:       getRedisAddr(),
		CacheExpiration: 5 * time.Minute,
		EnableCache:     true,
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	tilekey := "01230123"
	value := []byte("test_cache_bbolt")

	// 1. 写入数据（应同时写入缓存和持久化）
	if err := storage.Put(dataType, tilekey, value); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 2. 读取数据（应从缓存命中）
	got, err := storage.Get(dataType, tilekey)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("数据不匹配: got %s, want %s", got, value)
	}

	// 3. 验证缓存存在
	exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
	if !exists {
		t.Error("缓存未写入")
	}

	// 4. 清除缓存
	if err := storage.InvalidateCache(dataType, tilekey); err != nil {
		t.Fatalf("清除缓存失败: %v", err)
	}

	// 5. 再次读取（应从持久化加载并回填缓存）
	got2, err := storage.Get(dataType, tilekey)
	if err != nil {
		t.Fatalf("缓存失效后读取失败: %v", err)
	}
	if string(got2) != string(value) {
		t.Errorf("持久化数据不匹配: got %s, want %s", got2, value)
	}

	// 6. 等待缓存回填（异步）
	time.Sleep(100 * time.Millisecond)

	// 7. 验证缓存已回填
	exists2, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
	if !exists2 {
		t.Error("缓存回填失败")
	}

	// 清理
	_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
}

// TestTileStorageSQLiteWithCache 测试 SQLite + Redis 缓存
func TestTileStorageSQLiteWithCache(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:         BackendSQLite,
		DBDir:           tmpDir,
		RedisAddr:       getRedisAddr(),
		CacheExpiration: 5 * time.Minute,
		EnableCache:     true,
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "terrain"
	tilekey := "32103210"
	value := []byte("test_cache_sqlite")

	// 写入并验证
	if err := storage.Put(dataType, tilekey, value); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	got, err := storage.Get(dataType, tilekey)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("数据不匹配: got %s, want %s", got, value)
	}

	// 清理
	_ = storage.Delete(dataType, tilekey)
}

// TestTileStorageWithoutCache 测试仅持久化（不启用缓存）
func TestTileStorageWithoutCache(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:     BackendBBolt,
		DBDir:       tmpDir,
		EnableCache: false, // 禁用缓存
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "vector"
	tilekey := "01010101"
	value := []byte("no_cache_test")

	// 写入
	if err := storage.Put(dataType, tilekey, value); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 读取
	got, err := storage.Get(dataType, tilekey)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("数据不匹配: got %s, want %s", got, value)
	}

	// 验证缓存未启用
	if storage.IsCacheEnabled() {
		t.Error("缓存应该被禁用")
	}
}

// TestTileStorageBatchWrite 测试批量写入（缓存+持久化）
func TestTileStorageBatchWrite(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:         BackendBBolt,
		DBDir:           tmpDir,
		RedisAddr:       getRedisAddr(),
		CacheExpiration: 10 * time.Minute,
		EnableCache:     true,
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	records := map[string][]byte{
		"0123":     []byte("data1"),
		"3210":     []byte("data2"),
		"01230123": []byte("data3"),
		"32103210": []byte("data4"),
	}

	// 批量写入
	if err := storage.PutBatch(dataType, records); err != nil {
		t.Fatalf("批量写入失败: %v", err)
	}

	// 验证每条数据
	for tilekey, expectedValue := range records {
		got, err := storage.Get(dataType, tilekey)
		if err != nil {
			t.Errorf("读取 %s 失败: %v", tilekey, err)
			continue
		}
		if string(got) != string(expectedValue) {
			t.Errorf("数据不匹配 %s: got %s, want %s", tilekey, got, expectedValue)
		}

		// 验证缓存也写入了
		exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if !exists {
			t.Errorf("缓存未写入: %s", tilekey)
		}
	}

	// 清理
	for tilekey := range records {
		_ = storage.Delete(dataType, tilekey)
	}
}

// TestTileStorageWarmupCache 测试缓存预热
func TestTileStorageWarmupCache(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:         BackendBBolt,
		DBDir:           tmpDir,
		RedisAddr:       getRedisAddr(),
		CacheExpiration: 5 * time.Minute,
		EnableCache:     true,
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	records := map[string][]byte{
		"0000": []byte("warmup1"),
		"1111": []byte("warmup2"),
		"2222": []byte("warmup3"),
	}

	// 1. 只写入持久化（不写缓存）
	for tilekey, value := range records {
		switch config.Backend {
		case BackendBBolt:
			_ = PutTileBBolt(tmpDir, dataType, tilekey, value)
		case BackendSQLite:
			_ = PutTileSQLite(tmpDir, dataType, tilekey, value)
		}
	}

	// 2. 验证缓存中不存在
	for tilekey := range records {
		exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if exists {
			t.Errorf("预热前缓存不应存在: %s", tilekey)
		}
	}

	// 3. 预热缓存
	var tilekeys []string
	for k := range records {
		tilekeys = append(tilekeys, k)
	}
	if err := storage.WarmupCache(dataType, tilekeys); err != nil {
		t.Fatalf("预热缓存失败: %v", err)
	}

	// 4. 验证缓存已加载
	for tilekey := range records {
		exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if !exists {
			t.Errorf("预热后缓存应存在: %s", tilekey)
		}
	}

	// 清理
	for tilekey := range records {
		_ = storage.Delete(dataType, tilekey)
	}
}

// TestTileStoragePerformance 测试缓存性能提升（对比有/无缓存）
func TestTileStoragePerformance(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "imagery"
	const testCount = 1000

	// 准备测试数据
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	testData := make(map[string][]byte)
	for i := 0; i < testCount; i++ {
		tilekey := ""
		for j := 0; j < 8; j++ {
			tilekey += string('0' + byte(rng.Intn(4)))
		}
		value := make([]byte, 1024) // 1KB 数据
		rng.Read(value)
		testData[tilekey] = value
	}

	// 测试 1: 无缓存
	t.Run("无缓存", func(t *testing.T) {
		config := TileStorageConfig{
			Backend:     BackendBBolt,
			DBDir:       tmpDir + "/no_cache",
			EnableCache: false,
		}
		storage, _ := NewTileStorage(config)
		defer storage.Close()

		// 写入
		start := time.Now()
		for tilekey, value := range testData {
			_ = storage.Put(dataType, tilekey, value)
		}
		writeTime := time.Since(start)

		// 读取
		start = time.Now()
		for tilekey := range testData {
			_, _ = storage.Get(dataType, tilekey)
		}
		readTime := time.Since(start)

		t.Logf("无缓存 - 写入耗时: %v, 读取耗时: %v", writeTime, readTime)
	})

	// 测试 2: 有缓存
	t.Run("有缓存", func(t *testing.T) {
		config := TileStorageConfig{
			Backend:         BackendBBolt,
			DBDir:           tmpDir + "/with_cache",
			RedisAddr:       getRedisAddr(),
			CacheExpiration: 5 * time.Minute,
			EnableCache:     true,
		}
		storage, _ := NewTileStorage(config)
		defer storage.Close()

		// 写入
		start := time.Now()
		for tilekey, value := range testData {
			_ = storage.Put(dataType, tilekey, value)
		}
		writeTime := time.Since(start)

		// 读取（第一次，缓存命中）
		start = time.Now()
		for tilekey := range testData {
			_, _ = storage.Get(dataType, tilekey)
		}
		readTime := time.Since(start)

		t.Logf("有缓存 - 写入耗时: %v, 读取耗时: %v (缓存命中)", writeTime, readTime)

		// 清理 Redis
		for tilekey := range testData {
			_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
		}
	})
}

// TestTileStorageAsyncPersist 测试异步持久化模式（Redis 写缓冲 + 后台持久化）
func TestTileStorageAsyncPersist(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:            BackendBBolt,
		DBDir:              tmpDir,
		RedisAddr:          getRedisAddr(),
		CacheExpiration:    10 * time.Minute,
		EnableCache:        true,
		EnableAsyncPersist: true,        // 启用异步持久化
		PersistBatchSize:   10,           // 小批次便于测试
		PersistInterval:    2 * time.Second, // 2秒刷新一次
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	testData := map[string][]byte{
		"0000": []byte("async_data_0"),
		"0001": []byte("async_data_1"),
		"0010": []byte("async_data_2"),
		"0011": []byte("async_data_3"),
		"0100": []byte("async_data_4"),
	}

	// 1. 写入数据（应立即写入 Redis）
	for tilekey, value := range testData {
		if err := storage.Put(dataType, tilekey, value); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	// 2. 立即从 Redis 读取（应能读到）
	for tilekey, expectedValue := range testData {
		got, err := GetTileRedis(config.RedisAddr, dataType, tilekey)
		if err != nil {
			t.Errorf("Redis 读取失败: %v", err)
			continue
		}
		if string(got) != string(expectedValue) {
			t.Errorf("Redis 数据不匹配: got %s, want %s", got, expectedValue)
		}
	}

	// 3. 立即从持久化读取（可能还未持久化）
	t.Logf("当前待持久化任务数: %d", storage.GetPendingPersistCount())

	// 4. 等待批次刷新（超过 PersistBatchSize 或 PersistInterval）
	t.Log("等待异步持久化...")
	time.Sleep(3 * time.Second) // 等待定时器触发

	// 5. 验证持久化成功
	for tilekey, expectedValue := range testData {
		var got []byte
		var err error

		switch config.Backend {
		case BackendBBolt:
			got, err = GetTileBBolt(tmpDir, dataType, tilekey)
		case BackendSQLite:
			got, err = GetTileSQLite(tmpDir, dataType, tilekey)
		}

		if err != nil {
			t.Errorf("持久化读取失败: %v", err)
			continue
		}
		if string(got) != string(expectedValue) {
			t.Errorf("持久化数据不匹配: got %s, want %s", got, expectedValue)
		}
	}

	t.Log("异步持久化验证通过")

	// 清理
	for tilekey := range testData {
		_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
	}
}

// TestTileStorageAsyncPersistBatch 测试异步持久化批次触发
func TestTileStorageAsyncPersistBatch(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:            BackendBBolt,
		DBDir:              tmpDir,
		RedisAddr:          getRedisAddr(),
		CacheExpiration:    10 * time.Minute,
		EnableCache:        true,
		EnableAsyncPersist: true,
		PersistBatchSize:   5,            // 批次大小 5
		PersistInterval:    60 * time.Second, // 长时间间隔，不依赖定时器
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"

	// 写入 5 条数据（刚好达到批次大小）
	testKeys := []string{"0000", "0001", "0010", "0011", "0100"}
	for i, tilekey := range testKeys {
		value := []byte(fmt.Sprintf("batch_data_%d", i))
		if err := storage.Put(dataType, tilekey, value); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	// 等待批次触发（应该很快）
	time.Sleep(1 * time.Second) // 增加等待时间

	// 验证持久化
	for i, tilekey := range testKeys {
		got, err := GetTileBBolt(tmpDir, dataType, tilekey)
		if err != nil {
			t.Errorf("持久化读取失败 (tilekey=%s): %v", tilekey, err)
		}
		expected := fmt.Sprintf("batch_data_%d", i)
		if string(got) != expected {
			t.Errorf("数据不匹配: got %s, want %s", got, expected)
		}
	}

	t.Log("批次触发持久化验证通过")

	// 清理
	for _, tilekey := range testKeys {
		_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
	}
}

// TestTileStorageAsyncPersistCloseFlush 测试关闭时刷新剩余数据
func TestTileStorageAsyncPersistCloseFlush(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:            BackendBBolt,
		DBDir:              tmpDir,
		RedisAddr:          getRedisAddr(),
		CacheExpiration:    10 * time.Minute,
		EnableCache:        true,
		EnableAsyncPersist: true,
		PersistBatchSize:   100,          // 大批次
		PersistInterval:    60 * time.Second, // 长间隔
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}

	dataType := "imagery"

	// 写入少量数据（不达到批次大小）
	for i := 0; i < 3; i++ {
		tilekey := fmt.Sprintf("%04d", i)
		value := []byte(fmt.Sprintf("close_flush_%d", i))
		if err := storage.Put(dataType, tilekey, value); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	t.Logf("当前待持久化任务数: %d", storage.GetPendingPersistCount())

	// 立即关闭（应该刷新剩余数据）
	storage.Close()

	// 验证持久化成功
	for i := 0; i < 3; i++ {
		tilekey := fmt.Sprintf("%04d", i)
		got, err := GetTileBBolt(tmpDir, dataType, tilekey)
		if err != nil {
			t.Errorf("持久化读取失败 (tilekey=%s): %v", tilekey, err)
			continue
		}
		expected := fmt.Sprintf("close_flush_%d", i)
		if string(got) != expected {
			t.Errorf("数据不匹配: got %s, want %s", got, expected)
		}
	}

	t.Log("关闭时刷新验证通过")

	// 清理
	for i := 0; i < 3; i++ {
		tilekey := fmt.Sprintf("%04d", i)
		_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
	}
}

// TestTileStorageAsyncPersistClearRedis 测试持久化后自动清理 Redis
func TestTileStorageAsyncPersistClearRedis(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:                BackendBBolt,
		DBDir:                  tmpDir,
		RedisAddr:              getRedisAddr(),
		CacheExpiration:        10 * time.Minute,
		EnableCache:            true,
		EnableAsyncPersist:     true,
		PersistBatchSize:       10,           // 增大批次，依赖定时触发
		PersistInterval:        2 * time.Second,
		ClearRedisAfterPersist: func() *bool { b := true; return &b }(), // 持久化后清理 Redis
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	testKeys := []string{"0000", "0001", "0010", "0011", "0100"}

	// 1. 写入数据到 Redis
	for i, tilekey := range testKeys {
		value := []byte(fmt.Sprintf("clear_test_%d", i))
		if err := storage.Put(dataType, tilekey, value); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	// 2. 立即验证 Redis 中存在
	time.Sleep(100 * time.Millisecond) // 等待 Redis 写入完成
	for _, tilekey := range testKeys {
		exists, err := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if err != nil {
			t.Errorf("检查 Redis 失败: %v", err)
		}
		if !exists {
			t.Errorf("Redis 中应该存在 key: %s", tilekey)
		}
	}

	// 3. 等待批次触发持久化 + 清理
	time.Sleep(3 * time.Second)

	// 4. 验证持久化成功
	for i, tilekey := range testKeys {
		got, err := GetTileBBolt(tmpDir, dataType, tilekey)
		if err != nil {
			t.Errorf("持久化读取失败: %v", err)
		}
		expected := fmt.Sprintf("clear_test_%d", i)
		if string(got) != expected {
			t.Errorf("数据不匹配: got %s, want %s", got, expected)
		}
	}

	// 5. 验证 Redis 已清理
	t.Log("验证 Redis 清理状态...")
	time.Sleep(2 * time.Second) // 等待异步删除完成

	for _, tilekey := range testKeys {
		exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if exists {
			t.Errorf("Redis 中不应该存在 key: %s (应已清理)", tilekey)
		}
	}

	t.Log("持久化后 Redis 清理验证通过")
}

// TestTileStorageAsyncPersistKeepRedis 测试保留 Redis 模式
func TestTileStorageAsyncPersistKeepRedis(t *testing.T) {
	tmpDir := t.TempDir()

	config := TileStorageConfig{
		Backend:                BackendBBolt,
		DBDir:                  tmpDir,
		RedisAddr:              getRedisAddr(),
		CacheExpiration:        10 * time.Minute,
		EnableCache:            true,
		EnableAsyncPersist:     true,
		PersistBatchSize:       3,
		PersistInterval:        2 * time.Second,
		ClearRedisAfterPersist: func() *bool { b := false; return &b }(), // 不清理 Redis
	}

	storage, err := NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建存储管理器失败: %v", err)
	}
	defer storage.Close()

	dataType := "imagery"
	testKeys := []string{"0000", "0001", "0010"}

	// 写入数据
	for i, tilekey := range testKeys {
		value := []byte(fmt.Sprintf("keep_redis_%d", i))
		if err := storage.Put(dataType, tilekey, value); err != nil {
			t.Fatalf("写入失败: %v", err)
		}
	}

	// 等待持久化
	time.Sleep(3 * time.Second)

	// 验证持久化成功
	for _, tilekey := range testKeys {
		_, err := GetTileBBolt(tmpDir, dataType, tilekey)
		if err != nil {
			t.Errorf("持久化读取失败: %v", err)
		}
	}

	// 验证 Redis 仍然存在
	time.Sleep(1 * time.Second) // 等待异步操作完成
	for _, tilekey := range testKeys {
		exists, _ := ExistsTileRedis(config.RedisAddr, dataType, tilekey)
		if !exists {
			t.Errorf("Redis 中应该保留 key: %s (ClearRedisAfterPersist=false)", tilekey)
		}
	}

	t.Log("保留 Redis 模式验证通过")

	// 清理
	for _, tilekey := range testKeys {
		_ = DeleteTileRedis(config.RedisAddr, dataType, tilekey)
	}
}
