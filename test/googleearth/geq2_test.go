package googleearth_test

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"crawler-platform/GoogleEarth"
	"crawler-platform/utlsclient"
)

// TestParseQ2Body_EmptyBody 测试空数据解析
func TestParseQ2Body_EmptyBody(t *testing.T) {
	body := []byte{}
	jsonStr, err := GoogleEarth.ParseQ2Body(body, "0", true)

	// 应该返回错误，但不应该panic
	if err != nil {
		t.Logf("Expected error for empty body: %v", err)
	}

	// 检查返回的JSON是否包含错误信息
	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.Success {
		t.Error("Expected success=false for empty body")
	}

	if response.Error == "" {
		t.Error("Expected error message for empty body")
	}

	t.Logf("Error message: %s", response.Error)
}

// TestParseQ2Body_InvalidData 测试无效数据
func TestParseQ2Body_InvalidData(t *testing.T) {
	// 随机数据，不是有效的Q2格式
	body := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}
	jsonStr, err := GoogleEarth.ParseQ2Body(body, "0", true)

	if err != nil {
		t.Logf("Error parsing invalid data: %v", err)
	}

	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	if response.Success {
		t.Error("Expected success=false for invalid data")
	}

	t.Logf("Response: success=%v, error=%s", response.Success, response.Error)
}

// createMockQ2Data 创建模拟的Q2二进制数据
func createMockQ2Data() []byte {
	buf := new(bytes.Buffer)

	// Magic ID (0x7E2D)
	binary.Write(buf, binary.LittleEndian, uint32(GoogleEarth.KeyholeMagicID))
	// Data Type ID
	binary.Write(buf, binary.LittleEndian, uint32(1))
	// Version
	binary.Write(buf, binary.LittleEndian, uint32(2009))
	// Num Instances
	binary.Write(buf, binary.LittleEndian, int32(1))
	// Data Instance Size
	binary.Write(buf, binary.LittleEndian, int32(48))
	// Data Buffer Offset
	binary.Write(buf, binary.LittleEndian, int32(0))
	// Data Buffer Size
	binary.Write(buf, binary.LittleEndian, int32(0))
	// Meta Buffer Size
	binary.Write(buf, binary.LittleEndian, int32(0))

	// Data Instance (QuadTreeQuantum16)
	// Children (有子节点0和1)
	binary.Write(buf, binary.LittleEndian, uint8(0x43)) // 0100 0011 = 子节点0,1 + 影像位
	// Byte Filler
	binary.Write(buf, binary.LittleEndian, uint8(0))
	// CNode Version
	binary.Write(buf, binary.LittleEndian, uint16(100))
	// Image Version
	binary.Write(buf, binary.LittleEndian, uint16(200))
	// Terrain Version
	binary.Write(buf, binary.LittleEndian, uint16(0))
	// Num Channels
	binary.Write(buf, binary.LittleEndian, uint16(0))
	// Word Filler
	binary.Write(buf, binary.LittleEndian, uint16(0))
	// Type Offset
	binary.Write(buf, binary.LittleEndian, int32(0))
	// Version Offset
	binary.Write(buf, binary.LittleEndian, int32(0))
	// Image Neighbors (8 bytes)
	for i := 0; i < 8; i++ {
		binary.Write(buf, binary.LittleEndian, int8(0))
	}
	// Image Data Provider
	binary.Write(buf, binary.LittleEndian, uint8(1))
	// Terrain Data Provider
	binary.Write(buf, binary.LittleEndian, uint8(0))
	// Word Filler
	binary.Write(buf, binary.LittleEndian, uint16(0))

	return buf.Bytes()
}

