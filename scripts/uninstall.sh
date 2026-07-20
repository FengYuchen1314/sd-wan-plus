#!/bin/bash
# PathWeaver 完全卸载（控制机 / 子节点通用）— 纯本机，不联网
#
# 离线（推荐，安装时已写入本机）:
#   sudo bash /opt/pathweaver/uninstall.sh
#   sudo bash /opt/pathweaver/uninstall.sh --yes
#
# 有网时也可从仓库拉取（非必需）:
#   curl -fsSL .../scripts/uninstall.sh | sudo bash -s -- --yes
set -euo pipefail

YES=0
INSTALL_DIR=/opt/pathweaver

while [[ $# -gt 0 ]]; do
  case "$1" in
    -y|--yes) YES=1; shift ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    -h|--help)
      echo "用法: sudo bash uninstall.sh [--yes] [--install-dir DIR]"
      echo "默认安装目录: /opt/pathweaver（纯本机操作，不访问网络）"
      exit 0
      ;;
    *) echo "未知参数: $1"; exit 1 ;;
  esac
done

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "请使用 sudo 运行"
  exit 1
fi

# 若脚本位于安装目录内，先拷到 /tmp 再执行，避免删目录时踩掉自身
SCRIPT_SRC=${BASH_SOURCE[0]:-$0}
SCRIPT_PATH=$(readlink -f "$SCRIPT_SRC" 2>/dev/null || realpath "$SCRIPT_SRC" 2>/dev/null || echo "$SCRIPT_SRC")
case "$SCRIPT_PATH" in
  "$INSTALL_DIR"|"$INSTALL_DIR"/*)
    if [[ "${PW_UNINSTALL_REEXEC:-}" != "1" ]]; then
      tmp=$(mktemp /tmp/pathweaver-uninstall.XXXXXX)
      cp -f "$SCRIPT_PATH" "$tmp"
      chmod 700 "$tmp"
      args=()
      [[ "$YES" == "1" ]] && args+=(--yes)
      args+=(--install-dir "$INSTALL_DIR")
      export PW_UNINSTALL_REEXEC=1
      exec bash "$tmp" "${args[@]}"
    fi
    ;;
esac
cleanup_tmp() {
  if [[ "${PW_UNINSTALL_REEXEC:-}" == "1" && -n "${BASH_SOURCE[0]:-}" ]]; then
    case "${BASH_SOURCE[0]}" in
      /tmp/pathweaver-uninstall.*) rm -f "${BASH_SOURCE[0]}" 2>/dev/null || true ;;
    esac
  fi
}
trap cleanup_tmp EXIT

if [[ ! -t 0 ]] && [[ -r /dev/tty ]]; then
  exec </dev/tty
fi

echo "============================================"
echo "  PathWeaver 完全卸载（离线）"
echo "============================================"
echo "将停止并删除："
echo "  - systemd: pathweaver pathweaver-agent pathweaver-netd pathweaver-updater"
echo "  - 目录:    $INSTALL_DIR"
echo "  - 接口:    pw-lo 及 pwl-* WireGuard 接口"
echo "  - 本机所有 PathWeaver 数据（不可恢复）"
echo ""

if [[ "$YES" != "1" ]]; then
  read -rp "确认卸载？输入 yes 继续: " ans
  [[ "$ans" == "yes" ]] || { echo "已取消"; exit 0; }
fi

echo "[1/4] 停止服务..."
for u in pathweaver pathweaver-agent pathweaver-netd pathweaver-updater; do
  systemctl disable --now "$u" 2>/dev/null || true
  rm -f "/etc/systemd/system/${u}.service"
done
systemctl daemon-reload 2>/dev/null || true

echo "[2/4] 清理网络接口..."
if command -v ip >/dev/null 2>&1; then
  ip link del pw-lo 2>/dev/null || true
  for iface in $(ip -o link show 2>/dev/null | awk -F': ' '{print $2}' | grep -E '^pwl-' || true); do
    ip link del "$iface" 2>/dev/null || true
  done
fi

echo "[3/4] 删除安装目录 $INSTALL_DIR ..."
rm -rf "$INSTALL_DIR"

echo "[4/4] 清理残留..."
rm -f /run/pathweaver/netd.sock 2>/dev/null || true
rm -rf /run/pathweaver 2>/dev/null || true

echo ""
echo "PathWeaver 已完全卸载。"
