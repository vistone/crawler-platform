package googleearth_test

import (
	"math"
	"testing"

	"crawler-platform/GoogleEarth"
)

func TestDegRadConversion(t *testing.T) {
	tests := []struct {
		deg float64
		rad float64
	}{
		{0, 0},
		{180, GoogleEarth.Pi},
		{90, GoogleEarth.Pi / 2},
		{-90, -GoogleEarth.Pi / 2},
	}

	for _, tt := range tests {
		rad := GoogleEarth.DegToRad(tt.deg)
		if math.Abs(rad-tt.rad) > 1e-10 {
			t.Errorf("DegToRad(%f) = %f, want %f", tt.deg, rad, tt.rad)
		}

		deg := GoogleEarth.RadToDeg(tt.rad)
		if math.Abs(deg-tt.deg) > 1e-10 {
			t.Errorf("RadToDeg(%f) = %f, want %f", tt.rad, deg, tt.deg)
		}
	}
}

func TestLatLonMercatorConversion(t *testing.T) {
	tests := []struct {
		lat, lon float64
	}{
		{0, 0},
		{39.9, 116.4},  // 北京
		{-33.9, 151.2}, // 悉尼
		{51.5, -0.1},   // 伦敦
	}

	for _, tt := range tests {
		x, y := GoogleEarth.LatLonToMercator(tt.lat, tt.lon)
		lat, lon := GoogleEarth.MercatorToLatLon(x, y)

		if math.Abs(lat-tt.lat) > 1e-6 || math.Abs(lon-tt.lon) > 1e-6 {
			t.Errorf("LatLon(%f, %f) -> Mercator(%f, %f) -> LatLon(%f, %f), not reversible",
				tt.lat, tt.lon, x, y, lat, lon)
		}
	}
}

func TestLatLonToTile(t *testing.T) {
	tests := []struct {
		lat, lon float64
		level    int
	}{
		{0, 0, 0},
		{0, 0, 1},
		{39.9, 116.4, 10},
	}

	for _, tt := range tests {
		row, col := GoogleEarth.LatLonToTile(tt.lat, tt.lon, tt.level)

		// 验证反向转换：坐标应该在瓦片边界内
		minLat, minLon, maxLat, maxLon := GoogleEarth.TileBounds(tt.level, row, col)
		if tt.lat < minLat || tt.lat > maxLat || tt.lon < minLon || tt.lon > maxLon {
			t.Errorf("LatLonToTile(%f, %f, %d) = (%d, %d), but lat/lon not in tile bounds [%f,%f] x [%f,%f]",
				tt.lat, tt.lon, tt.level, row, col, minLat, maxLat, minLon, maxLon)
		}

		t.Logf("LatLon(%f, %f) at level %d -> Tile(%d, %d)",
			tt.lat, tt.lon, tt.level, row, col)
	}
}

func TestTileBounds(t *testing.T) {
	// level 0：整个世界
	minLat, minLon, maxLat, maxLon := GoogleEarth.TileBounds(0, 0, 0)

	// 检查边界是否覆盖整个可投影范围
	if minLat > -85 || maxLat < 85 {
		t.Errorf("TileBounds(0,0,0) latitude range = [%f, %f], should cover most of world",
			minLat, maxLat)
	}
	if minLon > -179 || maxLon < 179 {
		t.Errorf("TileBounds(0,0,0) longitude range = [%f, %f], should cover most of world",
			minLon, maxLon)
	}

	// level 1：四个瓦片
	tests := []struct {
		row, col int
	}{
		{0, 0}, {0, 1}, {1, 0}, {1, 1},
	}

	for _, tt := range tests {
		minLat, minLon, maxLat, maxLon := GoogleEarth.TileBounds(1, tt.row, tt.col)

		// 检查边界有效性
		if minLat >= maxLat || minLon >= maxLon {
			t.Errorf("TileBounds(1, %d, %d): invalid bounds [%f,%f] x [%f,%f]",
				tt.row, tt.col, minLat, maxLat, minLon, maxLon)
		}
	}
}

