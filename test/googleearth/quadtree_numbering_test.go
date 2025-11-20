package googleearth_test

import (
	"testing"

	"crawler-platform/GoogleEarth"
)

func TestQuadtreeNumbering_Basic(t *testing.T) {
	qn := GoogleEarth.NewQuadtreeNumbering(GoogleEarth.DefaultDepth, true)

	if qn.Depth() != GoogleEarth.DefaultDepth {
		t.Errorf("Depth() = %d, want %d", qn.Depth(), GoogleEarth.DefaultDepth)
	}

	if qn.BranchingFactor() != 4 {
		t.Errorf("BranchingFactor() = %d, want 4", qn.BranchingFactor())
	}
}

func TestQuadtreeNumbering_SubindexToLevelXY(t *testing.T) {
	qn := GoogleEarth.NewQuadtreeNumbering(GoogleEarth.DefaultDepth, true)

	tests := []struct {
		level, x, y int
	}{
		{0, 0, 0},
		{1, 0, 0},
		{1, 1, 1},
		{2, 3, 2},
	}

	for _, tt := range tests {
		subindex := qn.LevelXYToSubindex(tt.level, tt.x, tt.y)
		if subindex < 0 {
			t.Errorf("LevelXYToSubindex(%d, %d, %d) = %d, invalid",
				tt.level, tt.x, tt.y, subindex)
			continue
		}

		// 验证双向转换
		gotLevel, gotX, gotY := qn.SubindexToLevelXY(subindex)
		if gotLevel != tt.level || gotX != tt.x || gotY != tt.y {
			t.Errorf("Level/X/Y (%d,%d,%d) -> Subindex %d -> (%d,%d,%d), not equal",
				tt.level, tt.x, tt.y, subindex, gotLevel, gotX, gotY)
		}
	}
}

func TestQuadtreeNumbering_GlobalNodeNumber(t *testing.T) {
	tests := []struct {
		pathStr string
		nodeNum uint64
	}{
		{"", 0},
		{"0", 1},
		{"1", 2},
		{"2", 3},
		{"3", 4},
		{"00", 5},
		{"01", 6},
		{"10", 9},
	}

	for _, tt := range tests {
		path := GoogleEarth.NewQuadtreePathFromString(tt.pathStr)
		num := GoogleEarth.TraversalPathToGlobalNodeNumber(path)

		if num != tt.nodeNum {
			t.Errorf("TraversalPathToGlobalNodeNumber(%q) = %d, want %d",
				tt.pathStr, num, tt.nodeNum)
		}

		// 验证反向转换
		backPath := GoogleEarth.GlobalNodeNumberToTraversalPath(num)
		if !backPath.Equal(path) {
			t.Errorf("NodeNum %d -> Path %q -> NodeNum, got path %q",
				num, tt.pathStr, backPath.AsString())
		}
	}
}

func TestQuadtreeNumbering_QuadsetSplit(t *testing.T) {
	tests := []struct {
		pathStr         string
		expectedQuadset uint64
		description     string
	}{
		{"", 0, "根节点在 quadset 0"},
		{"0", 0, "level 1 在 quadset 0"},
		{"012", 0, "level 3 在 quadset 0（根 quadset）"},
		{"0123", 27, "level 4 的 quadset 是 '012' 的全局编号 27"},
		{"01230123", 7195, "level 8 的 quadset 是 '0123012' 的全局编号"},
	}

	for _, tt := range tests {
		path := GoogleEarth.NewQuadtreePathFromString(tt.pathStr)
		quadsetNum, subindex := GoogleEarth.TraversalPathToQuadsetAndSubindex(path)

		if quadsetNum != tt.expectedQuadset {
			t.Errorf("Path %q: quadset = %d, want %d (%s)",
				tt.pathStr, quadsetNum, tt.expectedQuadset, tt.description)
		}

		// 验证反向转换
		backPath := GoogleEarth.QuadsetAndSubindexToTraversalPath(quadsetNum, subindex)
		if !backPath.Equal(path) {
			t.Errorf("Path %q -> (quadset=%d, subindex=%d) -> Path %q, not equal",
				tt.pathStr, quadsetNum, subindex, backPath.AsString())
		}
	}
}

func TestQuadtreeNumbering_IsQuadsetRootLevel(t *testing.T) {
	tests := []struct {
		level    uint32
		expected bool
	}{
		{0, true}, // 根 quadset 的根
		{1, false},
		{2, false},
		{3, true}, // 第一个默认 quadset 的根
		{4, false},
		{7, true},  // 第二个默认 quadset 的根
		{11, true}, // 第三个默认 quadset 的根
		{8, false},
	}

	for _, tt := range tests {
		result := GoogleEarth.IsQuadsetRootLevel(tt.level)
		if result != tt.expected {
			t.Errorf("IsQuadsetRootLevel(%d) = %v, want %v",
				tt.level, result, tt.expected)
		}
	}
}

func TestQuadtreeNumbering_MapsTraversalPath(t *testing.T) {
	tests := []struct {
		level, row, col int
		expectedMaps    string
	}{
		{0, 0, 0, "t"},
		{1, 0, 0, "tt"},  // row=0, col=0 -> child 0 -> 't'
		{1, 0, 1, "ts"},  // row=0, col=1 -> child 1 -> 's'
		{1, 1, 1, "tr"},  // row=1, col=1 -> child 2 -> 'r'
		{1, 1, 0, "tq"},  // row=1, col=0 -> child 3 -> 'q'
		{2, 0, 0, "ttt"}, // level 2, row=0, col=0
	}

	for _, tt := range tests {
		mapsPath := GoogleEarth.LevelRowColumnToMapsTraversalPath(tt.level, tt.row, tt.col)
		if mapsPath != tt.expectedMaps {
			t.Errorf("LevelRowColumnToMapsTraversalPath(%d, %d, %d) = %q, want %q",
				tt.level, tt.row, tt.col, mapsPath, tt.expectedMaps)
		}

		// 验证反向转换
		gotLevel, gotRow, gotCol := GoogleEarth.MapsTraversalPathToLevelRowColumn(mapsPath)
		if gotLevel != tt.level || gotRow != tt.row || gotCol != tt.col {
			t.Errorf("MapsPath %q -> (%d,%d,%d), want (%d,%d,%d)",
				mapsPath, gotLevel, gotRow, gotCol, tt.level, tt.row, tt.col)
		}
	}
}

func TestQuadtreeNumbering_IsMapsTile(t *testing.T) {
	tests := []struct {
		key      string
		expected bool
	}{
		{"t", true},
		{"tqrst", true},
		{"", false},
		{"abc", false},
		{"0123", false},
	}

	for _, tt := range tests {
		result := GoogleEarth.IsMapsTile(tt.key)
		if result != tt.expected {
			t.Errorf("IsMapsTile(%q) = %v, want %v", tt.key, result, tt.expected)
		}
	}
}

func TestQuadtreeNumbering_NumNodes(t *testing.T) {
	// 根 quadset 节点数：深度4，1+4+16+64 = 85
	rootNodes := GoogleEarth.NumNodes(0)
	expectedRoot := 85
	if rootNodes != expectedRoot {
		t.Errorf("NumNodes(0) = %d, want %d", rootNodes, expectedRoot)
	}

	// 默认 quadset 节点数：深度5，1+4+16+64+256 = 341
	defaultNodes := GoogleEarth.NumNodes(1)
	expectedDefault := 341
	if defaultNodes != expectedDefault {
		t.Errorf("NumNodes(1) = %d, want %d", defaultNodes, expectedDefault)
	}
}
