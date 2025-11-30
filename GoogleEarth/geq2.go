package GoogleEarth

import (
	"encoding/json"
	"fmt"
)

// Q2Parser 定义Q2解析接口，支持输出结构体或JSON
type Q2Parser interface {
	// Parse 解析为结构体（简化输出结构）
	Parse(body []byte, tilekey string, rootNode bool) (*Q2Response, error)
	// ParseToJSON 解析为JSON字符串（与 ParseQ2Body 输出一致）
	ParseToJSON(body []byte, tilekey string, rootNode bool) (string, error)
}

// DefaultQ2Parser 默认实现，复用现有解析逻辑
type DefaultQ2Parser struct{}

// NewQ2Parser 工厂方法，获取默认解析器
func NewQ2Parser() Q2Parser { return &DefaultQ2Parser{} }

// ParseToJSON 直接复用现有 JSON 输出逻辑
func (p *DefaultQ2Parser) ParseToJSON(body []byte, tilekey string, rootNode bool) (string, error) {
	return ParseQ2Body(body, tilekey, rootNode)
}

// Parse 返回结构体形式的简化响应
func (p *DefaultQ2Parser) Parse(body []byte, tilekey string, rootNode bool) (*Q2Response, error) {
	jsonStr, err := p.ParseToJSON(body, tilekey, rootNode)
	if err != nil {
		return nil, err
	}
	var resp Q2Response
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Q2URLBuilder URL构造策略接口
type Q2URLBuilder interface {
	Imagery(tilekey string, version uint16) string
	Terrain(tilekey string, version uint16) string
	Q2(tilekey string, version uint16) string
}

// PathOnlyURLBuilder 仅输出URI路径的默认实现
type PathOnlyURLBuilder struct {
	Base string
}

func (b PathOnlyURLBuilder) Imagery(tilekey string, version uint16) string {
	if b.Base == "" {
		return ""
	}
	return fmt.Sprintf("/flatfile?f1-%s-i.%d", tilekey, version)
}

func (b PathOnlyURLBuilder) Terrain(tilekey string, version uint16) string {
	if b.Base == "" {
		return ""
	}
	return fmt.Sprintf("/flatfile?f1c-%s-t.%d", tilekey, version)
}

func (b PathOnlyURLBuilder) Q2(tilekey string, version uint16) string {
	if b.Base == "" {
		return ""
	}
	return fmt.Sprintf("/flatfile?q2-%s-q.%d", tilekey, version)
}

// Q2Filter 数据项过滤策略接口
type Q2Filter interface {
	IncludeImagery(tilekey string, version uint16, provider uint16) bool
	IncludeTerrain(tilekey string, version uint16, provider uint16) bool
	IncludeQ2(tilekey string, version uint16) bool
}

// DefaultQ2Filter 默认过滤策略：
// - 影像/地形全部保留
// - Q2子节点仅保留tilekey长度为4的倍数（保持既有行为）
type DefaultQ2Filter struct{}

func (DefaultQ2Filter) IncludeImagery(tilekey string, version uint16, provider uint16) bool {
	return true
}
func (DefaultQ2Filter) IncludeTerrain(tilekey string, version uint16, provider uint16) bool {
	return true
}
func (DefaultQ2Filter) IncludeQ2(tilekey string, version uint16) bool { return len(tilekey)%4 == 0 }

// Q2Response Q2数据的简化JSON响应结构（输出格式）
type Q2Response struct {
	Tilekey     string          `json:"tilekey"`         // 父节点的瓦片路径
	ImageryList []Q2DataRefJSON `json:"imagery_list"`    // 影像数据列表
	TerrainList []Q2DataRefJSON `json:"terrain_list"`    // 地形数据列表
	VectorList  []Q2DataRefJSON `json:"vector_list"`     // 矢量数据列表
	Q2List      []Q2DataRefJSON `json:"q2_list"`         // Q2子节点列表
	Success     bool            `json:"success"`         // 解析是否成功
	Error       string          `json:"error,omitempty"` // 错误信息

	// 内部字段，不输出到JSON
	MagicID        string        `json:"-"` // Magic ID (十六进制)
	DataTypeID     uint32        `json:"-"` // 数据类型ID
	Version        uint32        `json:"-"` // 版本号
	NodeCount      int           `json:"-"` // 节点数量
	Nodes          []Q2NodeJSON  `json:"-"` // 节点列表（不输出到JSON）
	DataReferences *Q2References `json:"-"` // 数据引用（不输出到JSON）
}

// Q2NodeJSON Q2节点的JSON表示
type Q2NodeJSON struct {
	Index           int             `json:"index"`                      // 节点索引
	Path            string          `json:"path"`                       // 四叉树路径
	Subindex        int             `json:"subindex"`                   // 子索引
	Children        []int           `json:"children"`                   // 子节点索引列表
	ChildCount      int             `json:"child_count"`                // 子节点数量
	HasCache        bool            `json:"has_cache"`                  // 是否有缓存节点
	HasImage        bool            `json:"has_image"`                  // 是否有影像数据
	HasTerrain      bool            `json:"has_terrain"`                // 是否有地形数据
	HasVector       bool            `json:"has_vector"`                 // 是否有矢量数据
	CNodeVersion    uint16          `json:"cache_node_version"`         // 缓存节点版本
	ImageVersion    uint16          `json:"image_version,omitempty"`    // 影像版本
	TerrainVersion  uint16          `json:"terrain_version,omitempty"`  // 地形版本
	ImageProvider   uint8           `json:"image_provider,omitempty"`   // 影像提供商
	TerrainProvider uint8           `json:"terrain_provider,omitempty"` // 地形提供商
	Channels        []Q2ChannelJSON `json:"channels,omitempty"`         // 通道列表
}

// Q2ChannelJSON 通道JSON表示
type Q2ChannelJSON struct {
	Type    uint16 `json:"type"`    // 通道类型
	Version uint16 `json:"version"` // 通道版本
}

// Q2References 数据引用的内部结构（不输出到JSON）
type Q2References struct {
	ImageryRefs []Q2DataRefJSON // 影像引用列表
	TerrainRefs []Q2DataRefJSON // 地形引用列表
	VectorRefs  []Q2DataRefJSON // 矢量引用列表
	Q2ChildRefs []Q2DataRefJSON // Q2子节点引用
}

// Q2DataRefJSON 数据引用的JSON表示
type Q2DataRefJSON struct {
	Tilekey  string `json:"tilekey"`            // 四叉树路径（瓦片键）
	Version  uint16 `json:"version"`            // 版本号
	Channel  uint16 `json:"channel,omitempty"`  // 通道号
	Provider uint16 `json:"provider,omitempty"` // 提供商
	URL      string `json:"url,omitempty"`      // 构造的请求URL
}

// ParseQ2Body 解析Q2数据包的body,返回JSON字符串
// body: HTTP响应解密后的二进制数据
// tilekey: 当前瓦片的路径（如"0", "0123"等）
// rootNode: 是否为根节点(tilekey长度<4)
// Returns: JSON字符串和错误
func ParseQ2Body(body []byte, tilekey string, rootNode bool) (string, error) {
	response := &Q2Response{
		Success:     false,
		Tilekey:     tilekey,
		ImageryList: []Q2DataRefJSON{},
		TerrainList: []Q2DataRefJSON{},
		VectorList:  []Q2DataRefJSON{},
		Q2List:      []Q2DataRefJSON{},
	}

	// Q2数据使用二进制格式（QuadTreePacket16）
	qtp16 := NewQuadTreePacket16()
	err := qtp16.Decode(body)
	if err != nil {
		response.Error = fmt.Sprintf("Failed to parse Q2 binary format: %v", err)
		return marshalResponse(response)
	}

	// 填充基本信息
	response.MagicID = fmt.Sprintf("0x%X", qtp16.MagicID)
	response.DataTypeID = qtp16.DataTypeID
	response.Version = qtp16.Version
	response.NodeCount = len(qtp16.DataInstances)
	response.Success = true

	// 获取编号系统
	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}

	// 转换所有节点
	nodeIndex := 0
	path := QuadtreePath{} // 从根路径开始
	response.Nodes = []Q2NodeJSON{}
	convertBinaryNodesToJSON(qtp16, numbering, &nodeIndex, path, &response.Nodes)

	// 提取数据引用
	pathPrefix := NewQuadtreePathFromString(tilekey)
	references := &QuadtreeDataReferenceGroup{}
	qtp16.GetDataReferences(references, pathPrefix, rootNode)

	// 转换数据引用为JSON
	refs := convertReferencesToJSON(references)
	response.DataReferences = refs

	// 填充输出列表
	response.ImageryList = refs.ImageryRefs
	response.TerrainList = refs.TerrainRefs
	response.VectorList = refs.VectorRefs
	response.Q2List = refs.Q2ChildRefs

	return marshalResponse(response)
}

