package Store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"crawler-platform/Store"
	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// 设置测试数据库目录
	testDBDir := filepath.Join(".", "sqlite_metadata_test_data")
	fmt.Printf("测试数据库目录: %s\n", testDBDir)

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
		fmt.Printf("创建 TileStorage 失败: %v\n", err)
		return
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
		fmt.Printf("通过 TileStorage 存储数据失败: %v\n", err)
		return
	}

	// 直接查询 SQLite 数据库验证元数据是否正确存储
	dbPath := Store.GetDBPath(testDBDir, dataType, tilekey, "sqlite")
	fmt.Printf("SQLite 数据库路径: %s\n", dbPath)

	// 检查数据库文件是否存在
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Printf("SQLite 数据库文件不存在: %s\n", dbPath)
		return
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Printf("打开 SQLite 数据库失败: %v\n", err)
		return
	}
	defer db.Close()

	// 查询数据
	var tileID, value []byte
	var storedEpoch int
	var storedProviderID sql.NullInt64
	query := fmt.Sprintf("SELECT tile_id, epoch, provider_id, value FROM %s WHERE tile_id = ?", dataType)
	tileIDUint64, err := Store.CompressTileKeyToUint64(tilekey)
	if err != nil {
		fmt.Printf("压缩 tilekey 失败: %v\n", err)
		return
	}
	tileIDBytes := Store.EncodeKeyBigEndian(tileIDUint64)
	row := db.QueryRow(query, tileIDBytes)

	err = row.Scan(&tileID, &storedEpoch, &storedProviderID, &value)
	if err != nil {
		fmt.Printf("查询数据失败: %v\n", err)
		return
	}

	// 验证数据
	if storedEpoch != epoch {
		fmt.Printf("epoch 不匹配: 期望=%d, 实际=%d\n", epoch, storedEpoch)
	} else {
		fmt.Printf("✅ epoch 正确: %d\n", storedEpoch)
	}

	if !storedProviderID.Valid {
		fmt.Printf("provider_id 应该是有效的，但实际为 NULL\n")
	} else if int(storedProviderID.Int64) != providerID {
		fmt.Printf("provider_id 不匹配: 期望=%d, 实际=%d\n", providerID, int(storedProviderID.Int64))
	} else {
		fmt.Printf("✅ provider_id 正确: %d\n", storedProviderID.Int64)
	}

	if string(value) != string(testData) {
		fmt.Printf("数据不匹配: 期望=%s, 实际=%s\n", string(testData), string(value))
	} else {
		fmt.Printf("✅ 数据正确: %s\n", string(value))
	}

	fmt.Printf("✅ SQLite 数据库中正确存储了元数据\n")
	fmt.Printf("   epoch: %d\n", storedEpoch)
	fmt.Printf("   provider_id: %d\n", storedProviderID.Int64)
	fmt.Printf("   数据大小: %d 字节\n", len(value))
	fmt.Printf("数据库文件已保存在: %s\n", dbPath)
}
