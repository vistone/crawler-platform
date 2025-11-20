package GoogleEarth

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

// QuadtreeNumbering 四叉树编号系统（Keyhole 特有）
// 提供 quadset 分割与 subindex 映射逻辑
type QuadtreeNumbering struct {
	*TreeNumbering // 嵌入通用树编号
}

const (
	// Keyhole 的 quadset 深度常量
	DefaultDepth = 5 // 默认 quadset 深度
	RootDepth    = 4 // 根 quadset 深度
)

var (
	// 全局单例：根 quadset 与默认 quadset 的编号对象
	defaultNumbering *QuadtreeNumbering
	rootNumbering    *QuadtreeNumbering
)

func init() {
	// 初始化全局编号对象
	defaultNumbering = NewQuadtreeNumbering(DefaultDepth, true) // 默认 quadset，特殊排序第二行
	rootNumbering = NewQuadtreeNumbering(RootDepth, false)      // 根 quadset，不特殊排序
}

// NewQuadtreeNumbering 创建四叉树编号对象
func NewQuadtreeNumbering(depth int, mangleSecondRow bool) *QuadtreeNumbering {
	return &QuadtreeNumbering{
		TreeNumbering: NewTreeNumbering(4, depth, mangleSecondRow),
	}
}

// GetNumbering 获取指定 quadset 的编号对象
func GetNumbering(quadsetNum uint64) *QuadtreeNumbering {
	if quadsetNum == 0 {
		return rootNumbering
	}
	return defaultNumbering
}

// SubindexToLevelXY 将 Subindex 转换为层级和 (x, y) 坐标
func (qn *QuadtreeNumbering) SubindexToLevelXY(subindex int) (level, x, y int) {
	if subindex < 0 || subindex >= qn.NumNodes() {
		return -1, -1, -1
	}

	path := qn.SubindexToTraversalPath(subindex)
	l, row, col := path.GetLevelRowCol()
	return int(l), int(col), int(row)
}

// LevelXYToSubindex 将层级和 (x, y) 坐标转换为 Subindex
func (qn *QuadtreeNumbering) LevelXYToSubindex(level, x, y int) int {
	if level < 0 || level >= qn.Depth() {
		return -1
	}

	path := NewQuadtreePath(uint32(level), uint32(y), uint32(x))
	return qn.TraversalPathToSubindex(path)
}

// TraversalPathToGlobalNodeNumber 将遍历路径转换为全局节点号
// 全局节点号是一个唯一标识，用于 quadset 的计算
func TraversalPathToGlobalNodeNumber(path QuadtreePath) uint64 {
	var num uint64 = 0
	for i := uint32(0); i < path.Level(); i++ {
		num = (num * 4) + uint64(path.At(i)) + 1
	}
	return num
}

// GlobalNodeNumberToTraversalPath 将全局节点号转换为遍历路径
func GlobalNodeNumberToTraversalPath(num uint64) QuadtreePath {
	path := QuadtreePath{}
	for num > 0 {
		blist := byte((num - 1) & 3)
		singlePath := NewQuadtreePathFromString(string('0' + blist))
		path = singlePath.Concatenate(path)
		num = (num - 1) / 4
	}
	return path
}

// TraversalPathToQuadsetAndSubindex 将遍历路径拆分为 quadset 号与 subindex
// Keyhole 的 quadset 分割规则：
// - 根 quadset 到 level 3（深度4）
// - 之后每 4 层为一个 quadset（深度5，但边界在 level-1）
func TraversalPathToQuadsetAndSubindex(path QuadtreePath) (quadsetNum uint64, subindex int) {
	level := int(path.Level())

	if level < rootNumbering.Depth() {
		// 在根 quadset 内
		quadsetNum = 0
		subindex = rootNumbering.TraversalPathToSubindex(path)
		return
	}

	// 分割路径：根 quadset 到 level 3，之后每 4 层一个 quadset
	// split = 4 * (level / 4) - 1
	split := 4*(level/4) - 1
	quadsetPath := path.Truncate(uint32(split))
	quadsetNum = TraversalPathToGlobalNodeNumber(quadsetPath)

	// 计算相对路径
	relPath, err := RelativePath(quadsetPath, path)
	if err != nil {
		// 如果无法计算相对路径，使用完整路径
		relPath = path
	}

	subindex = defaultNumbering.TraversalPathToSubindex(relPath)
	return
}

