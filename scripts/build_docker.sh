#!/bin/bash
# 构建 Docker 镜像并打版本 tag
# 用法:
#   bash scripts/build_docker.sh          构建并打版本 tag
#
# 说明: 仅本地构建并打 tag 到 wsczx/remlink，镜像推送由 make release 负责。

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR/.."

ver=$(cat version)
echo "版本: $ver"

# 构建镜像
docker buildx build -t wsczx/remlink:latest --platform linux/amd64 \
  --progress=plain \
  --build-arg CN="yes" --build-arg appVer=$ver --build-arg commitId=$(git rev-parse HEAD) \
  -f docker/Dockerfile .

echo "docker tag latest $ver"
docker tag wsczx/remlink:latest wsczx/remlink:$ver
docker run --rm wsczx/remlink:$ver -v
