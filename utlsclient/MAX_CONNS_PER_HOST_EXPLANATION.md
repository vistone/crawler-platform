# max_conns_per_host 配置说明

## 参数含义

`max_conns_per_host` 限制的是**每个主机（域名）的最大连接数**，而不是每个 IP 的连接数。

## 工作原理

- **连接存储方式**：连接池按 IP 地址存储连接，每个 IP 可以有一个连接
- **主机映射**：一个主机（域名）可以对应多个 IP 地址
- **限制逻辑**：`max_conns_per_host` 限制的是每个主机（域名）最多可以建立多少个连接（对应多少个不同的 IP）

## 配置建议

### 场景 1：希望所有 IP 都参与请求（推荐）

如果你有 800 个 IP，希望每个 IP 都参与请求：

```toml
max_conns_per_host = 0  # 0 表示不限制，让所有 IP 都参与
# 或者
max_conns_per_host = 1000  # 设置为足够大的值，确保所有 IP 都能建立连接
```

### 场景 2：限制每个主机的连接数

如果你只想为每个主机建立少量连接（例如用于测试或资源限制）：

```toml
max_conns_per_host = 10  # 每个主机最多 10 个连接（对应 10 个 IP）
```

### 场景 3：每个主机只使用 1 个 IP（不推荐用于多 IP 场景）

```toml
max_conns_per_host = 1  # 每个主机最多 1 个连接（对应 1 个 IP）
# 注意：这样会导致其他 IP 不会参与请求
```

## 你的情况

根据你的需求：
- 有 800 多个 IP
- 希望每个 IP 都参与请求

**推荐配置**：
```toml
max_conns_per_host = 0  # 不限制，让所有 IP 都参与
```

或者：
```toml
max_conns_per_host = 1000  # 设置为足够大的值
```

## 代码逻辑

在 `pool_manager.go` 的 `maintainPool()` 方法中：

```go
// 检查是否超过每个主机的最大连接数限制
if pm.config.MaxConnsPerHost > 0 && currentConnCount >= pm.config.MaxConnsPerHost {
    // 已达到该主机的最大连接数限制，跳过此 IP
    continue
}
```

**注意**：
- 如果 `MaxConnsPerHost = 0` 或负数，表示不限制
- 如果 `MaxConnsPerHost > 0`，会限制每个主机的连接数
- 每个 IP 只能有一个连接（在 `AddConnection` 中检查）

## 验证方法

1. **查看预热日志**：
   ```
   连接预热成功并加入白名单: kh.google.com -> 2a00:1450:400c:c1d::be
   连接预热成功并加入白名单: kh.google.com -> 2404:6800:4003:c01::5d
   ...
   ```
   应该看到多个 IP 的预热成功日志

2. **查看连接数统计**：
   可以通过日志或监控查看实际建立的连接数

3. **查看请求日志**：
   应该看到不同的目标 IP 被使用，而不是总是同一个 IP

## 相关配置

- `max_concurrent_pre_warms`：并发预热数，影响预热速度
- `pre_warm_interval`：预热间隔，影响维护频率

