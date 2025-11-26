package Store_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crawler-platform/Store"
)

// TestSQLitePutAndGet 测试 sqlite 写入与读取
func TestSQLitePutAndGet(t *testing.T) {
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
			err := Store.PutTileSQLite(tmpDir, dataType, tc.tilekey, tc.value)
			if err != nil {
				t.Fatalf("写入失败: %v", err)
			}

			// 读取数据
			got, err := Store.GetTileSQLite(tmpDir, dataType, tc.tilekey)
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

// TestSQLiteUpsert 测试 sqlite UPSERT 操作
func TestSQLiteUpsert(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "terrain"
	tilekey := "012301230123"

	// 第一次写入
	value1 := []byte("original_value")
	if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value1); err != nil {
		t.Fatalf("第一次写入失败: %v", err)
	}

	// 读取验证
	got1, err := Store.GetTileSQLite(tmpDir, dataType, tilekey)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(got1) != string(value1) {
		t.Errorf("第一次数据不匹配: got %q, want %q", got1, value1)
	}

	// 第二次写入（更新）
	value2 := []byte("updated_value")
	if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value2); err != nil {
		t.Fatalf("第二次写入失败: %v", err)
	}

	// 读取验证更新后的值
	got2, err := Store.GetTileSQLite(tmpDir, dataType, tilekey)
	if err != nil {
		t.Fatalf("读取更新后数据失败: %v", err)
	}
	if string(got2) != string(value2) {
		t.Errorf("更新后数据不匹配: got %q, want %q", got2, value2)
	}
	t.Log("UPSERT 测试通过")
}

// TestSQLiteDelete 测试 sqlite 删除操作
func TestSQLiteDelete(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "vector"
	tilekey := "0123012301230123"
	value := []byte("delete_test_data")

	// 先写入
	if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 验证存在
	if _, err := Store.GetTileSQLite(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("读取失败: %v", err)
	}

	// 删除
	if err := Store.DeleteTileSQLite(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("删除失败: %v", err)
	}

	// 验证已删除
	if _, err := Store.GetTileSQLite(tmpDir, dataType, tilekey); err == nil {
		t.Error("期望删除后读取失败，但成功了")
	}
	t.Log("删除测试通过")
}

// TestSQLiteCorruptRecovery 测试 sqlite 损坏文件自动修复
func TestSQLiteCorruptRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "imagery"
	tilekey := "01230123"

	// 正常写入
	originalValue := []byte("original_data")
	if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, originalValue); err != nil {
		t.Fatalf("写入失败: %v", err)
	}

	// 获取数据库文件路径并关闭连接
	dbPath := Store.GetDBPathForTest(tmpDir, dataType, tilekey)
	if err := Store.CloseAllSQLite(); err != nil {
		t.Fatalf("关闭连接失败: %v", err)
	}

	// 模拟损坏：写入无效数据
	if err := os.WriteFile(dbPath, []byte("corrupted_invalid_sqlite_data"), 0600); err != nil {
		t.Fatalf("模拟损坏失败: %v", err)
	}
	t.Logf("已模拟数据库损坏: %s", dbPath)

	// 重新写入（应触发自动修复）
	newValue := []byte("recovered_data")
	if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, newValue); err != nil {
		t.Fatalf("修复后写入失败: %v", err)
	}

	// 验证能正常读取新数据
	got, err := Store.GetTileSQLite(tmpDir, dataType, tilekey)
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

// TestSQLitePathStrategy 测试路径生成策略
func TestSQLitePathStrategy(t *testing.T) {
	tmpDir := "/data/tiles"
	dataType := "imagery"

	testCases := []struct {
		tilekey      string
		expectedPath string
	}{
		{"012", tmpDir + "/imagery/base.g3db"},                         // 长度3: 基础层
		{"01230123", tmpDir + "/imagery/base.g3db"},                    // 长度8: 基础层
		{"012301230", tmpDir + "/imagery/8/0123.g3db"},                 // 长度9: 8/目录管理
		{"012301230123", tmpDir + "/imagery/8/0123.g3db"},              // 长度12: 8/目录管理
		{"0123012301230", tmpDir + "/imagery/12/0123.g3db"},            // 长度13: 12/目录管理
		{"0123012301230123", tmpDir + "/imagery/12/0123.g3db"},         // 长度16: 12/目录管理
		{"01230123012301230", tmpDir + "/imagery/17/0123.g3db"},        // 长度17: 独立目录
		{"012301230123012301230123", tmpDir + "/imagery/24/0123.g3db"}, // 长度24: 独立目录
	}

	for _, tc := range testCases {
		t.Run(tc.tilekey, func(t *testing.T) {
			got := Store.GetDBPathForTest(tmpDir, dataType, tc.tilekey)
			if got != tc.expectedPath {
				t.Errorf("路径不匹配:\n  got:  %s\n  want: %s", got, tc.expectedPath)
			} else {
				t.Logf("✓ tilekey长度%d -> %s", len(tc.tilekey), got)
			}
		})
	}
}

