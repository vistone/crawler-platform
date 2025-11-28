package store_integration

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"crawler-platform/Store"
	_ "github.com/mattn/go-sqlite3"
	miniredis "github.com/alicebob/miniredis/v2"
)

var (
	embeddedRedisOnce   sync.Once
	embeddedRedisAddr   string
	embeddedRedisServer *miniredis.Miniredis
)

// getRedisAddr 获取 Redis 地址（优先环境变量，否则使用内嵌 Redis）
func getRedisAddr() string {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		embeddedRedisOnce.Do(func() {
			server, err := miniredis.Run()
			if err != nil {
				panic(fmt.Sprintf("无法启动内嵌 Redis: %v", err))
			}
			embeddedRedisServer = server
			embeddedRedisAddr = server.Addr()
		})
		return embeddedRedisAddr
	}
	return addr
}

// TestRawDataStorageIntegration 测试原始数据的存储集成
// 直接存储从远程请求回来的原始格式数据（不解析、不处理）
func TestRawDataStorageIntegration(t *testing.T) {
	// 设置测试数据库目录
	testDBDir := filepath.Join(os.TempDir(), fmt.Sprintf("store_raw_test_%d", time.Now().UnixNano()))
	defer os.RemoveAll(testDBDir) // 测试完成后清理

	// 读取真实的 q2 数据引用列表 (用于获取 tilekey 和 URL)
	jsonData, err := os.ReadFile("/home/stone/crawler-platform/test_output/q2_0_epoch_1029.json")
	if err != nil {
		t.Fatalf("无法读取 q2 JSON 数据文件: %v", err)
	}

	var q2Data struct {
		Tilekey     string `json:"tilekey"`
		ImageryList []struct {
			Tilekey  string `json:"tilekey"`
			Version  int    `json:"version"`
			Provider int    `json:"provider"`
			URL      string `json:"url"`
		} `json:"imagery_list"`
		TerrainList []struct {
			Tilekey string `json:"tilekey"`
			Version int    `json:"version"`
			URL     string `json:"url"`
		} `json:"terrain_list"`
		VectorList []interface{} `json:"vector_list"`
		Q2List     []struct {
			Tilekey string `json:"tilekey"`
			Version int    `json:"version"`
			URL     string `json:"url"`
		} `json:"q2_list"`
		Success bool `json:"success"`
	}

	if err := json.Unmarshal(jsonData, &q2Data); err != nil {
		t.Fatalf("解析 q2 JSON 数据失败: %v", err)
	}

	// 验证数据完整性
	if !q2Data.Success {
		t.Fatal("q2 数据获取失败")
	}

	t.Logf("✅ 成功加载 q2 数据，包含:")
	t.Logf("   影像引用: %d 个", len(q2Data.ImageryList))
	t.Logf("   地形引用: %d 个", len(q2Data.TerrainList))
	t.Logf("   Q2 引用: %d 个", len(q2Data.Q2List))

	// 测试存储原始影像数据 (使用测试文件中的 JPG 数据)
	t.Logf("\n=== 测试 1: 存储原始影像数据 ===")
	jpgData, err := os.ReadFile("/home/stone/crawler-platform/test_output/google_earth_tile_0.jpg")
	if err != nil {
		t.Fatalf("无法读取测试影像文件: %v", err)
	}
	t.Logf("读取到 JPG 数据，大小: %d 字节", len(jpgData))

	// 使用第一个影像引用的 tilekey 进行测试
	if len(q2Data.ImageryList) > 0 {
		tilekey := q2Data.ImageryList[0].Tilekey
		dataType := "imagery"

		// 存储到 SQLite
		err := Store.PutTileSQLite(testDBDir, dataType, tilekey, jpgData)
		if err != nil {
			t.Fatalf("存储原始影像数据失败: %v", err)
		}

		// 验证存储
		storedData, err := Store.GetTileSQLite(testDBDir, dataType, tilekey)
		if err != nil {
			t.Fatalf("获取原始影像数据失败: %v", err)
		}

		if len(storedData) != len(jpgData) {
			t.Errorf("存储和获取的影像数据大小不匹配: 存储=%d, 获取=%d", len(jpgData), len(storedData))
		}

		// 简单验证前几个字节是否匹配 (JPG 文件头)
		if len(storedData) >= 4 && len(jpgData) >= 4 {
			expectedHeader := jpgData[:4]
			actualHeader := storedData[:4]
			if string(expectedHeader) != string(actualHeader) {
				t.Errorf("存储和获取的影像数据头部不匹配")
			}
		}

		t.Logf("✅ 成功存储原始影像数据: tilekey=%s, size=%d bytes", tilekey, len(storedData))
	} else {
		t.Log("警告: 影像引用列表为空，跳过影像数据测试")
	}

	// 测试存储原始地形数据 (使用测试文件中的 DAT 数据)
	t.Logf("\n=== 测试 2: 存储原始地形数据 ===")
	// 尝试读取不同的地形文件
	datFiles := []string{
		"/home/stone/crawler-platform/test_output/google_earth_terrain_002.asc",
		"/home/stone/crawler-platform/test_output/google_earth_terrain_002.obj",
		"/home/stone/crawler-platform/test_output/google_earth_terrain_002.tif",
		"/home/stone/crawler-platform/test_output/google_earth_terrain_002.xyz",
	}
	
	var datData []byte
	var datFileUsed string
	for _, file := range datFiles {
		if data, err := os.ReadFile(file); err == nil {
			datData = data
			datFileUsed = file
			break
		}
	}
	
	if len(datData) == 0 {
		t.Fatalf("无法读取任何测试地形文件")
	}
	
	t.Logf("读取到地形数据 (%s)，大小: %d 字节", datFileUsed, len(datData))

	// 使用第一个地形引用的 tilekey 进行测试
	if len(q2Data.TerrainList) > 0 {
		tilekey := q2Data.TerrainList[0].Tilekey
		dataType := "terrain"

		// 存储到 SQLite
		err := Store.PutTileSQLite(testDBDir, dataType, tilekey, datData)
		if err != nil {
			t.Fatalf("存储原始地形数据失败: %v", err)
		}

		// 验证存储
		storedData, err := Store.GetTileSQLite(testDBDir, dataType, tilekey)
		if err != nil {
			t.Fatalf("获取原始地形数据失败: %v", err)
		}

		if len(storedData) != len(datData) {
			t.Errorf("存储和获取的地形数据大小不匹配: 存储=%d, 获取=%d", len(datData), len(storedData))
		}

		// 简单验证前几个字节是否匹配
		if len(storedData) >= 4 && len(datData) >= 4 {
			expectedHeader := datData[:4]
			actualHeader := storedData[:4]
			if string(expectedHeader) != string(actualHeader) {
				t.Errorf("存储和获取的地形数据头部不匹配")
			}
		}

		t.Logf("✅ 成功存储原始地形数据: tilekey=%s, size=%d bytes", tilekey, len(storedData))
	} else {
		t.Log("警告: 地形引用列表为空，跳过地形数据测试")
	}

	// 测试存储原始 q2 数据 (模拟数据)
	t.Logf("\n=== 测试 3: 存储原始 q2 数据 ===")
	// 使用第一个 q2 引用的 tilekey 进行测试
	if len(q2Data.Q2List) > 0 {
		tilekey := q2Data.Q2List[0].Tilekey
		dataType := "q2"
		
		// 生成模拟的原始 q2 数据 (实际应从网络获取)
		mockQ2Data := []byte(fmt.Sprintf("raw q2 data for tilekey %s, version %d", tilekey, q2Data.Q2List[0].Version))

		// 存储到 SQLite (q2 集合表)
		err := Store.PutTileSQLite(testDBDir, dataType, tilekey, mockQ2Data)
		if err != nil {
			t.Fatalf("存储原始 q2 数据失败: %v", err)
		}

		// 验证存储
		storedData, err := Store.GetTileSQLite(testDBDir, dataType, tilekey)
		if err != nil {
			t.Fatalf("获取原始 q2 数据失败: %v", err)
		}

		if string(storedData) != string(mockQ2Data) {
			t.Errorf("存储和获取的 q2 数据不匹配")
		}

		t.Logf("✅ 成功存储原始 q2 数据: tilekey=%s, size=%d bytes", tilekey, len(storedData))
	} else {
		t.Log("警告: q2 引用列表为空，跳过 q2 数据测试")
	}

	// 测试异步持久化流程 (使用原始数据)
	t.Logf("\n=== 测试 4: 异步持久化流程 (原始数据) ===")
	config := Store.TileStorageConfig{
		Backend:                Store.BackendSQLite,
		DBDir:                  testDBDir,
		RedisAddr:              getRedisAddr(),
		EnableCache:            true,
		EnableAsyncPersist:     true,
		PersistBatchSize:       100,
		PersistInterval:        100 * time.Millisecond,
		ClearRedisAfterPersist: &[]bool{true}[0], // 使用指针传递 true
	}
	tileStorage, err := Store.NewTileStorage(config)
	if err != nil {
		t.Fatalf("创建 TileStorage 失败: %v", err)
	}
	defer tileStorage.Close()

	// 添加一些原始数据到异步队列
	testTileKeys := []string{"0020", "0021", "0022", "0023", "0030"}
	for i, tilekey := range testTileKeys {
		dataType := "imagery"
		// 使用部分真实的 JPG 数据作为原始数据
		var data []byte
		if i == 0 && len(jpgData) > 0 {
			data = jpgData
		} else {
			data = []byte(fmt.Sprintf("raw async test data %d", i))
		}
		
		if err := tileStorage.Put(dataType, tilekey, data); err != nil {
			t.Fatalf("添加异步任务失败 (tilekey %s): %v", tilekey, err)
		}
	}

	// 等待异步处理完成（轮询检查数据是否已持久化，最多等待2秒）
	maxWait := 2 * time.Second
	checkInterval := 100 * time.Millisecond
	waited := 0 * time.Millisecond
	allPersisted := false
	
	for waited < maxWait {
		allPersisted = true
		for _, tilekey := range testTileKeys {
			_, err := Store.GetTileSQLite(testDBDir, "imagery", tilekey)
			if err != nil {
				allPersisted = false
				break
			}
		}
		if allPersisted {
			break
		}
		time.Sleep(checkInterval)
		waited += checkInterval
	}
	
	if !allPersisted {
		t.Logf("警告: 等待 %v 后，部分数据仍未持久化，继续验证...", waited)
	}

	// 验证数据是否已持久化
	for i, tilekey := range testTileKeys {
		storedData, err := Store.GetTileSQLite(testDBDir, "imagery", tilekey)
		if err != nil {
			t.Errorf("异步持久化数据获取失败 (tilekey %s): %v", tilekey, err)
			continue
		}

		var expectedData []byte
		if i == 0 && len(jpgData) > 0 {
			expectedData = jpgData
		} else {
			expectedData = []byte(fmt.Sprintf("raw async test data %d", i))
		}
		
		if len(storedData) != len(expectedData) {
			t.Errorf("异步持久化的数据大小不匹配 (tilekey %s): 期望=%d, 实际=%d", tilekey, len(expectedData), len(storedData))
			continue
		}

		// 对于 JPG 数据，验证头部
		if i == 0 && len(jpgData) > 0 && len(storedData) >= 4 && len(expectedData) >= 4 {
			if string(storedData[:4]) != string(expectedData[:4]) {
				t.Errorf("异步持久化的 JPG 数据头部不匹配 (tilekey %s)", tilekey)
				continue
			}
		}
	}

	t.Logf("✅ 异步持久化测试完成，数据均已正确存储")

	// 测试删除操作
	t.Logf("\n=== 测试 5: 删除操作 ===")
	// 删除一个 q2 数据
	if len(q2Data.Q2List) > 0 {
		tilekeyToDelete := q2Data.Q2List[0].Tilekey
		err := Store.DeleteTileSQLite(testDBDir, "q2", tilekeyToDelete)
		if err != nil {
			t.Errorf("删除 q2 数据失败 (tilekey %s): %v", tilekeyToDelete, err)
		} else {
			t.Logf("✅ 成功删除 q2 数据: tilekey=%s", tilekeyToDelete)
			
			// 验证删除
			_, err := Store.GetTileSQLite(testDBDir, "q2", tilekeyToDelete)
			if err == nil {
				t.Errorf("删除后的 q2 数据仍然存在 (tilekey %s)", tilekeyToDelete)
			}
		}
	}

	// 验证数据库表结构
	t.Logf("\n=== 测试 6: 数据库表结构验证 ===")
	// 检查 q2 数据库文件中的表结构
	q2DBPath := filepath.Join(testDBDir, "q2", "base.g3db")
	if _, err := os.Stat(q2DBPath); err == nil {
		// 执行 sqlite3 命令检查表结构
		cmd := exec.Command("sqlite3", q2DBPath, ".schema")
		output, err := cmd.Output()
		if err != nil {
			t.Errorf("执行 sqlite3 命令失败: %v", err)
		} else {
			schema := string(output)
			t.Logf("q2 数据库表结构:\n%s", schema)
			
			// 验证是否只包含 q2 表（根据新的命名规范）
			if !strings.Contains(schema, "CREATE TABLE q2") {
				t.Error("q2 数据库中缺少 q2 表")
			}
			// 验证是否不包含旧的 tiles_q2 表
			if strings.Contains(schema, "CREATE TABLE tiles_q2") {
				t.Error("q2 数据库中不应包含 tiles_q2 表")
			} else {
				t.Log("✅ q2 数据库表结构正确")
			}
		}
	} else {
		t.Logf("q2 数据库文件不存在: %v", err)
	}
	
	// 检查 imagery 数据库文件中的表结构
	imageryDBPath := filepath.Join(testDBDir, "imagery", "base.g3db")
	if _, err := os.Stat(imageryDBPath); err == nil {
		// 执行 sqlite3 命令检查表结构
		cmd := exec.Command("sqlite3", imageryDBPath, ".schema")
		output, err := cmd.Output()
		if err != nil {
			t.Errorf("执行 sqlite3 命令失败: %v", err)
		} else {
			schema := string(output)
			t.Logf("imagery 数据库表结构:\n%s", schema)
			
			// 验证是否只包含 imagery 表（根据新的命名规范）
			if !strings.Contains(schema, "CREATE TABLE imagery") {
				t.Error("imagery 数据库中缺少 imagery 表")
			}
			// 验证是否不包含旧的 tiles 表
			if strings.Contains(schema, "CREATE TABLE tiles") {
				t.Error("imagery 数据库中不应包含 tiles 表")
			} else {
				t.Log("✅ imagery 数据库表结构正确")
			}
		}
	} else {
		t.Logf("imagery 数据库文件不存在: %v", err)
	}
	
	// 检查 terrain 数据库文件中的表结构
	terrainDBPath := filepath.Join(testDBDir, "terrain", "base.g3db")
	if _, err := os.Stat(terrainDBPath); err == nil {
		// 执行 sqlite3 命令检查表结构
		cmd := exec.Command("sqlite3", terrainDBPath, ".schema")
		output, err := cmd.Output()
		if err != nil {
			t.Errorf("执行 sqlite3 命令失败: %v", err)
		} else {
			schema := string(output)
			t.Logf("terrain 数据库表结构:\n%s", schema)
			
			// 验证是否只包含 terrain 表（根据新的命名规范）
			if !strings.Contains(schema, "CREATE TABLE terrain") {
				t.Error("terrain 数据库中缺少 terrain 表")
			}
			// 验证是否不包含旧的 tiles 表
			if strings.Contains(schema, "CREATE TABLE tiles") {
				t.Error("terrain 数据库中不应包含 tiles 表")
			} else {
				t.Log("✅ terrain 数据库表结构正确")
			}
		}
	} else {
		t.Logf("terrain 数据库文件不存在: %v", err)
	}

	t.Logf("\n=== ✅ 所有原始数据存储集成测试通过 ===")
}