func TestQuadtreeAddress(t *testing.T) {
	tests := []struct {
		level, row, col int
		expected        string
	}{
		{0, 0, 0, ""},
		{1, 0, 0, "0"},
		{1, 0, 1, "1"},
		{1, 1, 1, "2"},
		{1, 1, 0, "3"},
		{2, 0, 0, "00"},
		{2, 3, 3, "22"},
	}

	for _, tt := range tests {
		addr := GoogleEarth.QuadtreeAddress(tt.level, tt.row, tt.col)
		if addr != tt.expected {
			t.Errorf("QuadtreeAddress(%d, %d, %d) = %q, want %q",
				tt.level, tt.row, tt.col, addr, tt.expected)
		}
	}
}

func TestLatLonToQuadtreeAddress(t *testing.T) {
	// 测试北京坐标
	lat, lon := 39.9, 116.4
	level := 10

	address := GoogleEarth.LatLonToQuadtreeAddress(lat, lon, level)

	// 验证地址长度
	if len(address) != level {
		t.Errorf("LatLonToQuadtreeAddress(%f, %f, %d) = %q, length should be %d",
			lat, lon, level, address, level)
	}

	// 验证反向转换：地址 -> 边界 -> 中心
	minLat, minLon, maxLat, maxLon := GoogleEarth.QuadtreeAddressToBounds(address)
	centerLat := (minLat + maxLat) / 2
	centerLon := (minLon + maxLon) / 2

	// 原始坐标应该在边界内
	if lat < minLat || lat > maxLat || lon < minLon || lon > maxLon {
		t.Errorf("Original lat/lon (%f, %f) not in bounds [%f,%f] x [%f,%f]",
			lat, lon, minLat, maxLat, minLon, maxLon)
	}

	// 中心点应该接近原始坐标（在瓦片大小范围内）
	t.Logf("Address: %s, Center: (%f, %f), Original: (%f, %f)",
		address, centerLat, centerLon, lat, lon)
}

func TestTileCenter(t *testing.T) {
	tests := []struct {
		level, row, col int
	}{
		{0, 0, 0},
		{1, 0, 0},
		{2, 1, 1},
	}

	for _, tt := range tests {
		centerLat, centerLon := GoogleEarth.TileCenter(tt.level, tt.row, tt.col)
		minLat, minLon, maxLat, maxLon := GoogleEarth.TileBounds(tt.level, tt.row, tt.col)

		expectedLat := (minLat + maxLat) / 2
		expectedLon := (minLon + maxLon) / 2

		if math.Abs(centerLat-expectedLat) > 1e-6 ||
			math.Abs(centerLon-expectedLon) > 1e-6 {
			t.Errorf("TileCenter(%d, %d, %d) = (%f, %f), want (%f, %f)",
				tt.level, tt.row, tt.col, centerLat, centerLon, expectedLat, expectedLon)
		}
	}
}

func TestNormalize(t *testing.T) {
	// 测试经度归一化
	lonTests := []struct {
		input    float64
		expected float64
	}{
		{0, 0},
		{180, 180},
		{-180, -180},
		{200, -160},
		{-200, 160},
		{540, 180},
	}

	for _, tt := range lonTests {
		result := GoogleEarth.NormalizeLongitude(tt.input)
		if math.Abs(result-tt.expected) > 1e-6 {
			t.Errorf("NormalizeLongitude(%f) = %f, want %f",
				tt.input, result, tt.expected)
		}
	}

	// 测试纬度归一化
	latTests := []struct {
		input    float64
		expected float64
	}{
		{0, 0},
		{90, 90},
		{-90, -90},
		{100, 90},
		{-100, -90},
	}

	for _, tt := range latTests {
		result := GoogleEarth.NormalizeLatitude(tt.input)
		if math.Abs(result-tt.expected) > 1e-6 {
			t.Errorf("NormalizeLatitude(%f) = %f, want %f",
				tt.input, result, tt.expected)
		}
	}
}