// QuadsetAndSubindexToTraversalPath 将 quadset 号与 subindex 组合为遍历路径
func QuadsetAndSubindexToTraversalPath(quadsetNum uint64, subindex int) QuadtreePath {
	if quadsetNum == 0 {
		return rootNumbering.SubindexToTraversalPath(subindex)
	}

	path := GlobalNodeNumberToTraversalPath(quadsetNum)
	subPath := defaultNumbering.SubindexToTraversalPath(subindex)
	return path.Concatenate(subPath)
}

// IsQuadsetRootLevel 判断指定层级是否是 quadset 的根层
// quadset 根层：level 0, level 3, level 7, level 11, ...
func IsQuadsetRootLevel(level uint32) bool {
	if level == 0 {
		return true
	}
	if level >= uint32(RootDepth-1) {
		return (level-uint32(RootDepth-1))%(uint32(DefaultDepth-1)) == 0
	}
	return false
}

// QuadsetAndSubindexToLevelRowColumn 将 quadset 号与 subindex 转换为 level/row/col
func QuadsetAndSubindexToLevelRowColumn(quadsetNum uint64, subindex int) (level, row, col int) {
	path := QuadsetAndSubindexToTraversalPath(quadsetNum, subindex)
	l, r, c := path.GetLevelRowCol()
	return int(l), int(r), int(c)
}

// NumNodes 返回指定 quadset 的节点数
func NumNodes(quadsetNum uint64) int {
	if quadsetNum == 0 {
		return rootNumbering.NumNodes()
	}
	return defaultNumbering.NumNodes()
}

// QuadsetAndSubindexToInorder 将 quadset 号与 subindex 转换为 Inorder
func QuadsetAndSubindexToInorder(quadsetNum uint64, subindex int) int {
	if quadsetNum == 0 {
		return rootNumbering.SubindexToInorder(subindex)
	}
	return defaultNumbering.SubindexToInorder(subindex)
}

// LevelRowColumnToMapsTraversalPath 将 level/row/col 转换为 Maps 格式的路径
// Maps 使用反向编号：'t' = 0, 's' = 1, 'r' = 2, 'q' = 3
func LevelRowColumnToMapsTraversalPath(level, row, col int) string {
	path := NewQuadtreePath(uint32(level), uint32(row), uint32(col))
	result := "t" // Maps 路径以 't' 开头

	for i := uint32(0); i < path.Level(); i++ {
		val := path.At(i)
		// Maps 反向编号：t-0, s-1, r-2, q-3
		result += string('t' - byte(val))
	}

	return result
}

// MapsTraversalPathToLevelRowColumn 将 Maps 格式的路径转换为 level/row/col
func MapsTraversalPathToLevelRowColumn(mapsPath string) (level, row, col int) {
	if len(mapsPath) == 0 || mapsPath[0] != 't' {
		return -1, -1, -1
	}

	// 跳过开头的 't'
	path := QuadtreePath{}
	for i := 1; i < len(mapsPath); i++ {
		// 反向解码：'t'-path[i] 得到 0-3
		val := byte('t' - mapsPath[i])
		if val > 3 {
			return -1, -1, -1
		}
		path = path.Child(uint32(val))
	}

	l, r, c := path.GetLevelRowCol()
	return int(l), int(r), int(c)
}

// IsMapsTile 判断字符串是否是 Maps 格式的 tile 路径
func IsMapsTile(key string) bool {
	return len(key) > 0 && key[0] == 't'
}
