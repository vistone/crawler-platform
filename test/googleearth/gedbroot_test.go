package googleearth_test

import (
	"encoding/binary"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"crawler-platform/GoogleEarth"
	pb "crawler-platform/GoogleEarth/pb"
	"crawler-platform/utlsclient"

	"google.golang.org/protobuf/proto"
)

// TestUpdateCryptKeyFromDBRoot_Valid 测试有效的 dbRoot 数据解析
func TestUpdateCryptKeyFromDBRoot_Valid(t *testing.T) {
	// 创建一个模拟的 dbRoot 响应（至少需要 1025 字节，因为 body[8:8+1016] 需要到 body[1023]）
	body := make([]byte, 1025)

	// 设置魔法数（前4字节）
	magic := uint32(0x12345678)
	binary.LittleEndian.PutUint32(body[:4], magic)

	// 设置 unk（字节 4-5）
	unk := uint16(0xABCD)
	binary.LittleEndian.PutUint16(body[4:6], unk)

	// 设置版本号（字节 6-7）
	rawVer := uint16(0x4210) // 解析后应该是 0x4210 ^ 0x4200 = 0x0010 = 16
	binary.LittleEndian.PutUint16(body[6:8], rawVer)

	// 填充密钥数据（字节 8-1023）
	for i := 8; i < 1024; i++ {
		body[i] = byte(i % 256)
	}

	// 调用函数
	version, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body)

	// 验证结果
	if err != nil {
		t.Fatalf("UpdateCryptKeyFromDBRoot() unexpected error: %v", err)
	}

	expectedVersion := uint16(0x0010) // 0x4210 ^ 0x4200
	if version != expectedVersion {
		t.Errorf("UpdateCryptKeyFromDBRoot() version = %d, want %d", version, expectedVersion)
	}

	// 验证全局变量 DBRootVersion 被正确设置
	if GoogleEarth.DBRootVersion != expectedVersion {
		t.Errorf("DBRootVersion = %d, want %d", GoogleEarth.DBRootVersion, expectedVersion)
	}

	// 验证 CryptKey 的前8字节为0
	for i := 0; i < 8; i++ {
		if GoogleEarth.CryptKey[i] != 0 {
			t.Errorf("CryptKey[%d] = %d, want 0", i, GoogleEarth.CryptKey[i])
		}
	}

	// 验证 CryptKey 的第 8-1023 字节是从 body 复制的
	for i := 8; i < 1024; i++ {
		expected := byte((i) % 256)
		if GoogleEarth.CryptKey[i] != expected {
			t.Errorf("CryptKey[%d] = %d, want %d", i, GoogleEarth.CryptKey[i], expected)
			break
		}
	}
}