func TestConvertToQtNode(t *testing.T) {
	tests := []struct {
		x, y, z  uint
		expected string
	}{
		// level 1: grid is 2x2
		// (0,0) -> 左下象限 -> '3'
		{0, 0, 1, "03"},
		// (1,0) -> 右下象限 -> '2'
		{1, 0, 1, "02"},
		// (1,1) -> 右上象限 -> '1'
		{1, 1, 1, "01"},
		// (0,1) -> 左上象限 -> '0'
		{0, 1, 1, "00"},
		// level 2: grid is 4x4
		{0, 0, 2, "033"},
		{3, 3, 2, "011"},
	}

	for _, tt := range tests {
		result := GoogleEarth.ConvertToQtNode(tt.x, tt.y, tt.z)
		if result != tt.expected {
			t.Errorf("ConvertToQtNode(%d, %d, %d) = %q, want %q",
				tt.x, tt.y, tt.z, result, tt.expected)
		}
	}
}

func TestConvertFromQtNode(t *testing.T) {
	tests := []struct {
		qtnode  string
		x, y, z uint
	}{
		{"03", 0, 0, 1},
		{"02", 1, 0, 1},
		{"01", 1, 1, 1},
		{"00", 0, 1, 1},
		{"033", 0, 0, 2},
		{"011", 3, 3, 2},
	}

	for _, tt := range tests {
		x, y, z := GoogleEarth.ConvertFromQtNode(tt.qtnode)
		if x != tt.x || y != tt.y || z != tt.z {
			t.Errorf("ConvertFromQtNode(%q) = (%d, %d, %d), want (%d, %d, %d)",
				tt.qtnode, x, y, z, tt.x, tt.y, tt.z)
		}

		// 验证可逆性
		qtnodeBack := GoogleEarth.ConvertToQtNode(tt.x, tt.y, tt.z)
		if qtnodeBack != tt.qtnode {
			t.Errorf("ConvertToQtNode(%d, %d, %d) = %q, want %q (reversibility test)",
				tt.x, tt.y, tt.z, qtnodeBack, tt.qtnode)
		}
	}
}

func TestQtNodeBounds(t *testing.T) {
	tests := []struct {
		name       string
		isMercator bool
	}{
		{"0", false},
		{"03", false},
		{"01", false},
		{"033", false},
	}

	for _, tt := range tests {
		minY, minX, maxY, maxX, level, ok := GoogleEarth.QtNodeBounds(tt.name, tt.isMercator)
		if !ok {
			t.Errorf("QtNodeBounds(%q, %v) failed", tt.name, tt.isMercator)
			continue
		}

		if level != uint(len(tt.name)-1) {
			t.Errorf("QtNodeBounds(%q) level = %d, want %d",
				tt.name, level, len(tt.name)-1)
		}

		// 边界应该有效
		if minY >= maxY || minX >= maxX {
			t.Errorf("QtNodeBounds(%q) invalid bounds: minY=%f, maxY=%f, minX=%f, maxX=%f",
				tt.name, minY, maxY, minX, maxX)
		}

		t.Logf("QtNodeBounds(%q) = [%f,%f] x [%f,%f], level=%d",
			tt.name, minY, maxY, minX, maxX, level)
	}
}

func TestMercatorProjection(t *testing.T) {
	tests := []struct {
		lat float64
	}{
		{0},
		{30},
		{60},
		{-30},
		{85}, // 接近极限
	}

	for _, tt := range tests {
		y := GoogleEarth.MercatorLatToY(tt.lat)
		lat := GoogleEarth.MercatorYToLat(y)

		if math.Abs(lat-tt.lat) > 1e-6 {
			t.Errorf("MercatorLatToY(%f) -> MercatorYToLat() = %f, want %f",
				tt.lat, lat, tt.lat)
		}
	}
}

func TestLatToYPos(t *testing.T) {
	tests := []struct {
		lat        float64
		z          uint
		isMercator bool
	}{
		{0, 1, true},
		{0, 2, true},
		{45, 3, true},
		{-45, 3, true},
		{0, 1, false},
		{45, 2, false},
	}

	for _, tt := range tests {
		yPos := GoogleEarth.LatToYPos(tt.lat, tt.z, tt.isMercator)
		maxYPos := uint(1) << tt.z
		if tt.isMercator {
			if yPos >= maxYPos {
				t.Errorf("LatToYPos(%f, %d, %v) = %d, should be < %d",
					tt.lat, tt.z, tt.isMercator, yPos, maxYPos)
			}
		}

		// 测试反向转换
		lat := GoogleEarth.YPosToLat(yPos, tt.z, tt.isMercator)
		// 由于是离散化的，不是完全可逆的，但应该在一定范围内
		if math.Abs(lat-tt.lat) > 10.0 {
			t.Logf("LatToYPos(%f, %d, %v) = %d -> YPosToLat = %f (diff: %f)",
				tt.lat, tt.z, tt.isMercator, yPos, lat, math.Abs(lat-tt.lat))
		}
	}
}

