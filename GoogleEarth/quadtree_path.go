package GoogleEarth

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

import "fmt"

// 四叉树路径常量
const (
	MaxLevel   = 24 // 最大层级
	ChildCount = 4  // 每层子节点数量
)

// QuadtreePath 四叉树路径结构
// 使用 64 位整数压缩存储：低位存储层级，高位存储路径
type QuadtreePath struct {
	path uint64 // 压缩存储：高 48 位存路径（每层 2 bit），低 16 位存层级
}

const (
	levelBits    = 2                                       // 每层使用 2 bit
	levelBitMask = 0x03                                    // 层级位掩码
	totalBits    = 64                                      // 总位数
	pathMask     = ^(^uint64(0) >> (MaxLevel * levelBits)) // 路径掩码
	levelMask    = ^pathMask                               // 层级掩码
)

// NewQuadtreePath 从层级、行、列构造路径
// 四叉树编号规则：
//
//	c0  c1
//
// r1 [3] [2]
// r0 [0] [1]
func NewQuadtreePath(level, row, col uint32) QuadtreePath {
	if level > MaxLevel {
		level = MaxLevel
	}

	var path uint64
	// order[colBit][rowBit] 对应四叉树的编号
	order := [2][2]uint64{{0, 3}, {1, 2}}

	for j := uint32(0); j < level; j++ {
		right := (col >> (level - j - 1)) & 0x01
		top := (row >> (level - j - 1)) & 0x01
		path |= order[right][top] << (totalBits - (j+1)*levelBits)
	}

	path |= uint64(level)
	return QuadtreePath{path: path}
}

// NewQuadtreePathFromString 从字符串构造路径（如 "0123"）
func NewQuadtreePathFromString(s string) QuadtreePath {
	level := len(s)
	if level > MaxLevel {
		level = MaxLevel
	}

	var path uint64
	for j := 0; j < level; j++ {
		val := uint64(s[j] - '0')
		if val > 3 {
			val = 0
		}
		path |= (val & levelBitMask) << (totalBits - uint64(j+1)*levelBits)
	}
	path |= uint64(level)
	return QuadtreePath{path: path}
}

// Level 返回路径层级
func (p QuadtreePath) Level() uint32 {
	return uint32(p.path & levelMask)
}

// GetLevelRowCol 获取层级、行、列
func (p QuadtreePath) GetLevelRowCol() (level, row, col uint32) {
	rowBits := [4]uint32{0, 0, 1, 1}
	colBits := [4]uint32{0, 1, 1, 0}

	level = p.Level()
	var rowVal, colVal uint32

	for j := uint32(0); j < level; j++ {
		levelBits := p.levelBitsAtPos(j)
		rowVal = (rowVal << 1) | rowBits[levelBits]
		colVal = (colVal << 1) | colBits[levelBits]
	}

	return level, rowVal, colVal
}

// levelBitsAtPos 获取指定位置的层级位
func (p QuadtreePath) levelBitsAtPos(position uint32) uint32 {
	return uint32((p.path >> (totalBits - (position+1)*levelBits)) & levelBitMask)
}

// Parent 返回父路径
func (p QuadtreePath) Parent() QuadtreePath {
	level := p.Level()
	if level == 0 {
		return p
	}
	newLevel := level - 1
	newPath := (p.path & (pathMask << (levelBits * (MaxLevel - newLevel)))) | uint64(newLevel)
	return QuadtreePath{path: newPath}
}

// Child 返回第 i 个子路径（i 取值 0-3）
func (p QuadtreePath) Child(child uint32) QuadtreePath {
	if child > 3 {
		child = 0
	}
	level := p.Level()
	if level >= MaxLevel {
		return p
	}
	newLevel := level + 1
	newPath := p.pathBits() | (uint64(child) << (totalBits - newLevel*levelBits)) | uint64(newLevel)
	return QuadtreePath{path: newPath}
}

// pathBits 返回路径位
func (p QuadtreePath) pathBits() uint64 {
	return p.path & pathMask
}

// WhichChild 返回当前节点是父节点的第几个孩子（0-3）
func (p QuadtreePath) WhichChild() uint32 {
	level := p.Level()
	if level == 0 {
		return 0
	}
	return uint32((p.path >> (totalBits - level*levelBits)) & levelBitMask)
}