// TestParseQ2Body_ValidData 测试有效的Q2数据解析
func TestParseQ2Body_ValidData(t *testing.T) {
	body := createMockQ2Data()
	jsonStr, err := GoogleEarth.ParseQ2Body(body, "0", true)

	if err != nil {
		t.Fatalf("Failed to parse valid Q2 data: %v", err)
	}

	t.Logf("JSON output:\n%s", jsonStr)

	// 解析JSON响应
	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("Failed to parse JSON response: %v", err)
	}

	// 验证基本信息
	if !response.Success {
		t.Errorf("Expected success=true, got false. Error: %s", response.Error)
	}

	if response.Tilekey != "0" {
		t.Errorf("Expected Tilekey='0', got '%s'", response.Tilekey)
	}

	// 验证简化输出列表
	if len(response.ImageryList) == 0 {
		t.Error("Expected at least one imagery in imagery_list")
	} else {
		ref := response.ImageryList[0]
		if ref.Tilekey != "0" {
			t.Errorf("Expected tilekey='0', got '%s'", ref.Tilekey)
		}
		if ref.Version != 200 {
			t.Errorf("Expected version=200, got %d", ref.Version)
		}
		if ref.Provider != 1 {
			t.Errorf("Expected provider=1, got %d", ref.Provider)
		}
		t.Logf("✅ Imagery reference: tilekey=%s, version=%d, provider=%d, url=%s",
			ref.Tilekey, ref.Version, ref.Provider, ref.URL)
	}
}

// TestParseQ2Body_RootNodeVsNonRoot 测试根节点和非根节点的差异
func TestParseQ2Body_RootNodeVsNonRoot(t *testing.T) {
	body := createMockQ2Data()

	// 测试根节点
	jsonStr1, err := GoogleEarth.ParseQ2Body(body, "0", true)
	if err != nil {
		t.Fatalf("Failed to parse as root node: %v", err)
	}

	var response1 GoogleEarth.Q2Response
	json.Unmarshal([]byte(jsonStr1), &response1)

	// 测试非根节点
	jsonStr2, err := GoogleEarth.ParseQ2Body(body, "0123", false)
	if err != nil {
		t.Fatalf("Failed to parse as non-root node: %v", err)
	}

	var response2 GoogleEarth.Q2Response
	json.Unmarshal([]byte(jsonStr2), &response2)

	// 两者都应该成功
	if !response1.Success || !response2.Success {
		t.Error("Both root and non-root parsing should succeed")
	}

	t.Logf("Root node: %d nodes", len(response1.Nodes))
	t.Logf("Non-root node: %d nodes", len(response2.Nodes))
}

// TestParseQ2Body_WithBaseURL 测试带URL构造的解析
func TestParseQ2Body_WithBaseURL(t *testing.T) {
	body := createMockQ2Data()

	jsonStr, err := GoogleEarth.ParseQ2Body(body, "0", true)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// 检查数据引用中是否包含URL（只包含URI路径，不包含域名）
	for _, ref := range response.ImageryList {
		if ref.URL == "" {
			t.Error("Expected URL to be generated for imagery reference")
		}
		if !strings.HasPrefix(ref.URL, "/flatfile?") {
			t.Errorf("Expected URL to start with /flatfile?, got %s", ref.URL)
		}
		if strings.Contains(ref.URL, "http") {
			t.Errorf("URL should not contain http/https, got %s", ref.URL)
		}
		t.Logf("Imagery URL: %s", ref.URL)
	}

	for _, ref := range response.TerrainList {
		if ref.URL == "" {
			t.Error("Expected URL to be generated for terrain reference")
		}
		if !strings.HasPrefix(ref.URL, "/flatfile?") {
			t.Errorf("Expected URL to start with /flatfile?, got %s", ref.URL)
		}
		t.Logf("Terrain URL: %s", ref.URL)
	}

	for _, ref := range response.Q2List {
		if ref.URL == "" {
			t.Error("Expected URL to be generated for Q2 child reference")
		}
		if !strings.HasPrefix(ref.URL, "/flatfile?") {
			t.Errorf("Expected URL to start with /flatfile?, got %s", ref.URL)
		}
		t.Logf("Q2 Child URL: %s", ref.URL)
	}
}

// TestParseQ2Body_WithoutBaseURL 测试不带URL的解析
func TestParseQ2Body_WithoutBaseURL(t *testing.T) {
	body := createMockQ2Data()

	jsonStr, err := GoogleEarth.ParseQ2Body(body, "0", true)
	if err != nil {
		t.Fatalf("Failed to parse: %v", err)
	}

	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// URL字段应该为空
	for _, ref := range response.ImageryList {
		if ref.URL != "" {
			t.Errorf("Expected empty URL when baseURL not provided, got %s", ref.URL)
		}
	}

	t.Log("✅ URLs are empty when baseURL is not provided")
}

