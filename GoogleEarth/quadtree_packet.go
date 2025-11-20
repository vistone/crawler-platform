package GoogleEarth

import (
	"bytes"
	"encoding/binary"
	"fmt"

	pb "crawler-platform/GoogleEarth/pb"

	"google.golang.org/protobuf/proto"
)

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

const (
	// Keyhole Magic ID
	KeyholeMagicID uint32 = 0x7E2D
)

// QuadTreeQuantum16 四叉树量子节点（二进制格式）
type QuadTreeQuantum16 struct {
	Children            uint8    // 子节点标志位（低4位）+ 其他标志（高4位）
	CNodeVersion        uint16   // 缓存节点版本
	ImageVersion        uint16   // 影像版本
	TerrainVersion      uint16   // 地形版本
	ImageNeighbors      [8]int8  // 影像邻居
	ImageDataProvider   uint8    // 影像数据提供商
	TerrainDataProvider uint8    // 地形数据提供商
	ChannelType         []uint16 // 通道类型列表
	ChannelVersion      []uint16 // 通道版本列表
}

// GetBit 获取子节点位（0-3）
func (q *QuadTreeQuantum16) GetBit(bit int) bool {
	return (q.Children & (1 << uint(bit))) != 0
}

// GetCacheNodeBit 获取缓存节点位（bit 4）
func (q *QuadTreeQuantum16) GetCacheNodeBit() bool {
	return (q.Children & 0x10) != 0
}

// GetDrawableBit 获取可绘制位（bit 5，矢量数据）
func (q *QuadTreeQuantum16) GetDrawableBit() bool {
	return (q.Children & 0x20) != 0
}

// GetImageBit 获取影像位（bit 6）
func (q *QuadTreeQuantum16) GetImageBit() bool {
	return (q.Children & 0x40) != 0
}

// GetTerrainBit 获取地形位（bit 7）
func (q *QuadTreeQuantum16) GetTerrainBit() bool {
	return (q.Children & 0x80) != 0
}

// HasLayerOfType 检查是否有指定类型的图层
func (q *QuadTreeQuantum16) HasLayerOfType(layerType pb.QuadtreeLayer_LayerType) bool {
	switch layerType {
	case pb.QuadtreeLayer_LAYER_TYPE_IMAGERY:
		return q.GetImageBit()
	case pb.QuadtreeLayer_LAYER_TYPE_TERRAIN:
		return q.GetTerrainBit()
	case pb.QuadtreeLayer_LAYER_TYPE_VECTOR:
		return q.GetDrawableBit()
	}
	return false
}

// GetDataReferences 获取量子节点的数据引用
func (q *QuadTreeQuantum16) GetDataReferences(references *QuadtreeDataReferenceGroup, qtPath QuadtreePath) {
	// 缓存节点引用
	if q.GetCacheNodeBit() {
		references.QtpRefs = append(references.QtpRefs, QuadtreeDataReference{
			QtPath:   qtPath,
			Version:  q.CNodeVersion,
			Channel:  0,
			Provider: 0,
		})
	}

	// 影像引用
	if q.GetImageBit() {
		references.ImgRefs = append(references.ImgRefs, QuadtreeDataReference{
			QtPath:   qtPath,
			Version:  q.ImageVersion,
			Channel:  uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY),
			Provider: uint16(q.ImageDataProvider),
		})
	}

	// 地形引用
	if q.GetTerrainBit() {
		references.TerRefs = append(references.TerRefs, QuadtreeDataReference{
			QtPath:   qtPath,
			Version:  q.TerrainVersion,
			Channel:  uint16(pb.QuadtreeLayer_LAYER_TYPE_TERRAIN),
			Provider: uint16(q.TerrainDataProvider),
		})
	}

	// 矢量引用（通道）
	if q.GetDrawableBit() {
		for i := 0; i < len(q.ChannelType); i++ {
			references.VecRefs = append(references.VecRefs, QuadtreeDataReference{
				QtPath:   qtPath,
				Version:  q.ChannelVersion[i],
				Channel:  q.ChannelType[i],
				Provider: 0,
			})
		}
	}
}

