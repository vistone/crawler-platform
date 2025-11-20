package GoogleEarth

import (
	"math"
)

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

const (
	// 地球半径（米）
	EarthRadius = 6378137.0
	// 墨卡托投影的最大纬度（约85.05度）
	MaxMercatorLatitude = 85.05112878
	// 墨卡托投影的最大经度
	MaxMercatorLongitude = 180.0
	// π
	Pi = math.Pi
	// 最大层级
	MAX_LEVEL = 32
	// Google Tile 瓦片大小
	TILE_SIZE = 256
)

// DegToRad 角度转弧度
func DegToRad(deg float64) float64 {
	return deg * Pi / 180.0
}

// RadToDeg 弧度转角度
func RadToDeg(rad float64) float64 {
	return rad * 180.0 / Pi
}

// LatLonToMercator 经纬度转墨卡托投影坐标
// lat: 纬度（度）
// lon: 经度（度）
// 返回：墨卡托 X, Y 坐标（米）
func LatLonToMercator(lat, lon float64) (x, y float64) {
	// 限制纬度范围
	if lat > MaxMercatorLatitude {
		lat = MaxMercatorLatitude
	}
	if lat < -MaxMercatorLatitude {
		lat = -MaxMercatorLatitude
	}

	// 墨卡托投影公式
	x = EarthRadius * DegToRad(lon)
	y = EarthRadius * math.Log(math.Tan(Pi/4+DegToRad(lat)/2))

	return x, y
}

// MercatorToLatLon 墨卡托投影坐标转经纬度
// x: 墨卡托 X 坐标（米）
// y: 墨卡托 Y 坐标（米）
// 返回：纬度、经度（度）
func MercatorToLatLon(x, y float64) (lat, lon float64) {
	lon = RadToDeg(x / EarthRadius)
	lat = RadToDeg(2*math.Atan(math.Exp(y/EarthRadius)) - Pi/2)

	return lat, lon
}

// TileBounds 计算指定瓦片的地理边界
// level: 瓦片层级
// row: 瓦片行号
// col: 瓦片列号
// 返回：minLat, minLon, maxLat, maxLon（度）
func TileBounds(level, row, col int) (minLat, minLon, maxLat, maxLon float64) {
	gridSize := 1 << uint(level) // 2^level

	// 墨卡托投影的世界范围
	worldSize := 2.0 * Pi * EarthRadius

	// 瓦片大小（墨卡托坐标）
	tileSize := worldSize / float64(gridSize)

	// 瓦片的墨卡托边界（Y轴向下为正）
	minX := -worldSize/2 + float64(col)*tileSize
	maxX := minX + tileSize
	maxY := worldSize/2 - float64(row)*tileSize
	minY := maxY - tileSize

	// 转换为经纬度
	minLat, minLon = MercatorToLatLon(minX, minY)
	maxLat, maxLon = MercatorToLatLon(maxX, maxY)

	return minLat, minLon, maxLat, maxLon
}

// LatLonToTile 将经纬度转换为指定层级的瓦片坐标
// lat: 纬度（度）
// lon: 经度（度）
// level: 瓦片层级
// 返回：row, col
func LatLonToTile(lat, lon float64, level int) (row, col int) {
	gridSize := 1 << uint(level)

	// 转换为墨卡托坐标
	x, y := LatLonToMercator(lat, lon)

	// 世界范围
	worldSize := 2.0 * Pi * EarthRadius

	// 归一化到 [0, 1]
	normX := (x + worldSize/2) / worldSize
	normY := (worldSize/2 - y) / worldSize

	// 转换为瓦片坐标
	col = int(normX * float64(gridSize))
	row = int(normY * float64(gridSize))

	// 边界检查
	if col < 0 {
		col = 0
	}
	if col >= gridSize {
		col = gridSize - 1
	}
	if row < 0 {
		row = 0
	}
	if row >= gridSize {
		row = gridSize - 1
	}

	return row, col
}