// marshalResponse 将响应转换为JSON字符串
func marshalResponse(response *Q2Response) (string, error) {
	jsonBytes, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return string(jsonBytes), nil
}

// convertBinaryNodesToJSON 递归转换二进制节点为JSON
func convertBinaryNodesToJSON(
	qtp *QuadTreePacket16,
	numbering *QuadtreeNumbering,
	nodeIndex *int,
	path QuadtreePath,
	nodes *[]Q2NodeJSON,
) {
	if *nodeIndex >= len(qtp.DataInstances) {
		return
	}

	if int(path.Level()) >= numbering.Depth() {
		return
	}

	quantum := qtp.DataInstances[*nodeIndex]
	if quantum == nil {
		return
	}

	// 创建节点JSON
	nodeJSON := Q2NodeJSON{
		Path:     path.AsString(),
		Channels: []Q2ChannelJSON{},
	}

	// 计算subindex
	subindex := numbering.InorderToSubindex(numbering.TraversalPathToInorder(path))
	nodeJSON.Index = *nodeIndex
	nodeJSON.Subindex = subindex

	// 解析子节点
	childFlags := quantum.Children & 0x0F
	for i := 0; i < 4; i++ {
		if (childFlags & (1 << uint(i))) != 0 {
			nodeJSON.Children = append(nodeJSON.Children, i)
			nodeJSON.ChildCount++
		}
	}

	// 标志位
	nodeJSON.HasCache = quantum.GetCacheNodeBit()
	nodeJSON.HasImage = quantum.GetImageBit()
	nodeJSON.HasTerrain = quantum.GetTerrainBit()
	nodeJSON.HasVector = quantum.GetDrawableBit()

	// 版本信息
	if nodeJSON.HasCache {
		nodeJSON.CNodeVersion = quantum.CNodeVersion
	}
	if nodeJSON.HasImage {
		nodeJSON.ImageVersion = quantum.ImageVersion
		nodeJSON.ImageProvider = quantum.ImageDataProvider
	}
	if nodeJSON.HasTerrain {
		nodeJSON.TerrainVersion = quantum.TerrainVersion
		nodeJSON.TerrainProvider = quantum.TerrainDataProvider
	}

	// 通道(矢量数据)
	for i := 0; i < len(quantum.ChannelType); i++ {
		channel := Q2ChannelJSON{
			Type:    quantum.ChannelType[i],
			Version: quantum.ChannelVersion[i],
		}
		nodeJSON.Channels = append(nodeJSON.Channels, channel)
	}

	*nodes = append(*nodes, nodeJSON)

	// 递归处理子节点
	for i := 0; i < 4; i++ {
		if quantum.GetBit(i) {
			newPath := path.Child(uint32(i))
			*nodeIndex++
			convertBinaryNodesToJSON(qtp, numbering, nodeIndex, newPath, nodes)
		}
	}
}

