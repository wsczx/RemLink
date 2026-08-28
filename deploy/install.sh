#!/usr/bin/env bash
#
# RemLink 一键安装脚本
# 自动下载最新版二进制 -> 安装到 /usr/local/remlink -> 注册 systemd 服务 -> 启动
#
# 用法:
#   sudo bash install.sh                 # 安装最新版并启动
#   sudo VERSION=0.16.1 bash install.sh  # 安装指定版本
#   sudo bash install.sh --no-start      # 仅安装，不启动服务
#   sudo bash install.sh --help          # 查看帮助
#
set -euo pipefail

REPO="wsczx/RemLink"
INSTALL_DIR="/usr/local/remlink"
BIN_NAME="remlink"
SERVICE_NAME="remlink"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info()  { echo -e "${GREEN}[INFO]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
error() { echo -e "${RED}[ERROR]${NC} $*" >&2; exit 1; }

# ---- 参数解析 ----
START=1
for a in "$@"; do
  case "$a" in
    --no-start) START=0 ;;
    --help|-h)  sed -n '2,12p' "$0"; exit 0 ;;
  esac
done

# ---- 前置检查 ----
[ "$(id -u)" -eq 0 ] || error "请使用 root 权限运行: sudo bash $0"

# 架构检测
case "$(uname -m)" in
  x86_64|amd64)  PKG_ARCH="amd64" ;;
  aarch64|arm64) PKG_ARCH="arm64" ;;
  *) error "不支持的 CPU 架构: $(uname -m)（仅支持 amd64 / arm64）" ;;
esac

# 版本解析（默认取最新）
if [ -z "${VERSION:-}" ]; then
  info "获取最新版本..."
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep -oP '"tag_name":\s*"\K[^"]+')"
  [ -n "$VERSION" ] || error "无法获取最新版本，请检查网络或手动指定 VERSION=xxx"
fi
info "目标版本: ${VERSION} (${PKG_ARCH})"

# ---- 下载工具 ----
if command -v curl >/dev/null 2>&1; then
  DL="curl -fL"
elif command -v wget >/dev/null 2>&1; then
  DL="wget -q"
else
  error "需要 curl 或 wget 才能下载"
fi

# ---- 若服务已在运行，先停掉以便覆盖二进制 ----
if command -v systemctl >/dev/null 2>&1 && systemctl list-unit-files | grep -q "^${SERVICE_NAME}.service"; then
  systemctl is-active --quiet "$SERVICE_NAME" && { info "停止已运行的服务..."; systemctl stop "$SERVICE_NAME"; }
fi

# ---- 下载二进制 ----
mkdir -p "$INSTALL_DIR"
cd "$INSTALL_DIR"
ASSET="remlink-linux-${PKG_ARCH}"
URL="https://github.com/${REPO}/releases/download/${VERSION}/${ASSET}"
info "下载二进制: $URL"
if command -v curl >/dev/null 2>&1; then
  curl -fL "$URL" -o "$BIN_NAME"
else
  wget -q "$URL" -O "$BIN_NAME"
fi
chmod +x "$BIN_NAME"
info "已安装: ${INSTALL_DIR}/${BIN_NAME} ($(du -h "$BIN_NAME" | cut -f1))"

# ---- 注册 systemd 服务 ----
info "写入 systemd 服务: $SERVICE_FILE"
cat > "$SERVICE_FILE" <<EOF
[Unit]
Description=RemLink Server Service
Documentation=https://github.com/${REPO}
After=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=${INSTALL_DIR}
Restart=on-failure
RestartSec=5s
ExecStart=${INSTALL_DIR}/${BIN_NAME}

[Install]
WantedBy=multi-user.target
EOF

# ---- 防火墙放行 ----
open_port() {
  local proto="$1" port="$2"

  # 1) firewalld
  if command -v firewall-cmd >/dev/null 2>&1 && systemctl is-active --quiet firewalld; then
    firewall-cmd --permanent --add-port="${port}/${proto}" >/dev/null 2>&1 && \
      firewall-cmd --reload >/dev/null 2>&1 && info "firewalld 已放行 ${port}/${proto}"
    return
  fi

  # 2) ufw
  if command -v ufw >/dev/null 2>&1 && ufw status | grep -q "active"; then
    ufw allow "${port}/${proto}" >/dev/null 2>&1 && info "ufw 已放行 ${port}/${proto}" || true
    return
  fi

  # 3) iptables
  if command -v iptables >/dev/null 2>&1; then
    if iptables -C INPUT -p "${proto}" --dport "${port}" -j ACCEPT 2>/dev/null; then
      info "iptables 已存在放行规则 ${port}/${proto}"
    elif iptables -I INPUT -p "${proto}" --dport "${port}" -j ACCEPT 2>/dev/null; then
      warn "iptables 已临时放行 ${port}/${proto}（重启后失效，建议安装 firewalld/ufw 持久化管理）"
    else
      warn "iptables 放行 ${port}/${proto} 失败，请手动检查防火墙"
    fi
    return
  fi

  info "未检测到 firewalld/ufw/iptables，若系统无防火墙则端口 ${port}/${proto} 默认已开放"
}
open_port tcp 443
open_port udp 443
open_port tcp 8800

# ---- 启动 ----
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
  systemctl enable "$SERVICE_NAME" >/dev/null 2>&1
  if [ "$START" -eq 1 ]; then
    systemctl restart "$SERVICE_NAME"
    sleep 2
    if systemctl is-active --quiet "$SERVICE_NAME"; then
      info "✅ RemLink 服务已启动"
    else
      warn "服务启动失败，请查看日志: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
    fi
    info "管理后台: https://<服务器IP>:8800"
    info "首次管理员密码见日志: journalctl -u ${SERVICE_NAME} -n 30 --no-pager"
  else
    info "已跳过启动（--no-start）。手动启动: systemctl start ${SERVICE_NAME}"
  fi
else
  warn "未检测到 systemd，请手动运行: ${INSTALL_DIR}/${BIN_NAME}"
fi

info "安装目录: ${INSTALL_DIR}  配置文件/数据库将生成于 ${INSTALL_DIR}/conf"
