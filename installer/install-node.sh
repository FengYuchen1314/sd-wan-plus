#!/bin/bash
# 兼容入口：子节点应优先用父节点 bootstrap；本地有包时可用统一脚本
set -euo pipefail
DIR=$(cd "$(dirname "$0")/.." && pwd)
exec bash "$DIR/install.sh" --role node "$@"
