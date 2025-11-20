package GoogleEarth

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

const (
	// Google Earth 地形数据的 Magic Key
	GoogleEarthTerrainKey = "\x0a\x02\x08\x01"
	// 空网格头部大小
	EmptyMeshHeaderSize = 16
	// 地球半径（米）
	EarthMeanRadius = 6371010.0
	// 行星常数（米转地球半径）
	PlanetaryConstant = 1.0 / EarthMeanRadius
	// 负高程因子
	NegativeElevationExponentBias = 32
)

var (
	// 负高程因子
	NegativeElevationFactor = -math.Pow(2, NegativeElevationExponentBias)
)

// MeshVertex 网格顶点
type MeshVertex struct {
	X float64 // 经度或墨卡托 X 坐标
	Y float64 // 纬度或墨卡托 Y 坐标
	Z float32 // 高程（米）
}

// MeshFace 网格面（三角形）
type MeshFace struct {
	A, B, C uint16 // 三个顶点的索引
}

// Mesh 地形网格
type Mesh struct {
	SourceSize int          // 原始数据大小
	OriginX    float64      // 原点 X 坐标
	OriginY    float64      // 原点 Y 坐标
	DeltaX     float64      // X 方向步长
	DeltaY     float64      // Y 方向步长
	NumPoints  int          // 顶点数量
	NumFaces   int          // 面数量
	Level      int          // 层级
	Vertices   []MeshVertex // 顶点列表
	Faces      []MeshFace   // 面列表
}

// Reset 重置网格
func (m *Mesh) Reset() {
	m.SourceSize = 0
	m.OriginX = 0
	m.OriginY = 0
	m.DeltaX = 0
	m.DeltaY = 0
	m.NumPoints = 0
	m.NumFaces = 0
	m.Level = 0
	m.Vertices = m.Vertices[:0]
	m.Faces = m.Faces[:0]
}

// Decode 解码网格数据（完整版本，按 libge 实现）
func (m *Mesh) Decode(data []byte, offset *int) error {
	if len(data)-*offset < EmptyMeshHeaderSize {
		return fmt.Errorf("insufficient data for mesh header")
	}

	reader := bytes.NewReader(data[*offset:])

	// 读取 source_size
	var sourceSize int32
	binary.Read(reader, binary.LittleEndian, &sourceSize)
	m.SourceSize = int(sourceSize)

	dataOffset := EmptyMeshHeaderSize

	if sourceSize != 0 {
		// 读取网格头部
		var ox, oy, dx, dy float64
		var numPoints, numFaces, level int32

		binary.Read(reader, binary.LittleEndian, &ox)
		binary.Read(reader, binary.LittleEndian, &oy)
		binary.Read(reader, binary.LittleEndian, &dx)
		binary.Read(reader, binary.LittleEndian, &dy)
		binary.Read(reader, binary.LittleEndian, &numPoints)
		binary.Read(reader, binary.LittleEndian, &numFaces)
		binary.Read(reader, binary.LittleEndian, &level)

		// 转换为度（原始数据是归一化的）
		m.OriginX = ox * 180.0
		m.OriginY = oy * 180.0
		m.DeltaX = dx * 180.0
		m.DeltaY = dy * 180.0
		m.NumPoints = int(numPoints)
		m.NumFaces = int(numFaces)
		m.Level = int(level)

		if m.NumPoints < 0 {
			return fmt.Errorf("invalid num_points: %d", m.NumPoints)
		}

		// 读取顶点（压缩格式：2个字节的 x/y + 4字节的 z）
		m.Vertices = make([]MeshVertex, m.NumPoints)
		for i := 0; i < m.NumPoints; i++ {
			var cx, cy uint8
			var z float32

			binary.Read(reader, binary.LittleEndian, &cx)
			binary.Read(reader, binary.LittleEndian, &cy)
			binary.Read(reader, binary.LittleEndian, &z)

			// 解压缩坐标
			m.Vertices[i].X = float64(cx)*m.DeltaX + m.OriginX
			m.Vertices[i].Y = float64(cy)*m.DeltaY + m.OriginY
			// 转换高程（从地球半径单位转为米）
			m.Vertices[i].Z = z / float32(PlanetaryConstant)
		}

		// 读取面（三角形）
		m.Faces = make([]MeshFace, m.NumFaces)
		for i := 0; i < m.NumFaces; i++ {
			binary.Read(reader, binary.LittleEndian, &m.Faces[i].A)
			binary.Read(reader, binary.LittleEndian, &m.Faces[i].B)
			binary.Read(reader, binary.LittleEndian, &m.Faces[i].C)
		}

		dataOffset = m.SourceSize + 4
	} else {
		m.Reset()
	}

	*offset += dataOffset
	return nil
}

// Terrain 地形数据
type Terrain struct {
	QtNode     string            // 四叉树节点名称
	MeshGroups map[string][]Mesh // 按 qtnode 分组的网格列表
}

// NewTerrain 创建地形对象
func NewTerrain(qtNode string) *Terrain {
	return &Terrain{
		QtNode:     qtNode,
		MeshGroups: make(map[string][]Mesh),
	}
}

