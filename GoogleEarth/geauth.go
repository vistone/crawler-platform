package GoogleEarth

import (
	"bytes"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"time"

	"crawler-platform/utlsclient"
)

// Auth Google Earth 认证管理器
type Auth struct {
	pool    *utlsclient.UTLSHotConnPool
	Session string // 认证会话 ID（从 Cookie 中提取的 sessionid）
}

// NewAuth 创建新的认证管理器实例
func NewAuth(pool *utlsclient.UTLSHotConnPool) *Auth {
	return &Auth{
		pool: pool,
	}
}

// GetAuth 获取 Google Earth 认证 Session
// 功能：
// 1. 建立热连接到 kh.google.com
// 2. 发送 POST 请求到 /geauth
// 3. 从响应 Cookie 中提取 sessionid
// 4. 将 sessionid 保存到热连接中，后续请求自动携带
func (a *Auth) GetAuth() (string, error) {
	// 确保 pool 已初始化
	if a.pool == nil {
		return "", fmt.Errorf("pool 未初始化，请使用 NewAuth(pool) 创建")
	}

	// 1. 从连接池获取热连接
	conn, err := a.pool.GetConnection(HOST_NAME)
	if err != nil {
		return "", fmt.Errorf("获取热连接失败: %w", err)
	}
	defer a.pool.PutConnection(conn)

	// 2. 使用连接的 IP 地址构建 URL（而不是域名）
	url := "https://" + HOST_NAME + "/geauth"

	// 生成随机认证密钥（49字节）
	authKey := generateRandomAuthKey()

	// 2. 创建 HTTP 客户端
	client := utlsclient.NewUTLSClient(conn)
	client.SetTimeout(30 * time.Second)

	// 3. 创建 POST 请求
	req, err := http.NewRequest("POST", url, bytes.NewReader(authKey))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	// 4. 设置请求头
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(authKey)))
	// req.Header.Set("User-Agent", RandomUserAgent()) // 使用随机 User-Agent
	req.Header.Set("Host", HOST_NAME) // 设置 Host header 为域名（因为使用 IP 访问）

	// 5. 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("认证请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 6. 检查响应状态
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("认证失败，状态码: %d", resp.StatusCode)
	}

	// 7. 读取响应 body
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应 body 失败: %w", err)
	}

	// 8. 从响应 body 中解析 sessionid
	session, err := parseSessionFromResponse(responseBody)
	if err != nil {
		return "", fmt.Errorf("从 body 解析 sessionid 失败: %w", err)
	}

	// 9. 保存 session
	a.Session = session
	return session, nil
}

// ClearAuth 清除当前的认证会话信息
func (a *Auth) ClearAuth() {
	a.Session = ""
}

// GetSession 获取当前的 session（如果已认证）
func (a *Auth) GetSession() string {
	return a.Session
}

// parseSessionFromResponse 从响应 body 中解析 sessionid
// 响应格式：前 8 字节为头部，之后是以 NULL 结尾的 sessionid 字符串
// 这个 sessionid 会被保存到热连接中，后续所有请求都会自动携带
func parseSessionFromResponse(responseBody []byte) (string, error) {
	if len(responseBody) <= 8 {
		return "", fmt.Errorf("响应 body 长度不足，实际长度为 %d 字节", len(responseBody))
	}

	// 从第 8 字节开始提取 sessionid（直到遇到 NULL 字节）
	var sessionBytes []byte
	for i := 8; i < len(responseBody); i++ {
		if responseBody[i] == 0 {
			break
		}
		sessionBytes = append(sessionBytes, responseBody[i])
	}

	if len(sessionBytes) == 0 {
		return "", fmt.Errorf("未找到有效的 sessionid 数据")
	}

	return string(sessionBytes), nil
}