// TestUpdateCryptKeyFromDBRoot_RealData 使用真实的 dbRoot.v5 数据进行测试
// 这是一个集成测试，需要网络连接
func TestUpdateCryptKeyFromDBRoot_RealData(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// 1. 从配置文件加载连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("../../config.toml")
	if err != nil {
		// 如果配置文件不存在，使用默认配置
		t.Logf("无法加载配置文件，使用默认配置: %v", err)
		config = &utlsclient.PoolConfig{
			MaxConnections:         100,
			MaxConnsPerHost:        10,
			MaxIdleConns:           20,
			ConnTimeout:            30 * time.Second,
			IdleTimeout:            60 * time.Second,
			MaxLifetime:            300 * time.Second,
			TestTimeout:            10 * time.Second,
			HealthCheckInterval:    30 * time.Second,
			CleanupInterval:        60 * time.Second,
			BlacklistCheckInterval: 300 * time.Second,
			DNSUpdateInterval:      1800 * time.Second,
			MaxRetries:             3,
		}
	}

	// 2. 创建连接池
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	// 3. 获取到 kh.google.com 的连接
	conn, err := pool.GetConnection(GoogleEarth.HOST_NAME)
	if err != nil {
		t.Fatalf("获取连接失败: %v", err)
	}
	defer pool.PutConnection(conn)

	// 4. 创建 HTTP 客户端并请求 dbRoot.v5
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(30 * time.Second)

	dbRootURL := "https://" + GoogleEarth.HOST_NAME + GoogleEarth.DBROOT_PATH
	t.Logf("正在请求: %s", dbRootURL)

	// 创建自定义请求，添加必要的请求头
	req, err := http.NewRequest("GET", dbRootURL, nil)
	if err != nil {
		t.Fatalf("创建请求失败: %v", err)
	}

	// 设置请求头（模拟真实的 Google Earth 客户端）
	req.Header.Set("User-Agent", "GoogleEarth/7.3.6.9345(Windows;Microsoft Windows (10.0.22631.0);en;kml:2.2;client:EC;type:default)")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate")
	req.Header.Set("Connection", "keep-alive")

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求 dbRoot.v5 失败: %v", err)
	}
	defer resp.Body.Close()

	// 5. 检查响应状态
	if resp.StatusCode != 200 {
		t.Fatalf("dbRoot.v5 返回非200状态码: %d", resp.StatusCode)
	}

	// 6. 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("读取响应体失败: %v", err)
	}

	t.Logf("dbRoot.v5 响应长度: %d 字节", len(body))

	// 7. 验证数据长度
	if len(body) <= 1024 {
		t.Fatalf("dbRoot.v5 数据太短: %d 字节（期望 > 1024）", len(body))
	}

	// 8. 解析 dbRoot 数据
	version, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body)
	if err != nil {
		t.Fatalf("UpdateCryptKeyFromDBRoot() 失败: %v", err)
	}

	t.Logf("✅ 成功解析真实 dbRoot 数据")
	t.Logf("   版本号: %d (0x%04X)", version, version)
	t.Logf("   原始版本值: 0x%04X", version^0x4200)

	// 9. 验证版本号已更新
	if GoogleEarth.DBRootVersion != version {
		t.Errorf("全局 DBRootVersion 未正确更新: got %d, want %d",
			GoogleEarth.DBRootVersion, version)
	}

	// 10. 验证 CryptKey 已更新
	if len(GoogleEarth.CryptKey) != 1024 {
		t.Errorf("CryptKey 长度错误: got %d, want 1024", len(GoogleEarth.CryptKey))
	}

	// 11. 验证前8字节为0
	allZero := true
	for i := 0; i < 8; i++ {
		if GoogleEarth.CryptKey[i] != 0 {
			allZero = false
			t.Errorf("CryptKey[%d] = %d, want 0", i, GoogleEarth.CryptKey[i])
			break
		}
	}
	if allZero {
		t.Logf("✅ CryptKey 前8字节正确置零")
	}

	// 12. 显示部分密钥数据（用于调试）
	t.Logf("   CryptKey[8:16]: % X", GoogleEarth.CryptKey[8:16])
	t.Logf("   CryptKey[1016:1024]: % X", GoogleEarth.CryptKey[1016:1024])

	// 13. 验证密钥数据不是全零（前8字节除外）
	hasNonZero := false
	for i := 8; i < 1024; i++ {
		if GoogleEarth.CryptKey[i] != 0 {
			hasNonZero = true
			break
		}
	}
	if !hasNonZero {
		t.Error("CryptKey[8:1024] 全部为零，可能解析有误")
	} else {
		t.Logf("✅ CryptKey 包含有效的非零数据")
	}

	// 14. 显示原始 body 的头部信息（调试用）
	magic := binary.LittleEndian.Uint32(body[:4])
	unk := binary.LittleEndian.Uint16(body[4:6])
	rawVer := binary.LittleEndian.Uint16(body[6:8])
	t.Logf("   原始数据头部:")
	t.Logf("     Magic: 0x%08X", magic)
	t.Logf("     Unknown: 0x%04X", unk)
	t.Logf("     Raw Version: 0x%04X", rawVer)
	t.Logf("     Calculated Version: 0x%04X ^ 0x4200 = 0x%04X", rawVer, rawVer^0x4200)

	// 15. 检查 1024 字节后的 protobuf 数据
	t.Logf("\n=== 检查 Protobuf 数据 ===")
	if len(body) > 1024 {
		protoData := body[1024:]
		t.Logf("原始 Protobuf 数据长度: %d 字节", len(protoData))
		if len(protoData) > 20 {
			t.Logf("前 20 字节: % X", protoData[:20])
		}
	}

	// 16. 尝试解析为 EncryptedDbRootProto
	t.Logf("\n=== 尝试解析 EncryptedDbRootProto ===")
	encrypted := &pb.EncryptedDbRootProto{}
	if err := proto.Unmarshal(body[1024:], encrypted); err != nil {
		t.Logf("解析 EncryptedDbRootProto 失败: %v", err)
	} else {
		t.Logf("✅ 成功解析 EncryptedDbRootProto")
		if encrypted.EncryptionType != nil {
			t.Logf("   加密类型: %v", *encrypted.EncryptionType)
		}
		if encrypted.EncryptionData != nil {
			t.Logf("   加密数据长度: %d", len(encrypted.EncryptionData))
		}
		if encrypted.DbrootData != nil {
			t.Logf("   DbRoot 数据长度: %d", len(encrypted.DbrootData))
		}
	}

	// 17. 使用 ParseDbRootComplete 完整解析 DbRoot 数据
	t.Logf("\n=== 使用 ParseDbRootComplete 完整解析 DbRoot ===")

	dbRootData, err := GoogleEarth.ParseDbRootComplete(body)
	if err != nil {
		t.Fatalf("ParseDbRootComplete() 失败: %v", err)
	}

	t.Logf("✅ 成功解析完整的 dbRoot 数据")
	t.Logf("   版本号: %d (0x%04X)", dbRootData.Version, dbRootData.Version)
	t.Logf("   CryptKey 长度: %d 字节", len(dbRootData.CryptKey))
	t.Logf("   CryptKey[8:16]: % X", dbRootData.CryptKey[8:16])
	t.Logf("   XML 数据长度: %d 字节", len(dbRootData.XMLData))
	t.Logf("   XML 前 200 字符:\n%s", string(dbRootData.XMLData[:min(200, len(dbRootData.XMLData))]))

	// 验证返回的数据
	if dbRootData.Version != version {
		t.Errorf("版本号不匹配: ParseDbRootComplete 返回 %d, UpdateCryptKeyFromDBRoot 返回 %d",
			dbRootData.Version, version)
	}

	if len(dbRootData.CryptKey) != 1024 {
		t.Errorf("CryptKey 长度错误: got %d, want 1024", len(dbRootData.CryptKey))
	}

	if len(dbRootData.XMLData) == 0 {
		t.Error("XML 数据为空")
	}

	// 保存为 XML 文件
	xmlFilePath := "../../output/dbRoot.xml"
	if err := os.WriteFile(xmlFilePath, dbRootData.XMLData, 0644); err != nil {
		t.Logf("⚠️  保存 XML 文件失败: %v", err)
	} else {
		t.Logf("✅ XML 文件已保存到: %s", xmlFilePath)
	}

	// 18. 测试 DbRootParser 接口
	t.Logf("\n=== 测试 DbRootParser 接口 ===")

	parser := GoogleEarth.NewDbRootParser()
	parseResult, err := parser.Parse(body)
	if err != nil {
		t.Fatalf("Parser.Parse() 失败: %v", err)
	}

	t.Logf("✅ 使用接口成功解析 dbRoot 数据")
	t.Logf("   接口返回版本号: %d", parser.GetVersion())
	t.Logf("   接口返回 CryptKey 长度: %d", len(parser.GetCryptKey()))
	t.Logf("   接口返回 XML 数据长度: %d", len(parser.GetXMLData()))

	// 验证接口返回的数据与直接解析的数据一致
	if parser.GetVersion() != dbRootData.Version {
		t.Errorf("接口版本号不匹配: got %d, want %d", parser.GetVersion(), dbRootData.Version)
	}

	if len(parser.GetXMLData()) != len(dbRootData.XMLData) {
		t.Errorf("接口 XML 数据长度不匹配: got %d, want %d",
			len(parser.GetXMLData()), len(dbRootData.XMLData))
	}

	if parseResult.Version != dbRootData.Version {
		t.Errorf("Parse 返回的版本号不匹配: got %d, want %d", parseResult.Version, dbRootData.Version)
	}

	// 19. 验证 Provider 解析
	t.Logf("\n=== 验证 Provider 信息解析 ===")

	if len(dbRootData.Providers) == 0 {
		t.Error("Providers 为空，未解析到任何数据")
	} else {
		t.Logf("✅ 成功解析 %d 个 Provider 信息", len(dbRootData.Providers))

		// 显示前 10 个 Provider
		count := 0
		for id, copyright := range dbRootData.Providers {
			if count < 10 {
				t.Logf("   Provider[%d]: %s", id, copyright)
				count++
			} else {
				break
			}
		}

		// 测试查找特定 Provider
		if copyright, exists := dbRootData.Providers[394]; exists {
			t.Logf("✅ 找到 Provider 394: %s", copyright)
		}

		if copyright, exists := dbRootData.Providers[65539]; exists {
			t.Logf("✅ 找到 Provider 65539: %s", copyright)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestUpdateCryptKeyFromDBRoot_TooShort 测试数据太短的情况
func TestUpdateCryptKeyFromDBRoot_TooShort(t *testing.T) {
	tests := []struct {
		name string
		size int
	}{
		{"empty data", 0},
		{"1 byte", 1},
		{"100 bytes", 100},
		{"1024 bytes exactly", 1024}, // 边界情况：正好1024字节
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := make([]byte, tt.size)
			_, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body)

			if tt.size <= 1024 {
				if err == nil {
					t.Errorf("UpdateCryptKeyFromDBRoot() with %d bytes should return error, got nil", tt.size)
				}
				if err != nil && err.Error() != "dbroot response too short" {
					t.Errorf("UpdateCryptKeyFromDBRoot() error = %q, want %q",
						err.Error(), "dbroot response too short")
				}
			}
		})
	}
}

