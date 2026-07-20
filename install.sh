#!/bin/bash
# Convenience wrapper — prefer installer/install-controller.sh
set -euo pipefail
DIR=$(cd "$(dirname "$0")" && pwd)
exec bash "$DIR/installer/install-controller.sh"