// TestSQLiteConcurrency 测试并发写入
func TestSQLiteConcurrency(t *testing.T) {
	tmpDir := t.TempDir()
	dataType := "terrain"
	const goroutines = 10

	done := make(chan bool, goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			tilekey := "0123012301"
			value := []byte{byte(idx)}
			if err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value); err != nil {
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
	if _, err := Store.GetTileSQLite(tmpDir, dataType, tilekey); err != nil {
		t.Fatalf("并发后读取失败: %v", err)
	}
	t.Log("并发写入测试通过")
}

// TestSQLiteMultipleDataTypes 测试多数据类型隔离
func TestSQLiteMultipleDataTypes(t *testing.T) {
	tmpDir := t.TempDir()
	tilekey := "012301230123"

	dataTypes := []struct {
		name  string
		value []byte
	}{
		{"imagery", []byte("imagery_data")},
		{"terrain", []byte("terrain_data")},
		{"vector", []byte("vector_data")},
	}

	// 写入不同类型数据
	for _, dt := range dataTypes {
		if err := Store.PutTileSQLite(tmpDir, dt.name, tilekey, dt.value); err != nil {
			t.Fatalf("写入 %s 失败: %v", dt.name, err)
		}
	}

	// 验证各类型数据独立且正确
	for _, dt := range dataTypes {
		got, err := Store.GetTileSQLite(tmpDir, dt.name, tilekey)
		if err != nil {
			t.Fatalf("读取 %s 失败: %v", dt.name, err)
		}
		if string(got) != string(dt.value) {
			t.Errorf("%s 数据不匹配: got %q, want %q", dt.name, got, dt.value)
		}
		t.Logf("✓ %s 数据隔离正确", dt.name)
	}
}

// TestSQLiteBulkWrite 测试 sqlite 批量高速写入 200 条数据
func TestSQLiteBulkWrite(t *testing.T) {
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

		// 随机生成 64KB 以内的 value（避免占用过多内存）
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
		if err := Store.PutTileSQLite(tmpDir, dataType, rec.tilekey, rec.value); err != nil {
			t.Fatalf("写入第 %d 条数据失败: %v", i, err)
		}
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024) // MB/s

	t.Logf("SQLite 批量写入完成 (WAL 模式):")
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
		got, err := Store.GetTileSQLite(tmpDir, dataType, testData[idx].tilekey)
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

// TestSQLiteBulkWriteBatch 测试 sqlite 批量事务写入 200 条数据（优化版）
func TestSQLiteBulkWriteBatch(t *testing.T) {
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

	if err := Store.PutTilesSQLiteBatch(tmpDir, dataType, records); err != nil {
		t.Fatalf("批量写入失败: %v", err)
	}

	elapsed := time.Since(start)
	qps := float64(totalRecords) / elapsed.Seconds()
	throughput := float64(totalBytes) / elapsed.Seconds() / (1024 * 1024)

	t.Logf("SQLite 批量事务写入完成 (WAL+优化版):")
	t.Logf("  总记录数: %d 条", totalRecords)
	t.Logf("  总数据量: %.2f MB", float64(totalBytes)/(1024*1024))
	t.Logf("  总耗时: %v", elapsed)
	t.Logf("  写入速度: %.2f 条/秒", qps)
	t.Logf("  吞吐量: %.2f MB/s", throughput)
	t.Logf("  平均延迟: %v/条", elapsed/time.Duration(totalRecords))

	// 验证随机抽样读取
	sampleSize := 20
	var sampleKeys []string
	for k := range records {
		sampleKeys = append(sampleKeys, k)
		if len(sampleKeys) >= sampleSize {
			break
		}
	}

	for _, key := range sampleKeys {
		got, err := Store.GetTileSQLite(tmpDir, dataType, key)
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
