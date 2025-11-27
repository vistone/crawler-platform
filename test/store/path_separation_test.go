package Store_test

import (
	"os"
	"path/filepath"
	"testing"

	"crawler-platform/Store"

	_ "github.com/mattn/go-sqlite3"
)

// TestPathSeparation 测试SQLite和BBolt路径分离
func TestPathSeparation(t *testing.T) {
	// 创建临时测试目录
	tmpDir := t.TempDir()
	tilekey := "01230123"
	value := []byte("test_data")

	// 测试SQLite路径
	sqlitePath := Store.GetDBPathForTest(tmpDir, "imagery", tilekey, "sqlite")
	expectedSqlitePath := filepath.Join(tmpDir, "sqlite", "imagery", "base.g3db")
	if sqlitePath != expectedSqlitePath {
		t.Errorf("SQLite路径不正确: got %s, want %s", sqlitePath, expectedSqlitePath)
	}

	// 测试BBolt路径
	bboltPath := Store.GetDBPathForTest(tmpDir, "imagery", tilekey, "bbolt")
	expectedBboltPath := filepath.Join(tmpDir, "bbolt", "imagery", "base.g3db")
	if bboltPath != expectedBboltPath {
		t.Errorf("BBolt路径不正确: got %s, want %s", bboltPath, expectedBboltPath)
	}

	// 验证路径确实不同
	if sqlitePath == bboltPath {
		t.Error("SQLite和BBolt路径应该不同")
	}

	// 测试实际写入
	t.Log("测试SQLite写入...")
	err := Store.PutTileSQLite(tmpDir, "imagery", tilekey, value)
	if err != nil {
		t.Fatalf("SQLite写入失败: %v", err)
	}

	t.Log("测试BBolt写入...")
	err = Store.PutTileBBolt(tmpDir, "imagery", tilekey, value)
	if err != nil {
		t.Fatalf("BBolt写入失败: %v", err)
	}

	// 验证文件确实创建在不同目录
	if _, err := os.Stat(expectedSqlitePath); os.IsNotExist(err) {
		t.Error("SQLite数据库文件未创建")
	}

	if _, err := os.Stat(expectedBboltPath); os.IsNotExist(err) {
		t.Error("BBolt数据库文件未创建")
	}

	t.Log("✅ 路径分离测试通过")
}
