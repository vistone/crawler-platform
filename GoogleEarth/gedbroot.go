package GoogleEarth

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	pb "crawler-platform/GoogleEarth/pb"

	"google.golang.org/protobuf/proto"
)

var DBRootVersion uint16

// DbRootParser DbRoot 解析器接口
type DbRootParser interface {
	// Parse 解析 dbRoot.v5 原始数据
	// 输入：完整的 dbRoot 响应字节
	// 输出：解析结果或错误
	Parse(body []byte) (*DbRootData, error)

	// GetVersion 获取版本号
	GetVersion() uint16

	// GetCryptKey 获取解密密钥
	GetCryptKey() []byte

	// GetXMLData 获取 XML 数据
	GetXMLData() []byte
}

// DbRootData DbRoot 解析结果
type DbRootData struct {
	Version   uint16         // 版本号（已经过 XOR 0x4200 处理）
	CryptKey  []byte         // 解密密钥（1024 字节）
	XMLData   []byte         // 解密并解压缩后的 XML 数据
	Providers map[int]string // Provider ID -> Copyright 信息映射
}

// UpdateCryptKeyFromDBRoot 解析 dbRoot.v5 原始字节并更新全局 CryptKey 与版本号
// 输入：完整的 dbRoot 响应字节
// 输出：解析后的版本号（ver ^ 0x4200）
func UpdateCryptKeyFromDBRoot(body []byte) (uint16, error) {
	if len(body) <= 1024 {
		return 0, errors.New("dbroot response too short")
	}
	magic := binary.LittleEndian.Uint32(body[:4])
	_ = magic // 暂不使用
	unk := binary.LittleEndian.Uint16(body[4:6])
	_ = unk // 暂不使用
	ver := binary.LittleEndian.Uint16(body[6:8])
	// 拼装 CryptKey：前8字节为0，随后1016字节取自响应
	if len(CryptKey) < 1024 {
		CryptKey = make([]byte, 1024)
	}
	for i := 0; i < 8; i++ {
		CryptKey[i] = 0
	}
	copy(CryptKey[8:], body[8:8+1016])
	DBRootVersion = ver ^ 0x4200
	return DBRootVersion, nil
}

// ParseDbRoot 解析完整的 DbRoot protobuf 数据
// 输入：完整的 dbRoot 响应字节（已经过 UpdateCryptKeyFromDBRoot 处理）
// 输出：解析后的 DbRootProto 对象
func ParseDbRoot(body []byte) (*pb.DbRootProto, error) {
	if len(body) <= 1024 {
		return nil, errors.New("dbroot response too short")
	}

	// dbRoot 数据从第 1024 字节开始（前 1024 字节是密钥和头部信息）
	protoData := body[1024:]

	if len(protoData) == 0 {
		return nil, errors.New("no protobuf data found after header")
	}

	// 解析 protobuf 数据
	dbRoot := &pb.DbRootProto{}
	if err := proto.Unmarshal(protoData, dbRoot); err != nil {
		return nil, fmt.Errorf("failed to parse DbRootProto: %w", err)
	}

	return dbRoot, nil
}

// ParseEncryptedDbRoot 解析加密的 DbRoot 数据
// 如果数据使用 EncryptedDbRootProto 包装，需要先解密
func ParseEncryptedDbRoot(body []byte) (*pb.DbRootProto, error) {
	if len(body) <= 1024 {
		return nil, errors.New("dbroot response too short")
	}

	// 尝试解析为 EncryptedDbRootProto
	encrypted := &pb.EncryptedDbRootProto{}
	protoData := body[1024:]

	if err := proto.Unmarshal(protoData, encrypted); err != nil {
		// 如果解析失败，可能不是加密的，直接尝试解析为 DbRootProto
		return ParseDbRoot(body)
	}

	// 检查加密类型
	if encrypted.EncryptionType != nil && *encrypted.EncryptionType == pb.EncryptedDbRootProto_ENCRYPTION_XOR {
		// 使用 XOR 解密
		if encrypted.DbrootData == nil {
			return nil, errors.New("encrypted dbroot data is nil")
		}

		// 解密数据
		decryptedData := make([]byte, len(encrypted.DbrootData))
		copy(decryptedData, encrypted.DbrootData)
		geDecrypt(decryptedData, CryptKey)

		// 解析解密后的数据
		dbRoot := &pb.DbRootProto{}
		if err := proto.Unmarshal(decryptedData, dbRoot); err != nil {
			return nil, fmt.Errorf("failed to parse decrypted DbRootProto: %w", err)
		}
		return dbRoot, nil
	}

	// 如果没有加密，直接解析 dbroot_data
	if encrypted.DbrootData != nil {
		dbRoot := &pb.DbRootProto{}
		if err := proto.Unmarshal(encrypted.DbrootData, dbRoot); err != nil {
			return nil, fmt.Errorf("failed to parse DbRootProto from encrypted wrapper: %w", err)
		}
		return dbRoot, nil
	}

	return nil, errors.New("no valid dbroot data found")
}

