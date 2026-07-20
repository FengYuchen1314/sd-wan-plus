#!/bin/bash
# PathWeaver node installer is normally served by parent bootstrap URL.
# This file documents offline/manual node install.
set -euo pipefail
echo "请使用控制台生成的一次性安装命令："
echo '  curl -fsSL "http://<parent>:<port>/bootstrap/install.sh?token=<TOKEN>" | sudo bash'
exit 0
