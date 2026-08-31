#!/bin/bash
# 本地编译
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/../server"

echo "=============================="

ver=$(cat "$SCRIPT_DIR/../version")
echo "  版本:   $ver"

commitId=$(git -C "$SCRIPT_DIR/.." rev-parse HEAD)
echo "  Commit: ${commitId:0:8}"

buildDate=$(date -Iseconds)
echo "  日期:   $buildDate"

echo "=============================="

# 链接器: 用 musl-gcc
if command -v musl-gcc &>/dev/null; then
    export CC=musl-gcc
    echo "链接器: musl-gcc ✅"
else
    echo "⚠ 未安装 musl-gcc，正在尝试安装 musl-tools..."
    if sudo -n true 2>/dev/null; then
        sudo apt-get update -qq && sudo apt-get install -y -qq musl-tools
        export CC=musl-gcc
        echo "链接器: musl-gcc ✅"
    else
        echo "⚠ 无 sudo 权限，无法安装 musl-tools"
        echo "   → 手动执行: sudo apt install musl-tools"
        echo "   → 当前使用 gcc (glibc)"
        echo ""
        export CC=gcc
        echo "链接器: gcc (glibc)"
    fi
fi

go mod tidy

echo ""
echo "开始编译..."

export CGO_ENABLED=1

ldflags="-s -w \
  -X main.appVer=$ver \
  -X main.commitId=$commitId \
  -X main.buildDate=$buildDate \
  -extldflags '-static'"

go build -v -o remlink -trimpath -ldflags "$ldflags"

# UPX 压缩（传 noupx 参数跳过）
if [ "$1" != "noupx" ]; then
  if command -v upx &>/dev/null; then
      echo ""
      echo "UPX 压缩..."
      upx --best remlink
  else
      echo ""
      echo "⚠ 未安装 upx: apt install upx-ucl"
  fi
else
  echo ""
  echo "跳过 UPX 压缩"
fi

echo ""
echo "=============================="
ls -lh remlink
echo "=============================="
./remlink -v
echo "=============================="
echo "完成: $SCRIPT_DIR/../server/remlink"
