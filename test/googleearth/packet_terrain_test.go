package googleearth_test

import (
	"testing"
	"time"

	"crawler-platform/GoogleEarth"
	pb "crawler-platform/GoogleEarth/pb"
)

func TestJpegCommentDate_Basic(t *testing.T) {
	// 测试创建日期
	date := GoogleEarth.NewJpegCommentDate(2023, 11, 15)

	if date.Year() != 2023 {
		t.Errorf("Year() = %d, want 2023", date.Year())
	}
	if date.Month() != 11 {
		t.Errorf("Month() = %d, want 11", date.Month())
	}
	if date.Day() != 15 {
		t.Errorf("Day() = %d, want 15", date.Day())
	}
}

func TestJpegCommentDate_FromInt(t *testing.T) {
	tests := []struct {
		dateInt int32
		year    int16
		month   int8
		day     int8
	}{
		{20231115, 2023, 11, 15},
		{20200101, 2020, 1, 1},
		{19991231, 1999, 12, 31},
		{0, 0, 0, 0},
	}

	for _, tt := range tests {
		date := GoogleEarth.NewJpegCommentDateFromInt(tt.dateInt)

		if date.Year() != tt.year || date.Month() != tt.month || date.Day() != tt.day {
			t.Errorf("NewJpegCommentDateFromInt(%d) = %d-%02d-%02d, want %d-%02d-%02d",
				tt.dateInt, date.Year(), date.Month(), date.Day(),
				tt.year, tt.month, tt.day)
		}
	}
}

func TestJpegCommentDate_ToString(t *testing.T) {
	tests := []struct {
		year     int16
		month    int8
		day      int8
		expected string
	}{
		{2023, 11, 15, "2023-11-15"},
		{2020, 1, 0, "2020-01"},
		{1999, 0, 0, "1999"},
		{0, 0, 0, "Unknown"},
		{-1, -1, -1, "MatchAll"},
	}

	for _, tt := range tests {
		date := GoogleEarth.NewJpegCommentDate(tt.year, tt.month, tt.day)
		result := date.ToString()

		if result != tt.expected {
			t.Errorf("ToString() = %q, want %q", result, tt.expected)
		}
	}
}

func TestJpegCommentDate_Compare(t *testing.T) {
	date1 := GoogleEarth.NewJpegCommentDate(2023, 11, 15)
	date2 := GoogleEarth.NewJpegCommentDate(2023, 11, 15)
	date3 := GoogleEarth.NewJpegCommentDate(2023, 11, 16)
	date4 := GoogleEarth.NewJpegCommentDate(2022, 11, 15)

	if !date1.Equal(date2) {
		t.Error("date1 should equal date2")
	}

	if date1.Before(date2) {
		t.Error("date1 should not be before date2")
	}

	if !date1.Before(date3) {
		t.Error("date1 should be before date3")
	}

	if !date1.After(date4) {
		t.Error("date1 should be after date4")
	}
}

func TestJpegCommentDate_FromTime(t *testing.T) {
	tm := time.Date(2023, 11, 15, 0, 0, 0, 0, time.UTC)
	date := GoogleEarth.NewJpegCommentDateFromTime(tm)

	if date.Year() != 2023 || date.Month() != 11 || date.Day() != 15 {
		t.Errorf("NewJpegCommentDateFromTime() = %s, want 2023-11-15", date.ToString())
	}
}

func TestJpegCommentDate_Parse(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2023-11-15", "2023-11-15"},
		{"2023-11", "2023-11"},
		{"2023", "2023"},
		{"20231115", "2023-11-15"},
		{"Unknown", "Unknown"},
		{"MatchAll", "MatchAll"},
	}

	for _, tt := range tests {
		date, err := GoogleEarth.ParseJpegCommentDateString(tt.input)
		if err != nil {
			t.Errorf("ParseJpegCommentDateString(%q) error: %v", tt.input, err)
			continue
		}

		result := date.ToString()
		if result != tt.expected {
			t.Errorf("ParseJpegCommentDateString(%q) = %q, want %q",
				tt.input, result, tt.expected)
		}
	}
}

func TestQuadtreePacket_Basic(t *testing.T) {
	qtp := GoogleEarth.NewQuadtreePacketProtoBuf()

	if qtp == nil {
		t.Fatal("NewQuadtreePacketProtoBuf() returned nil")
	}

	packet := qtp.GetPacket()
	if packet == nil {
		t.Error("GetPacket() returned nil")
	}
}

func TestQuadtreeDataReferenceGroup_Reset(t *testing.T) {
	group := &GoogleEarth.QuadtreeDataReferenceGroup{
		ImgRefs: make([]GoogleEarth.QuadtreeDataReference, 0),
		TerRefs: make([]GoogleEarth.QuadtreeDataReference, 0),
	}

	// 添加一些引用
	group.ImgRefs = append(group.ImgRefs, GoogleEarth.QuadtreeDataReference{})
	group.TerRefs = append(group.TerRefs, GoogleEarth.QuadtreeDataReference{})

	if len(group.ImgRefs) == 0 || len(group.TerRefs) == 0 {
		t.Fatal("Failed to add references")
	}

	// 重置
	group.Reset()

	if len(group.ImgRefs) != 0 || len(group.TerRefs) != 0 {
		t.Error("Reset() did not clear all references")
	}
}

func TestTerrain_Basic(t *testing.T) {
	terrain := GoogleEarth.NewTerrain("test")

	if terrain.QtNode != "test" {
		t.Errorf("QtNode = %q, want 'test'", terrain.QtNode)
	}

	if terrain.NumMeshes() != 0 {
		t.Errorf("NumMeshes() = %d, want 0", terrain.NumMeshes())
	}
}

func TestMesh_Reset(t *testing.T) {
	mesh := &GoogleEarth.Mesh{
		Level:     10,
		NumPoints: 100,
		Vertices:  make([]GoogleEarth.MeshVertex, 100),
	}

	mesh.Reset()

	if mesh.Level != 0 || mesh.NumPoints != 0 || len(mesh.Vertices) != 0 {
		t.Error("Reset() did not clear all fields")
	}
}

func TestQuadtreeDataReference_IsHistoricalImagery(t *testing.T) {
	// 非历史影像
	ref1 := GoogleEarth.QuadtreeDataReference{
		Channel: uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY),
	}
	if ref1.IsHistoricalImagery() {
		t.Error("Regular imagery should not be historical")
	}

	// 历史影像但日期未知
	ref2 := GoogleEarth.QuadtreeDataReference{
		Channel:  uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY),
		JpegDate: GoogleEarth.JpegCommentDate{},
	}
	if ref2.IsHistoricalImagery() {
		t.Error("Historical imagery with unknown date should not be marked as historical")
	}

	// 真正的历史影像
	ref3 := GoogleEarth.QuadtreeDataReference{
		Channel:  uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY),
		JpegDate: GoogleEarth.NewJpegCommentDate(2023, 11, 15),
	}
	if !ref3.IsHistoricalImagery() {
		t.Error("Historical imagery with valid date should be marked as historical")
	}
}