// convertReferencesToJSON 将数据引用转换为JSON格式
func convertReferencesToJSON(refs *QuadtreeDataReferenceGroup) *Q2References {
	result := &Q2References{
		ImageryRefs: []Q2DataRefJSON{},
		TerrainRefs: []Q2DataRefJSON{},
		VectorRefs:  []Q2DataRefJSON{},
		Q2ChildRefs: []Q2DataRefJSON{},
	}

	// 构造策略对象
	builder := PathOnlyURLBuilder{Base: ""}
	filter := DefaultQ2Filter{}

	// 转换影像引用
	for _, ref := range refs.ImgRefs {
		tilekey := ref.QtPath.AsString()
		if !filter.IncludeImagery(tilekey, ref.Version, ref.Provider) {
			continue
		}
		url := builder.Imagery(tilekey, ref.Version)
		result.ImageryRefs = append(result.ImageryRefs, Q2DataRefJSON{
			Tilekey:  tilekey,
			Version:  ref.Version,
			Provider: ref.Provider,
			URL:      url,
		})
	}

	// 转换地形引用
	for _, ref := range refs.TerRefs {
		tilekey := ref.QtPath.AsString()
		if !filter.IncludeTerrain(tilekey, ref.Version, ref.Provider) {
			continue
		}
		url := builder.Terrain(tilekey, ref.Version)
		result.TerrainRefs = append(result.TerrainRefs, Q2DataRefJSON{
			Tilekey:  tilekey,
			Version:  ref.Version,
			Provider: ref.Provider,
			URL:      url,
		})
	}

	// 转换矢量引用
	for _, ref := range refs.VecRefs {
		tilekey := ref.QtPath.AsString()
		result.VectorRefs = append(result.VectorRefs, Q2DataRefJSON{
			Tilekey: tilekey,
			Version: ref.Version,
			Channel: ref.Channel,
		})
	}

	// 转换QTP引用（Q2子节点）
	for _, ref := range refs.QtpRefs {
		tilekey := ref.QtPath.AsString()
		if !filter.IncludeQ2(tilekey, ref.Version) {
			continue
		}
		url := builder.Q2(tilekey, ref.Version)
		result.Q2ChildRefs = append(result.Q2ChildRefs, Q2DataRefJSON{
			Tilekey: tilekey,
			Version: ref.Version,
			URL:     url,
		})
	}

	return result
}