// QuadtreeAddress 生成四叉树地址（Google/Bing 格式）
// level: 层级
// row: 行号
// col: 列号
// 返回：四叉树地址字符串
func QuadtreeAddress(level, row, col int) string {
	path := NewQuadtreePath(uint32(level), uint32(row), uint32(col))
	return path.AsString()
}

// LatLonToQuadtreeAddress 将经纬度转换为四叉树地址
// lat: 纬度（度）
// lon: 经度（度）
// level: 层级
// 返回：四叉树地址字符串
func LatLonToQuadtreeAddress(lat, lon float64, level int) string {
	row, col := LatLonToTile(lat, lon, level)
	return QuadtreeAddress(level, row, col)
}

// QuadtreeAddressToBounds 将四叉树地址转换为地理边界
// address: 四叉树地址字符串
// 返回：minLat, minLon, maxLat, maxLon（度）
func QuadtreeAddressToBounds(address string) (minLat, minLon, maxLat, maxLon float64) {
	path := NewQuadtreePathFromString(address)
	level, row, col := path.GetLevelRowCol()
	return TileBounds(int(level), int(row), int(col))
}

// TileCenter 计算瓦片的中心点经纬度
// level: 层级
// row: 行号
// col: 列号
// 返回：centerLat, centerLon（度）
func TileCenter(level, row, col int) (centerLat, centerLon float64) {
	minLat, minLon, maxLat, maxLon := TileBounds(level, row, col)
	centerLat = (minLat + maxLat) / 2
	centerLon = (minLon + maxLon) / 2
	return centerLat, centerLon
}

// NormalizeLongitude 归一化经度到 [-180, 180]
func NormalizeLongitude(lon float64) float64 {
	for lon > 180 {
		lon -= 360
	}
	for lon < -180 {
		lon += 360
	}
	return lon
}

// NormalizeLatitude 归一化纬度到 [-90, 90]
func NormalizeLatitude(lat float64) float64 {
	if lat > 90 {
		lat = 90
	}
	if lat < -90 {
		lat = -90
	}
	return lat
}

// ConvertFlatToMercatorQtAddresses 将平面卡里投影的四叉树地址转换为墨卡托投影的地址列表
// flatQtAddress: 平面卡里投影的四叉树地址
// 返回：墨卡托投影的四叉树地址列表
func ConvertFlatToMercatorQtAddresses(flatQtAddress string) []string {
	x, y, z := ConvertFromQtNode(flatQtAddress)
	if z == MAX_LEVEL {
		return nil
	}

	// 网格维度是 2^z x 2^z
	maxYPos := float64(uint(1) << z)
	yValue := float64(y)

	// 计算瓦片的顶部和底部纬度
	// 使用360度范围，因为只有网格的中间部分用于平面卡里投影
	// 整个网格实际上跨越(-180,180)，但(-180,-90)和(90,180)是黑色的
	minLat := 180.0 - (360.0 * yValue / maxYPos)
	maxLat := 180.0 - (360.0 * (yValue + 1.0) / maxYPos)

	// 找到墨卡托地图上的相应 y 范围
	// 积极包含，并允许未找到的瓦片被忽略
	yBottom := LatToYPos(minLat, z, true)
	yTop := LatToYPos(maxLat, z, true)

	// 为每个 y 值添加一个墨卡托四叉树地址
	var mercatorQtAddresses []string
	for yNext := yBottom; yNext <= yTop; yNext++ {
		mercatorQtAddresses = append(mercatorQtAddresses, ConvertToQtNode(x, yNext, z))
	}

	return mercatorQtAddresses
}