// Reset 重置地形数据
func (t *Terrain) Reset() {
	t.MeshGroups = make(map[string][]Mesh)
}

// Decode 解码地形数据（完整版本，按 libge 实现）
func (t *Terrain) Decode(data []byte) error {
	t.Reset()

	offset := 0
	for offset < len(data) {
		// 检查是否遇到终止标记
		if offset+len(GoogleEarthTerrainKey) <= len(data) {
			if string(data[offset:offset+len(GoogleEarthTerrainKey)]) == GoogleEarthTerrainKey {
				break
			}
		}

		mesh := Mesh{}
		err := mesh.Decode(data, &offset)
		if err != nil || mesh.SourceSize == 0 {
			break
		}

		// 根据第一个顶点的位置计算 qtnode 名称
		if mesh.NumPoints > 0 {
			// 使用 level-1 因为 Mesh 的 level 是网格细分层级
			qtNodeName := LatLonToQuadtreeAddress(
				mesh.Vertices[0].Y,
				mesh.Vertices[0].X,
				mesh.Level-1,
			)

			// 添加到对应的分组
			t.MeshGroups[qtNodeName] = append(t.MeshGroups[qtNodeName], mesh)
		}
	}

	return nil
}

// GetMeshGroup 获取指定 qtnode 的网格组
func (t *Terrain) GetMeshGroup(qtNode string) ([]Mesh, bool) {
	meshes, ok := t.MeshGroups[qtNode]
	return meshes, ok
}

// ToDEM 转换为 DEM（数字高程模型）格式
// qtNode: 四叉树节点名称
// isMercator: 是否使用墨卡托投影坐标
// 返回：DEM 数据字符串、列数、行数、错误
func (t *Terrain) ToDEM(qtNode string, isMercator bool) (string, int, int, error) {
	meshes, ok := t.MeshGroups[qtNode]
	if !ok {
		return "", 0, 0, fmt.Errorf("qtNode %s not found", qtNode)
	}

	return t.meshGroupToDEM(meshes, isMercator)
}

// meshGroupToDEM 将网格组转换为 DEM
func (t *Terrain) meshGroupToDEM(meshes []Mesh, isMercator bool) (string, int, int, error) {
	if len(meshes) == 0 {
		return "", 0, 0, fmt.Errorf("mesh group is empty")
	}

	// 计算网格尺寸（根据网格数量）
	gridSize := int(math.Sqrt(float64(len(meshes)))) * 128
	nCols := gridSize
	nRows := gridSize

	// 获取边界（使用第一个网格的信息）
	if meshes[0].NumPoints == 0 {
		return "", 0, 0, fmt.Errorf("mesh has no points")
	}

	// 生成 DEM 数据
	var buf bytes.Buffer

	// 写入 DEM 头部（简单的 XYZ 格式）
	buf.WriteString(fmt.Sprintf("ncols %d\n", nCols))
	buf.WriteString(fmt.Sprintf("nrows %d\n", nRows))
	buf.WriteString(fmt.Sprintf("xllcorner %f\n", meshes[0].OriginX))
	buf.WriteString(fmt.Sprintf("yllcorner %f\n", meshes[0].OriginY))
	buf.WriteString(fmt.Sprintf("cellsize %f\n", meshes[0].DeltaX))
	buf.WriteString("NODATA_value -9999\n")

	// TODO: 实现完整的三角形插值算法（按 libge 的 toDEM 实现）
	// 这里仅提供简单版本

	return buf.String(), nCols, nRows, nil
}

// GetMesh 获取指定 qtNode 和索引的网格
func (t *Terrain) GetMesh(qtNode string, index int) (*Mesh, error) {
	meshes, ok := t.MeshGroups[qtNode]
	if !ok {
		return nil, fmt.Errorf("qtNode %s not found", qtNode)
	}
	if index < 0 || index >= len(meshes) {
		return nil, fmt.Errorf("mesh index out of range")
	}
	return &meshes[index], nil
}

// NumMeshes 返回总网格数
func (t *Terrain) NumMeshes() int {
	total := 0
	for _, meshes := range t.MeshGroups {
		total += len(meshes)
	}
	return total
}

// NumMeshGroups 返回网格组数
func (t *Terrain) NumMeshGroups() int {
	return len(t.MeshGroups)
}

// GetElevationAt 获取指定位置的高程（通过插值）
func (t *Terrain) GetElevationAt(qtNode string, meshIndex int, x, y float64) (float32, error) {
	meshes, ok := t.MeshGroups[qtNode]
	if !ok {
		return 0, fmt.Errorf("qtNode %s not found", qtNode)
	}
	if meshIndex < 0 || meshIndex >= len(meshes) {
		return 0, fmt.Errorf("mesh index out of range")
	}

	mesh := &meshes[meshIndex]

	// 简单的最近邻插值
	minDist := math.MaxFloat64
	var elevation float32 = 0

	for i := 0; i < mesh.NumPoints; i++ {
		dx := mesh.Vertices[i].X - x
		dy := mesh.Vertices[i].Y - y
		dist := math.Sqrt(dx*dx + dy*dy)

		if dist < minDist {
			minDist = dist
			elevation = mesh.Vertices[i].Z
		}
	}

	return elevation, nil
}