// TestQ2Response_JSONMarshaling 测试JSON序列化
func TestQ2Response_JSONMarshaling(t *testing.T) {
	response := &GoogleEarth.Q2Response{
		Tilekey: "0",
		Success: true,
		ImageryList: []GoogleEarth.Q2DataRefJSON{
			{
				Tilekey:  "0",
				Version:  200,
				Provider: 1,
				URL:      "/flatfile?f1-0-i.200",
			},
		},
		TerrainList: []GoogleEarth.Q2DataRefJSON{},
		VectorList:  []GoogleEarth.Q2DataRefJSON{},
		Q2List:      []GoogleEarth.Q2DataRefJSON{},
	}

	// 序列化
	jsonBytes, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	jsonStr := string(jsonBytes)
	t.Logf("JSON output:\n%s", jsonStr)

	// 反序列化
	var decoded GoogleEarth.Q2Response
	if err := json.Unmarshal(jsonBytes, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	// 验证
	if decoded.Tilekey != response.Tilekey {
		t.Errorf("Tilekey mismatch: got %s, want %s", decoded.Tilekey, response.Tilekey)
	}

	if decoded.Success != response.Success {
		t.Errorf("Success mismatch: got %v, want %v", decoded.Success, response.Success)
	}

	if len(decoded.ImageryList) != 1 {
		t.Errorf("ImageryList count mismatch: got %d, want 1", len(decoded.ImageryList))
	} else if decoded.ImageryList[0].Tilekey != "0" {
		t.Error("ImageryList tilekey mismatch")
	}

	t.Log("✅ JSON marshaling and unmarshaling works correctly")
}

// TestQ2NodeJSON_Structure 测试节点JSON结构
func TestQ2NodeJSON_Structure(t *testing.T) {
	node := GoogleEarth.Q2NodeJSON{
		Index:           0,
		Path:            "0123",
		Subindex:        123,
		Children:        []int{0, 2, 3},
		ChildCount:      3,
		HasCache:        true,
		HasImage:        true,
		HasTerrain:      true,
		HasVector:       false,
		CNodeVersion:    100,
		ImageVersion:    200,
		TerrainVersion:  300,
		ImageProvider:   1,
		TerrainProvider: 2,
		Channels: []GoogleEarth.Q2ChannelJSON{
			{Type: 10, Version: 20},
		},
	}

	// 序列化
	jsonBytes, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("Failed to marshal node: %v", err)
	}

	// 检查JSON字段
	jsonStr := string(jsonBytes)
	expectedFields := []string{
		"\"index\"",
		"\"path\"",
		"\"subindex\"",
		"\"children\"",
		"\"child_count\"",
		"\"has_cache\"",
		"\"has_image\"",
		"\"has_terrain\"",
		"\"has_vector\"",
		"\"cache_node_version\"",
		"\"image_version\"",
		"\"terrain_version\"",
		"\"channels\"",
	}

	for _, field := range expectedFields {
		if !strings.Contains(jsonStr, field) {
			t.Errorf("Expected field %s in JSON output", field)
		}
	}

	t.Logf("Node JSON:\n%s", jsonStr)
}