// LatToYPos 返回给定纬度对应的网格 y 位置
// lat: 纬度（度）
// z: 深度（层级）
// isMercator: 是否墨卡托投影
// 返回：yPos 是一个在 [0, 2^z) 范围内的整数
func LatToYPos(lat float64, z uint, isMercator bool) uint {
	var y, minY, maxY float64
	var yOff uint
	maxYPos := uint(1) << z

	// y 是倒置的（单调向下）
	lat = -lat
	if isMercator {
		y = MercatorLatToY(lat)
		minY = -Pi
		maxY = Pi
		yOff = 0
	} else {
		y = lat
		minY = -90.0
		maxY = 90.0
		// 对于非墨卡托，我们只使用一半的 y 空间
		maxYPos >>= 1
		// 影像从网格的四分之一处开始
		yOff = maxYPos >> 1
	}

	// 对于超出极值的非法输入，强制合法的 y 位置
	if y >= maxY {
		return maxYPos - 1 + yOff
	} else if y < minY {
		return yOff
	} else {
		return uint((y-minY)/(maxY-minY)*float64(maxYPos)) + yOff
	}
}

// YPosToLat 返回给定网格 y 位置对应的纬度
// y: y 位置
// z: 深度（层级）
// isMercator: 是否墨卡托投影
// 返回：纬度（度）
func YPosToLat(y, z uint, isMercator bool) float64 {
	var minY, maxY float64
	var yOff uint
	maxYPos := uint(1) << z

	if isMercator {
		minY = -Pi
		maxY = Pi
		yOff = 0
	} else {
		minY = -90.0
		maxY = 90.0
		// 对于非墨卡托，我们只使用一半的 y 空间
		maxYPos >>= 1
		// 影像从网格的四分之一处开始
		yOff = maxYPos >> 1
	}

	if y >= maxYPos {
		return minY
	} else if y <= yOff {
		return maxY
	} else {
		lat := float64(y-yOff)*(maxY-minY)/float64(maxYPos) + minY
		if isMercator {
			lat = MercatorYToLat(lat)
		}
		return -lat
	}
}

// MercatorLatToY 返回与给定纬度相关的 y 位置
// lat: 纬度（度）
// 返回：y 是一个在 (-pi, pi) 范围内的浮点数
func MercatorLatToY(lat float64) float64 {
	if lat >= MaxMercatorLatitude {
		return Pi
	} else if lat <= -MaxMercatorLatitude {
		return -Pi
	}

	return math.Log(math.Tan(Pi/4.0 + lat/360.0*Pi))
}

// MercatorLngToX 经度转墨卡托 X
func MercatorLngToX(lng float64) float64 {
	if lng >= MaxMercatorLongitude {
		return Pi
	} else if lng <= -MaxMercatorLongitude {
		return -Pi
	}

	return lng
}

// MercatorYToLat 返回与给定 y 位置相关的纬度
// y: y 是一个在 (-pi, pi) 范围内的浮点数
// 返回：纬度（度）
func MercatorYToLat(y float64) float64 {
	if y >= Pi {
		return MaxMercatorLatitude
	} else if y <= -Pi {
		return -MaxMercatorLatitude
	}

	return (math.Atan(math.Exp(y)) - Pi/4.0) * 360.0 / Pi
}

// MercatorXToLng 墨卡托 X 转经度
func MercatorXToLng(x float64) float64 {
	if x >= Pi {
		return MaxMercatorLongitude
	} else if x <= -Pi {
		return -MaxMercatorLongitude
	}

	return x
}

// YToYPos 返回给定深度的墨卡托网格上对应于归一化线性 y 值的位置
// y: y 是一个在 (-pi, pi) 范围内的浮点数
// z: 深度（层级）
// 返回：yPos 是一个在 [0, 2^z) 范围内的整数
func YToYPos(y float64, z uint) uint {
	maxYPos := int(1 << z)
	yPos := int(float64(maxYPos) * ((y + Pi) / (2.0 * Pi)))
	if yPos < 0 {
		return 0
	} else if yPos >= maxYPos {
		return uint(maxYPos - 1)
	} else {
		return uint(yPos)
	}
}

// BisectLatitudes 返回在墨卡托地图上两个给定纬度之间中间的纬度
// south: 南纬度
// north: 北纬度
// isMercator: 是否墨卡托投影
// 返回：中间纬度
func BisectLatitudes(south, north float64, isMercator bool) float64 {
	if isMercator {
		y1 := MercatorLatToY(south)
		y2 := MercatorLatToY(north)
		return MercatorYToLat((y1 + y2) / 2.0)
	} else {
		return (south + north) / 2.0
	}
}

