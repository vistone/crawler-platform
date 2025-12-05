# Tools Directory

This directory contains utility programs and tools for development and testing purposes.

## Files

- `read_bbolt_data.go` - Utility to read and analyze BBolt database files
- `test_bbolt_metadata.go` - Test program for BBolt metadata storage
- `test_sqlite_metadata.go` - Test program for SQLite metadata storage
- `cert_manager/` - TLS 证书管理工具目录，支持生成自签名证书（域名和 IP 地址）

## Usage

### 证书管理工具

生成自签名证书（域名）：
```bash
cd /home/stone/crawler-platform
go run tools/cert_manager/main.go -mode self -domain "example.com" -output ./certs
```

生成自签名证书（IP 地址，支持 IPv4 和 IPv6）：
```bash
go run tools/cert_manager/main.go -mode ip -ips "1.2.3.4,2001:db8::1" -output ./certs
```

更多说明请参考 `certs/README.md`。

### 其他工具

```bash
cd /home/stone/crawler-platform
go run tools/read_bbolt_data.go
```