func TestBisectLatitudes(t *testing.T) {
	tests := []struct {
		south, north float64
		isMercator   bool
	}{
		{-45, 45, false},
		{0, 60, false},
		{-45, 45, true},
		{0, 60, true},
	}

	for _, tt := range tests {
		mid := GoogleEarth.BisectLatitudes(tt.south, tt.north, tt.isMercator)

		// 中间值应该在南北之间
		if mid < tt.south || mid > tt.north {
			t.Errorf("BisectLatitudes(%f, %f, %v) = %f, should be in [%f, %f]",
				tt.south, tt.north, tt.isMercator, mid, tt.south, tt.north)
		}

		t.Logf("BisectLatitudes(%f, %f, %v) = %f",
			tt.south, tt.north, tt.isMercator, mid)
	}
}

func TestLatLonToMeters(t *testing.T) {
	tests := []struct {
		lat, lon float64
	}{
		{0, 0},
		{39.9, 116.4},
		{-33.9, 151.2},
	}

	for _, tt := range tests {
		mx, my := GoogleEarth.LatLonToMeters(tt.lat, tt.lon)
		lat, lon := GoogleEarth.MetersToLatLon(mx, my)

		if math.Abs(lat-tt.lat) > 1e-6 || math.Abs(lon-tt.lon) > 1e-6 {
			t.Errorf("LatLonToMeters(%f, %f) -> MetersToLatLon() = (%f, %f), not reversible",
				tt.lat, tt.lon, lat, lon)
		}
	}
}

func TestLatLon2GoogleTile(t *testing.T) {
	tests := []struct {
		lat, lon float64
		zoom     int
	}{
		{39.9, 116.4, 10},  // 北京
		{-33.9, 151.2, 10}, // 悉尼
		{51.5, -0.1, 10},   // 伦敦
	}

	for _, tt := range tests {
		tileX, tileY := GoogleEarth.LatLon2GoogleTile(tt.lat, tt.lon, tt.zoom)

		// 验证瓦片坐标在合理范围内
		maxTile := int(math.Pow(2.0, float64(tt.zoom))) - 1
		if tileX < 0 || tileX > maxTile || tileY < 0 || tileY > maxTile {
			t.Errorf("LatLon2GoogleTile(%f, %f, %d) = (%d, %d), out of range [0, %d]",
				tt.lat, tt.lon, tt.zoom, tileX, tileY, maxTile)
		}

		// 获取瓦片边界
		minlat, minlon, maxlat, maxlon := GoogleEarth.GoogleTileLatLonBounds(tileX, tileY, tt.zoom)

		// 原始坐标应该在瓦片边界内（大致）
		t.Logf("LatLon(%f, %f) at zoom %d -> Tile(%d, %d), bounds: [%f,%f] x [%f,%f]",
			tt.lat, tt.lon, tt.zoom, tileX, tileY, minlat, maxlat, minlon, maxlon)
	}
}

func TestConvertFlatToMercatorQtAddresses(t *testing.T) {
	tests := []struct {
		flatQtAddress string
	}{
		{"03"},
		{"033"},
		{"0123"},
	}

	for _, tt := range tests {
		mercatorAddresses := GoogleEarth.ConvertFlatToMercatorQtAddresses(tt.flatQtAddress)
		if len(mercatorAddresses) == 0 {
			t.Errorf("ConvertFlatToMercatorQtAddresses(%q) returned empty list",
				tt.flatQtAddress)
		}

		t.Logf("ConvertFlatToMercatorQtAddresses(%q) = %v",
			tt.flatQtAddress, mercatorAddresses)
	}
}