// ConvertToQtNode 从地图空间转换为 qtnode 地址（基于 x, y, z）
// x: 列号
// y: 行号
// z: 层级
// 返回：qtnode 地址字符串，如果输入无效则返回空字符串
func ConvertToQtNode(x, y, z uint) string {
	qtnode := "0"
	// 目标 LOD 的地图坐标的宽度或高度的一半
	// 即顶级象限的大小
	halfNdim := uint(1) << (z - 1)

	for i := uint(0); i < z; i++ {
		// 根据 x, y 落入的象限选择四叉树地址字符
		if (y >= halfNdim) && (x < halfNdim) {
			qtnode += "0"
			y -= halfNdim
		} else if (y >= halfNdim) && (x >= halfNdim) {
			qtnode += "1"
			y -= halfNdim
			x -= halfNdim
		} else if (y < halfNdim) && (x >= halfNdim) {
			qtnode += "2"
			x -= halfNdim
		} else {
			qtnode += "3"
		}

		// 为下一级象限减半
		halfNdim >>= 1
	}

	// x 和 y 最后应该被清零
	if x != 0 || y != 0 {
		return ""
	}

	return qtnode
}

// ConvertFromQtNode 从 qtnode 地址转换为地图空间
// qtnode: qtnode 地址字符串（期望以 "0" 开头）
// 返回：x（列号），y（行号），z（层级）。如果有错误，z 返回 MAX_LEVEL
func ConvertFromQtNode(qtnode string) (x, y, z uint) {
	z = uint(len(qtnode))
	if z == 0 {
		z = MAX_LEVEL
		return
	}

	// LOD 是四叉树地址的长度 - 1
	z -= 1

	if qtnode[0] != '0' {
		z = MAX_LEVEL
		return
	}

	// 对于每个四叉树地址字符，为 x 和 y 位置添加1位信息
	x = 0
	y = 0
	// 目标 LOD 的地图坐标的宽度或高度的一半
	// 即顶级象限的大小
	halfNdim := uint(1) << (z - 1)

	for i := uint(0); i < z; i++ {
		switch qtnode[i+1] {
		case '0':
			y += halfNdim
		case '1':
			x += halfNdim
			y += halfNdim
		case '2':
			x += halfNdim
		case '3':
			// 无变化
		default:
			z = MAX_LEVEL
			return
		}

		// 为下一级象限减半
		halfNdim >>= 1
	}

	return
}

// QtNodeBounds 计算 qtnode 的地理边界
// name: qtnode 地址字符串
// isMercator: 是否墨卡托投影
// 返回：minY, minX, maxY, maxX, level
func QtNodeBounds(name string, isMercator bool) (minY, minX, maxY, maxX float64, level uint, ok bool) {
	if name == "" {
		return 0, 0, 0, 0, 0, false
	}

	minX = -180.0
	maxY = 180.0
	maxX = 180.0
	minY = -180.0
	level = uint(len(name) - 1)

	for i := 1; i < len(name); i++ {
		switch name[i] {
		case '0':
			maxY = (maxY + minY) / 2.0
			maxX = (minX + maxX) / 2.0
		case '1':
			maxY = (maxY + minY) / 2.0
			minX = (minX + maxX) / 2.0
		case '2':
			minY = (maxY + minY) / 2.0
			minX = (minX + maxX) / 2.0
		case '3':
			minY = (maxY + minY) / 2.0
			maxX = (minX + maxX) / 2.0
		default:
			// 无效字符
		}
	}

	if isMercator {
		minX, minY = LatLonToMeters(minY, minX)
		maxX, maxY = LatLonToMeters(maxY, maxX)
	}

	return minY, minX, maxY, maxX, level, true
}

