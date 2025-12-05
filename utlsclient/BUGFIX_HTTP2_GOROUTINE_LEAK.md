# HTTP/2 连接 Goroutine 泄漏修复

## 问题描述

在 VPS 上部署时，出现以下错误：

```
goroutine 1882 gp=0xc0011381c0 m=nil [semacquire]:
runtime.gcMarkDone()
golang.org/x/net/http2.(*Framer).ReadFrameForHeader(...)
golang.org/x/net/http2.(*clientConnReadLoop).run(...)
```

**根本原因**：
- `http2.ClientConn` 在创建时会启动一个 `readLoop` goroutine 来读取 HTTP/2 帧
- 当连接被关闭时，如果 `readLoop` goroutine 没有正确退出，会导致：
  1. Goroutine 泄漏
  2. 资源无法释放
  3. GC 阻塞（因为 goroutine 持有资源引用）

## 修复内容

### 1. 改进 `roundTripH2` 方法

**问题**：
- 没有检查连接健康状态就创建/使用 HTTP/2 连接
- 请求失败后没有清理 HTTP/2 连接，导致 `readLoop` goroutine 继续运行

**修复**：
- 在创建/使用 HTTP/2 连接前检查连接健康状态
- 请求失败时立即关闭并清空 `h2ClientConn`，防止 `readLoop` goroutine 泄漏
- 使用局部变量保存 `h2ClientConn` 引用，避免在锁外访问时被其他 goroutine 修改

### 2. 改进 `Close` 方法

**问题**：
- 关闭顺序不当，可能导致 `readLoop` goroutine 无法及时退出
- 没有先清空 `h2ClientConn` 引用，其他 goroutine 可能继续使用已关闭的连接

**修复**：
- 先关闭 HTTP/2 连接（使用 `h2Mu` 锁保护）
- 在关闭前先清空 `h2ClientConn` 引用，防止其他 goroutine 继续使用
- 确保关闭顺序：HTTP/2 连接 → TLS 连接 → TCP 连接

## 修复后的行为

1. **连接健康检查**：在 `roundTripH2` 中，如果连接已被标记为不健康，立即返回错误
2. **错误处理**：请求失败时，立即关闭并清空 `h2ClientConn`，防止 goroutine 泄漏
3. **优雅关闭**：`Close()` 方法按正确顺序关闭所有资源，确保 `readLoop` goroutine 能够退出

## 测试建议

1. **长时间运行测试**：运行服务数小时，检查 goroutine 数量是否稳定
2. **压力测试**：大量并发请求，观察是否有 goroutine 泄漏
3. **连接关闭测试**：频繁创建和关闭连接，检查资源是否完全释放

## 相关代码

- `utlsclient/utlshotconnpool.go:161-184` - `roundTripH2` 方法
- `utlsclient/utlshotconnpool.go:82-130` - `Close` 方法

## 注意事项

- HTTP/2 连接的 `readLoop` goroutine 会在底层连接关闭后自动退出
- 确保在关闭 HTTP/2 连接前先清空引用，避免其他 goroutine 继续使用
- 使用 `h2Mu` 锁保护 `h2ClientConn` 的访问，确保线程安全