// ParseDbRootComplete 完整解析 dbRoot.v5 数据
// 输入：完整的 dbRoot 响应字节
// 输出：版本号、CryptKey（1024字节）、XML数据
func ParseDbRootComplete(body []byte) (*DbRootData, error) {
	if len(body) <= 1024 {
		return nil, errors.New("dbroot response too short")
	}

	// 1. 解析头部信息
	magic := binary.LittleEndian.Uint32(body[:4])
	_ = magic // 保留供后续使用
	unk := binary.LittleEndian.Uint16(body[4:6])
	_ = unk // 保留供后续使用
	rawVersion := binary.LittleEndian.Uint16(body[6:8])

	// 2. 提取并保存 CryptKey（前8字节置0，随后1016字节来自响应）
	cryptKey := make([]byte, 1024)
	for i := 0; i < 8; i++ {
		cryptKey[i] = 0
	}
	copy(cryptKey[8:], body[8:8+1016])

	// 3. 计算版本号
	version := rawVersion ^ 0x4200

	// 4. 提取并解密加密数据
	encryptedData := body[1024:]
	if len(encryptedData) == 0 {
		return &DbRootData{
			Version:  version,
			CryptKey: cryptKey,
			XMLData:  nil,
		}, nil
	}

	// 解密数据
	decryptedData := make([]byte, len(encryptedData))
	copy(decryptedData, encryptedData)
	GeDecrypt(decryptedData, cryptKey)

	// 5. 查找并解压缩 zlib 数据
	var xmlData []byte
	for i := 0; i < len(decryptedData)-1; i++ {
		// 查找 zlib 魔法数 (0x78 0x9C 或 0x78 0x01 或 0x78 0xDA)
		if decryptedData[i] == 0x78 &&
			(decryptedData[i+1] == 0x9C || decryptedData[i+1] == 0x01 || decryptedData[i+1] == 0xDA) {
			// 找到 zlib 数据起始位置，开始解压缩
			zr, err := zlib.NewReader(bytes.NewReader(decryptedData[i:]))
			if err != nil {
				return nil, fmt.Errorf("failed to create zlib reader: %w", err)
			}
			defer zr.Close()

			var xmlBuffer bytes.Buffer
			if _, err := io.Copy(&xmlBuffer, zr); err != nil {
				return nil, fmt.Errorf("failed to decompress zlib data: %w", err)
			}

			xmlData = xmlBuffer.Bytes()
			break
		}
	}

	// 如果没有找到 zlib 数据，返回解密后的原始数据
	if xmlData == nil {
		xmlData = decryptedData
	}

	// 6. 解析 ProviderInfo 到 map
	providers := ParseProviderInfo(xmlData)

	return &DbRootData{
		Version:   version,
		CryptKey:  cryptKey,
		XMLData:   xmlData,
		Providers: providers,
	}, nil
}

