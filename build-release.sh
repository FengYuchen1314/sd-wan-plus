#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")" && pwd)
cd "$ROOT"
mkdir -p bin
GOOS=${GOOS:-linux}
GOARCH=${GOARCH:-amd64}
export CGO_ENABLED=0
for cmd in controller agent netd updater cli; do
  echo "building pathweaver-$cmd ($GOOS/$GOARCH)..."
  GOOS=$GOOS GOARCH=$GOARCH go build -trimpath -ldflags="-s -w" \
    -o "bin/pathweaver-$cmd" "./cmd/pathweaver-$cmd"
done
(cd web && npm ci && npm run build)
echo "artifacts in bin/ and web/dist/"