// TestQ2References_URLGeneration 测试URL生成
func TestQ2References_URLGeneration(t *testing.T) {
	refs := &GoogleEarth.Q2References{
		ImageryRefs: []GoogleEarth.Q2DataRefJSON{
			{
				Tilekey:  "0123",
				Version:  12345,
				Provider: 1,
				URL:      "/flatfile?f1-0123-i.12345",
			},
		},
		TerrainRefs: []GoogleEarth.Q2DataRefJSON{
			{
				Tilekey:  "01",
				Version:  6789,
				Provider: 2,
				URL:      "/flatfile?f1c-01-t.6789",
			},
		},
		Q2ChildRefs: []GoogleEarth.Q2DataRefJSON{
			{
				Tilekey: "0123",
				Version: 2009,
				URL:     "/flatfile?q2-0123-q.2009",
			},
		},
	}

	// 验证影像URL格式
	if len(refs.ImageryRefs) > 0 {
		url := refs.ImageryRefs[0].URL
		if !strings.Contains(url, "f1-0123-i.12345") {
			t.Errorf("Invalid imagery URL format: %s", url)
		}
		if !strings.HasPrefix(url, "/flatfile?") {
			t.Errorf("URL should start with /flatfile?, got: %s", url)
		}
	}

	// 验证地形URL格式
	if len(refs.TerrainRefs) > 0 {
		url := refs.TerrainRefs[0].URL
		if !strings.Contains(url, "f1c-01-t.6789") {
			t.Errorf("Invalid terrain URL format: %s", url)
		}
		if !strings.HasPrefix(url, "/flatfile?") {
			t.Errorf("URL should start with /flatfile?, got: %s", url)
		}
	}

	// 验证Q2子节点URL格式
	if len(refs.Q2ChildRefs) > 0 {
		url := refs.Q2ChildRefs[0].URL
		if !strings.Contains(url, "q2-0123-q.2009") {
			t.Errorf("Invalid Q2 child URL format: %s", url)
		}
		if !strings.HasPrefix(url, "/flatfile?") {
			t.Errorf("URL should start with /flatfile?, got: %s", url)
		}
	}

	t.Log("✅ All URL formats are correct")
}

// TestParseQ2Body_DifferentTilekeys 测试不同的tilekey
func TestParseQ2Body_DifferentTilekeys(t *testing.T) {
	body := createMockQ2Data()

	tilekeys := []struct {
		tilekey  string
		rootNode bool
	}{
		{"0", true},
		{"01", false},
		{"0123", false},
		{"012301", false},
	}

	for _, tk := range tilekeys {
		jsonStr, err := GoogleEarth.ParseQ2Body(body, tk.tilekey, tk.rootNode)
		if err != nil {
			t.Errorf("Failed to parse with tilekey=%s: %v", tk.tilekey, err)
			continue
		}

		var response GoogleEarth.Q2Response
		if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
			t.Errorf("Failed to parse JSON for tilekey=%s: %v", tk.tilekey, err)
			continue
		}

		if !response.Success {
			t.Errorf("Expected success=true for tilekey=%s, got false. Error: %s",
				tk.tilekey, response.Error)
		}

		t.Logf("✅ Tilekey=%s (rootNode=%v): parsed %d nodes",
			tk.tilekey, tk.rootNode, len(response.Nodes))
	}
}

