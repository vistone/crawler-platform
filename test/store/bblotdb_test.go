package Store_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crawler-platform/Store"
)

// TestBBoltPutAndGet 测试 bbolt 写入与读取
func TestBBoltPutAndGet(t *testing.T) {
	// 创建临时测试目录
	tmpDir := t.TempDir()
	dataType := "imagery"

	testCases := []struct {
		name    string
		tilekey string
		value   []byte
	}{
		{"8层基础层", "01230123", []byte("base_layer_data")},
		{"10层集合目录8", "0123012301", []byte("layer10_data")},
		{"15层集合目录12", "012301230123012", []byte("layer15_data")},
		{"17层独立目录", "01230123012301230", []byte("layer17_data")},
		{"20层独立目录", "01230123012301230123", []byte("layer20_data")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 写入数据
			err := Store.PutTileBBolt(tmpDir, dataType, tc.tilekey, tc.value)
			if err != nil {
				t.Fatalf("写入失败: %v", err)
			}

			// 读取数据
			got, err := Store.GetTileBBolt(tmpDir, dataType, tc.tilekey)
			if err != nil {
				t.Fatalf("读取失败: %v", err)
			}

			// 验证数据一致性
			if string(got) != string(tc.value) {
				t.Errorf("数据不匹配: got %q, want %q", got, tc.value)
			}

			// 打印生成的数据库路径（验证分层策略）
			dbPath := Store.GetDBPathForTest(tmpDir, dataType, tc.tilekey)
			t.Logf("tilekey=%s (长度%d) -> 数据库路径: %s", tc.tilekey, len(tc.tilekey), dbPath)
		})
	}
}

// TestBBoltDelete 测试 bbolt 删除操作
func TestBBoltDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "terrain"
	tilekey := "012301230123"
	value := []byte("delete_test_data")

	// 先写入
	if err := Store.PutTileBBolt(tmpDir, dataType, tilekey, value); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 验证存在
	if _, err := Store.GetTileBBolt(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	// 删除
	if err := Store.DeleteTileBBolt(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 验证已删除
	if _, err := Store.GetTileBBolt(tmpDir, dataType, tilekey); err == nil {
		t.Error("期望删除后读取失败，但成功了")
	}
	t.Log("删除测试通过")
}

// TestBBoltCorruptRecovery 测试 bbolt 损坏文件自动修复
func TestBBoltCorruptRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "vector"
	tilekey := "01230123"

	// 正常写入
	originalValue := []byte("original_data")
	if err := Store.PutTileBBolt(tmpDir, dataType, tilekey, originalValue); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 获取数据库文件路径并关闭连接
	dbPath := Store.GetDBPathForTest(tmpDir, dataType, tilekey)
	if err := Store.CloseAllBBolt(); err != nil {
		t.Fatalf("关闭连接失败: %v", err)
	}

	// 模拟损坏：写入无效数据
	if err := os.WriteFile(dbPath, []byte("corrupted_invalid_bbolt_data"), 0600); err != nil {
		t.Fatalf("模拟损坏失败: %v", err)
	}
	t.Logf("已模拟数据库损坏: %s", dbPath)

	// 重新写入（应触发自动修复）
	newValue := []byte("recovered_data")
	if err := Store.PutTileBBolt(tmpDir, dataType, tilekey, newValue); err != nil {
		t.Fatalf("修复后写入失败: %v", err)
	}

	// 验证能正常读取新数据
	got, err := Store.GetTileBBolt(tmpDir, dataType, tilekey)
	if err != nil {
		t.Fatalf("修复后读取失败: %v", err)
	}
	if string(got) != string(newValue) {
		t.Errorf("数据不匹配: got %q, want %q", got, newValue)
	}

	// 验证备份文件已创建
	matches, _ := filepath.Glob(dbPath + ".corrupt.*")
	if len(matches) == 0 {
		t.Error("未找到损坏文件备份")
	} else {
		t.Logf("损坏文件已备份至: %s", matches[0])
	}
	t.Log("损坏修复测试通过")
}

// TestBBoltCompression 测试 tilekey 压缩唯一性
func TestBBoltCompression(t *testing.T) {
	testCases := []struct {
		tilekey string
		wantErr bool
	}{
		{"0", false},
		{"0123", false},
		{"012301230123012301230123", false},  // 24层最大长度
		{"", true},                           // 空字符串
		{"01230123012301230123012345", true}, // 超长
		{"0123456", true},                    // 非法字符
	}

	for _, tc := range testCases {
		t.Run(tc.tilekey, func(t *testing.T) {
			id, err := Store.CompressTileKeyToUint64(tc.tilekey)
			if (err != nil) != tc.wantErr {
				t.Errorf("compressTileKeyToUint64(%q) error = %v, wantErr %v", tc.tilekey, err, tc.wantErr)
				return
			}
			if err == nil {
				t.Logf("tilekey=%q (长度%d) -> 压缩ID=0x%016X", tc.tilekey, len(tc.tilekey), id)
			}
		})
	}
}

