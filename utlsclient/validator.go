package utlsclient

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	ge "crawler-platform/GoogleEarth"
	projlogger "crawler-platform/logger"
)

// ValidationResult 包含验证成功后的结果。
type ValidationResult struct {
	SessionID string
}

// Validator 定义了验证一个新连接的接口。
type Validator interface {
	Validate(conn *UTLSConnection) (*ValidationResult, error)
	// CheckRecovery 检查连接是否恢复（只检查是否返回200，不要求SessionID）
	// 用于黑名单恢复检查，只要返回200就认为恢复成功
	CheckRecovery(conn *UTLSConnection) (bool, error)
}

// ConfigurableValidator 是一个可配置的验证器实现。
type ConfigurableValidator struct {
	Path         string
	Method       string
	Body         []byte
	GenerateBody bool // 如果为true，每次验证时生成新的认证body（用于POST请求）
}

// NewConfigurableValidator 创建一个新的可配置验证器。
func NewConfigurableValidator(path, method string, body []byte) *ConfigurableValidator {
	m := strings.ToUpper(method)
	if m != "POST" && m != "GET" && m != "HEAD" {
		m = "GET" // 默认为GET
	}
	// 如果是POST请求，默认每次生成新的认证body
	generateBody := m == "POST"
	return &ConfigurableValidator{
		Path:         path,
		Method:       m,
		Body:         body,
		GenerateBody: generateBody,
	}
}

// Validate 实现 Validator 接口，并能区分不同类型的失败。
func (v *ConfigurableValidator) Validate(conn *UTLSConnection) (*ValidationResult, error) {
	// 使用域名而不是IP地址构建URL，因为TLS握手和Host头都使用域名
	url := "https://" + conn.targetHost + v.Path

	// 如果需要生成新的body（通常是POST请求），每次验证时生成新的认证body
	var requestBody []byte
	if v.GenerateBody {
		// 服务器只接受三个预定义密钥（GEAUTH1, GEAUTH2, GEAUTH3）
		// 为了确保每个连接使用不同的密钥，我们基于目标IP来选择
		// 这样不同IP的连接会使用不同的预定义密钥，从而可能获得不同的SessionID
		targetIP := conn.TargetIP()
		ipHash := 0
		for _, b := range []byte(targetIP) {
			ipHash += int(b)
		}
		// 使用IP哈希值来选择预定义密钥（版本号1,2,3对应GEAUTH2,GEAUTH3,GEAUTH1）
		// 确保不同IP使用不同的密钥
		version := byte((ipHash % 3) + 1)
		requestBody, _ = ge.GenerateRandomGeAuth(version)
	} else {
		requestBody = v.Body
	}

	req, err := http.NewRequest(v.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("构建验证请求失败: %w", err)
	}

	// 对于 POST 请求，必须设置 Content-Type 和 Content-Length
	// 注意：必须在调用 RoundTrip 之前设置，因为 RoundTrip 的自动设置可能不够及时
	if v.Method == "POST" && len(requestBody) > 0 {
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(requestBody)))
	}

	// 调试日志：记录请求详细信息
	projlogger.Debug("验证请求: URL=%s, 方法=%s, Path=%s, Body长度=%d, Content-Type=%s, Content-Length=%s",
		url, v.Method, v.Path, len(requestBody), req.Header.Get("Content-Type"), req.Header.Get("Content-Length"))
	resp, err := conn.RoundTrip(req)
	if err != nil {
		// 网络层错误，这不是IP被封，只是连接本身的问题。
		return nil, fmt.Errorf("验证请求网络失败: %w", err)
	}
	defer resp.Body.Close()

	// 根据响应状态码进行精确判断
	switch resp.StatusCode {
	case http.StatusOK:
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("读取响应体失败: %w", err)
		}
		projlogger.Debug("验证响应: 方法=%s, 状态码=200, 响应体长度=%d", v.Method, len(bodyBytes))
		if v.Method == "POST" {
			// 尝试解析 session ID（必须成功且非空，才认为验证通过）
			sessionID, err := ge.ParseSessionFromResponse(bodyBytes)
			if err != nil {
				projlogger.Warn("解析 session ID 失败: %v, 响应体长度: %d", err, len(bodyBytes))
				return nil, fmt.Errorf("解析 session ID 失败: %w", err)
			}
			if sessionID == "" {
				projlogger.Warn("解析 session ID 结果为空，响应体长度: %d", len(bodyBytes))
				return nil, fmt.Errorf("解析 session ID 失败: 解析结果为空")
			}

			// 验证 sessionid 是否有效：长度应该至少为 16 字节（通常更长）
			// 8 字节的 sessionid 可能不完整，需要继续尝试其他 IP
			if len(sessionID) < 16 {
				projlogger.Debug("解析的 SessionID 长度过短 (%d 字节)，可能不完整，继续尝试其他 IP", len(sessionID))
				return nil, fmt.Errorf("SessionID 长度过短 (%d 字节)，可能不完整", len(sessionID))
			}

			projlogger.Debug("成功解析有效的SessionID，长度: %d", len(sessionID))
			return &ValidationResult{SessionID: sessionID}, nil
		}
		// 非 POST（如 GET/HEAD）不需要 SessionID，视为验证失败（因为你要求必须有有效 SessionID）
		return nil, fmt.Errorf("验证失败: 非POST方法(%s)未返回SessionID", v.Method)

	case http.StatusForbidden:
		// IP被明确拒绝，返回特定错误
		return nil, ErrIPBlockedBy403
	case http.StatusNotFound:
		// 404 错误：这个IP可能无法访问SessionIdPath（如/geauth），但不影响其他IP
		// 这是正常的，某些IP可能无法访问特定路径，继续尝试其他IP即可
		return nil, fmt.Errorf("该IP无法访问SessionIdPath，状态码: 404")
	default:
		// 其他所有非200状态码，视为临时性或服务器端问题，不应拉黑IP。
		// 这些错误不影响其他IP的验证，继续尝试即可
		return nil, fmt.Errorf("验证失败，状态码: %d", resp.StatusCode)
	}
}