// QuadTreePacket16 四叉树数据包（二进制格式，Keyhole 旧格式）
type QuadTreePacket16 struct {
	MagicID          uint32               // Magic ID (0x7E2D)
	DataTypeID       uint32               // 数据类型 ID
	Version          uint32               // 版本号
	DataInstanceSize int32                // 数据实例大小
	DataBufferOffset int32                // 数据缓冲区偏移
	DataBufferSize   int32                // 数据缓冲区大小
	MetaBufferSize   int32                // 元数据缓冲区大小
	DataInstances    []*QuadTreeQuantum16 // 数据实例列表
}

// NewQuadTreePacket16 创建二进制格式的四叉树数据包
func NewQuadTreePacket16() *QuadTreePacket16 {
	return &QuadTreePacket16{
		DataInstances: make([]*QuadTreeQuantum16, 0),
	}
}

// Decode 解码二进制格式的四叉树数据包
func (qtp *QuadTreePacket16) Decode(data []byte) error {
	if len(data) < 32 {
		return fmt.Errorf("data too short for QuadTreePacket16 header")
	}

	reader := bytes.NewReader(data)

	// 读取头部
	binary.Read(reader, binary.LittleEndian, &qtp.MagicID)
	if qtp.MagicID != KeyholeMagicID {
		return fmt.Errorf("invalid magic ID: 0x%X, expected 0x%X", qtp.MagicID, KeyholeMagicID)
	}

	var numInstances int32
	binary.Read(reader, binary.LittleEndian, &qtp.DataTypeID)
	binary.Read(reader, binary.LittleEndian, &qtp.Version)
	binary.Read(reader, binary.LittleEndian, &numInstances)
	binary.Read(reader, binary.LittleEndian, &qtp.DataInstanceSize)
	binary.Read(reader, binary.LittleEndian, &qtp.DataBufferOffset)
	binary.Read(reader, binary.LittleEndian, &qtp.DataBufferSize)
	binary.Read(reader, binary.LittleEndian, &qtp.MetaBufferSize)

	if numInstances <= 0 {
		return nil // 空包
	}

	// 读取数据实例
	qtp.DataInstances = make([]*QuadTreeQuantum16, numInstances)
	for i := int32(0); i < numInstances; i++ {
		quantum := &QuadTreeQuantum16{}

		var byteFiller uint8
		var wordFiller uint16
		var typeOffset, versionOffset int32
		var numChannels uint16

		binary.Read(reader, binary.LittleEndian, &quantum.Children)
		binary.Read(reader, binary.LittleEndian, &byteFiller) // padding
		binary.Read(reader, binary.LittleEndian, &quantum.CNodeVersion)
		binary.Read(reader, binary.LittleEndian, &quantum.ImageVersion)
		binary.Read(reader, binary.LittleEndian, &quantum.TerrainVersion)
		binary.Read(reader, binary.LittleEndian, &numChannels)
		binary.Read(reader, binary.LittleEndian, &wordFiller) // padding
		binary.Read(reader, binary.LittleEndian, &typeOffset)
		binary.Read(reader, binary.LittleEndian, &versionOffset)
		binary.Read(reader, binary.LittleEndian, &quantum.ImageNeighbors)
		binary.Read(reader, binary.LittleEndian, &quantum.ImageDataProvider)
		binary.Read(reader, binary.LittleEndian, &quantum.TerrainDataProvider)
		binary.Read(reader, binary.LittleEndian, &wordFiller) // padding

		// 读取通道类型和版本（从数据缓冲区）
		if numChannels > 0 {
			quantum.ChannelType = make([]uint16, numChannels)
			quantum.ChannelVersion = make([]uint16, numChannels)

			// 从 dataBufferOffset + offset 位置读取
			typePos := int(qtp.DataBufferOffset + typeOffset)
			versionPos := int(qtp.DataBufferOffset + versionOffset)

			if typePos+int(numChannels)*2 <= len(data) {
				typeReader := bytes.NewReader(data[typePos:])
				binary.Read(typeReader, binary.LittleEndian, quantum.ChannelType)
			}

			if versionPos+int(numChannels)*2 <= len(data) {
				versionReader := bytes.NewReader(data[versionPos:])
				binary.Read(versionReader, binary.LittleEndian, quantum.ChannelVersion)
			}
		}

		qtp.DataInstances[i] = quantum
	}

	return nil
}

// FindNode 查找指定 subindex 的节点
func (qtp *QuadTreePacket16) FindNode(subindex int, rootNode bool) *QuadTreeQuantum16 {
	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}

	nodeIndex := 0
	return qtp.findNodeImpl(subindex, numbering, &nodeIndex, QuadtreePath{})
}