// 预定义的 Google Earth 认证密钥
var (
	// GEAUTH1 - 版本 0x03
	GEAUTH1 = []byte{
		0x03, 0x00, 0x00, 0x00, 0x02, 0xf1, 0x5b, 0x5e, 0x34, 0x86, 0x84, 0x38, 0x4f, 0xb9, 0x04, 0x0a,
		0x3a, 0xbf, 0x5e, 0x6a, 0x8d, 0x85, 0x3c, 0x6a, 0x3f, 0xaa, 0xd0, 0xf1, 0x77, 0x47, 0x6f, 0x6f,
		0x67, 0x6c, 0x65, 0x45, 0x61, 0x72, 0x74, 0x68, 0x57, 0x69, 0x6e, 0x2e, 0x65, 0x78, 0x65, 0x00,
	}

	// GEAUTH2 - 版本 0x01
	GEAUTH2 = []byte{
		0x01, 0x00, 0x00, 0x00, 0x02, 0xf1, 0x5b, 0x5e, 0x34, 0x86, 0x84, 0x38, 0x4f, 0xb9, 0x04, 0x0a,
		0x3a, 0xbf, 0x5e, 0x6a, 0x8d, 0xec, 0xc2, 0xa8, 0x1c, 0x43, 0x08, 0xc5, 0x77, 0x58, 0xe0, 0x48,
		0x9d, 0x8b, 0x80, 0xdb, 0x4d, 0x00, 0x06, 0x25, 0x31, 0x93, 0xaf, 0x8e, 0xf6, 0xfb, 0x0a, 0xa9,
		0x8b,
	}

	// GEAUTH3 - 版本 0x01
	GEAUTH3 = []byte{
		0x01, 0x00, 0x00, 0x00, 0x02, 0x72, 0xb7, 0x97, 0x7b, 0xae, 0x42, 0x3e, 0x43, 0x8b, 0x26, 0x19,
		0xca, 0xae, 0x24, 0x5b, 0x9f, 0x03, 0x29, 0xf2, 0xa6, 0xc4, 0x0e, 0x8d, 0x22, 0x5c, 0xd6, 0xf1,
		0x71, 0x12, 0x7c, 0xe0, 0xc7, 0x00, 0x06, 0x25, 0x31, 0x83, 0x5e, 0x79, 0x5c, 0xdc, 0x37, 0x19,
		0xc8,
	}
)

// generateRandomAuthKey 生成随机的认证密钥（49字节）
// 从预定义的三个密钥中随机选择一个
func generateRandomAuthKey() []byte {
	// 从三个预定义密钥中随机选择
	keys := [][]byte{GEAUTH1, GEAUTH2, GEAUTH3}
	return keys[rand.IntN(len(keys))]
}

// GenerateRandomGeAuth 生成指定版本的认证密钥（用于测试和自定义）
// version: 版本号（0x01-0xFF）
//   - 如果 version = 0，从预定义密钥中随机选择一个
//   - 如果 version = 1, 2, 3，返回对应的 GEAUTH1/2/3
//   - 其他版本号，生成带该版本号的随机密钥
//
// 返回：48 或 49 字节的认证密钥
func GenerateRandomGeAuth(version byte) ([]byte, error) {
	// version = 0：从预定义密钥中随机选择
	if version == 0 {
		keys := [][]byte{GEAUTH1, GEAUTH2, GEAUTH3}
		return keys[rand.IntN(len(keys))], nil
	}

	// version = 1, 2, 3：返回对应的预定义密钥
	switch version {
	case 1:
		return GEAUTH2, nil // GEAUTH2 的版本是 0x01
	case 2:
		return GEAUTH3, nil // GEAUTH3 的版本也是 0x01，但数据不同
	case 3:
		return GEAUTH1, nil // GEAUTH1 的版本是 0x03
	}

	// 其他版本号：生成随机密钥
	header := []byte{0x01, 0x00, 0x00, 0x00}
	data := make([]byte, 44)

	// 生成随机数据
	for i := range data {
		data[i] = byte(rand.IntN(256))
	}

	// 组合：header(4) + version(1) + randomData(44) = 49字节
	return append(append(header, version), data...), nil
}
