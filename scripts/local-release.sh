#!/bin/bash
# 本地发布脚本：绕过 GitHub Actions，直接在构建机上构建并发布到 GitHub Releases。
#
# 前置依赖（构建机需具备）：
#   - docker（已 docker login，或设置环境变量 DOCKERHUB_TOKEN）
#   - gh CLI（已 gh auth login，或设置环境变量 GH_TOKEN/GITHUB_TOKEN
#   - Gitee token（可选）：同步 Release 到 Gitee 镜像，
#     供在线升级在 GitHub 不可达时回退下载；不配则跳过。
#     示例：git config --global gitee.token <PAT>
#
# 用法：
#   ./scripts/local-release.sh [-l]   # 需 gh 已登录 + docker 已登录（或分别用 GH_TOKEN / DOCKERHUB_TOKEN 环境变量覆盖）
#   ARCH="linux/amd64" ./scripts/local-release.sh          # 仅 amd64
#   ./scripts/local-release.sh -l                          # 同时打 latest 镜像标签
# 预发布识别：version 含 beta/alpha/rc/preview/dev 时自动标记为 GitHub Prerelease（正式版不带标识则发布正式 Release）
set -euo pipefail

REPO_PRIVATE=wsczx/RemLink-private
REPO_PUBLIC=wsczx/RemLink
VER=$(cat version)
COMMIT=$(git rev-parse HEAD)
# 按版本号自动识别预发布：版本号含 beta/alpha/rc/preview 等标识时，GitHub Release 标记为 Prerelease
PRERELEASE=0
case "$VER" in
  *beta*|*alpha*|*rc*|*preview*|*dev*) PRERELEASE=1 ;;
esac
PRERELEASE_FLAG=""
if [ "$PRERELEASE" = "1" ]; then
  PRERELEASE_FLAG="--prerelease"
  echo "==> 版本 ${VER} 含预发布标识，将标记为 Prerelease"
fi
# Docker 构建架构，逗号分隔
ARCH="${ARCH:-linux/amd64,linux/arm64}"
# 是否启用国内源加速（容器内切 ustc apk 源 + goproxy.cn）：默认 yes（本地构建机在国内，加速 go mod/apk）；
# CI（release.yml 不走本脚本、不带 CN）由 Dockerfile 的 ARG CN="no" 兜底，国外 runner 不受影响。
# 如需强制关闭可显式 CN=no 覆盖。
CN="${CN:-yes}"

# 是否额外打 latest 镜像标签：-l / --latest / latest 均可
LATEST_TAG=0
for _arg in "$@"; do
  case "$_arg" in
    -l|--latest|latest) LATEST_TAG=1 ;;
  esac
done

echo "==> 构建版本 v${VER} commit ${COMMIT}"
echo "==> Docker 架构: ${ARCH}"

# 环境检测
command -v gh >/dev/null 2>&1 || { echo "错误：未安装 gh，请先 'apt install gh' 并 'gh auth login'"; exit 1; }
gh auth status >/dev/null 2>&1 || { echo "错误：gh 未登录，请先 'gh auth login'"; exit 1; }
command -v docker >/dev/null 2>&1 || { echo "错误：未安装 docker"; exit 1; }
if [ -n "${DOCKERHUB_TOKEN:-}" ]; then
  echo "$DOCKERHUB_TOKEN" | docker login -u wsczx --password-stdin
fi

# 前端构建
bash scripts/build_web.sh

# 打包私有仓库源码
rm -rf artifact-src && mkdir -p artifact-src
tar --exclude='.git' \
    --exclude='.env*' \
    --exclude='server/.env*' \
    --exclude='web/node_modules' \
    --exclude='server/ui' \
    --exclude='server/conf' \
    --exclude='artifact-dist' \
    --exclude='artifact-src' \
    -czf "artifact-src/RemLink-src-v${VER}.tar.gz" .