// ConvertToQtNodeFromLatLon 从经纬度转换为 qtnode（基于经纬度坐标）
// y: 纬度
// x: 经度
// level: 层级
// isMercator: 是否墨卡托投影
// 返回：qtnode 地址字符串
func ConvertToQtNodeFromLatLon(y, x float64, level uint, isMercator bool) string {
	if level >= MAX_LEVEL {
		return ""
	}

	minX := -180.0
	maxY := 180.0
	maxX := 180.0
	minY := -180.0

	if isMercator {
		y, x = MetersToLatLon(x, y)
	}

	if x < minX || x > maxX || y < minY || y > maxY {
		return ""
	}

	name := make([]byte, level+1)
	for i := range name {
		name[i] = '0'
	}

	for i := uint(1); i <= level; i++ {
		midX := (minX + maxX) / 2.0
		midY := (maxY + minY) / 2.0

		if x >= minX && x < midX && y >= minY && y < midY {
			maxY = midY
			maxX = midX
			name[i] = '0'
		} else if x >= midX && x < maxX && y >= minY && y < midY {
			maxY = midY
			minX = midX
			name[i] = '1'
		} else if x >= midX && x < maxX && y >= midY && y < maxY {
			minY = midY
			minX = midX
			name[i] = '2'
		} else if x >= minX && x < midX && y >= midY && y < maxY {
			minY = midY
			maxX = midX
			name[i] = '3'
		} else {
			return ""
		}
	}

	return string(name)
}

// ConvertToQtNodeFromBounds 从边界框转换为 qtnode 列表
// minY, minX, maxY, maxX: 边界框
// level: 层级
// isMercator: 是否墨卡托投影
// 返回：qtnode 地址字符串列表
func ConvertToQtNodeFromBounds(minY, minX, maxY, maxX float64, level uint, isMercator bool) []string {
	minx := ^uint(0) // math.MaxUint
	miny := ^uint(0)
	var maxx, maxy uint
	xOut := false
	yOut := false

	name := ConvertToQtNodeFromLatLon(minY, minX, level, isMercator)
	if name != "" {
		x, y, z := ConvertFromQtNode(name)
		if z != MAX_LEVEL {
			if x < minx {
				minx = x
			}
			if x > maxx {
				maxx = x
			}
			if y < miny {
				miny = y
			}
			if y > maxy {
				maxy = y
			}

			tmpMinY, tmpMinX, tmpMaxY, tmpMaxX, _, ok := QtNodeBounds(name, isMercator)
			if ok {
				xOut = tmpMaxX < maxX
				yOut = tmpMaxY < maxY
				_ = tmpMinY // 避免未使用变量警告
				_ = tmpMinX // 避免未使用变量警告
			}
		}
	}

	if xOut {
		name = ConvertToQtNodeFromLatLon(minY, maxX, level, isMercator)
		if name != "" {
			x, y, z := ConvertFromQtNode(name)
			if z != MAX_LEVEL {
				if x < minx {
					minx = x
				}
				if x > maxx {
					maxx = x
				}
				if y < miny {
					miny = y
				}
				if y > maxy {
					maxy = y
				}
			}
		}
	}

	if yOut {
		name = ConvertToQtNodeFromLatLon(maxY, minX, level, isMercator)
		if name != "" {
			x, y, z := ConvertFromQtNode(name)
			if z != MAX_LEVEL {
				if x < minx {
					minx = x
				}
				if x > maxx {
					maxx = x
				}
				if y < miny {
					miny = y
				}
				if y > maxy {
					maxy = y
				}
			}
		}

		if xOut {
			name = ConvertToQtNodeFromLatLon(maxY, maxX, level, isMercator)
			if name != "" {
				x, y, z := ConvertFromQtNode(name)
				if z != MAX_LEVEL {
					if x < minx {
						minx = x
					}
					if x > maxx {
						maxx = x
					}
					if y < miny {
						miny = y
					}
					if y > maxy {
						maxy = y
					}
				}
			}
		}
	}

	var names []string
	for x := minx; x <= maxx; x++ {
		for y := miny; y <= maxy; y++ {
			name = ConvertToQtNode(x, y, level)
			if name != "" {
				names = append(names, name)
			}
		}
	}

	return names
}

