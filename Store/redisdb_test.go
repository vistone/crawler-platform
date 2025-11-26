package Store

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// getRedisAddr 获取 Redis 地址（支持环境变量配置）
func getRedisAddr() string {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}
	return addr
}

// TestRedisPutAndGet 测试基本的读写操作
func TestRedisPutAndGet(t *testing.T) {
	addr := getRedisAddr()
	dataType := "imagery"
	tilekey := "01230123"
	value := []byte("test_value_123")

	// 写入
	if err := PutTileRedis(addr, dataType, tilekey, value, 0); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 读取
	got, err := GetTileRedis(addr, dataType, tilekey)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	if string(got) != string(value) {
		t.Errorf("数据不匹配: got %s, want %s", got, value)
	}

	// 清理
	_ = DeleteTileRedis(addr, dataType, tilekey)
}

// TestRedisDelete 测试删除操作
func TestRedisDelete(t *testing.T) {
	addr := getRedisAddr()
	dataType := "terrain"
	tilekey := "32103210"
	value := []byte("delete_me")

	// 写入
	if err := PutTileRedis(addr, dataType, tilekey, value, 0); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 删除
	if err := DeleteTileRedis(addr, dataType, tilekey); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 验证不存在
	exists, err := ExistsTileRedis(addr, dataType, tilekey)
	if err != nil {
		t.Fatalf("检查存在性失败: %v", err)
	}
	if exists {
		t.Error("删除后 key 仍然存在")
	}
}

// TestRedisExpiration 测试过期时间
func TestRedisExpiration(t *testing.T) {
	addr := getRedisAddr()
	dataType := "vector"
	tilekey := "01010101"
	value := []byte("expire_test")

	// 写入并设置 2 秒过期
	if err := PutTileRedis(addr, dataType, tilekey, value, 2*time.Second); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 立即读取应该成功
	if _, err := GetTileRedis(addr, dataType, tilekey); err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	// 检查 TTL
	ttl, err := GetTileTTLRedis(addr, dataType, tilekey)
	if err != nil {
		t.Fatalf("获取 TTL 失败: %v", err)
	}
	if ttl <= 0 || ttl > 2*time.Second {
		t.Errorf("TTL 不在预期范围: %v", ttl)
	}

	// 等待过期
	time.Sleep(3 * time.Second)

	// 验证已过期
	exists, err := ExistsTileRedis(addr, dataType, tilekey)
	if err != nil {
		t.Fatalf("检查存在性失败: %v", err)
	}
	if exists {
		t.Error("过期后 key 仍然存在")
	}
}

// TestRedisConcurrency 测试并发读写
func TestRedisConcurrency(t *testing.T) {
	addr := getRedisAddr()
	dataType := "imagery"
	const goroutines = 10
	const opsPerGoroutine = 100

	done := make(chan bool, goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			for i := 0; i < opsPerGoroutine; i++ {
				tilekey := string([]byte{
					'0' + byte(id%4),
					'0' + byte(i%4),
					'0' + byte((id+i)%4),
					'0' + byte((id*i)%4),
				})
				value := []byte{byte(id), byte(i)}

				if err := PutTileRedis(addr, dataType, tilekey, value, 0); err != nil {
					t.Errorf("goroutine %d 写入失败: %v", id, err)
				}

				if _, err := GetTileRedis(addr, dataType, tilekey); err != nil {
					t.Errorf("goroutine %d 读取失败: %v", id, err)
				}
			}
			done <- true
		}(g)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	t.Logf("并发测试完成: %d goroutines × %d ops = %d 总操作", goroutines, opsPerGoroutine, goroutines*opsPerGoroutine)
}

