#!/bin/bash
# PathWeaver 控制机一键安装
# 用法（交互）:
#   curl -fsSL https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh | sudo bash
# 用法（非交互）:
#   curl -fsSL ... | sudo bash -s -- --public-address 1.2.3.4 --password 'secret'
set -euo pipefail

REPO="${PW_REPO:-FengYuchen1314/sd-wan-plus}"
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
  echo "请使用 sudo 运行"
  exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

ASSET="pathweaver-linux-${GOARCH}.tar.gz"
echo "[1/4] 获取 Latest Release 中的 ${ASSET} ..."

# Prefer stable asset name from latest release; fallback to GitHub API browser_download_url match
DOWNLOAD_URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"
if ! curl -fsI "$DOWNLOAD_URL" >/dev/null 2>&1; then
  echo "稳定资源名暂不可用，尝试从 API 解析..."
  DOWNLOAD_URL=$(curl -fsSL "$API" | sed -n "s/.*\"browser_download_url\": \"\\([^\"]*linux-${GOARCH}\\.tar\\.gz\\)\".*/\\1/p" | head -n1)
fi
[[ -n "${DOWNLOAD_URL:-}" ]] || { echo "未找到 linux-${GOARCH} 安装包，请确认 Release 已发布: https://github.com/${REPO}/releases/latest"; exit 1; }

echo "[2/4] 下载: $DOWNLOAD_URL"
curl -fL --retry 3 --retry-delay 2 -o "$TMP/pathweaver.tar.gz" "$DOWNLOAD_URL"

echo "[3/4] 解压..."
tar -xzf "$TMP/pathweaver.tar.gz" -C "$TMP"
find "$TMP" -maxdepth 2 -type f -name install.sh | head -n1 | xargs -I{} dirname {}
PKG_DIR=$(find "$TMP" -maxdepth 2 -type f -name install.sh | head -n1 | xargs -r dirname)

[[ -n "$PKG_DIR" ]] || { echo "安装包内缺少 install.sh"; exit 1; }

echo "[4/4] 执行控制机安装..."
ARGS=(--role controller --name "$CTRL_NAME")
[[ -n "$PUBLIC_ADDR" ]] && ARGS+=(--public-address "$PUBLIC_ADDR")
[[ -n "$ADMIN_PASS" ]] && ARGS+=(--password "$ADMIN_PASS")
if [[ -n "$PUBLIC_ADDR" && -n "$ADMIN_PASS" ]]; then
  ARGS+=(--noninteractive)
fi
ARGS+=("${EXTRA_ARGS[@]}")

cd "$PKG_DIR"
bash ./install.sh "${ARGS[@]}"
