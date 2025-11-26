package Store_test

import (
	"path"
	"strings"
	"testing"

	"crawler-platform/Store"
)

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
			expected: path.Join(dbdir, dataType) + "/base.g3db",
		},
		{
			name:     "L=9 -> 8/0000.g3db",
			length:   9,
			prefix:   "0000",
			expected: path.Join(dbdir, dataType, "8") + "/0000.g3db",
		},
		{
			name:     "L=12 -> 8/0000.g3db",
			length:   12,
			prefix:   "0000",
			expected: path.Join(dbdir, dataType, "8") + "/0000.g3db",
		},
		{
			name:     "L=13 -> 12/1234.g3db",
			length:   13,
			prefix:   "1234",
			expected: path.Join(dbdir, dataType, "12") + "/1234.g3db",
		},
		{
			name:     "L=16 -> 12/1234.g3db",
			length:   16,
			prefix:   "1234",
			expected: path.Join(dbdir, dataType, "12") + "/1234.g3db",
		},
		{
			name:     "L=17 -> 17/0000.g3db",
			length:   17,
			prefix:   "0000",
			expected: path.Join(dbdir, dataType, "17") + "/0000.g3db",
		},
		{
			name:     "L=20 -> 20/0000.g3db",
			length:   20,
			prefix:   "0000",
			expected: path.Join(dbdir, dataType, "20") + "/0000.g3db",
		},
		{
			name:     "L=24 -> 24/0000.g3db",
			length:   24,
			prefix:   "0000",
			expected: path.Join(dbdir, dataType, "24") + "/0000.g3db",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tilekey := makeTileKey(tc.prefix, tc.length)
			got := Store.GetDBPathForTest(dbdir, dataType, tilekey)
			if got != tc.expected {
				t.Errorf("unexpected path for length=%d, prefix=%s\n got: %s\nwant: %s", tc.length, tc.prefix, got, tc.expected)
			}
			// 输出形态以便肉眼确认
			t.Logf("最终库路径形态: %s", got)
		})
	}
}
