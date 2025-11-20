package GoogleEarth

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

// TreeNumbering 树编号通用结构
// 提供 Subindex（子索引）与 Inorder（中序遍历）之间的转换
// 支持任意分支因子的树结构
type TreeNumbering struct {
	depth           int  // 树的深度
	branchingFactor int  // 分支因子（如四叉树为4）
	numNodes        int  // 树中节点总数
	mangleSecondRow bool // 是否对第二行进行特殊排序

	// 预计算的转换表
	nodes         []nodeInfo // 节点信息数组
	nodesAtLevels []int      // 每层累计节点数
}

// nodeInfo 节点信息
type nodeInfo struct {
	subindexToInorder int // Subindex -> Inorder 映射
	inorderToSubindex int // Inorder -> Subindex 映射
	inorderToLevel    int // Inorder -> Level 映射
	inorderToParent   int // Inorder -> Parent 映射
}

// NewTreeNumbering 创建树编号对象
// branchingFactor: 分支因子（如四叉树为4）
// depth: 树的深度
// mangleSecondRow: 是否对第二行进行特殊排序（Keyhole 根节点为 false，其他为 true）
func NewTreeNumbering(branchingFactor, depth int, mangleSecondRow bool) *TreeNumbering {
	tn := &TreeNumbering{
		branchingFactor: branchingFactor,
		depth:           depth,
		mangleSecondRow: mangleSecondRow,
	}

	// 预计算每层累计节点数
	tn.nodesAtLevels = make([]int, depth+1)
	tn.precomputeNodesAtLevels()

	// 计算节点总数
	tn.numNodes = tn.nodesAtLevel(depth)
	tn.nodes = make([]nodeInfo, tn.numNodes)

	// 预计算 Subindex <-> Inorder 转换表
	if mangleSecondRow {
		// 特殊处理第二行（Keyhole 的非根节点）
		if depth > 0 {
			tn.nodes[0].subindexToInorder = 0
			tn.nodes[0].inorderToLevel = 0
			if depth > 1 {
				num := 1
				// 对第二行的每个子节点进行中序遍历
				for i := 0; i < branchingFactor; i++ {
					tn.precomputeSubindexToInorder(num, 1, 0, 0, &num)
				}
			}
		}
	} else {
		// 正常遍历（Keyhole 的根节点）
		num := 0
		tn.precomputeSubindexToInorder(0, 0, 0, 0, &num)
	}

	// 反向构建 Inorder -> Subindex 映射
	for i := 0; i < tn.numNodes; i++ {
		inorder := tn.nodes[i].subindexToInorder
		tn.nodes[inorder].inorderToSubindex = i
	}

	// 预计算父节点映射
	tn.precomputeInorderToParent()

	return tn
}

// precomputeNodesAtLevels 预计算每层累计节点数
func (tn *TreeNumbering) precomputeNodesAtLevels() {
	num := 0
	numAtBottom := 1
	for i := 0; i <= tn.depth; i++ {
		tn.nodesAtLevels[i] = num
		num += numAtBottom
		numAtBottom *= tn.branchingFactor
	}
}

// nodesAtLevel 返回指定层级的累计节点数
func (tn *TreeNumbering) nodesAtLevel(level int) int {
	if level < 0 || level >= len(tn.nodesAtLevels) {
		return 0
	}
	return tn.nodesAtLevels[level]
}

// precomputeSubindexToInorder 递归预计算 Subindex -> Inorder 映射
// base: 基础偏移
// level: 当前层级
// offset: 同层偏移
// leftIndex: 本层最左节点索引
// num: 中序遍历序号（指针，递增）
func (tn *TreeNumbering) precomputeSubindexToInorder(base, level, offset, leftIndex int, num *int) {
	subindex := base + leftIndex + offset
	if subindex < 0 || subindex >= tn.numNodes {
		return
	}

	tn.nodes[subindex].subindexToInorder = *num
	tn.nodes[*num].inorderToLevel = level
	*num++

	// 递归处理子节点
	if level < tn.depth-1 {
		for i := 0; i < tn.branchingFactor; i++ {
			tn.precomputeSubindexToInorder(
				base,
				level+1,
				offset*tn.branchingFactor+i,
				leftIndex*tn.branchingFactor+1,
				num,
			)
		}
	}
}

// precomputeInorderToParent 预计算父节点映射
func (tn *TreeNumbering) precomputeInorderToParent() {
	// 根节点无父节点
	if tn.numNodes > 0 {
		tn.nodes[0].inorderToParent = -1
	}

	// 其他节点通过路径计算父节点
	for i := 1; i < tn.numNodes; i++ {
		path := tn.InorderToTraversalPath(i)
		parent := path.Parent()
		tn.nodes[i].inorderToParent = tn.TraversalPathToInorder(parent)
	}
}

// NumNodes 返回节点总数
func (tn *TreeNumbering) NumNodes() int {
	return tn.numNodes
}

// Depth 返回树的深度
func (tn *TreeNumbering) Depth() int {
	return tn.depth
}

