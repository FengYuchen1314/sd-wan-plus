#!/bin/bash
set -euo pipefail

# PathWeaver Production Build Script (Linux)
# Builds frontend + backend

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

echo "=== Building frontend ==="
cd web
npm install
npm run build
cd ..

echo "=== Building Rust backend ==="
cargo build --release -p pathweaver-controller

echo "=== Build complete ==="
echo ""
echo "Binary:  target/release/pathweaver-controller"
echo "Web:     web/dist/"
echo ""
echo "To run:"
echo "  PW_STATIC_DIR=web/dist ./target/release/pathweaver-controller"
