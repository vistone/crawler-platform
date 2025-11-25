#!/bin/bash

# 版本发布脚本 - 自动递增版本号并发布

set -e

# 解析脚本目录，确保可在 scripts 目录内执行
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$PROJECT_ROOT"

# 检查是否有未提交的更改
if [ -n "$(git status --porcelain)" ]; then
    echo "⚠️  警告: 有未提交的更改"
    git status --short
    echo ""
    if [ -n "$AUTO_CONTINUE" ]; then
        REPLY="$AUTO_CONTINUE"
        echo "AUTO_CONTINUE=$AUTO_CONTINUE, 自动选择"
    else
        read -p "是否继续? (y/N) " -n 1 -r
        echo
    fi
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# 运行版本递增脚本
"$SCRIPT_DIR/bump-version.sh"

# 获取新版本号
NEW_VERSION=$(grep -oP 'const Version = "\K[^"]+' cmd/utlsclient/main.go)

# 读取发布说明
if [ -n "$RELEASE_NOTES" ]; then
    RELEASE_BODY="$RELEASE_NOTES"
    echo ""
    echo "📝 使用环境变量 RELEASE_NOTES:"
    echo "───────────────────────────────────────"
    echo "$RELEASE_BODY"
else
    echo ""
    echo "📝 请输入本次版本的更新说明 (按 Ctrl+D 结束):"
    echo "───────────────────────────────────────"
    RELEASE_BODY=$(cat)
fi

# 提交更改
git add -A
git commit -m "chore: bump version to v$NEW_VERSION

$RELEASE_BODY"

# 创建标签
git tag -a "v$NEW_VERSION" -m "Release v$NEW_VERSION

$RELEASE_BODY"

echo ""
echo "✅ 版本 v$NEW_VERSION 已创建"
echo ""

if [ -n "$AUTO_PUSH" ]; then
    REPLY="$AUTO_PUSH"
    echo "AUTO_PUSH=$AUTO_PUSH, 自动选择"
else
    read -p "是否推送到远程仓库? (y/N) " -n 1 -r
    echo
fi

if [[ $REPLY =~ ^[Yy]$ ]]; then
    git push origin main
    git push origin "v$NEW_VERSION"
    echo ""
    echo "🎉 版本 v$NEW_VERSION 已成功发布到 GitHub!"
else
    echo ""
    echo "ℹ️  版本已在本地创建，稍后可手动推送:"
    echo "   git push origin main && git push origin v$NEW_VERSION"
fi
