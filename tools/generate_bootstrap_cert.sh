#!/bin/bash

# 生成引导节点证书脚本
# 为所有引导节点 IP 生成包含在同一个证书中的自签名证书

# 引导节点 IP 列表
BOOTSTRAP_IPS="172.93.47.57,107.182.186.123,65.49.192.85,65.49.195.185,65.49.194.100,66.112.211.45,45.78.5.252"

# 证书输出目录
OUTPUT_DIR="./certs"

# 密钥长度（2048 或 4096）
KEY_SIZE=2048

# 有效期（天数）
VALID_DAYS=3650

echo "=========================================="
echo "生成引导节点证书"
echo "=========================================="
echo "IP 地址列表: $BOOTSTRAP_IPS"
echo "输出目录: $OUTPUT_DIR"
echo "密钥长度: $KEY_SIZE"
echo "有效期: $VALID_DAYS 天"
echo "=========================================="
echo ""

# 运行证书生成工具
go run tools/cert_manager/main.go \
  -mode ip \
  -ips "$BOOTSTRAP_IPS" \
  -output "$OUTPUT_DIR" \
  -key-size $KEY_SIZE \
  -valid-days $VALID_DAYS

if [ $? -eq 0 ]; then
  # 创建兼容性文件（cert.pem 和 key.pem）
  if [ -f "$OUTPUT_DIR/server.crt" ] && [ -f "$OUTPUT_DIR/server.key" ]; then
    cp "$OUTPUT_DIR/server.crt" "$OUTPUT_DIR/cert.pem"
    cp "$OUTPUT_DIR/server.key" "$OUTPUT_DIR/key.pem"
    echo "已创建兼容性文件: cert.pem 和 key.pem"
  fi
  
  echo ""
  echo "=========================================="
  echo "✅ 证书生成成功！"
  echo "=========================================="
  echo ""
  echo "生成的文件："
  echo "  - $OUTPUT_DIR/server.crt (证书文件)"
  echo "  - $OUTPUT_DIR/server.key (私钥文件)"
  echo "  - $OUTPUT_DIR/cert.pem (证书文件，兼容格式)"
  echo "  - $OUTPUT_DIR/key.pem (私钥文件，兼容格式)"
  echo ""
  echo "证书包含的 IP 地址："
  openssl x509 -in "$OUTPUT_DIR/server.crt" -text -noout | grep -A 1 'Subject Alternative Name' | grep 'IP Address' | sed 's/^[[:space:]]*/    /'
  echo ""
  echo "⚠️  注意：这是自签名证书，客户端需要："
  echo "  1. 手动信任此证书，或"
  echo "  2. 在客户端配置中跳过证书验证（仅用于开发/测试）"
  echo ""
  echo "查看完整证书信息："
  echo "  openssl x509 -in $OUTPUT_DIR/server.crt -text -noout"
  echo ""
else
  echo ""
  echo "❌ 证书生成失败，请检查错误信息"
  exit 1
fi
