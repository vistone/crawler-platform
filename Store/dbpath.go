package Store

import (
	"os"
	"path"
	"strconv"
)

// getDBPath 根据数据类型和瓦片键生成数据库文件路径
// 参数:
//
//	dbdir: 数据库根目录
//	dataType: 数据类型(如imagery、terrain、vector)
//	tilekey: 四叉树瓦片键(用于确定存储层级)
//	storageType: 存储类型("sqlite" 或 "bbolt")
//
// 返回:
//
//	string: 完整的数据库文件路径
func getDBPath(dbdir string, dataType string, tilekey string, storageType string) string {
	// 根据存储类型确定子目录
	var subDir string
	switch storageType {
	case "sqlite":
		subDir = "sqlite"
	case "bbolt":
		subDir = "bbolt"
	default:
		subDir = storageType // fallback to storage type name
	}
	
	// 截取瓦片键的前4个字符作为文件名前缀
	// 若长度不足4则全取，确保不越界
	prefixLen := len(tilekey)
	if prefixLen > 4 {
		prefixLen = 4
	}
	FileName := tilekey[0:prefixLen] + ".g3db"

	// 根据瓦片键长度判断存储层级
	// 0-8层基础层数据量小,统一管理; 9-12层8目录, 13-16层12目录
	// 17+层数据量巨大,每层独立目录管理
	keyLen := len(tilekey)

	// 长度≤8: 存储在base.g3db基础文件中(第0-8层,数据量小,统一管理)
	if keyLen <= 8 {
		// 拼接数据库根目录、存储类型子目录和数据类型形成路径
		pathDir := path.Join(dbdir, subDir, dataType)
		// 若目录不存在则创建
		_ = os.MkdirAll(pathDir, 0o755)
		// 返回基础数据库文件路径
		return pathDir + "/base.g3db"
		// 长度9-12: 存储在8级目录(8层管理这4层数据: 9,10,11,12层)
	} else if keyLen <= 12 {
		// 拼接路径: 根目录/存储类型子目录/数据类型/8级目录
		pathDir := path.Join(dbdir, subDir, dataType, "8")
		// 若目录不存在则创建
		_ = os.MkdirAll(pathDir, 0o755)
		// 返回8级目录下的数据库文件路径
		return pathDir + "/" + FileName
		// 长度13-16: 存储在12级目录(12层管理这4层数据: 13,14,15,16层)
	} else if keyLen <= 16 {
		// 拼接路径: 根目录/存储类型子目录/数据类型/12级目录
		pathDir := path.Join(dbdir, subDir, dataType, "12")
		// 若目录不存在则创建
		_ = os.MkdirAll(pathDir, 0o755)
		// 返回12级目录下的数据库文件路径
		return pathDir + "/" + FileName
		// 长度≥17: 数据量巨大,每层独立目录管理(动态按长度生成目录名)
	} else {
		// 将tilekey长度转为字符串作为目录名
		// 例如: 长度17->"17"目录, 长度18->"18"目录, 以此类推
		// 这样可以最大程度分散高层级数据,避免单个数据库文件过大
		dirName := strconv.Itoa(keyLen)
		// 拼接路径: 根目录/存储类型子目录/数据类型/长度目录
		pathDir := path.Join(dbdir, subDir, dataType, dirName)
		// 若目录不存在则创建
		_ = os.MkdirAll(pathDir, 0o755)
		// 返回对应长度目录下的数据库文件路径
		return pathDir + "/" + FileName
	}
}

// GetDBPathForTest 暴露 getDBPath 供测试使用
func GetDBPathForTest(dbdir string, dataType string, tilekey string, storageType string) string {
	return getDBPath(dbdir, dataType, tilekey, storageType)
}

// GetDBPath 暴露 getDBPath 供外部使用
func GetDBPath(dbdir string, dataType string, tilekey string, storageType string) string {
	return getDBPath(dbdir, dataType, tilekey, storageType)
}