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

go mod tidy

echo ""
echo "开始编译..."

export CGO_ENABLED=0

ldflags="-s -w \
  -X main.appVer=$ver \
  -X main.commitId=$commitId \
  -X main.buildDate=$buildDate"

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
