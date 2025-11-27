package Store_test

import (
	"fmt"
	"log"

	"crawler-platform/Store"
)

func main() {
	// 读取BBolt数据库中的数据
	dataType := "imagery"
	tilekey := "002"
	dbDir := "/home/stone/crawler-platform/data"

	// 从BBolt数据库中读取数据
	data, err := Store.GetTileBBolt(dbDir, dataType, tilekey)
	if err != nil {
		log.Fatalf("从BBolt数据库读取数据失败: %v", err)
	}

	fmt.Printf("从BBolt数据库读取到的数据:\n")
	fmt.Printf("  数据类型: %s\n", dataType)
	fmt.Printf("  TileKey: %s\n", tilekey)
	fmt.Printf("  数据大小: %d 字节\n", len(data))
	fmt.Printf("  前64字节数据（十六进制）: %x\n", data[:64])

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