// findNodeImpl 递归查找节点
func (qtp *QuadTreePacket16) findNodeImpl(
	subindex int,
	numbering *QuadtreeNumbering,
	nodeIndex *int,
	qtPath QuadtreePath,
) *QuadTreeQuantum16 {
	if *nodeIndex >= len(qtp.DataInstances) {
		return nil
	}

	if int(qtPath.Level()) >= numbering.Depth() {
		return nil
	}

	node := qtp.DataInstances[*nodeIndex]
	currentSubindex := numbering.InorderToSubindex(numbering.TraversalPathToInorder(qtPath))

	if subindex == currentSubindex {
		return node
	}

	// 遍历子节点
	for i := 0; i < 4; i++ {
		if node.GetBit(i) {
			newPath := qtPath.Child(uint32(i))
			*nodeIndex++
			childNode := qtp.findNodeImpl(subindex, numbering, nodeIndex, newPath)
			if childNode != nil {
				return childNode
			}
		}
	}

	return nil
}

// GetDataReferences 获取所有数据引用
func (qtp *QuadTreePacket16) GetDataReferences(
	references *QuadtreeDataReferenceGroup,
	pathPrefix QuadtreePath,
	rootNode bool,
) {
	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}

	nodeIndex := 0
	qtp.traverser(references, numbering, &nodeIndex, pathPrefix, QuadtreePath{})
}

// traverser 遍历节点树
func (qtp *QuadTreePacket16) traverser(
	references *QuadtreeDataReferenceGroup,
	numbering *QuadtreeNumbering,
	nodeIndex *int,
	pathPrefix QuadtreePath,
	qtPath QuadtreePath,
) {
	if *nodeIndex >= len(qtp.DataInstances) {
		return
	}

	if int(qtPath.Level()) >= numbering.Depth() {
		return
	}

	node := qtp.DataInstances[*nodeIndex]
	absolutePath := pathPrefix.Concatenate(qtPath)

	// 收集当前节点的引用
	node.GetDataReferences(references, absolutePath)

	// 遍历子节点
	for i := 0; i < 4; i++ {
		if node.GetBit(i) {
			newPath := qtPath.Child(uint32(i))
			*nodeIndex++
			qtp.traverser(references, numbering, nodeIndex, pathPrefix, newPath)
		}
	}
}

// HasLayerOfType 检查是否包含指定类型的图层
func (qtp *QuadTreePacket16) HasLayerOfType(layerType pb.QuadtreeLayer_LayerType) bool {
	for _, node := range qtp.DataInstances {
		if node != nil && node.HasLayerOfType(layerType) {
			return true
		}
	}
	return false
}

// QuadtreeDataReference 四叉树数据引用
type QuadtreeDataReference struct {
	QtPath   QuadtreePath    // 四叉树路径
	Version  uint16          // 版本号
	Channel  uint16          // 通道（0 表示非矢量）
	Provider uint16          // 提供商
	JpegDate JpegCommentDate // JPEG 注释日期（历史影像）
}

// IsHistoricalImagery 判断是否为历史影像
func (ref *QuadtreeDataReference) IsHistoricalImagery() bool {
	return ref.Channel == uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY) &&
		!ref.JpegDate.IsCompletelyUnknown()
}

// QuadtreeDataReferenceGroup 四叉树数据引用组
type QuadtreeDataReferenceGroup struct {
	QtpRefs  []QuadtreeDataReference // QuadTree 包引用
	Qtp2Refs []QuadtreeDataReference // QuadTree2 包引用
	ImgRefs  []QuadtreeDataReference // 影像引用
	TerRefs  []QuadtreeDataReference // 地形引用
	VecRefs  []QuadtreeDataReference // 矢量引用
}

// Reset 清空所有引用
func (group *QuadtreeDataReferenceGroup) Reset() {
	group.QtpRefs = group.QtpRefs[:0]
	group.Qtp2Refs = group.Qtp2Refs[:0]
	group.ImgRefs = group.ImgRefs[:0]
	group.TerRefs = group.TerRefs[:0]
	group.VecRefs = group.VecRefs[:0]
}

// QuadtreePacketProtoBuf 四叉树数据包（Protobuf 格式）
type QuadtreePacketProtoBuf struct {
	packet *pb.QuadtreePacket
}

