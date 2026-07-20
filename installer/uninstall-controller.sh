#!/bin/bash
# 兼容入口：转发到本机已安装的卸载脚本（不联网）
set -euo pipefail
DIR=/opt/pathweaver
if [[ -f "$DIR/uninstall.sh" ]]; then
  exec bash "$DIR/uninstall.sh" "$@"
fi
if [[ -f "$(dirname "$0")/../scripts/uninstall.sh" ]]; then
  exec bash "$(dirname "$0")/../scripts/uninstall.sh" "$@"
fi
echo "未找到 $DIR/uninstall.sh，请用仓库内 scripts/uninstall.sh 在本机执行（无需联网）。" >&2
exit 1
