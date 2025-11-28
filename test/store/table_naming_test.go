package Store_test

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"crawler-platform/Store"
	_ "github.com/mattn/go-sqlite3"
)

// TestTableNamingConvention 测试表命名规范修改
func TestTableNamingConvention(t *testing.T) {
	// 检查 sqlite3 命令是否可用
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skipf("跳过测试: sqlite3 命令不可用: %v", err)
	}
	
	// 创建临时测试目录
	tmpDir := t.TempDir()
	tilekey := "01230123"
	value := []byte("test_data")

	// 测试不同的数据类型
	dataTypes := []string{"imagery", "terrain", "vector", "q2", "qp"}

	for _, dataType := range dataTypes {
		t.Run(dataType, func(t *testing.T) {
			// 写入数据
			err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value)
			if err != nil {
				t.Fatalf("写入 %s 数据失败: %v", dataType, err)
			}

			// 读取数据
			got, err := Store.GetTileSQLite(tmpDir, dataType, tilekey)
			if err != nil {
				t.Fatalf("读取 %s 数据失败: %v", dataType, err)
			}

			// 验证数据一致性
			if string(got) != string(value) {
				t.Errorf("%s 数据不匹配: got %q, want %q", dataType, got, value)
			}

			// 验证数据库表结构
			dbPath := Store.GetDBPathForTest(tmpDir, dataType, tilekey, "sqlite")
			if _, err := os.Stat(dbPath); err == nil {
				// 执行 sqlite3 命令检查表结构
				cmd := exec.Command("sqlite3", dbPath, ".schema")
				output, err := cmd.Output()
				if err != nil {
					// 如果 sqlite3 命令失败，跳过表结构验证
					if _, lookErr := exec.LookPath("sqlite3"); lookErr != nil {
						t.Skipf("跳过表结构验证: sqlite3 命令不可用: %v", lookErr)
					}
					t.Errorf("执行 sqlite3 命令失败: %v", err)
				} else {
					schema := string(output)
					t.Logf("%s 数据库表结构:\n%s", dataType, schema)
					
					// 验证表名是否符合新规范
					expectedTableName := dataType
					if dataType == "q2" || dataType == "qp" {
						// q2 和 qp 类型保持原命名
						if !strings.Contains(schema, fmt.Sprintf("CREATE TABLE %s", expectedTableName)) {
							t.Errorf("%s 数据库中缺少 %s 表", dataType, expectedTableName)
						}
					} else {
						// 其他类型直接使用数据类型作为表名
						if !strings.Contains(schema, fmt.Sprintf("CREATE TABLE %s", expectedTableName)) {
							t.Errorf("%s 数据库中缺少 %s 表", dataType, expectedTableName)
						}
						// 验证是否不包含旧的 tiles 表
						if strings.Contains(schema, "CREATE TABLE tiles") {
							t.Errorf("%s 数据库中不应包含 tiles 表", dataType)
						}
					}
				}
			}
		})
	}
}

// TestTableNamingWithSpecialCharacters 测试特殊字符数据类型的表命名
func TestTableNamingWithSpecialCharacters(t *testing.T) {
	tmpDir := t.TempDir()
	tilekey := "01230123"
	value := []byte("test_data")

	// 测试一些可能包含特殊字符的数据类型
	specialDataTypes := []string{"image-data", "3d-model", "point.cloud"}

	for _, dataType := range specialDataTypes {
		t.Run(dataType, func(t *testing.T) {
			// 对于包含特殊字符的数据类型，我们预期会失败，因为表名不能包含特殊字符
			err := Store.PutTileSQLite(tmpDir, dataType, tilekey, value)
			if err != nil {
				// 预期的错误，因为表名不能包含连字符等特殊字符
				t.Logf("数据类型 '%s' 创建表失败（预期行为）: %v", dataType, err)
			} else {
				t.Errorf("数据类型 '%s' 应该创建表失败，因为包含特殊字符", dataType)
			}
		})
	}
}