// TestUpdateCryptKeyFromDBRoot_VersionXOR 测试版本号的异或运算
func TestUpdateCryptKeyFromDBRoot_VersionXOR(t *testing.T) {
	tests := []struct {
		rawVersion      uint16
		expectedVersion uint16
		description     string
	}{
		{0x4200, 0x0000, "0x4200 ^ 0x4200 = 0x0000"},
		{0x4201, 0x0001, "0x4201 ^ 0x4200 = 0x0001"},
		{0x4210, 0x0010, "0x4210 ^ 0x4200 = 0x0010"},
		{0x42FF, 0x00FF, "0x42FF ^ 0x4200 = 0x00FF"},
		{0x0000, 0x4200, "0x0000 ^ 0x4200 = 0x4200"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			body := make([]byte, 1025)
			binary.LittleEndian.PutUint16(body[6:8], tt.rawVersion)

			version, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body)
			if err != nil {
				t.Fatalf("UpdateCryptKeyFromDBRoot() unexpected error: %v", err)
			}

			if version != tt.expectedVersion {
				t.Errorf("Version = 0x%04X, want 0x%04X", version, tt.expectedVersion)
			}
		})
	}
}

// TestUpdateCryptKeyFromDBRoot_CryptKeySize 测试 CryptKey 数组大小
func TestUpdateCryptKeyFromDBRoot_CryptKeySize(t *testing.T) {
	body := make([]byte, 1025)
	_, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body)
	if err != nil {
		t.Fatalf("UpdateCryptKeyFromDBRoot() unexpected error: %v", err)
	}

	// 验证 CryptKey 长度正确
	if len(GoogleEarth.CryptKey) != 1024 {
		t.Errorf("CryptKey length = %d, want 1024", len(GoogleEarth.CryptKey))
	}
}