// TestBBoltConcurrency 测试并发写入
func TestBBoltConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "imagery"
	const goroutines = 10

	done := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			tilekey := "0123012301"
			value := []byte{byte(idx)}
			if err := Store.PutTileBBolt(tmpDir, dataType, tilekey, value); err != nil {
				t.Errorf("并发写入失败 [goroutine %d]: %v", idx, err)
			}
			done <- true
		}(i)
	}

	// 等待所有协程完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 验证能正常读取（最后一次写入的值）
	tilekey := "0123012301"
	if _, err := Store.GetTileBBolt(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("并发后读取失败: %v", err)
	}
	t.Log("并发写入测试通过")
}

// TestBBoltBulkWrite 测试 bbolt 批量高速写入 200 条数据
func TestBBoltBulkWrite(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "imagery"
	const totalRecords = 200

	// 初始化随机种子
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))

	// 预生成测试数据（避免随机生成影响性能测试）
	type record struct {
		tilekey string
		value   []byte
	}
	testData := make([]record, totalRecords)

	t.Log("正在生成测试数据...")
	// 使用 map 确保 tilekey 唯一
	usedKeys := make(map[string]bool)
	for i := 0; i < totalRecords; {
		// 随机生成合法的 tilekey（长度 8-20，仅包含 '0'-'3'）
		length := 8 + rng.Intn(13) // 8-20 层
		tilekey := make([]byte, length)
		for j := 0; j < length; j++ {
			tilekey[j] = '0' + byte(rng.Intn(4)) // 随机 '0'-'3'
		}
		key := string(tilekey)

		// 检查是否重复
		if usedKeys[key] {
			continue // 重新生成
		}
		usedKeys[key] = true

		// 随机生成 64KB 以内的 value（减少内存压力）
		valueSize := 100 + rng.Intn(64*1024-100) // 100B ~ 64KB
		value := make([]byte, valueSize)
		rng.Read(value) // 填充随机字节

		testData[i] = record{
			tilekey: key,
			value:   value,
		}
		i++ // 仅在成功生成唯一 key 时递增
	}

	var totalBytes int64
	for _, rec := range testData {
		totalBytes += int64(len(rec.value))
	}
	t.Logf("测试数据生成完成: %d 条记录, 总大小 %.2f MB", totalRecords, float64(totalBytes)/(1024*1024))

	// 开始写入性能测试
	start := time.Now()

	for i, rec := range testData {
		if err := Store.PutTileBBolt(tmpDir, dataType, rec.tilekey, rec.value); err != nil {
			t.Fatalf("写入第 %d 条数据失败: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024) // MB/s

	t.Logf("BBolt 批量写入完成:")
	t.Logf("  总记录数: %d 条", totalRecords)
	t.Logf("  总数据量: %.2f MB", float64(totalBytes)/(1024*1024))
	t.Logf("  总耗时: %v", elapsed)
	t.Logf("  写入速度: %.2f 条/秒", qps)
	t.Logf("  吞吐量: %.2f MB/s", throughput)
	t.Logf("  平均延迟: %v/条", elapsed/time.Duration(totalRecords))

	// 验证随机抽样读取
	sampleSize := 20
	for i := 0; i < sampleSize; i++ {
		idx := rng.Intn(totalRecords)
		got, err := Store.GetTileBBolt(tmpDir, dataType, testData[idx].tilekey)
		if err != nil {
			t.Errorf("读取第 %d 条数据失败: %v", idx, err)
			continue
		}
		if len(got) != len(testData[idx].value) {
			t.Errorf("数据长度不匹配: got %d, want %d", len(got), len(testData[idx].value))
		}
	}
	t.Logf("随机抽样验证 %d 条记录通过", sampleSize)
}

// TestBBoltBulkWriteBatch 测试 bbolt 批量事务写入 200 条数据（优化版）
func TestBBoltBulkWriteBatch(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "imagery"
	const totalRecords = 200

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

		// 随机生成 64KB 以内的 value
		valueSize := 100 + rng.Intn(64*1024-100)
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

	if err := Store.PutTilesBBoltBatch(tmpDir, dataType, records); err != nil {
		t.Fatalf("批量写入失败: %v", err)
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	t.Logf("BBolt 批量事务写入完成 (优化版):")
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
		got, err := Store.GetTileBBolt(tmpDir, dataType, key)
		if err != nil {
			t.Errorf("读取 key=%s 失败: %v", key, err)
			continue
		}
		if len(got) != len(records[key]) {
			t.Errorf("数据长度不匹配: got %d, want %d", len(got), len(records[key]))
		}
	}
	t.Logf("随机抽样验证 %d 条记录通过", len(sampleKeys))
}
