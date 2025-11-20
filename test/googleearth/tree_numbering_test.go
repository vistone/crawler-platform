package googleearth_test

import (
	"testing"

	"crawler-platform/GoogleEarth"
)

func TestTreeNumbering_Basic(t *testing.T) {
	// 创建一个深度为3的四叉树编号（不特殊处理第二行）
	tn := GoogleEarth.NewTreeNumbering(4, 3, false)

	if tn.Depth() != 3 {
		t.Errorf("Depth() = %d, want 3", tn.Depth())
	}
	if tn.BranchingFactor() != 4 {
		t.Errorf("BranchingFactor() = %d, want 4", tn.BranchingFactor())
	}

	// 节点总数 = 1 + 4 + 16 = 21
	expectedNodes := 21
	if tn.NumNodes() != expectedNodes {
		t.Errorf("NumNodes() = %d, want %d", tn.NumNodes(), expectedNodes)
	}
}

func TestTreeNumbering_SubindexInorderConversion(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 3, false)

	// 测试双向转换
	for subindex := 0; subindex < tn.NumNodes(); subindex++ {
		inorder := tn.SubindexToInorder(subindex)
		if !tn.InRange(inorder) {
			t.Errorf("SubindexToInorder(%d) = %d, out of range", subindex, inorder)
			continue
		}

		backToSubindex := tn.InorderToSubindex(inorder)
		if backToSubindex != subindex {
			t.Errorf("Subindex %d -> Inorder %d -> Subindex %d, not reversible",
				subindex, inorder, backToSubindex)
		}
	}
}

func TestTreeNumbering_PathConversion(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 4, false) // 深度改为4，支持3层路径

	tests := []struct {
		pathStr string
	}{
		{""},
		{"0"},
		{"1"},
		{"01"},
		{"23"},
		{"012"},
	}

	for _, tt := range tests {
		path := GoogleEarth.NewQuadtreePathFromString(tt.pathStr)

		// Path -> Inorder -> Path
		inorder := tn.TraversalPathToInorder(path)
		if inorder < 0 {
			t.Errorf("TraversalPathToInorder(%q) = %d, invalid", tt.pathStr, inorder)
			continue
		}

		backPath := tn.InorderToTraversalPath(inorder)
		if !backPath.Equal(path) {
			t.Errorf("Path %q -> Inorder %d -> Path %q, not equal",
				tt.pathStr, inorder, backPath.AsString())
		}

		// Path -> Subindex -> Path
		subindex := tn.TraversalPathToSubindex(path)
		if subindex < 0 {
			t.Errorf("TraversalPathToSubindex(%q) = %d, invalid", tt.pathStr, subindex)
			continue
		}

		backPath2 := tn.SubindexToTraversalPath(subindex)
		if !backPath2.Equal(path) {
			t.Errorf("Path %q -> Subindex %d -> Path %q, not equal",
				tt.pathStr, subindex, backPath2.AsString())
		}
	}
}

func TestTreeNumbering_ParentChild(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 3, false)

	// 测试根节点（inorder=0）
	rootInorder := 0
	parent := tn.GetParentInorder(rootInorder)
	if parent != -1 {
		t.Errorf("Root parent should be -1, got %d", parent)
	}

	// 测试根节点的子节点
	children, ok := tn.GetChildrenInorder(rootInorder)
	if !ok {
		t.Fatal("Root should have children")
	}
	if len(children) != 4 {
		t.Errorf("Root should have 4 children, got %d", len(children))
	}

	// 验证每个子节点的父节点是根节点
	for i, childInorder := range children {
		childParent := tn.GetParentInorder(childInorder)
		if childParent != rootInorder {
			t.Errorf("Child %d (inorder=%d) parent = %d, want %d",
				i, childInorder, childParent, rootInorder)
		}
	}
}

func TestTreeNumbering_Level(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 4, false) // 深度改为4

	// 测试不同路径的层级
	tests := []struct {
		pathStr       string
		expectedLevel int
	}{
		{"", 0},
		{"0", 1},
		{"12", 2},
		{"301", 3},
	}

	for _, tt := range tests {
		path := GoogleEarth.NewQuadtreePathFromString(tt.pathStr)
		inorder := tn.TraversalPathToInorder(path)
		level := tn.GetLevelInorder(inorder)

		if level != tt.expectedLevel {
			t.Errorf("Path %q level = %d, want %d", tt.pathStr, level, tt.expectedLevel)
		}

		// 验证 Subindex 方式获取层级
		subindex := tn.TraversalPathToSubindex(path)
		levelFromSubindex := tn.GetLevelSubindex(subindex)
		if levelFromSubindex != tt.expectedLevel {
			t.Errorf("Path %q (subindex) level = %d, want %d",
				tt.pathStr, levelFromSubindex, tt.expectedLevel)
		}
	}
}

func TestTreeNumbering_MangledSecondRow(t *testing.T) {
	// 测试特殊处理第二行的情况（Keyhole 的非根节点）
	tn := GoogleEarth.NewTreeNumbering(4, 3, true)

	// 验证节点总数不变
	expectedNodes := 21
	if tn.NumNodes() != expectedNodes {
		t.Errorf("NumNodes() = %d, want %d", tn.NumNodes(), expectedNodes)
	}

	// 验证根节点
	if tn.GetLevelInorder(0) != 0 {
		t.Error("Root level should be 0")
	}

	// 验证所有节点的 Subindex <-> Inorder 可逆
	for i := 0; i < tn.NumNodes(); i++ {
		inorder := tn.SubindexToInorder(i)
		back := tn.InorderToSubindex(inorder)
		if back != i {
			t.Errorf("Mangled: Subindex %d -> Inorder %d -> Subindex %d",
				i, inorder, back)
		}
	}
}

func TestTreeNumbering_GetChildrenSubindex(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 2, false)

	// 根节点的 Subindex
	rootSubindex := tn.InorderToSubindex(0)
	children, ok := tn.GetChildrenSubindex(rootSubindex)
	if !ok {
		t.Fatal("Root should have children (subindex)")
	}
	if len(children) != 4 {
		t.Errorf("Root should have 4 children, got %d", len(children))
	}

	// 叶子节点应该没有子节点
	for _, child := range children {
		childChildren, hasChildren := tn.GetChildrenSubindex(child)
		if hasChildren {
			t.Errorf("Leaf node (subindex=%d) should not have children: %v",
				child, childChildren)
		}
	}
}

func TestTreeNumbering_BoundaryConditions(t *testing.T) {
	tn := GoogleEarth.NewTreeNumbering(4, 2, false)

	// 测试越界情况
	if tn.SubindexToInorder(-1) != -1 {
		t.Error("SubindexToInorder(-1) should return -1")
	}
	if tn.SubindexToInorder(tn.NumNodes()) != -1 {
		t.Error("SubindexToInorder(NumNodes) should return -1")
	}

	if tn.InorderToSubindex(-1) != -1 {
		t.Error("InorderToSubindex(-1) should return -1")
	}
	if tn.InorderToSubindex(tn.NumNodes()) != -1 {
		t.Error("InorderToSubindex(NumNodes) should return -1")
	}
}
