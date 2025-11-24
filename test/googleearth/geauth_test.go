package googleearth_test

import (
	"bytes"
	"testing"
	"time"

	"crawler-platform/GoogleEarth"
	"crawler-platform/utlsclient"
)

// TestGenerateRandomGeAuth 测试随机认证密钥生成
func TestGenerateRandomGeAuth(t *testing.T) {
	tests := []struct {
		name    string
		version byte
		wantErr bool
	}{
		{
			name:    "随机选择预定义密钥",
			version: 0,
			wantErr: false,
		},
		{
			name:    "选择 GEAUTH2 (version=1)",
			version: 1,
			wantErr: false,
		},
		{
			name:    "选择 GEAUTH3 (version=2)",
			version: 2,
			wantErr: false,
		},
		{
			name:    "选择 GEAUTH1 (version=3)",
			version: 3,
			wantErr: false,
		},
		{
			name:    "生成版本 0x05 的随机密钥",
			version: 0x05,
			wantErr: false,
		},
		{
			name:    "生成版本 0xFF 的随机密钥",
			version: 0xFF,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authKey, err := GoogleEarth.GenerateRandomGeAuth(tt.version)

			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateRandomGeAuth() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				// 验证密钥长度（GEAUTH1 是 48 字节，其他是 49 字节）
				if len(authKey) < 48 || len(authKey) > 49 {
					t.Errorf("密钥长度错误: got %d, want 48 or 49", len(authKey))
				}

				// 如果是版本 1, 2, 3，验证返回的是预定义密钥
				switch tt.version {
				case 1:
					if !bytes.Equal(authKey, GoogleEarth.GEAUTH2) {
						t.Error("version=1 应该返回 GEAUTH2")
					}
				case 2:
					if !bytes.Equal(authKey, GoogleEarth.GEAUTH3) {
						t.Error("version=2 应该返回 GEAUTH3")
					}
				case 3:
					if !bytes.Equal(authKey, GoogleEarth.GEAUTH1) {
						t.Error("version=3 应该返回 GEAUTH1")
					}
				default:
					// 对于其他版本，验证格式
					if tt.version > 3 {
						// 验证头部
						if authKey[0] != 0x01 || authKey[1] != 0x00 || authKey[2] != 0x00 || authKey[3] != 0x00 {
							t.Errorf("密钥头部错误: % X", authKey[:4])
						}
						// 验证版本号
						if authKey[4] != tt.version {
							t.Errorf("版本号错误: got 0x%02X, want 0x%02X", authKey[4], tt.version)
						}
					}
				}
			}
		})
	}
}

// TestAuth_GetAuth 测试认证流程
func TestAuth_GetAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（使用 -short 标志）")
	}

	// 1. 从配置文件加载连接池配置
	config, err := utlsclient.LoadPoolConfigFromFile("../../config/config.toml")
	if err != nil {
		// 如果配置文件不存在，使用默认配置
		t.Logf("无法加载配置文件，使用默认配置: %v", err)
		config = &utlsclient.PoolConfig{
			MaxConnections:         100,
			MaxConnsPerHost:        10,
			MaxIdleConns:           20,
			ConnTimeout:            30 * time.Second,
			IdleTimeout:            60 * time.Second,
			MaxLifetime:            300 * time.Second,
			TestTimeout:            10 * time.Second,
			HealthCheckInterval:    30 * time.Second,
			CleanupInterval:        60 * time.Second,
			BlacklistCheckInterval: 300 * time.Second,
			DNSUpdateInterval:      1800 * time.Second,
			MaxRetries:             3,
		}
	}

	// 2. 创建连接池
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	// 3. 创建认证管理器
	auth := GoogleEarth.NewAuth(pool)

	// 4. 执行认证
	t.Logf("\n=== 测试 Google Earth 认证 ===")
	session, err := auth.GetAuth()
	if err != nil {
		t.Fatalf("GetAuth() 失败: %v", err)
	}

	// 5. 验证 session
	if session == "" {
		t.Error("session 为空")
	}

	t.Logf("✅ 成功获取 session")
	t.Logf("   Session 长度: %d 字节", len(session))
	t.Logf("   Session 前 20 字符: %s", session[:min(20, len(session))])

	// 6. 验证保存的 session
	savedSession := auth.GetSession()
	if savedSession != session {
		t.Errorf("保存的 session 不匹配: got %s, want %s", savedSession, session)
	}

	// 7. 测试清除 session
	auth.ClearAuth()
	if auth.GetSession() != "" {
		t.Error("ClearAuth() 后 session 应该为空")
	}

	t.Logf("✅ 认证流程测试通过")
}

// TestAuth_NewAuth 测试认证管理器创建
func TestAuth_NewAuth(t *testing.T) {
	// 创建一个简单的连接池
	config := &utlsclient.PoolConfig{
		MaxConnections:         10,
		MaxConnsPerHost:        5,
		MaxIdleConns:           5,
		ConnTimeout:            30 * time.Second,
		IdleTimeout:            60 * time.Second,
		MaxLifetime:            300 * time.Second,
		TestTimeout:            10 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		CleanupInterval:        60 * time.Second,
		BlacklistCheckInterval: 300 * time.Second,
		DNSUpdateInterval:      1800 * time.Second,
		MaxRetries:             3,
	}
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	// 测试创建认证管理器
	auth := GoogleEarth.NewAuth(pool)
	if auth == nil {
		t.Fatal("NewAuth() 返回 nil")
	}

	// 验证初始状态
	if auth.GetSession() != "" {
		t.Error("新创建的 Auth 的 session 应该为空")
	}
}

// TestAuth_GetSession 测试 session 获取和清除
func TestAuth_GetSession(t *testing.T) {
	config := &utlsclient.PoolConfig{
		MaxConnections:         10,
		MaxConnsPerHost:        5,
		MaxIdleConns:           5,
		ConnTimeout:            30 * time.Second,
		IdleTimeout:            60 * time.Second,
		MaxLifetime:            300 * time.Second,
		TestTimeout:            10 * time.Second,
		HealthCheckInterval:    30 * time.Second,
		CleanupInterval:        60 * time.Second,
		BlacklistCheckInterval: 300 * time.Second,
		DNSUpdateInterval:      1800 * time.Second,
		MaxRetries:             3,
	}
	pool := utlsclient.NewUTLSHotConnPool(config)
	defer pool.Close()

	auth := GoogleEarth.NewAuth(pool)

	// 初始状态应该为空
	if session := auth.GetSession(); session != "" {
		t.Errorf("初始 session 应该为空，got: %s", session)
	}

	// 清除空 session 应该不会出错
	auth.ClearAuth()
	if session := auth.GetSession(); session != "" {
		t.Error("清除后 session 应该仍为空")
	}
}