// NewQuadtreePacketProtoBuf 创建四叉树数据包
func NewQuadtreePacketProtoBuf() *QuadtreePacketProtoBuf {
	return &QuadtreePacketProtoBuf{
		packet: &pb.QuadtreePacket{},
	}
}

// Parse 解析 protobuf 数据
func (qtp *QuadtreePacketProtoBuf) Parse(data []byte) error {
	if err := proto.Unmarshal(data, qtp.packet); err != nil {
		return fmt.Errorf("failed to unmarshal quadtree packet: %w", err)
	}
	return nil
}

// GetPacket 获取底层的 protobuf 数据包
func (qtp *QuadtreePacketProtoBuf) GetPacket() *pb.QuadtreePacket {
	return qtp.packet
}

// FindNode 查找指定 subindex 的节点
func (qtp *QuadtreePacketProtoBuf) FindNode(subindex int, rootNode bool) *pb.QuadtreeNode {
	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}

	nodeIndex := 0
	path := QuadtreePath{}
	return qtp.findNodeImpl(subindex, numbering, &nodeIndex, path)
}

// findNodeImpl 递归查找节点
func (qtp *QuadtreePacketProtoBuf) findNodeImpl(
	subindex int,
	numbering *QuadtreeNumbering,
	currentNodeIndex *int,
	path QuadtreePath,
) *pb.QuadtreeNode {
	if qtp.packet == nil || qtp.packet.Sparsequadtreenode == nil {
		return nil
	}

	// 检查当前节点索引是否超出范围
	if *currentNodeIndex >= len(qtp.packet.Sparsequadtreenode) {
		return nil
	}

	sparseNode := qtp.packet.Sparsequadtreenode[*currentNodeIndex]
	currentSubindex := int(sparseNode.GetIndex())

	// 找到目标节点
	if currentSubindex == subindex {
		return sparseNode.Node
	}

	// 如果当前节点的 subindex 大于目标，说明不存在
	if currentSubindex > subindex {
		return nil
	}

	// 检查是否有子节点
	node := sparseNode.Node
	if node == nil || node.Flags == nil {
		return nil
	}

	childFlags := int(*node.Flags) & 0x0F
	if childFlags == 0 {
		return nil // 没有子节点
	}

	// 遍历子节点
	for i := 0; i < 4; i++ {
		if (childFlags & (1 << uint(i))) != 0 {
			*currentNodeIndex++
			childPath := path.Child(uint32(i))
			result := qtp.findNodeImpl(subindex, numbering, currentNodeIndex, childPath)
			if result != nil {
				return result
			}
		}
	}

	return nil
}

// HasLayerOfType 检查数据包中是否包含指定类型的图层
func (qtp *QuadtreePacketProtoBuf) HasLayerOfType(layerType pb.QuadtreeLayer_LayerType) bool {
	if qtp.packet == nil || qtp.packet.Sparsequadtreenode == nil {
		return false
	}

	for _, sparseNode := range qtp.packet.Sparsequadtreenode {
		if sparseNode.Node == nil {
			continue
		}
		for _, layer := range sparseNode.Node.Layer {
			if layer != nil && layer.Type != nil && *layer.Type == layerType {
				return true
			}
		}
	}

	return false
}

// GetDataReferences 获取数据引用
func (qtp *QuadtreePacketProtoBuf) GetDataReferences(
	references *QuadtreeDataReferenceGroup,
	pathPrefix QuadtreePath,
	jpegDate JpegCommentDate,
	rootNode bool,
) {
	if qtp.packet == nil || qtp.packet.Sparsequadtreenode == nil {
		return
	}

	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}

	// 遍历所有节点
	for _, sparseNode := range qtp.packet.Sparsequadtreenode {
		if sparseNode.Node == nil {
			continue
		}

		subindex := int(sparseNode.GetIndex())
		nodePath := pathPrefix.Concatenate(numbering.SubindexToTraversalPath(subindex))

		qtp.collectNodeReferences(sparseNode.Node, nodePath, references, jpegDate)
	}
}

