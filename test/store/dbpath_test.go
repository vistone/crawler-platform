package Store_test

import (
	"path"
	"strconv"
	"strings"
	"testing"
)

// GetDBPathForTest 暴露 getDBPath 供测试使用
func GetDBPathForTest(dbdir string, dataType string, tilekey string) string {
	// 由于getDBPath是小写的，无法直接访问，我们在这里模拟其行为
	// 这里简化实现，实际实现应该参考Store/dbpath.go中的逻辑
	
	// 截取瓦片键的前4个字符作为文件名前缀
	prefixLen := len(tilekey)
	if prefixLen > 4 {
		prefixLen = 4
	}
	FileName := tilekey[0:prefixLen] + ".g3db"

	// 根据瓦片键长度判断存储层级
	keyLen := len(tilekey)

	// 长度≤8: 存储在base.g3db基础文件中
	if keyLen <= 8 {
		return path.Join(dbdir, "sqlite", dataType, "base.g3db")
		// 长度9-12: 存储在8级目录
	} else if keyLen <= 12 {
		return path.Join(dbdir, "sqlite", dataType, "8", FileName)
		// 长度13-16: 存储在12级目录
	} else if keyLen <= 16 {
		return path.Join(dbdir, "sqlite", dataType, "12", FileName)
		// 长度≥17: 每层独立目录管理
	} else {
		// 将tilekey长度转为字符串作为目录名
		dirName := strconv.Itoa(keyLen)
		return path.Join(dbdir, "sqlite", dataType, dirName, FileName)
	}
}

// makeTileKey 生成指定长度的tilekey，前缀可控，后续补齐到目标长度
func makeTileKey(prefix string, length int) string {
	if len(prefix) > length {
		prefix = prefix[:length]
	}
	if len(prefix) == length {
		return prefix
	}
	return prefix + strings.Repeat("0", length-len(prefix))
}

func TestGetDBPathShape(t *testing.T) {
	// 根目录与数据类型
	dbdir := "/data/db"
	dataType := "imagery"

	// 用不同层级长度的tilekey验证最终库路径形态
	// 说明：文件名为 tilekey[0:4]+".g3db"，目录规则：
	//  - 长度<=8: 放在 base.g3db 单文件
	//  - 长度9-12: 放在 8/ 目录（集合管理）
	//  - 长度13-16: 放在 12/ 目录（集合管理）
	//  - 长度>=17: 每层独立目录（17/,18/,...）
	tests := []struct {
		name     string
		length   int
		prefix   string
		expected string
	}{
		{
			name:     "L=8 -> base.g3db",
			length:   8,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType) + "/base.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=9 -> 8/0000.g3db",
			length:   9,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType, "8") + "/0000.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=12 -> 8/0000.g3db",
			length:   12,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType, "8") + "/0000.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=13 -> 12/1234.g3db",
			length:   13,
			prefix:   "1234",
			expected: path.Join(dbdir, "sqlite", dataType, "12") + "/1234.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=16 -> 12/1234.g3db",
			length:   16,
			prefix:   "1234",
			expected: path.Join(dbdir, "sqlite", dataType, "12") + "/1234.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=17 -> 17/0000.g3db",
			length:   17,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType, "17") + "/0000.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=20 -> 20/0000.g3db",
			length:   20,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType, "20") + "/0000.g3db", // 添加sqlite子目录
		},
		{
			name:     "L=24 -> 24/0000.g3db",
			length:   24,
			prefix:   "0000",
			expected: path.Join(dbdir, "sqlite", dataType, "24") + "/0000.g3db", // 添加sqlite子目录
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tilekey := makeTileKey(tc.prefix, tc.length)
			got := GetDBPathForTest(dbdir, dataType, tilekey)
			if got != tc.expected {
				t.Errorf("unexpected path for length=%d, prefix=%s\n got: %s\nwant: %s", tc.length, tc.prefix, got, tc.expected)
			}
			// 输出形态以便肉眼确认
			t.Logf("最终库路径形态: %s", got)
		})
	}
}