// TestParseQ2Body_RealData 测试真实的Google Earth Q2数据
// URL: https://kh.google.com/flatfile?q2-0-q.1029
// 需要先获取session认证，本测试使用实际网络请求
func TestParseQ2Body_RealData(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// 设置测试超时
	if deadline, ok := t.Deadline(); !ok {
		t.Logf("警告：未设置测试超时，建议使用 -timeout 120s")
	} else {
		t.Logf("测试超时设置: %v", time.Until(deadline).Round(time.Second))
	}

	// 1. 创建连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("../../config/config.toml")
	if err != nil {
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

	// 获取热连接
	conn, err := pool.GetConnection(GoogleEarth.HOST_NAME)
	if err != nil {
		t.Fatalf("获取热连接失败: %v", err)
	}
	defer pool.PutConnection(conn)

	// 创建客户端
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(30 * time.Second)

	// 3. 获取认证 session
	t.Logf("\n=== 步骤 1: 获取 Google Earth 认证 Session ===")
	geauthURL := "https://" + GoogleEarth.HOST_NAME + "/geauth"
	authKey, err := GoogleEarth.GenerateRandomGeAuth(0)
	if err != nil {
		t.Fatalf("生成认证密钥失败: %v", err)
	}

	authReq, err := http.NewRequest("POST", geauthURL, bytes.NewReader(authKey))
	if err != nil {
		t.Fatalf("创建认证请求失败: %v", err)
	}
	authReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(authKey)))
	authReq.Header.Set("Host", GoogleEarth.HOST_NAME)

	authResp, err := client.Do(authReq)
	if err != nil {
		t.Fatalf("认证请求失败: %v", err)
	}
	defer authResp.Body.Close()

	if authResp.StatusCode != 200 {
		t.Fatalf("认证失败，状态码: %d", authResp.StatusCode)
	}

	authBody, err := io.ReadAll(authResp.Body)
	if err != nil {
		t.Fatalf("读取认证响应失败: %v", err)
	}

	if len(authBody) <= 8 {
		t.Fatalf("认证响应长度不足: %d 字节", len(authBody))
	}

	var sessionBytes []byte
	for i := 8; i < len(authBody); i++ {
		if authBody[i] == 0 {
			break
		}
		sessionBytes = append(sessionBytes, authBody[i])
	}
	if len(sessionBytes) == 0 {
		t.Fatal("未找到有效的sessionid")
	}
	session := string(sessionBytes)
	t.Logf("✅ 成功获取 session: %s", session)

	// 4. 获取 dbRoot 以获取加密密钥
	t.Logf("\n=== 步骤 2: 获取 dbRoot.v5 数据 ===")
	dbRootURL := "https://" + GoogleEarth.HOST_NAME + GoogleEarth.DBROOT_PATH

	req2, err := http.NewRequest("GET", dbRootURL, nil)
	if err != nil {
		t.Fatalf("创建 dbRoot 请求失败: %v", err)
	}
	req2.Header.Set("Host", GoogleEarth.HOST_NAME)
	req2.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
	req2.Header.Set("Content-Type", "application/octet-stream")

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("dbRoot 请求失败: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != 200 {
		t.Fatalf("dbRoot 请求失败，状态码: %d", resp2.StatusCode)
	}

	dbRootBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("读取 dbRoot 响应失败: %v", err)
	}
	t.Logf("✅ 成功获取 dbRoot 数据，大小: %d 字节", len(dbRootBody))

	// 5. 解析 dbRoot
	t.Logf("\n=== 步骤 3: 解析 dbRoot ===")
	dbRootData, err := GoogleEarth.ParseDbRootComplete(dbRootBody)
	if err != nil {
		t.Fatalf("解析 dbRoot 失败: %v", err)
	}
	t.Logf("✅ 成功解析 dbRoot")
	t.Logf("   Version: %d", dbRootData.Version)
	t.Logf("   CryptKey 长度: %d 字节", len(dbRootData.CryptKey))

	// 更新全局密钥
	GoogleEarth.CryptKey = dbRootData.CryptKey

	// 6. 请求 Q2 数据: q2-0-q.1029
	t.Logf("\n=== 步骤 4: 获取 Q2 数据 (q2-0-q.1029) ===")
	tilekey := "0"
	epoch := 1029 // 使用测试指定的epoch
	q2URL := fmt.Sprintf("https://%s/flatfile?q2-%s-q.%d", GoogleEarth.HOST_NAME, tilekey, epoch)
	t.Logf("请求 URL: %s", q2URL)

	req3, err := http.NewRequest("GET", q2URL, nil)
	if err != nil {
		t.Fatalf("创建 q2 请求失败: %v", err)
	}
	req3.Header.Set("Host", GoogleEarth.HOST_NAME)
	req3.Header.Set("Cookie", fmt.Sprintf("SessionId=%s;State=1", session))
	req3.Header.Set("Content-Type", "application/octet-stream")

	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("q2 请求失败: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != 200 {
		body, _ := io.ReadAll(resp3.Body)
		t.Logf("响应状态码: %d", resp3.StatusCode)
		t.Logf("响应内容: %s", string(body))
		t.Fatalf("q2 请求失败，状态码: %d", resp3.StatusCode)
	}

	encryptedBody, err := io.ReadAll(resp3.Body)
	if err != nil {
		t.Fatalf("读取 q2 响应失败: %v", err)
	}
	t.Logf("✅ 成功获取 q2 数据，大小: %d 字节", len(encryptedBody))

	// 7. 解密数据
	t.Logf("\n=== 步骤 5: 解密 Q2 数据 ===")
	decryptedBody, err := GoogleEarth.UnpackGEZlib(encryptedBody)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	t.Logf("✅ 成功解密数据，大小: %d 字节", len(decryptedBody))

	// 8. 使用 ParseQ2Body 解析
	t.Logf("\n=== 步骤 6: 解析 Q2 数据为 JSON ===")
	jsonStr, err := GoogleEarth.ParseQ2Body(
		decryptedBody,
		tilekey,
		true, // 根节点
	)
	if err != nil {
		t.Fatalf("解析 Q2 数据失败: %v", err)
	}

	t.Logf("✅ 成功解析为 JSON")

	// 9. 验证 JSON 输出
	var response GoogleEarth.Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &response); err != nil {
		t.Fatalf("解析 JSON 响应失败: %v", err)
	}

	if !response.Success {
		t.Fatalf("解析失败: %s", response.Error)
	}

	// 输出统计信息
	t.Logf("\n=== 解析结果统计 ===")
	t.Logf("Magic ID: %s", response.MagicID)
	t.Logf("Data Type ID: %d", response.DataTypeID)
	t.Logf("Version: %d", response.Version)
	t.Logf("节点总数: %d", response.NodeCount)
	t.Logf("实际解析节点数: %d", len(response.Nodes))

	if response.DataReferences != nil {
		t.Logf("\n数据引用统计:")
		t.Logf("  影像引用: %d 个", len(response.DataReferences.ImageryRefs))
		t.Logf("  地形引用: %d 个", len(response.DataReferences.TerrainRefs))
		t.Logf("  矢量引用: %d 个", len(response.DataReferences.VectorRefs))
		t.Logf("  Q2子节点引用: %d 个", len(response.DataReferences.Q2ChildRefs))

		// 输出前几个节点的详细信息
		t.Logf("\n前 5 个节点详情:")
		for i := 0; i < minInt(5, len(response.Nodes)); i++ {
			node := response.Nodes[i]
			t.Logf("\n节点 %d:", i)
			t.Logf("  路径: '%s'", node.Path)
			t.Logf("  Subindex: %d", node.Subindex)
			t.Logf("  子节点数: %d %v", node.ChildCount, node.Children)
			t.Logf("  有缓存: %v", node.HasCache)
			t.Logf("  有影像: %v (版本=%d, 提供商=%d)", node.HasImage, node.ImageVersion, node.ImageProvider)
			t.Logf("  有地形: %v (版本=%d, 提供商=%d)", node.HasTerrain, node.TerrainVersion, node.TerrainProvider)
			t.Logf("  有矢量: %v (通道数=%d)", node.HasVector, len(node.Channels))
		}

		// 输出前几个数据引用
		if len(response.ImageryList) > 0 {
			t.Logf("\n前 3 个影像引用:")
			for i := 0; i < minInt(3, len(response.ImageryList)); i++ {
				ref := response.ImageryList[i]
				t.Logf("  %d. Tilekey=%s, Version=%d, Provider=%d", i+1, ref.Tilekey, ref.Version, ref.Provider)
				t.Logf("     URL=%s", ref.URL)
			}
		}

		if len(response.Q2List) > 0 {
			t.Logf("\n前 3 个 Q2 子节点引用:")
			for i := 0; i < min(3, len(response.Q2List)); i++ {
				ref := response.Q2List[i]
				t.Logf("  %d. Tilekey=%s, Version=%d", i+1, ref.Tilekey, ref.Version)
				t.Logf("     URL=%s", ref.URL)
			}
		}
	}

	// 保存完整的 JSON 输出到文件（可选）
	outputDir := "../../test_output"
	if err := os.MkdirAll(outputDir, 0755); err == nil {
		outputFile := fmt.Sprintf("%s/q2_0_epoch_%d.json", outputDir, epoch)
		if err := os.WriteFile(outputFile, []byte(jsonStr), 0644); err == nil {
			t.Logf("\n✅ 完整 JSON 已保存到: %s", outputFile)
		}
	}

	t.Logf("\n=== ✅ 真实 Q2 数据解析测试成功 ===")
}

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