// ParseDbRootToXML 解析 dbRoot.v5 数据并返回 XML 内容
// 输入：完整的 dbRoot 响应字节
// 输出：解密并解压缩后的 XML 数据
func ParseDbRootToXML(body []byte) ([]byte, error) {
	if len(body) <= 1024 {
		return nil, errors.New("dbroot response too short")
	}

	// 1. 提取加密数据（跳过前 1024 字节的头部和密钥）
	encryptedData := body[1024:]
	if len(encryptedData) == 0 {
		return nil, errors.New("no encrypted data found")
	}

	// 2. 解密数据
	decryptedData := make([]byte, len(encryptedData))
	copy(decryptedData, encryptedData)
	GeDecrypt(decryptedData, CryptKey)

	// 3. 查找并解压缩 zlib 数据
	// dbRoot 数据格式：[Header] [zlib compressed XML]
	// Header 通常以 0xAD 0xDE 开头，zlib 数据以 0x78 开头
	for i := 0; i < len(decryptedData)-1; i++ {
		// 查找 zlib 魔法数 (0x78 0x9C 或 0x78 0x01 或 0x78 0xDA)
		if decryptedData[i] == 0x78 &&
			(decryptedData[i+1] == 0x9C || decryptedData[i+1] == 0x01 || decryptedData[i+1] == 0xDA) {
			// 找到 zlib 数据起始位置，开始解压缩
			zr, err := zlib.NewReader(bytes.NewReader(decryptedData[i:]))
			if err != nil {
				return nil, fmt.Errorf("failed to create zlib reader: %w", err)
			}
			defer zr.Close()

			var xmlData bytes.Buffer
			if _, err := io.Copy(&xmlData, zr); err != nil {
				return nil, fmt.Errorf("failed to decompress zlib data: %w", err)
			}

			return xmlData.Bytes(), nil
		}
	}

	// 如果没有找到 zlib 数据，返回解密后的原始数据
	return decryptedData, nil
}

// SaveDbRootAsXML 解析 dbRoot.v5 数据并保存为 XML 文件
// 输入：完整的 dbRoot 响应字节，输出文件路径
// 返回：错误信息
func SaveDbRootAsXML(body []byte, xmlFilePath string) error {
	// 解析 XML 数据
	xmlData, err := ParseDbRootToXML(body)
	if err != nil {
		return fmt.Errorf("failed to parse dbRoot to XML: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(xmlFilePath, xmlData, 0644); err != nil {
		return fmt.Errorf("failed to write XML file: %w", err)
	}

	return nil
}

// DefaultDbRootParser 默认的 DbRoot 解析器实现
type DefaultDbRootParser struct {
	data *DbRootData
}

// NewDbRootParser 创建默认的 DbRoot 解析器
func NewDbRootParser() DbRootParser {
	return &DefaultDbRootParser{}
}

// Parse 实现 DbRootParser 接口
func (p *DefaultDbRootParser) Parse(body []byte) (*DbRootData, error) {
	data, err := ParseDbRootComplete(body)
	if err != nil {
		return nil, err
	}
	p.data = data
	return data, nil
}

// GetVersion 实现 DbRootParser 接口
func (p *DefaultDbRootParser) GetVersion() uint16 {
	if p.data == nil {
		return 0
	}
	return p.data.Version
}

// GetCryptKey 实现 DbRootParser 接口
func (p *DefaultDbRootParser) GetCryptKey() []byte {
	if p.data == nil {
		return nil
	}
	return p.data.CryptKey
}

// GetXMLData 实现 DbRootParser 接口
func (p *DefaultDbRootParser) GetXMLData() []byte {
	if p.data == nil {
		return nil
	}
	return p.data.XMLData
}

// ParseProviderInfo 从 XML 数据中解析 ProviderInfo 信息
// 返回 map[providerID]copyright
func ParseProviderInfo(xmlData []byte) map[int]string {
	providers := make(map[int]string)
	if len(xmlData) == 0 {
		return providers
	}

	// 将 XML 数据转换为字符串便于处理
	xmlStr := string(xmlData)

	// 查找所有 <etProviderInfo> 条目
	// 格式: <etProviderInfo> [a]{id "copyright"}
	// 或: <etProviderInfo> [p]{id "copyright"}
	lines := bytes.Split(xmlData, []byte("\n"))
	for _, line := range lines {
		// 检查是否是 ProviderInfo 行
		if !bytes.Contains(line, []byte("<etProviderInfo>")) {
			continue
		}

		// 提取 ID 和 copyright
		// 格式示例: <etProviderInfo> [a]{394 "Image SCRD"}
		var id int
		var copyright string

		// 查找 {id "copyright"} 模式
		startIdx := bytes.IndexByte(line, '{')
		endIdx := bytes.IndexByte(line, '}')
		if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
			continue
		}

		content := string(line[startIdx+1 : endIdx])

		// 解析 ID 和 copyright
		// 使用 fmt.Sscanf 解析格式 "id \"copyright\""
		n, err := fmt.Sscanf(content, "%d %q", &id, &copyright)
		if err != nil || n != 2 {
			// 尝试不带引号的格式
			continue
		}

		providers[id] = copyright
	}

	// 输出调试信息（可选）
	_ = xmlStr // 避免未使用变量警告

	return providers
}
