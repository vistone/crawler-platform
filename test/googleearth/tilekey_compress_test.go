package googleearth_test

import (
	"encoding/binary"
	"fmt"
	"testing"

	"crawler-platform/GoogleEarth"
)

// to6Bytes 将路径位(高48位)导出为6字节
func to6Bytes(p GoogleEarth.QuadtreePath) [6]byte {
	bits := p.PathBits() // 仅高位路径位(48bit)
	var buf [8]byte
	// 将64位按大端写入，再截取前6字节
	binary.BigEndian.PutUint64(buf[:], bits)
	var out [6]byte
	copy(out[:], buf[:6])
	return out
}

func TestTilekeyCompressionExample(t *testing.T) {
	// 示例tilekey（可按需替换）
	tilekey := "0123012301230123" // 长度16

	// 压缩为位串（使用已有QuadtreePath实现）
	p := GoogleEarth.NewQuadtreePathFromString(tilekey)

	// 校验还原字符串一致
	if p.AsString() != tilekey {
		t.Fatalf("AsString不一致: got=%s want=%s", p.AsString(), tilekey)
	}

	// 导出压缩后的关键信息
	lvl := p.Level()                            // 层级（低位）
	pathBits := p.PathBits()                    // 高48位路径位
	bytes6 := to6Bytes(p)                       // 6字节表示（仅路径位）
	hex48 := fmt.Sprintf("%012X", pathBits>>16) // 48bit十六进制（去掉低16位零）

	// 输出压缩形态，便于肉眼确认
	t.Logf("tilekey=%s", tilekey)
	t.Logf("Level(层级)=%d", lvl)
	t.Logf("PathBits(高48位) HEX=%s", hex48)
	t.Logf("6字节路径位=% X", bytes6[:])

	// 断言基本正确性（长度与层级匹配）
	if int(lvl) != len(tilekey) {
		t.Errorf("层级与长度不符: lvl=%d len=%d", lvl, len(tilekey))
	}
}
