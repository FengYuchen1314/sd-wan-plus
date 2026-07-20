#!/bin/bash
# 兼容入口：转发到统一安装包脚本
set -euo pipefail
DIR=$(cd "$(dirname "$0")/.." && pwd)
exec bash "$DIR/install.sh" --role controller "$@"
