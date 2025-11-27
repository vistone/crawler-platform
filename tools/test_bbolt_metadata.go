package Store

import (
	"fmt"
	"path/filepath"

	"crawler-platform/Store"
)

func main() {
	// 设置测试数据库目录
	testDBDir := filepath.Join(".", "bbolt_metadata_test_data")
	fmt.Printf("测试数据库目录: %s\n", testDBDir)

	// 创建 TileStorage 实例
	config := Store.TileStorageConfig{
		Backend:                Store.BackendBBolt, // 使用 BBolt 作为持久化后端
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

	// 从 BBolt 数据库中读取数据
	data, err := Store.GetTileBBolt(testDBDir, dataType, tilekey)
	if err != nil {
		fmt.Printf("从BBolt数据库读取数据失败: %v\n", err)
		return
	}

	fmt.Printf("从BBolt数据库读取到的数据:\n")
	fmt.Printf("  数据类型: %s\n", dataType)
	fmt.Printf("  TileKey: %s\n", tilekey)
	fmt.Printf("  数据大小: %d 字节\n", len(data))

	// 显示前几字节数据（十六进制）
	displayLen := len(data)
	if displayLen > 32 {
		displayLen = 32
	}
	fmt.Printf("  前%d字节数据（十六进制）: %x\n", displayLen, data[:displayLen])

	// 检查数据是否包含元数据
	if len(data) > 0 && data[0] == 1 {
		fmt.Printf("  数据包含元数据\n")
		if len(data) > 1 {
			// 尝试解析元数据
			parseMetadata(data)
		}
	} else {
		fmt.Printf("  数据不包含元数据\n")
	}

	fmt.Printf("数据库文件已保存在: %s\n", testDBDir)
}

func parseMetadata(data []byte) {
	if len(data) < 10 {
		fmt.Printf("  数据太短，无法解析元数据\n")
		return
	}

	// 检查是否有元数据标志
	hasMetadata := data[0]
	if hasMetadata != 1 {
		fmt.Printf("  没有元数据标志\n")
		return
	}

	pos := 1
	// 读取 epoch (4字节)
	if pos+4 <= len(data) {
		epoch := int32(uint32(data[pos])<<24 | uint32(data[pos+1])<<16 | uint32(data[pos+2])<<8 | uint32(data[pos+3]))
		fmt.Printf("  Epoch: %d\n", epoch)
		pos += 4
	}

	// 读取 provider_id 标志 (1字节)
	if pos < len(data) {
		hasProviderID := data[pos]
		pos++
		if hasProviderID == 1 {
			// 读取 provider_id (4字节)
			if pos+4 <= len(data) {
				providerID := int32(uint32(data[pos])<<24 | uint32(data[pos+1])<<16 | uint32(data[pos+2])<<8 | uint32(data[pos+3]))
				fmt.Printf("  Provider ID: %d\n", providerID)
				pos += 4
			}
		} else {
			fmt.Printf("  Provider ID: NULL\n")
		}
	}

	// 读取原始数据长度 (4字节)
	if pos+4 <= len(data) {
		dataLen := int32(uint32(data[pos])<<24 | uint32(data[pos+1])<<16 | uint32(data[pos+2])<<8 | uint32(data[pos+3]))
		fmt.Printf("  原始数据长度: %d\n", dataLen)
		pos += 4

		// 显示原始数据的前几字节
		if pos < len(data) {
			end := pos + 32
			if end > len(data) {
				end = len(data)
			}
			fmt.Printf("  原始数据前%d字节（十六进制）: %x\n", end-pos, data[pos:end])
		}
	}
}
