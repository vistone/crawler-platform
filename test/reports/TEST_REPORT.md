# 真实测试报告

## 测试目标

验证 `utlsProxy` 和 `utlsclient` 的配合工作，包括：
1. 基本连接测试
2. 单请求测试
3. 高并发测试

## 测试环境

- 操作系统: Linux
- Go版本: 需要检查
- 编译状态: 待测试

## 测试步骤

### 1. 编译检查

```bash
cd /home/stone/crawler-platform
go build -o utlsProxy ./cmd/utlsProxy
go build -o utlsclient-cli ./cmd/utlsclient
```

### 2. 启动服务器

```bash
# 生成证书
openssl req -x509 -newkey rsa:2048 -keyout server.key -out server.crt -days 365 -nodes -subj "/CN=localhost"

# 启动服务器
./utlsProxy -listen 127.0.0.1:8443 -token test-token -cert server.crt -key server.key -log debug &
```

### 3. 运行客户端测试

```bash
# 单请求测试
./utlsclient-cli -proxy 127.0.0.1:8443 -token test-token -url https://www.google.com

# 并发测试
./utlsclient-cli -concurrent-test -proxy 127.0.0.1:8443 -token test-token \
    -url https://www.google.com -concurrency 10 -requests 100
```

## 测试结果

**待执行真实测试后填写**

## 发现的问题

**待测试后记录**