// TestUpdateCryptKeyFromDBRoot_MultipleUpdates 测试多次更新 CryptKey
func TestUpdateCryptKeyFromDBRoot_MultipleUpdates(t *testing.T) {
	// 第一次更新
	body1 := make([]byte, 1025)
	for i := 8; i < 1024; i++ {
		body1[i] = 0xAA
	}
	binary.LittleEndian.PutUint16(body1[6:8], 0x4201)

	version1, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body1)
	if err != nil {
		t.Fatalf("First UpdateCryptKeyFromDBRoot() unexpected error: %v", err)
	}

	if version1 != 0x0001 {
		t.Errorf("First update: version = %d, want 1", version1)
	}

	// 验证第一次更新的密钥
	for i := 8; i < 1024; i++ {
		if GoogleEarth.CryptKey[i] != 0xAA {
			t.Errorf("First update: CryptKey[%d] = 0x%02X, want 0xAA", i, GoogleEarth.CryptKey[i])
			break
		}
	}

	// 第二次更新（覆盖）
	body2 := make([]byte, 1025)
	for i := 8; i < 1024; i++ {
		body2[i] = 0xBB
	}
	binary.LittleEndian.PutUint16(body2[6:8], 0x4202)

	version2, err := GoogleEarth.UpdateCryptKeyFromDBRoot(body2)
	if err != nil {
		t.Fatalf("Second UpdateCryptKeyFromDBRoot() unexpected error: %v", err)
	}

	if version2 != 0x0002 {
		t.Errorf("Second update: version = %d, want 2", version2)
	}

	// 验证第二次更新覆盖了第一次的密钥
	for i := 8; i < 1024; i++ {
		if GoogleEarth.CryptKey[i] != 0xBB {
			t.Errorf("Second update: CryptKey[%d] = 0x%02X, want 0xBB", i, GoogleEarth.CryptKey[i])
			break
		}
	}
}
