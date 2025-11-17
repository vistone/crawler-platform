#!/bin/bash

# 版本号自动递增脚本

set -e

# 读取当前版本号
CURRENT_VERSION=$(grep -oP 'const Version = "\K[^"]+' cmd/utlsclient/main.go)

echo "当前版本: v$CURRENT_VERSION"

# 解析版本号 (格式: major.minor.patch)
IFS='.' read -r major minor patch <<< "$CURRENT_VERSION"

# 递增 patch 版本号
new_patch=$((patch + 1))
NEW_VERSION="$major.$minor.$new_patch"

echo "新版本: v$NEW_VERSION"

# 更新 cmd/utlsclient/main.go
sed -i "s/const Version = \"$CURRENT_VERSION\"/const Version = \"$NEW_VERSION\"/" cmd/utlsclient/main.go

# 更新 version.go (如果存在)
if [ -f "version.go" ]; then
    sed -i "s/const Version = \"$CURRENT_VERSION\"/const Version = \"$NEW_VERSION\"/" version.go
fi

# 更新 README.md
sed -i "s/Version: v$CURRENT_VERSION/Version: v$NEW_VERSION/" README.md

echo ""
echo "✅ 版本号已更新到 v$NEW_VERSION"
echo ""
echo "更新的文件:"
echo "  - cmd/utlsclient/main.go"
[ -f "version.go" ] && echo "  - version.go"
echo "  - README.md"
echo ""
echo "建议执行以下命令提交:"
echo "  git add -A"
echo "  git commit -m \"chore: bump version to v$NEW_VERSION\""
echo "  git tag -a v$NEW_VERSION -m \"Release v$NEW_VERSION\""
echo "  git push origin main && git push origin v$NEW_VERSION"
