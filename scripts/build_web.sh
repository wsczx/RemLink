#!/bin/bash
# 前端编译脚本（使用 Docker 容器编译）
# 需要: docker 已安装
# 源码: new_ui/（Vue3 + TS，pnpm；旧前端 web/ 已不再参与构建）
# 产物: new_ui/ui → server/ui（供后端 embed 使用）

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

REGISTRY="${NPM_REGISTRY:-https://registry.npmmirror.com}"

rm -rf new_ui/ui server/ui

# corepack 按 package.json 的 packageManager 字段启用固定版本 pnpm；
# HOME 指向容器内可写目录，避免以宿主 uid 运行时 corepack/pnpm 缓存无权限写入
docker run --rm --user "$(id -u):$(id -g)" -v "$PWD/new_ui:/app" -w /app \
  -e HOME=/tmp -e COREPACK_HOME=/tmp/corepack -e COREPACK_ENABLE_DOWNLOAD_PROMPT=0 \
  -e COREPACK_NPM_REGISTRY="$REGISTRY" -e npm_config_registry="$REGISTRY" \
  node:22-alpine \
  sh -c "corepack pnpm install --frozen-lockfile && corepack pnpm run build"

mkdir -p server/ui
cp -r new_ui/ui/. server/ui/