// AsString 转换为字符串表示（如 "0123"）
func (p QuadtreePath) AsString() string {
	level := p.Level()
	result := make([]byte, level)
	for i := uint32(0); i < level; i++ {
		result[i] = byte('0' + p.levelBitsAtPos(i))
	}
	return string(result)
}

// IsAncestorOf 判断是否是另一个路径的祖先（包括自身）
func (p QuadtreePath) IsAncestorOf(other QuadtreePath) bool {
	level := p.Level()
	otherLevel := other.Level()
	if level > otherLevel {
		return false
	}
	return p.pathBitsAtLevel(level) == other.pathBitsAtLevel(level)
}

// pathBitsAtLevel 返回指定层级的路径位
func (p QuadtreePath) pathBitsAtLevel(level uint32) uint64 {
	mask := pathMask << ((MaxLevel - level) * levelBits)
	return p.path & mask
}

// Concatenate 拼接路径
func (p QuadtreePath) Concatenate(subPath QuadtreePath) QuadtreePath {
	level := p.Level() + subPath.Level()
	if level > MaxLevel {
		return p
	}
	newPath := (p.path & pathMask) |
		((subPath.path & pathMask) >> (p.Level() * levelBits)) |
		uint64(level)
	return QuadtreePath{path: newPath}
}

// Advance 前进到下一个节点（前序遍历），返回是否成功
func (p *QuadtreePath) Advance(maxLevel uint32) bool {
	if maxLevel == 0 || maxLevel > MaxLevel {
		maxLevel = MaxLevel
	}
	level := p.Level()
	if level > maxLevel {
		return false
	}

	if level < maxLevel {
		*p = p.Child(0)
		return true
	}

	for p.WhichChild() == ChildCount-1 {
		if p.Level() == 0 {
			return false
		}
		*p = p.Parent()
	}
	return p.AdvanceInLevel()
}

// AdvanceInLevel 在同一层级前进到下一个节点
func (p *QuadtreePath) AdvanceInLevel() bool {
	pathBits := p.pathBits()
	level := p.Level()
	pathMaskAtLevel := pathMask << ((MaxLevel - level) * levelBits)

	if pathBits != pathMaskAtLevel {
		p.path += uint64(1) << (totalBits - level*levelBits)
		return true
	}
	return false
}

// Equal 判断两个路径是否相等
func (p QuadtreePath) Equal(other QuadtreePath) bool {
	return p.path == other.path
}

// LessThan 前序比较（用于排序）
func (p QuadtreePath) LessThan(other QuadtreePath) bool {
	level := p.Level()
	otherLevel := other.Level()
	minLevel := level
	if otherLevel < minLevel {
		minLevel = otherLevel
	}

	mask := ^(^uint64(0) >> (minLevel * levelBits))
	if mask&(p.path^other.path) != 0 {
		return p.pathBits() < other.pathBits()
	}
	return level < otherLevel
}

// AsIndex 转换为指定层级的索引（用于数组索引）
func (p QuadtreePath) AsIndex(level uint32) uint64 {
	return p.path >> (totalBits - level*levelBits)
}

// At 返回指定位置的分支值（0-3）
func (p QuadtreePath) At(position uint32) uint32 {
	if position >= p.Level() {
		return 0
	}
	return p.levelBitsAtPos(position)
}

// Truncate 截取前 n 层的路径
func (p QuadtreePath) Truncate(newLevel uint32) QuadtreePath {
	if newLevel >= p.Level() {
		return p
	}
	mask := pathMask << ((MaxLevel - newLevel) * levelBits)
	return QuadtreePath{path: (p.path & mask) | uint64(newLevel)}
}

// RelativePath 计算从 parent 到 child 的相对路径
func RelativePath(parent, child QuadtreePath) (QuadtreePath, error) {
	if !parent.IsAncestorOf(child) {
		return QuadtreePath{}, fmt.Errorf("parent is not ancestor of child")
	}
	levelDiff := child.Level() - parent.Level()
	newPath := (child.pathBits() << (parent.Level() * levelBits)) | uint64(levelDiff)
	return QuadtreePath{path: newPath}, nil
}