// LatLonToMeters 经纬度转米制坐标（Google Maps API 使用的墨卡托投影）
// lat: 纬度
// lon: 经度
// 返回：mx, my（米）
func LatLonToMeters(lat, lon float64) (mx, my float64) {
	originShift := 2 * Pi * 6378137.0 / 2.0
	mx = lon * originShift / 180.0
	my = math.Log(math.Tan((90.0+lat)*Pi/360.0)) / (Pi / 180.0)
	my = my * originShift / 180.0
	return
}

// MetersToLatLon 米制坐标转经纬度
// mx, my: 米制坐标
// 返回：lat, lon（度）
func MetersToLatLon(mx, my float64) (lat, lon float64) {
	originShift := 2 * Pi * 6378137.0 / 2.0
	lon = (mx / originShift) * 180.0
	lat = (my / originShift) * 180.0
	lat = 180.0 / Pi * (2.0*math.Atan(math.Exp(lat*Pi/180.0)) - Pi/2.0)
	return
}

// PixelsToMeters 像素坐标转米制坐标
func PixelsToMeters(px, py float64, zoom int) (mx, my float64) {
	originShift := 2 * Pi * 6378137.0 / 2.0
	initialResolution := 2 * Pi * 6378137.0 / float64(TILE_SIZE)
	res := initialResolution / math.Pow(2.0, float64(zoom))
	mx = px*res - originShift
	my = py*res - originShift
	return
}

// MetersToPixels 米制坐标转像素坐标
func MetersToPixels(mx, my float64, zoom int) (px, py float64) {
	originShift := 2 * Pi * 6378137.0 / 2.0
	initialResolution := 2 * Pi * 6378137.0 / float64(TILE_SIZE)
	res := initialResolution / math.Pow(2.0, float64(zoom))
	px = (mx + originShift) / res
	py = (my + originShift) / res
	return
}

// PixelsToTile 像素坐标转瓦片坐标
func PixelsToTile(px, py float64) (tileX, tileY int) {
	tileX = int(math.Ceil(px/float64(TILE_SIZE))) - 1
	tileY = int(math.Ceil(py/float64(TILE_SIZE))) - 1
	return
}

// MetersToTile 米制坐标转瓦片坐标
func MetersToTile(mx, my float64, zoom int) (tileX, tileY int) {
	px, py := MetersToPixels(mx, my, zoom)
	return PixelsToTile(px, py)
}

// GoogleTile 转换为 Google Tile 坐标（Y轴翻转）
func GoogleTile(tx, ty, zoom int) (tileX, tileY int) {
	tileX = tx
	tileY = int(math.Pow(2.0, float64(zoom))) - 1 - ty
	return
}

// LatLon2GoogleTile 经纬度转 Google Tile 坐标
func LatLon2GoogleTile(lat, lon float64, zoom int) (tileX, tileY int) {
	mx, my := LatLonToMeters(lat, lon)
	tileX, tileY = MetersToTile(mx, my, zoom)
	return GoogleTile(tileX, tileY, zoom)
}

// TileBoundsMeters 计算瓦片的米制边界
func TileBoundsMeters(tx, ty, zoom int) (minx, miny, maxx, maxy float64) {
	minx, miny = PixelsToMeters(float64(tx*TILE_SIZE), float64(ty*TILE_SIZE), zoom)
	maxx, maxy = PixelsToMeters(float64((tx+1)*TILE_SIZE), float64((ty+1)*TILE_SIZE), zoom)
	return
}

// GoogleTileLatLonBounds 计算 Google Tile 的经纬度边界
func GoogleTileLatLonBounds(tileX, tileY, zoom int) (minlat, minlon, maxlat, maxlon float64) {
	minx, miny, maxx, maxy := TileBoundsMeters(tileX, tileY, zoom)
	minlat, minlon = MetersToLatLon(minx, miny)
	maxlat, maxlon = MetersToLatLon(maxx, maxy)
	return
}