// CheckRecovery 检查连接是否恢复（只检查是否返回200，不要求SessionID）
// 用于黑名单恢复检查，只要返回200就认为恢复成功
func (v *ConfigurableValidator) CheckRecovery(conn *UTLSConnection) (bool, error) {
	// 使用域名而不是IP地址构建URL
	url := "https://" + conn.targetHost + v.Path

	// 如果需要生成新的body（通常是POST请求），每次验证时生成新的认证body
	var requestBody []byte
	if v.GenerateBody {
		targetIP := conn.TargetIP()
		ipHash := 0
		for _, b := range []byte(targetIP) {
			ipHash += int(b)
		}
		version := byte((ipHash % 3) + 1)
		requestBody, _ = ge.GenerateRandomGeAuth(version)
	} else {
		requestBody = v.Body
	}

	req, err := http.NewRequest(v.Method, url, bytes.NewReader(requestBody))
	if err != nil {
		return false, fmt.Errorf("构建恢复检查请求失败: %w", err)
	}

	// 对于 POST 请求，必须设置 Content-Type 和 Content-Length
	if v.Method == "POST" && len(requestBody) > 0 {
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("Content-Length", fmt.Sprintf("%d", len(requestBody)))
	}

	resp, err := conn.RoundTrip(req)
	if err != nil {
		// 网络层错误
		return false, fmt.Errorf("恢复检查请求网络失败: %w", err)
	}
	defer resp.Body.Close()

	// 只检查状态码，200就认为恢复成功，403就认为仍被封禁
	switch resp.StatusCode {
	case http.StatusOK:
		// 返回200，认为恢复成功
		return true, nil
	case http.StatusForbidden:
		// 返回403，认为仍被封禁
		return false, ErrIPBlockedBy403
	default:
		// 其他状态码，认为未恢复（但不是403，可能是临时性问题）
		return false, fmt.Errorf("恢复检查失败，状态码: %d", resp.StatusCode)
	}
}