// convertReferencesToJSONWithStrategy 使用自定义策略构造引用
func convertReferencesToJSONWithStrategy(refs *QuadtreeDataReferenceGroup, builder Q2URLBuilder, filter Q2Filter) *Q2References {
	result := &Q2References{
		ImageryRefs: []Q2DataRefJSON{},
		TerrainRefs: []Q2DataRefJSON{},
		VectorRefs:  []Q2DataRefJSON{},
		Q2ChildRefs: []Q2DataRefJSON{},
	}

	// 影像
	for _, ref := range refs.ImgRefs {
		tk := ref.QtPath.AsString()
		if !filter.IncludeImagery(tk, ref.Version, ref.Provider) {
			continue
		}
		url := builder.Imagery(tk, ref.Version)
		result.ImageryRefs = append(result.ImageryRefs, Q2DataRefJSON{Tilekey: tk, Version: ref.Version, Provider: ref.Provider, URL: url})
	}
	// 地形
	for _, ref := range refs.TerRefs {
		tk := ref.QtPath.AsString()
		if !filter.IncludeTerrain(tk, ref.Version, ref.Provider) {
			continue
		}
		url := builder.Terrain(tk, ref.Version)
		result.TerrainRefs = append(result.TerrainRefs, Q2DataRefJSON{Tilekey: tk, Version: ref.Version, Provider: ref.Provider, URL: url})
	}
	// 矢量
	for _, ref := range refs.VecRefs {
		tk := ref.QtPath.AsString()
		result.VectorRefs = append(result.VectorRefs, Q2DataRefJSON{Tilekey: tk, Version: ref.Version, Channel: ref.Channel})
	}
	// Q2子节点
	for _, ref := range refs.QtpRefs {
		tk := ref.QtPath.AsString()
		if !filter.IncludeQ2(tk, ref.Version) {
			continue
		}
		url := builder.Q2(tk, ref.Version)
		result.Q2ChildRefs = append(result.Q2ChildRefs, Q2DataRefJSON{Tilekey: tk, Version: ref.Version, URL: url})
	}
	return result
}

