package GoogleEarth

// 注意：libge 目录仅作参考，不参与运行。此包为纯 Go 的解析与处理库。

import (
	"encoding/binary"
	"errors"
)

var DBRootVersion uint16

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
