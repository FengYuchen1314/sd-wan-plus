#!/usr/bin/env bash
# Build a single PathWeaver release package (controller + node share the same tarball).
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

GOOS=${GOOS:-linux}
GOARCH=${GOARCH:-amd64}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.1.0)}
OUT_DIR=${OUT_DIR:-dist}
PKG_NAME="pathweaver-${VERSION}-${GOOS}-${GOARCH}"
STAGE="$OUT_DIR/$PKG_NAME"

rm -rf "$STAGE"
mkdir -p "$STAGE/bin" "$STAGE/web" "$STAGE/packaging/systemd"

export CGO_ENABLED=0
for cmd in controller agent netd updater cli; do
  echo "building pathweaver-$cmd ($GOOS/$GOARCH)..."
  GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags="-s -w -X github.com/FengYuchen1314/sd-wan-plus/internal/core.ProductVersion=${VERSION}" \
    -o "$STAGE/bin/pathweaver-$cmd" "./cmd/pathweaver-$cmd"
done

if [[ -d web ]]; then
  if [[ ! -d web/dist ]] || [[ "${FORCE_WEB_BUILD:-0}" == "1" ]]; then
    (cd web && npm ci && npm run build)
  fi
  cp -a web/dist/. "$STAGE/web/"
fi

cp install.sh "$STAGE/install.sh"
chmod +x "$STAGE/install.sh" "$STAGE/bin/"*
cp packaging/systemd/*.service "$STAGE/packaging/systemd/" 2>/dev/null || true

# Convenience copies for artifact cache naming
cp "$STAGE/install.sh" "$STAGE/bin/install-node.sh"

mkdir -p "$OUT_DIR"
TAR="$OUT_DIR/${PKG_NAME}.tar.gz"
tar -C "$OUT_DIR" -czf "$TAR" "$PKG_NAME"
echo "SHA256 $(sha256sum "$TAR" | awk '{print $1}')  $TAR"
echo "built $TAR"