// Q2ParseOptions 解析选项：控制输出类型
type Q2ParseOptions struct {
	IncludeImagery bool
	IncludeTerrain bool
	IncludeVector  bool
	IncludeQ2      bool
}

// OptionsFilter 根据选项过滤
type OptionsFilter struct{ opts Q2ParseOptions }

func (f OptionsFilter) IncludeImagery(tilekey string, version uint16, provider uint16) bool {
	return f.opts.IncludeImagery
}
func (f OptionsFilter) IncludeTerrain(tilekey string, version uint16, provider uint16) bool {
	return f.opts.IncludeTerrain
}
func (f OptionsFilter) IncludeQ2(tilekey string, version uint16) bool {
	if !f.opts.IncludeQ2 {
		return false
	}
	// 保持既有行为：仅保留长度为4的倍数
	return len(tilekey)%4 == 0
}

// ParseQ2BodyWithOptions 解析并根据选项控制输出类型
func ParseQ2BodyWithOptions(body []byte, tilekey string, rootNode bool, opts Q2ParseOptions) (string, error) {
	response := &Q2Response{
		Success:     false,
		Tilekey:     tilekey,
		ImageryList: []Q2DataRefJSON{},
		TerrainList: []Q2DataRefJSON{},
		VectorList:  []Q2DataRefJSON{},
		Q2List:      []Q2DataRefJSON{},
	}

	qtp16 := NewQuadTreePacket16()
	if err := qtp16.Decode(body); err != nil {
		response.Error = fmt.Sprintf("Failed to parse Q2 binary format: %v", err)
		return marshalResponse(response)
	}

	response.MagicID = fmt.Sprintf("0x%X", qtp16.MagicID)
	response.DataTypeID = qtp16.DataTypeID
	response.Version = qtp16.Version
	response.NodeCount = len(qtp16.DataInstances)
	response.Success = true

	// 节点（内部保留）
	numbering := GetNumbering(0)
	if !rootNode {
		numbering = GetNumbering(1)
	}
	nodeIndex := 0
	path := QuadtreePath{}
	convertBinaryNodesToJSON(qtp16, numbering, &nodeIndex, path, &response.Nodes)

	// 引用 + 选项策略
	pathPrefix := NewQuadtreePathFromString(tilekey)
	references := &QuadtreeDataReferenceGroup{}
	qtp16.GetDataReferences(references, pathPrefix, rootNode)
	builder := PathOnlyURLBuilder{Base: ""}
	filter := OptionsFilter{opts: opts}
	refs := convertReferencesToJSONWithStrategy(references, builder, filter)
	response.DataReferences = refs // 内部保留

	// 输出列表按选项控制
	if opts.IncludeImagery {
		response.ImageryList = refs.ImageryRefs
	}
	if opts.IncludeTerrain {
		response.TerrainList = refs.TerrainRefs
	}
	if opts.IncludeVector {
		response.VectorList = refs.VectorRefs
	}
	if opts.IncludeQ2 {
		response.Q2List = refs.Q2ChildRefs
	}

	return marshalResponse(response)
}