// TestRedisBulkWrite 测试 Redis 单条写入 10000 条数据
func TestRedisBulkWrite(t *testing.T) {
	addr := getRedisAddr()
	dataType := "imagery"
	const totalRecords = 10000

	// 初始化随机种子
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 预生成测试数据
	type record struct {
		tilekey string
		value   []byte
	}
	testData := make([]record, totalRecords)
	usedKeys := make(map[string]bool)

	t.Log("正在生成测试数据...")
	for i := 0; i < totalRecords; {
		// 随机生成合法的 tilekey
		length := 8 + rng.Intn(13)
		tilekey := make([]byte, length)
		for j := 0; j < length; j++ {
			tilekey[j] = '0' + byte(rng.Intn(4))
		}
		key := string(tilekey)

		if usedKeys[key] {
			continue
		}
		usedKeys[key] = true

		// 随机生成 1MB 以内的 value
		valueSize := 100 + rng.Intn(1024*1024-100)
		value := make([]byte, valueSize)
		rng.Read(value)

		testData[i] = record{
			tilekey: key,
			value:   value,
		}
		i++
	}

	var totalBytes int64
	for _, rec := range testData {
		totalBytes += int64(len(rec.value))
	}
	t.Logf("测试数据生成完成: %d 条记录, 总大小 %.2f MB", totalRecords, float64(totalBytes)/(1024*1024))

	// 开始写入性能测试
	start := time.Now()

	for i, rec := range testData {
		if err := PutTileRedis(addr, dataType, rec.tilekey, rec.value, 0); err != nil {
			t.Fatalf("写入第 %d 条数据失败: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	t.Logf("Redis 批量写入完成:")
	t.Logf("  总记录数: %d 条", totalRecords)
	t.Logf("  总数据量: %.2f MB", float64(totalBytes)/(1024*1024))
	t.Logf("  总耗时: %v", elapsed)
	t.Logf("  写入速度: %.2f 条/秒", qps)
	t.Logf("  吞吐量: %.2f MB/s", throughput)
	t.Logf("  平均延迟: %v/条", elapsed/time.Duration(totalRecords))

	// 验证随机抽样读取
	sampleSize := 100
	for i := 0; i < sampleSize; i++ {
		idx := rng.Intn(totalRecords)
		got, err := GetTileRedis(addr, dataType, testData[idx].tilekey)
		if err != nil {
			t.Errorf("读取第 %d 条数据失败: %v", idx, err)
			continue
		}
		if len(got) != len(testData[idx].value) {
			t.Errorf("数据长度不匹配: got %d, want %d", len(got), len(testData[idx].value))
		}
	}
	t.Logf("随机抽样验证 %d 条记录通过", sampleSize)

	// 清理测试数据
	t.Log("清理测试数据...")
	for _, rec := range testData {
		_ = DeleteTileRedis(addr, dataType, rec.tilekey)
	}
}

// TestRedisBulkWriteBatch 测试 Redis 批量事务写入 10000 条数据（优化版）
func TestRedisBulkWriteBatch(t *testing.T) {
	addr := getRedisAddr()
	dataType := "imagery"
	const totalRecords = 10000

	// 初始化随机种子
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 预生成测试数据
	records := make(map[string][]byte)
	usedKeys := make(map[string]bool)

	t.Log("正在生成测试数据...")
	for i := 0; i < totalRecords; {
		// 随机生成合法的 tilekey
		length := 8 + rng.Intn(13)
		tilekey := make([]byte, length)
		for j := 0; j < length; j++ {
			tilekey[j] = '0' + byte(rng.Intn(4))
		}
		key := string(tilekey)

		if usedKeys[key] {
			continue
		}
		usedKeys[key] = true

		// 随机生成 1MB 以内的 value
		valueSize := 100 + rng.Intn(1024*1024-100)
		value := make([]byte, valueSize)
		rng.Read(value)

		records[key] = value
		i++
	}

	var totalBytes int64
	for _, val := range records {
		totalBytes += int64(len(val))
	}
	t.Logf("测试数据生成完成: %d 条记录, 总大小 %.2f MB", totalRecords, float64(totalBytes)/(1024*1024))

	// 开始批量写入性能测试
	start := time.Now()

	if err := PutTilesRedisBatch(addr, dataType, records, 0); err != nil {
		t.Fatalf("批量写入失败: %v", err)
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	t.Logf("Redis 批量 Pipeline 写入完成 (优化版):")
	t.Logf("  总记录数: %d 条", totalRecords)
	t.Logf("  总数据量: %.2f MB", float64(totalBytes)/(1024*1024))
	t.Logf("  总耗时: %v", elapsed)
	t.Logf("  写入速度: %.2f 条/秒", qps)
	t.Logf("  吞吐量: %.2f MB/s", throughput)
	t.Logf("  平均延迟: %v/条", elapsed/time.Duration(totalRecords))

	// 验证随机抽样读取
	sampleSize := 100
	var sampleKeys []string
	for k := range records {
		sampleKeys = append(sampleKeys, k)
		if len(sampleKeys) >= sampleSize {
			break
		}
	}

	for _, key := range sampleKeys {
		got, err := GetTileRedis(addr, dataType, key)
		if err != nil {
			t.Errorf("读取 key=%s 失败: %v", key, err)
			continue
		}
		if len(got) != len(records[key]) {
			t.Errorf("数据长度不匹配: got %d, want %d", len(got), len(records[key]))
		}
	}
	t.Logf("随机抽样验证 %d 条记录通过", len(sampleKeys))

	// 清理测试数据
	t.Log("清理测试数据...")
	for key := range records {
		_ = DeleteTileRedis(addr, dataType, key)
	}
}

// TestRedisSafeDBSelection 测试自动选择安全数据库
func TestRedisSafeDBSelection(t *testing.T) {
	addr := getRedisAddr()

	// 1. 扫描所有数据库
	dbSizes, err := ScanAllRedisDBs(addr)
	if err != nil {
		t.Fatalf("扫描数据库失败: %v", err)
	}

	t.Log("当前 Redis 数据库使用情况:")
	for db := 0; db < 16; db++ {
		t.Logf("  DB %2d: %d keys", db, dbSizes[db])
	}

	// 2. 初始化并自动选择
	err = InitRedis(addr, nil)
	if err != nil {
		t.Fatalf("初始化 Redis 失败: %v", err)
	}
	defer CloseRedis()

	// 3. 检查选择的数据库
	allocation := GetAllRedisDBs()
	t.Logf("数据库分配: %+v", allocation)

	if len(allocation) == 0 {
		t.Error("应该有数据库分配")
	}
}

// TestRedisDBIsolation 测试数据库隔离
func TestRedisDBIsolation(t *testing.T) {
	addr := getRedisAddr()

	// 在 DB 1 中写入测试数据
	client1 := redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   1,
	})
	defer client1.Close()

	ctx := context.Background()
	client1.Set(ctx, "test:isolation:key1", "value1", 0)

	// 自动选择数据库（应该选择 DB 0 或其他空库）
	err := InitRedis(addr, nil)
	if err != nil {
		t.Fatalf("初始化 Redis 失败: %v", err)
	}
	defer CloseRedis()

	selectedDB := GetRedisDB("test")
	t.Logf("选择的数据库: DB %d", selectedDB)

	// 写入数据到选择的数据库
	err = PutTileRedis("", "test", "0000", []byte("isolation_test"), 0)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 验证 DB 1 中的数据未被影响
	val1, err := client1.Get(ctx, "test:isolation:key1").Result()
	if err != nil || val1 != "value1" {
		t.Errorf("DB 1 的数据被影响了")
	}

	// 清理
	client1.Del(ctx, "test:isolation:key1")
	_ = DeleteTileRedis("", "test", "0000")

	t.Log("数据库隔离验证通过")
}

// TestRedisMultiDataTypeSeparation 测试多个数据类型独立分配数据库
func TestRedisMultiDataTypeSeparation(t *testing.T) {
	addr := getRedisAddr()

	// 清理环境
	CloseRedis()

	// 写入三种数据类型
	dataTypes := []string{"imagery", "terrain", "vector"}

	for _, dataType := range dataTypes {
		err := PutTileRedis(addr, dataType, "0000", []byte(dataType+"_data"), 0)
		if err != nil {
			t.Fatalf("%s 写入失败: %v", dataType, err)
		}
	}

	// 获取数据库分配
	allocation := GetAllRedisDBs()
	t.Log("数据类型数据库分配:")
	for dt, db := range allocation {
		t.Logf("  %s -> DB %d", dt, db)
	}

	// 验证每个数据类型使用不同的数据库
	if len(allocation) != 3 {
		t.Errorf("应该有 3 个数据类型，实际: %d", len(allocation))
	}

	usedDBs := make(map[int]bool)
	for _, db := range allocation {
		if usedDBs[db] {
			t.Errorf("数据库 DB %d 被多个数据类型共用", db)
		}
		usedDBs[db] = true

		if db < 0 || db > 15 {
			t.Errorf("数据库编号无效: %d", db)
		}
	}

	// 验证数据隔离：在各自的数据库中读取
	for _, dataType := range dataTypes {
		data, err := GetTileRedis(addr, dataType, "0000")
		if err != nil {
			t.Errorf("%s 读取失败: %v", dataType, err)
		}
		expected := dataType + "_data"
		if string(data) != expected {
			t.Errorf("%s 数据不匹配: got %s, want %s", dataType, data, expected)
		}
	}

	// 验证跨数据库隔离：直接访问其他数据库应该查不到
	ctx := context.Background()
	for dt1, db1 := range allocation {
		for dt2, db2 := range allocation {
			if dt1 == dt2 {
				continue
			}

			// 用 dt1 的 key 去 dt2 的数据库查找
			client := redis.NewClient(&redis.Options{
				Addr: addr,
				DB:   db2,
			})
			key := fmt.Sprintf("%s:0000", dt1)
			exists, _ := client.Exists(ctx, key).Result()
			client.Close()

			if exists > 0 {
				t.Errorf("%s 的数据不应该在 DB %d (应该在 DB %d)",
					dt1, db2, db1)
			}
		}
	}

	// 清理
	for _, dataType := range dataTypes {
		_ = DeleteTileRedis(addr, dataType, "0000")
	}
	CloseRedis()

	t.Log("多数据类型隔离验证通过")
}

// TestRedisFiveDataTypes 测试五种数据类型独立分配
func TestRedisFiveDataTypes(t *testing.T) {
	addr := getRedisAddr()

	// 清理环境
	CloseRedis()

	// 写入五种数据类型
	dataTypes := []string{"imagery", "terrain", "vector", "q2", "qp"}

	for _, dataType := range dataTypes {
		err := PutTileRedis(addr, dataType, "0000", []byte(dataType+"_test_data"), 0)
		if err != nil {
			t.Fatalf("%s 写入失败: %v", dataType, err)
		}
	}

	// 获取数据库分配
	allocation := GetAllRedisDBs()
	t.Log("五种数据类型数据库分配:")
	for _, dt := range dataTypes {
		db := allocation[dt]
		t.Logf("  %s -> DB %d", dt, db)
	}

	// 验证每个数据类型使用不同的数据库
	if len(allocation) != 5 {
		t.Errorf("应该有 5 个数据类型，实际: %d", len(allocation))
	}

	usedDBs := make(map[int]string)
	for dt, db := range allocation {
		if other, exists := usedDBs[db]; exists {
			t.Errorf("数据库 DB %d 被 %s 和 %s 共用", db, other, dt)
		}
		usedDBs[db] = dt

		if db < 0 || db > 15 {
			t.Errorf("%s 数据库编号无效: %d", dt, db)
		}
	}

	// 验证数据隔离
	for _, dataType := range dataTypes {
		data, err := GetTileRedis(addr, dataType, "0000")
		if err != nil {
			t.Errorf("%s 读取失败: %v", dataType, err)
		}
		expected := dataType + "_test_data"
		if string(data) != expected {
			t.Errorf("%s 数据不匹配: got %s, want %s", dataType, data, expected)
		}
	}

	// 清理
	for _, dataType := range dataTypes {
		_ = DeleteTileRedis(addr, dataType, "0000")
	}
	CloseRedis()

	t.Log("五种数据类型独立分配验证通过")
}

// TestRedisEpochManagement 测试 epoch 元数据管理
func TestRedisEpochManagement(t *testing.T) {
	addr := getRedisAddr()

	// 清理环境
	CloseRedis()

	dataTypes := []string{"imagery", "terrain", "vector", "q2", "qp"}

	// 为每个数据类型设置不同的 epoch
	for i, dataType := range dataTypes {
		epoch := int64(1000 + i*100) // 1000, 1100, 1200, ...
		err := SetEpoch(addr, dataType, epoch)
		if err != nil {
			t.Fatalf("%s 设置 epoch 失败: %v", dataType, err)
		}
	}

	// 验证读取
	t.Log("数据类型 epoch 分配:")
	for i, dataType := range dataTypes {
		got, err := GetEpoch(addr, dataType)
		if err != nil {
			t.Errorf("%s 读取 epoch 失败: %v", dataType, err)
		}
		expected := int64(1000 + i*100)
		if got != expected {
			t.Errorf("%s epoch 不匹配: got %d, want %d", dataType, got, expected)
		}
		t.Logf("  %s -> epoch %d", dataType, got)
	}

	// 测试更新 epoch
	newEpoch := int64(2000)
	err := SetEpoch(addr, "imagery", newEpoch)
	if err != nil {
		t.Fatalf("更新 imagery epoch 失败: %v", err)
	}

	got, _ := GetEpoch(addr, "imagery")
	if got != newEpoch {
		t.Errorf("imagery epoch 更新失败: got %d, want %d", got, newEpoch)
	}
	t.Logf("imagery epoch 更新成功: %d -> %d", 1000, newEpoch)

	// 测试不存在的 epoch
	nonExist, err := GetEpoch(addr, "nonexistent")
	if err != nil {
		t.Errorf("读取不存在的 epoch 失败: %v", err)
	}
	if nonExist != 0 {
		t.Errorf("不存在的 epoch 应该返回 0, got %d", nonExist)
	}

	CloseRedis()
	t.Log("epoch 管理验证通过")
}

// TestRedisMetadataManagement 测试元数据管理（epoch + 其他字段）
func TestRedisMetadataManagement(t *testing.T) {
	addr := getRedisAddr()

	CloseRedis()

	dataType := "imagery"

	// 设置多个元数据字段
	metadata := map[string]interface{}{
		"epoch":       int64(12345),
		"version":     "v2.1.0",
		"update_time": "2024-01-15T10:30:00Z",
		"tile_count":  int64(1000000),
	}

	err := SetMetadata(addr, dataType, metadata)
	if err != nil {
		t.Fatalf("设置元数据失败: %v", err)
	}

	// 读取所有元数据
	got, err := GetMetadata(addr, dataType)
	if err != nil {
		t.Fatalf("读取元数据失败: %v", err)
	}

	t.Log("imagery 元数据:")
	for key, value := range got {
		t.Logf("  %s: %s", key, value)
	}

	// 验证 epoch
	epoch, err := GetEpoch(addr, dataType)
	if err != nil {
		t.Errorf("读取 epoch 失败: %v", err)
	}
	if epoch != 12345 {
		t.Errorf("epoch 不匹配: got %d, want 12345", epoch)
	}

	if got["version"] != "v2.1.0" {
		t.Errorf("version 不匹配: got %s, want v2.1.0", got["version"])
	}

	CloseRedis()
	t.Log("元数据管理验证通过")
}
