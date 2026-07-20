package artifacts

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

// InstallScript returns a node installer that uses the unified package layout,
// pulling ALL files only from parent BASE (no GitHub).
func InstallScript(token, base, nodeName, version string, nodePort int) string {
	base = strings.TrimRight(base, "/")
	return fmt.Sprintf(`#!/bin/bash
set -euo pipefail
echo "PathWeaver 链式节点安装（统一安装包，仅从父节点拉取）"
TOKEN=%q
BASE=%q
NAME=%q
NODE_PORT=%d
INSTALL_DIR=/opt/pathweaver
mkdir -p "$INSTALL_DIR"/{bin,data,artifacts,releases}

fetch() {
  local name="$1" dest="$2"
  echo "[fetch] $BASE/bootstrap/artifact/$name"
  curl -fsSL "$BASE/bootstrap/artifact/$name" -o "$dest"
  chmod +x "$dest" 2>/dev/null || true
  cp -f "$dest" "$INSTALL_DIR/artifacts/$(basename "$dest")" 2>/dev/null || true
}

# 统一包内全部节点侧二进制（与主控同一套，角色由参数区分）
fetch pathweaver-agent      "$INSTALL_DIR/bin/pathweaver-agent"
fetch pathweaver-netd       "$INSTALL_DIR/bin/pathweaver-netd"
fetch pathweaver-updater    "$INSTALL_DIR/bin/pathweaver-updater"
fetch pathweaver-cli        "$INSTALL_DIR/bin/pathweaver-cli" || true
fetch pathweaver-controller "$INSTALL_DIR/bin/pathweaver-controller" || true
fetch install-node.sh       "$INSTALL_DIR/artifacts/install-node.sh" || true

# 若父节点已缓存完整 install.sh，优先用统一安装器
if [[ -x "$INSTALL_DIR/artifacts/install-node.sh" ]]; then
  exec bash "$INSTALL_DIR/artifacts/install-node.sh" --role node \
    --parent-url "$BASE" --token "$TOKEN" --name "$NAME" --node-port "$NODE_PORT"
fi

# 回退：内联最小安装（仍只从父节点取文件）
WG_PRIV=$( (wg genkey) 2>/dev/null || openssl rand -base64 32 )
WG_PUB=$( (echo "$WG_PRIV" | wg pubkey) 2>/dev/null || echo "$WG_PRIV" )
RESP=$(curl -fsSL -X POST "$BASE/bootstrap/enroll" -H 'Content-Type: application/json' \
  -d "{\"token\":\"$TOKEN\",\"node_name\":\"$NAME\",\"wg_public_key\":\"$WG_PUB\",\"identity_public_key\":\"$WG_PUB\",\"agent_version\":\"%s\",\"protocol_version\":1}")
NODE_ID=$(printf '%%s' "$RESP" | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')
[[ -n "$NODE_ID" ]] || { echo "enroll failed: $RESP"; exit 1; }
echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
umask 077; echo "$WG_PRIV" > "$INSTALL_DIR/data/wg_private.key"
echo -e "PW_NODE_ID=$NODE_ID\nPW_PARENT_URL=$BASE\nPW_SERVE_CHILDREN=1\nPW_NODE_PORT=$NODE_PORT" > "$INSTALL_DIR/data/agent.env"

cat > /etc/systemd/system/pathweaver-netd.service <<'UNIT'
[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=/opt/pathweaver/bin/pathweaver-netd
Restart=always
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/pathweaver-agent.service <<UNIT
[Unit]
Description=PathWeaver agent
After=pathweaver-netd.service
[Service]
Environment=PW_DATA_DIR=/opt/pathweaver/data
Environment=PW_ARTIFACT_DIR=/opt/pathweaver/artifacts
Environment=PW_ROOT=/opt/pathweaver
Environment=PW_NODE_PORT=$NODE_PORT
Environment=PW_SERVE_CHILDREN=1
EnvironmentFile=-/opt/pathweaver/data/agent.env
ExecStart=/opt/pathweaver/bin/pathweaver-agent
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
cat > /etc/systemd/system/pathweaver-updater.service <<'UNIT'
[Unit]
Description=PathWeaver updater
[Service]
Environment=PW_ROOT=/opt/pathweaver
Environment=PW_ARTIFACT_DIR=/opt/pathweaver/artifacts
EnvironmentFile=-/opt/pathweaver/data/agent.env
ExecStart=/opt/pathweaver/bin/pathweaver-updater
Restart=always
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now pathweaver-netd pathweaver-agent pathweaver-updater
curl -fsSL -X POST "$BASE/bootstrap/commit" -H 'Content-Type: application/json' \
  -d "{\"node_id\":\"$NODE_ID\",\"token\":\"$TOKEN\",\"overlay_reachable\":true}" || true
echo "节点就绪: $NODE_ID"
`, token, base, nodeName, nodePort, version)
}

func ReverseProxy(parentURL string) (*httputil.ReverseProxy, error) {
	u, err := url.Parse(strings.TrimRight(parentURL, "/"))
	if err != nil {
		return nil, err
	}
	return httputil.NewSingleHostReverseProxy(u), nil
}

func ProxyHandler(parentURL string) http.HandlerFunc {
	p, err := ReverseProxy(parentURL)
	if err != nil {
		return func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, err.Error(), 500)
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		p.ServeHTTP(w, r)
	}
}
