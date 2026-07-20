#!/bin/bash
# PathWeaver 控制机一键安装
#
# 推荐（先下载再执行，避免 curl|bash 吞掉脚本）:
#   tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
#     "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
#     -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
#
# 非交互:
#   sudo bash /path/to/install-controller.sh --public-address IP --password 'SECRET'
set -euo pipefail

SCRIPT_REV="2026-07-20f"
REPO="${PW_REPO:-FengYuchen1314/sd-wan-plus}"
RAW_SELF="https://raw.githubusercontent.com/${REPO}/master/scripts/install-controller.sh"

log() { printf '%s\n' "$*" >&2; }

# curl|bash 时 stdin 是脚本本身；子进程里的 curl/apt 可能把剩余脚本吃掉。
# 若不是从普通文件启动，则重新下载到临时文件再 exec。
if [[ -z "${PW_IC_REEXEC:-}" ]]; then
  _src="${BASH_SOURCE[0]:-}"
  _need_reexec=0
  if [[ ! -t 0 ]]; then
    _need_reexec=1
  fi
  case "$_src" in
    ""|/dev/fd/*|/proc/self/fd/*|-) _need_reexec=1 ;;
  esac
  if [[ "$_need_reexec" -eq 1 ]]; then
    _f="$(mktemp /tmp/pathweaver-ic.XXXXXX)"
    log "[pathweaver] 检测到管道执行，改为下载到 ${_f} 后重入..."
    curl -fsSL --connect-timeout 15 --max-time 60 \
      -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
      "${RAW_SELF}?$(date +%s)" -o "$_f" </dev/null
    chmod +x "$_f"
    export PW_IC_REEXEC=1
    exec bash "$_f" "$@"
  fi
fi

log "[pathweaver] install-controller.sh rev=${SCRIPT_REV}"

API="https://api.github.com/repos/${REPO}/releases/latest"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

PUBLIC_ADDR="${PW_PUBLIC_ADDRESS:-}"
ADMIN_PASS="${PW_INITIAL_ADMIN_PASSWORD:-}"
CTRL_NAME="${PW_CONTROLLER_NAME:-controller}"
EXTRA_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --public-address) PUBLIC_ADDR="$2"; shift 2 ;;
    --password) ADMIN_PASS="$2"; shift 2 ;;
    --name) CTRL_NAME="$2"; shift 2 ;;
    *) EXTRA_ARGS+=("$1"); shift ;;
  esac
done

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  log "请使用 sudo 运行"
  exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) log "不支持的架构: $ARCH"; exit 1 ;;
esac

ASSET="pathweaver-linux-${GOARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

log "[1/4] 解析下载地址: ${ASSET}"
API_JSON=$(curl -fsSL --connect-timeout 5 --max-time 15 \
  -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  -H 'Accept: application/vnd.github+json' \
  "${API}?$(date +%s)" </dev/null 2>/dev/null || true)
if [[ -n "$API_JSON" ]]; then
  API_URL=$(printf '%s' "$API_JSON" | sed -n "s/.*\"browser_download_url\": \"\\([^\"]*${ASSET}\\)\".*/\\1/p" | head -n1)
  if [[ -z "$API_URL" ]]; then
    API_URL=$(printf '%s' "$API_JSON" | sed -n "s/.*\"browser_download_url\": \"\\([^\"]*linux-${GOARCH}\\.tar\\.gz\\)\".*/\\1/p" | head -n1)
  fi
  if [[ -n "$API_URL" ]]; then
    DOWNLOAD_URL="$API_URL"
    log "  使用 GitHub API 资产 URL"
  else
    log "  API 无匹配资产，改用 latest/download 直链"
  fi
else
  log "  GitHub API 不可用/超时，改用 latest/download 直链"
fi

log "[2/4] 下载: $DOWNLOAD_URL"
curl -fL --connect-timeout 15 --max-time 600 --retry 3 --retry-delay 2 \
  -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  -o "$TMP/pathweaver.tar.gz" "$DOWNLOAD_URL" </dev/null

SIZE=$(wc -c < "$TMP/pathweaver.tar.gz" | tr -d ' ')
if [[ "$SIZE" -lt 1000000 ]]; then
  log "下载内容过小 (${SIZE} bytes)，可能命中错误页/旧缓存。请检查 Release："
  log "  https://github.com/${REPO}/releases/latest"
  exit 1
fi
log "  已下载 ${SIZE} bytes"

log "[3/4] 解压..."
tar -xzf "$TMP/pathweaver.tar.gz" -C "$TMP"
INSTALL_SH=$(find "$TMP" -maxdepth 3 -type f -name install.sh | head -n1)
[[ -n "$INSTALL_SH" ]] || { log "安装包内缺少 install.sh"; exit 1; }
PKG_DIR=$(dirname "$INSTALL_SH")

log "  同步最新 install.sh ..."
curl -fsSL --connect-timeout 10 --max-time 60 \
  -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/${REPO}/master/install.sh?$(date +%s)" \
  -o "$PKG_DIR/install.sh" </dev/null \
  || log "  警告: 无法同步 install.sh，将使用包内版本"

log "[4/4] 执行控制机安装..."
ARGS=(--role controller --name "$CTRL_NAME")
[[ -n "$PUBLIC_ADDR" ]] && ARGS+=(--public-address "$PUBLIC_ADDR")
[[ -n "$ADMIN_PASS" ]] && ARGS+=(--password "$ADMIN_PASS")
if [[ -n "$PUBLIC_ADDR" && -n "$ADMIN_PASS" ]]; then
  ARGS+=(--noninteractive)
fi
ARGS+=("${EXTRA_ARGS[@]}")

cd "$PKG_DIR"

if [[ ! -t 0 ]]; then
  if [[ -r /dev/tty ]]; then
    exec </dev/tty
  else
    log "无法打开 /dev/tty。请改用非交互参数：--public-address 与 --password"
    exit 1
  fi
fi

bash ./install.sh "${ARGS[@]}"