// BranchingFactor 返回分支因子
func (tn *TreeNumbering) BranchingFactor() int {
	return tn.branchingFactor
}

// SubindexToInorder 将 Subindex 转换为 Inorder
func (tn *TreeNumbering) SubindexToInorder(subindex int) int {
	if subindex < 0 || subindex >= tn.numNodes {
		return -1
	}
	return tn.nodes[subindex].subindexToInorder
}

// InorderToSubindex 将 Inorder 转换为 Subindex
func (tn *TreeNumbering) InorderToSubindex(inorder int) int {
	if inorder < 0 || inorder >= tn.numNodes {
		return -1
	}
	return tn.nodes[inorder].inorderToSubindex
}

// GetLevelInorder 返回 Inorder 节点的层级
func (tn *TreeNumbering) GetLevelInorder(inorder int) int {
	if inorder < 0 || inorder >= tn.numNodes {
		return -1
	}
	return tn.nodes[inorder].inorderToLevel
}

// GetLevelSubindex 返回 Subindex 节点的层级
func (tn *TreeNumbering) GetLevelSubindex(subindex int) int {
	return tn.GetLevelInorder(tn.SubindexToInorder(subindex))
}

// GetParentInorder 返回 Inorder 节点的父节点
func (tn *TreeNumbering) GetParentInorder(inorder int) int {
	if inorder < 0 || inorder >= tn.numNodes {
		return -1
	}
	return tn.nodes[inorder].inorderToParent
}

// GetParentSubindex 返回 Subindex 节点的父节点
func (tn *TreeNumbering) GetParentSubindex(subindex int) int {
	parent := tn.GetParentInorder(tn.SubindexToInorder(subindex))
	if parent == -1 {
		return -1
	}
	return tn.InorderToSubindex(parent)
}

// TraversalPathToInorder 将遍历路径转换为 Inorder
func (tn *TreeNumbering) TraversalPathToInorder(path QuadtreePath) int {
	level := int(path.Level())
	if level >= tn.depth {
		return -1
	}

	index := 0
	for i := uint32(0); i < path.Level(); i++ {
		childIdx := int(path.At(i))
		if childIdx >= tn.branchingFactor {
			return -1
		}
		index += 1 + childIdx*tn.nodesAtLevel(tn.depth-int(i)-1)
	}
	return index
}

// TraversalPathToSubindex 将遍历路径转换为 Subindex
func (tn *TreeNumbering) TraversalPathToSubindex(path QuadtreePath) int {
	return tn.InorderToSubindex(tn.TraversalPathToInorder(path))
}

// InorderToTraversalPath 将 Inorder 转换为遍历路径
func (tn *TreeNumbering) InorderToTraversalPath(inorder int) QuadtreePath {
	if inorder < 0 || inorder >= tn.numNodes {
		return QuadtreePath{}
	}

	// 从根向下遍历，减去子树大小来确定路径
	path := QuadtreePath{}
	level := 1

	for inorder > 0 {
		nodesBelow := tn.nodesAtLevel(tn.depth - level)
		if nodesBelow == 0 {
			break
		}
		step := (inorder - 1) / nodesBelow
		if step >= tn.branchingFactor {
			step = tn.branchingFactor - 1
		}
		path = path.Child(uint32(step))
		inorder = inorder - step*nodesBelow - 1
		level++
	}

	return path
}

// SubindexToTraversalPath 将 Subindex 转换为遍历路径
func (tn *TreeNumbering) SubindexToTraversalPath(subindex int) QuadtreePath {
	return tn.InorderToTraversalPath(tn.SubindexToInorder(subindex))
}

// GetChildrenInorder 获取 Inorder 节点的所有子节点
func (tn *TreeNumbering) GetChildrenInorder(inorder int) ([]int, bool) {
	if inorder < 0 || inorder >= tn.numNodes {
		return nil, false
	}

	// 检查是否为叶子节点
	level := tn.GetLevelInorder(inorder)
	if level == tn.depth-1 {
		return nil, false
	}

	// 第一个子节点是 inorder+1，其他子节点按子树大小递增
	numNodesInSubtree := tn.nodesAtLevel(tn.depth - level - 1)
	children := make([]int, tn.branchingFactor)
	for i := 0; i < tn.branchingFactor; i++ {
		children[i] = inorder + 1 + i*numNodesInSubtree
	}

	return children, true
}

// GetChildrenSubindex 获取 Subindex 节点的所有子节点
func (tn *TreeNumbering) GetChildrenSubindex(subindex int) ([]int, bool) {
	children, ok := tn.GetChildrenInorder(tn.SubindexToInorder(subindex))
	if !ok {
		return nil, false
	}

	// 转换为 Subindex
	for i := range children {
		children[i] = tn.InorderToSubindex(children[i])
	}
	return children, true
}

// InRange 检查索引是否在有效范围内
func (tn *TreeNumbering) InRange(num int) bool {
	return num >= 0 && num < tn.numNodes
}