// collectNodeReferences 从节点收集数据引用
func (qtp *QuadtreePacketProtoBuf) collectNodeReferences(
	node *pb.QuadtreeNode,
	path QuadtreePath,
	references *QuadtreeDataReferenceGroup,
	jpegDate JpegCommentDate,
) {
	// 处理图层
	for _, layer := range node.Layer {
		if layer == nil || layer.Type == nil {
			continue
		}

		layerType := *layer.Type
		version := uint16(0)
		provider := uint16(0)

		if layer.LayerEpoch != nil {
			version = uint16(*layer.LayerEpoch)
		}
		if layer.Provider != nil {
			provider = uint16(*layer.Provider)
		}

		ref := QuadtreeDataReference{
			QtPath:   path,
			Version:  version,
			Channel:  uint16(layerType),
			Provider: provider,
		}

		switch layerType {
		case pb.QuadtreeLayer_LAYER_TYPE_IMAGERY:
			references.ImgRefs = append(references.ImgRefs, ref)

		case pb.QuadtreeLayer_LAYER_TYPE_TERRAIN:
			references.TerRefs = append(references.TerRefs, ref)

		case pb.QuadtreeLayer_LAYER_TYPE_VECTOR:
			references.VecRefs = append(references.VecRefs, ref)

		case pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY:
			// 历史影像需要处理日期
			if layer.DatesLayer != nil {
				qtp.collectHistoricalImagery(layer.DatesLayer, path, version, provider, references, jpegDate)
			}
		}
	}

	// 处理通道（矢量数据）
	for _, channel := range node.Channel {
		if channel == nil {
			continue
		}

		channelType := uint16(0)
		version := uint16(0)

		if channel.Type != nil {
			channelType = uint16(*channel.Type)
		}
		if channel.ChannelEpoch != nil {
			version = uint16(*channel.ChannelEpoch)
		}

		ref := QuadtreeDataReference{
			QtPath:   path,
			Version:  version,
			Channel:  channelType,
			Provider: 0,
		}

		references.VecRefs = append(references.VecRefs, ref)
	}
}

// collectHistoricalImagery 收集历史影像引用
func (qtp *QuadtreePacketProtoBuf) collectHistoricalImagery(
	datesLayer *pb.QuadtreeImageryDates,
	path QuadtreePath,
	version uint16,
	provider uint16,
	references *QuadtreeDataReferenceGroup,
	filterDate JpegCommentDate,
) {
	if datesLayer == nil {
		return
	}

	// 处理日期瓦片
	for _, datedTile := range datesLayer.DatedTile {
		if datedTile == nil {
			continue
		}

		date := JpegCommentDate{}
		if datedTile.Date != nil {
			date = NewJpegCommentDateFromInt(*datedTile.Date)
		}

		// 如果指定了过滤日期，检查是否匹配
		if !filterDate.IsCompletelyUnknown() && !filterDate.MatchAllDates() {
			if date.CompareTo(filterDate) > 0 {
				continue // 跳过未来的日期
			}
		}

		tileVersion := version
		if datedTile.DatedTileEpoch != nil {
			tileVersion = uint16(*datedTile.DatedTileEpoch)
		}

		tileProvider := provider
		if datedTile.Provider != nil {
			tileProvider = uint16(*datedTile.Provider)
		}

		ref := QuadtreeDataReference{
			QtPath:   path,
			Version:  tileVersion,
			Channel:  uint16(pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY),
			Provider: tileProvider,
			JpegDate: date,
		}

		references.ImgRefs = append(references.ImgRefs, ref)
	}
}

// GetImageryDates 获取所有历史影像日期
func (qtp *QuadtreePacketProtoBuf) GetImageryDates() []JpegCommentDate {
	dates := make(map[int32]bool)
	result := []JpegCommentDate{}

	if qtp.packet == nil || qtp.packet.Sparsequadtreenode == nil {
		return result
	}

	for _, sparseNode := range qtp.packet.Sparsequadtreenode {
		if sparseNode.Node == nil {
			continue
		}

		for _, layer := range sparseNode.Node.Layer {
			if layer == nil || layer.Type == nil {
				continue
			}

			if *layer.Type == pb.QuadtreeLayer_LAYER_TYPE_IMAGERY_HISTORY && layer.DatesLayer != nil {
				for _, datedTile := range layer.DatesLayer.DatedTile {
					if datedTile != nil && datedTile.Date != nil {
						dateVal := *datedTile.Date
						if !dates[dateVal] {
							dates[dateVal] = true
							result = append(result, NewJpegCommentDateFromInt(dateVal))
						}
					}
				}
			}
		}
	}

	return result
}