# 提取当前版本 changelog 作为 Release 正文
ver="${VER#v}"
body=$(awk -v ver="v$ver" '
  BEGIN { f=0 }
  /^## / { f=0 }
  $0 == "## " ver { f=1; next }
  /^## / && f { exit }
  f { print }
' CHANGELOG.md)
if [ -z "$body" ]; then
  body=$(awk -v ver="$ver" '
    BEGIN { f=0 }
    /^## / { f=0 }
    $0 == "## " ver { f=1; next }
    /^## / && f { exit }
    f { print }
  ' CHANGELOG.md)
fi
body=$(printf "%s\n" "$body" | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}' | sed '/./,$!d')
[ -z "$body" ] && body="RemLink v${ver} 发布"

# 多架构 Docker 镜像构建并推送
docker buildx build --push \
  --platform "$ARCH" \
  --build-arg "appVer=${VER}" \
  --build-arg "commitId=${COMMIT}" \
  --build-arg "CN=${CN}" \
  -f docker/Dockerfile \
  -t "wsczx/remlink:${VER}" .
if [ "$LATEST_TAG" = "1" ] && [ "$PRERELEASE" != "1" ]; then
  docker buildx imagetools create -t "wsczx/remlink:latest" "wsczx/remlink:${VER}"
elif [ "$LATEST_TAG" = "1" ] && [ "$PRERELEASE" = "1" ]; then
  echo "==> 预发布版不打 latest 标签（避免 latest 指向 beta 镜像）"
fi

# 提取二进制
RELEASE_ARCHES=$(echo "$ARCH" | tr ',' ' ') bash scripts/release.sh

# 发布到私有仓库（源码）
gh release delete "v${VER}" -R "$REPO_PRIVATE" 2>/dev/null || true
gh release create "v${VER}" -R "$REPO_PRIVATE" \
  -t "RemLink-src v${VER}" -n "$body" $PRERELEASE_FLAG artifact-src/*

# 发布到公共仓库（二进制）
gh release delete "v${VER}" -R "$REPO_PUBLIC" 2>/dev/null || true
gh release create "v${VER}" -R "$REPO_PUBLIC" \
  -t "RemLink v${VER}" -n "$body" $PRERELEASE_FLAG artifact-dist/*

# 同步 Release 到 Gitee 镜像（可选；预发布版不同步，避免用户误升级到 beta）
GITEE_TOKEN="${GITEE_TOKEN:-$(git config --get gitee.token 2>/dev/null || true)}"
if [ "$PRERELEASE" = "1" ]; then
  echo "==> 预发布版跳过 Gitee 镜像同步"
elif [ -n "${GITEE_TOKEN:-}" ]; then
  echo "==> 同步 Release 到 Gitee 镜像"
  gitee_api="https://gitee.com/api/v5/repos/${REPO_PUBLIC}"

  # 删除同名旧 release
  old_id=$(curl -s "${gitee_api}/releases/tags/v${VER}?access_token=${GITEE_TOKEN}" \
    | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2 || true)
  if [ -n "$old_id" ]; then
    curl -s -X DELETE "${gitee_api}/releases/${old_id}?access_token=${GITEE_TOKEN}" >/dev/null || true
  fi

  # 建 release（tag 不存在时 Gitee 会基于 target_commitish 自动打 tag）
  resp=$(curl -s -X POST "${gitee_api}/releases" \
    -d "access_token=${GITEE_TOKEN}" \
    -d "target_commitish=main" \
    --data-urlencode "tag_name=v${VER}" \
    --data-urlencode "name=RemLink v${VER}" \
    --data-urlencode "body=${body}")
  release_id=$(echo "$resp" | grep -o '"id":[0-9]*' | head -1 | cut -d: -f2 || true)

  if [ -z "$release_id" ]; then
    echo "警告：Gitee release 创建失败，跳过镜像同步。响应: $resp"
  else
    shopt -s nullglob
    for f in artifact-dist/*; do
      echo "    上传 $(basename "$f") 到 Gitee..."
      curl -sf -X POST "${gitee_api}/releases/${release_id}/attach_files?access_token=${GITEE_TOKEN}" \
        -F "file=@${f}" >/dev/null || echo "警告：$(basename "$f") 上传 Gitee 失败"
    done
    echo "    Gitee: https://gitee.com/${REPO_PUBLIC}/releases/tag/v${VER}"
  fi
else
  echo "提示：未配置 Gitee token（git config gitee.token 或 GITEE_TOKEN），跳过 Gitee 镜像同步（国内升级回退源将无此版本）"
fi

echo "==> 发布完成："
echo "    私有: https://github.com/${REPO_PRIVATE}/releases/tag/v${VER}"
echo "    公共: https://github.com/${REPO_PUBLIC}/releases/tag/v${VER}"
