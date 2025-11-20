package googleearth_test

import (
	"testing"

	"crawler-platform/GoogleEarth"
)

func TestQuadtreePath_NewFromLevelRowCol(t *testing.T) {
	// 测试从层级、行、列构造路径
	// 四叉树编号： c0  c1
	//           r1 [3] [2]
	//           r0 [0] [1]
	tests := []struct {
		level, row, col uint32
		expected        string
	}{
		{0, 0, 0, ""},
		{1, 0, 0, "0"}, // row=0, col=0 -> 0
		{1, 0, 1, "1"}, // row=0, col=1 -> 1
		{1, 1, 1, "2"}, // row=1, col=1 -> 2
		{1, 1, 0, "3"}, // row=1, col=0 -> 3
		{2, 0, 0, "00"},
		{2, 1, 1, "02"},  // row=1, col=1 at level 2 -> "02"
		{3, 2, 3, "021"}, // row=2, col=3 at level 3 -> "021"
	}

	for _, tt := range tests {
		path := GoogleEarth.NewQuadtreePath(tt.level, tt.row, tt.col)
		got := path.AsString()
		if got != tt.expected {
			t.Errorf("NewQuadtreePath(%d,%d,%d).AsString() = %q, want %q",
				tt.level, tt.row, tt.col, got, tt.expected)
		}

		// 验证双向转换
		gotLevel, gotRow, gotCol := path.GetLevelRowCol()
		if gotLevel != tt.level || gotRow != tt.row || gotCol != tt.col {
			t.Errorf("GetLevelRowCol() = (%d,%d,%d), want (%d,%d,%d)",
				gotLevel, gotRow, gotCol, tt.level, tt.row, tt.col)
		}
	}
}

func TestQuadtreePath_FromString(t *testing.T) {
	tests := []string{"", "0", "123", "0123", "321032"}

	for _, str := range tests {
		path := GoogleEarth.NewQuadtreePathFromString(str)
		got := path.AsString()
		if got != str {
			t.Errorf("NewQuadtreePathFromString(%q).AsString() = %q, want %q",
				str, got, str)
		}
	}
}

func TestQuadtreePath_ParentChild(t *testing.T) {
	// 测试父子关系
	parent := GoogleEarth.NewQuadtreePathFromString("012")

	for i := uint32(0); i < 4; i++ {
		child := parent.Child(i)
		if !parent.IsAncestorOf(child) {
			t.Errorf("parent %q should be ancestor of child %q",
				parent.AsString(), child.AsString())
		}

		gotParent := child.Parent()
		if !gotParent.Equal(parent) {
			t.Errorf("child.Parent() = %q, want %q",
				gotParent.AsString(), parent.AsString())
		}

		if child.WhichChild() != i {
			t.Errorf("child.WhichChild() = %d, want %d", child.WhichChild(), i)
		}
	}
}

func TestQuadtreePath_Concatenate(t *testing.T) {
	p1 := GoogleEarth.NewQuadtreePathFromString("01")
	p2 := GoogleEarth.NewQuadtreePathFromString("23")
	result := p1.Concatenate(p2)

	expected := "0123"
	if result.AsString() != expected {
		t.Errorf("Concatenate() = %q, want %q", result.AsString(), expected)
	}
}

func TestQuadtreePath_RelativePath(t *testing.T) {
	parent := GoogleEarth.NewQuadtreePathFromString("01")
	child := GoogleEarth.NewQuadtreePathFromString("0123")

	rel, err := GoogleEarth.RelativePath(parent, child)
	if err != nil {
		t.Fatalf("RelativePath() error: %v", err)
	}

	expected := "23"
	if rel.AsString() != expected {
		t.Errorf("RelativePath() = %q, want %q", rel.AsString(), expected)
	}

	// 测试非祖先关系
	notParent := GoogleEarth.NewQuadtreePathFromString("12")
	_, err = GoogleEarth.RelativePath(notParent, child)
	if err == nil {
		t.Error("RelativePath() should return error for non-ancestor")
	}
}

func TestQuadtreePath_Advance(t *testing.T) {
	// 测试前序遍历
	path := GoogleEarth.NewQuadtreePathFromString("0")
	maxLevel := uint32(2)

	visited := []string{path.AsString()}
	for path.Advance(maxLevel) {
		visited = append(visited, path.AsString())
		if len(visited) > 100 {
			t.Fatal("Advance() seems to loop infinitely")
		}
	}

	// 应该遍历所有 level <= 2 的节点
	if len(visited) < 5 { // 至少有 root + 4 children
		t.Errorf("Advance() visited only %d nodes, expected more", len(visited))
	}

	t.Logf("Visited %d nodes: %v", len(visited), visited[:10])
}

func TestQuadtreePath_LessThan(t *testing.T) {
	tests := []struct {
		p1, p2   string
		expected bool
	}{
		{"0", "1", true},
		{"1", "0", false},
		{"01", "1", true},
		{"1", "01", false},
		{"012", "013", true},
	}

	for _, tt := range tests {
		p1 := GoogleEarth.NewQuadtreePathFromString(tt.p1)
		p2 := GoogleEarth.NewQuadtreePathFromString(tt.p2)
		got := p1.LessThan(p2)
		if got != tt.expected {
			t.Errorf("%q.LessThan(%q) = %v, want %v",
				tt.p1, tt.p2, got, tt.expected)
		}
	}
}

func TestQuadtreePath_AsIndex(t *testing.T) {
	path := GoogleEarth.NewQuadtreePathFromString("23")
	index := path.AsIndex(2)

	// "23" 的二进制表示为 1011，应该等于 11
	expected := uint64(11)
	if index != expected {
		t.Errorf("AsIndex(2) = %d, want %d", index, expected)
	}
}
