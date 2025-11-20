# gecrypt.go - 加密解密模块

## 文件概述

实现 Google Earth 数据的加解密功能，包括 XOR 异或解密和 ZLIB 解压缩。这是从 C++ libge 库迁移而来的核心加密算法。

## 主要功能点

### 1. 核心解密算法 - `decryptXOR`

```go
func decryptXOR(keys []byte, data []byte, offset, length int)
```

**功能**：使用 1024 字节密钥对数据进行 XOR 异或解密

**参数**：
- `keys`: 解密密钥（1024字节）
- `data`: 要解密的数据（原地修改）
- `offset`: 数据起始偏移
- `length`: 要解密的长度

**算法特点**：
```go
keyLen := 1016
j := 16  // 从第16字节开始

for i := 0; i < length; i++ {
    data[offset+i] ^= keys[j+8]  // XOR 解密
    j++
    
    // 密钥索引跳转逻辑（核心算法）
    if j%8 == 0 {
        j += 16  // 每8个字节跳过16字节密钥区域
    }
    if j >= keyLen {
        j = (j + 8) % 24  // 回绕处理
    }
}
```

**关键点**：
- 密钥索引不是简单递增，而是有特殊的跳转规律
- 每处理8个字节，跳过16个字节的密钥区域
- 达到1016时执行特殊回绕处理

### 2. 简化解密函数 - `geDecrypt`

```go
func geDecrypt(data []byte, key []byte)
```

**功能**：对整个数据进行解密（从索引0开始）

**使用示例**：
```go
encryptedData := []byte{...}
geDecrypt(encryptedData, CryptKey)
// encryptedData 现在已解密
```

### 3. ZLIB 解包函数 - `UnpackGEZlib`

```go
func UnpackGEZlib(src []byte) ([]byte, error)
```

**功能**：解包 Google Earth 的 ZLIB 压缩数据，支持自动解密

**处理流程**：

```
1. 检查前4字节的魔法数
   ├─ 0x32789755 (CRYPTED_ZLIB_MAGIC)
   │  └─ 解密数据 → 重新检查魔法数
   │
   └─ 0x7468DEAD (DECRYPTED_ZLIB_MAGIC)
      └─ 解压 payload
```

**数据格式**：
```
[4 字节魔法数][4 字节未压缩大小][zlib 压缩数据]
```

**使用示例**：
```go
compressedData := []byte{...}
unpackedData, err := UnpackGEZlib(compressedData)
if err != nil {
    log.Fatal(err)
}
// unpackedData 是解压后的数据
```

**错误处理**：
- 数据太短（< 4字节）
- ZLIB payload 太短（< 8字节）
- ZLIB 解压失败

### 4. 默认解密密钥 - `CryptKey`

```go
var CryptKey = []byte{ /* 1024字节固定密钥 */ }
```

**特点**：
- 长度：1024 字节
- 前8字节全为 0x00
- 后1016字节是固定的密钥序列
- 可通过 `UpdateCryptKeyFromDBRoot` 更新

**用途**：
- 用作默认解密密钥
- 当 dbRoot 未加载时使用
- dbRoot 加载后会被更新为服务器下发的密钥

## 使用场景

### 场景1：解密四叉树数据包

```go
// 从服务器获取加密的四叉树数据
encryptedPacket := fetchQuadtreePacket(url)

// 检查并解包
unpackedData, err := UnpackGEZlib(encryptedPacket)
if err != nil {
    return err
}

// 现在可以解析 unpackedData
```

### 场景2：解密影像数据

```go
// 获取加密的JPEG影像
encryptedImage := fetchImagery(url)

// 检查魔法数
magic := binary.LittleEndian.Uint32(encryptedImage[:4])
if magic == CRYPTED_JPEG_MAGIC {
    geDecrypt(encryptedImage, CryptKey)
}

// 现在 encryptedImage 是解密后的 JPEG
```

### 场景3：使用 dbRoot 密钥

```go
// 1. 获取 dbRoot 并更新密钥
dbRootData := fetchDBRoot()
version, err := UpdateCryptKeyFromDBRoot(dbRootData)

// 2. 使用更新后的密钥解密数据
encryptedData := fetchData()
geDecrypt(encryptedData, CryptKey)
```

## 算法验证

这些加密算法已经过验证，能够正确处理 Google Earth 的数据：

1. **XOR 解密算法**：与 C++ libge 实现完全一致
2. **密钥跳转逻辑**：经过测试，正确处理各种边界情况
3. **ZLIB 解压**：支持标准 ZLIB 格式，兼容 Google Earth 数据

## 性能考虑

1. **原地解密**：`geDecrypt` 直接修改输入数据，避免额外内存分配
2. **延迟解压**：`UnpackGEZlib` 仅在检测到压缩数据时才解压
3. **密钥缓存**：`CryptKey` 是全局变量，避免重复加载

## 注意事项

1. **密钥长度**：密钥必须是 1024 字节，否则会导致解密失败
2. **数据完整性**：解密后应验证数据格式（如检查 protobuf 是否可解析）
3. **魔法数检查**：始终先检查魔法数，避免对非加密数据进行解密
4. **内存安全**：`geDecrypt` 会原地修改数据，调用前需要复制数据

## 依赖关系

**被以下模块使用**：
- `gedbroot.go`: 更新解密密钥
- `quadtree_packet.go`: 解密四叉树数据包
- `terrain.go`: 解密地形数据

**依赖的包**：
- `encoding/binary`: 读取魔法数和数据结构
- `compress/zlib`: ZLIB 解压
- `bytes`: 字节流处理

## 测试建议

```go
func TestDecryption(t *testing.T) {
    // 测试数据（实际应使用真实的加密数据）
    encrypted := []byte{...}
    expected := []byte{...}
    
    geDecrypt(encrypted, CryptKey)
    
    if !bytes.Equal(encrypted, expected) {
        t.Error("Decryption failed")
    }
}

func TestUnpackGEZlib(t *testing.T) {
    // 测试未压缩数据
    raw := []byte("test data")
    result, err := UnpackGEZlib(raw)
    if err != nil {
        t.Fatal(err)
    }
    
    // 测试 ZLIB 压缩数据
    compressed := compressZlib(raw)
    result, err = UnpackGEZlib(compressed)
    if err != nil || !bytes.Equal(result, raw) {
        t.Error("Unpack failed")
    }
}
```
