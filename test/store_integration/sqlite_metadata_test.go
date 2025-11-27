package store_integration

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"crawler-platform/Store"
	_ "github.com/mattn/go-sqlite3"
)

func TestSQLiteMetadataStorage(t *testing.T) {
	// 设置测试数据库目录
	testDBDir := filepath.Join(os.TempDir(), fmt.Sprintf("sqlite_metadata_test_%d", time.Now().UnixNano()))
	t.Logf("测试数据库目录: %s", testDBDir)
	defer os.RemoveAll(testDBDir) // 测试完成后清理

	// 创建 TileStorage 实例
	config := Store.TileStorageConfig{
		Backend:                Store.BackendSQLite, // 使用 SQLite 作为持久化后端
		DBDir:                  testDBDir,
		RedisAddr:              "localhost:6379", // 使用本地 Redis
		EnableCache:            true,             // 启用 Redis 缓存
		EnableAsyncPersist:     false,            // 禁用异步持久化，以便直接验证
		ClearRedisAfterPersist: &[]bool{true}[0], // 持久化后清理 Redis
	}
	tileStorage, err := Store.NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建 TileStorage 失败: %v", err)
	}
	defer tileStorage.Close() // 确保关闭所有连接

	// 测试数据
	dataType := "imagery"
	tilekey := "001"
	testData := []byte("test data for metadata storage")
	epoch := 1029
	providerID := 5

	// 使用 PutWithMetadata 存储数据
	err = tileStorage.PutWithMetadata(dataType, tilekey, testData, epoch, &providerID)
	if err != nil {
		t.Fatalf("通过 TileStorage 存储数据失败: %v", err)
	}

	// 直接查询 SQLite 数据库验证元数据是否正确存储
	dbPath := Store.GetDBPath(testDBDir, dataType, tilekey, "sqlite")
	t.Logf("SQLite 数据库路径: %s", dbPath)
	
	// 检查数据库文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("SQLite 数据库文件不存在: %s", dbPath)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("打开 SQLite 数据库失败: %v", err)
	}
	defer db.Close()

	// 查询数据
	var tileID, value []byte
	var storedEpoch int
	var storedProviderID sql.NullInt64
	query := fmt.Sprintf("SELECT tile_id, epoch, provider_id, value FROM %s WHERE tile_id = ?", dataType)
	tileIDUint64, err := Store.CompressTileKeyToUint64(tilekey)
	if err != nil {
		t.Fatalf("压缩 tilekey 失败: %v", err)
	}
	tileIDBytes := Store.EncodeKeyBigEndian(tileIDUint64)
	row := db.QueryRow(query, tileIDBytes)

	err = row.Scan(&tileID, &storedEpoch, &storedProviderID, &value)
	if err != nil {
		t.Fatalf("查询数据失败: %v", err)
	}

	// 验证数据
	if storedEpoch != epoch {
		t.Errorf("epoch 不匹配: 期望=%d, 实际=%d", epoch, storedEpoch)
	}

	if !storedProviderID.Valid {
		t.Error("provider_id 应该是有效的，但实际为 NULL")
	} else if int(storedProviderID.Int64) != providerID {
		t.Errorf("provider_id 不匹配: 期望=%d, 实际=%d", providerID, int(storedProviderID.Int64))
	}

	if string(value) != string(testData) {
		t.Errorf("数据不匹配: 期望=%s, 实际=%s", string(testData), string(value))
	}

	t.Logf("✅ SQLite 数据库中正确存储了元数据")
	t.Logf("   epoch: %d", storedEpoch)
	t.Logf("   provider_id: %d", storedProviderID.Int64)
	t.Logf("   数据大小: %d 字节", len(value))
}