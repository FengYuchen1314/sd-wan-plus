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
  local optional="${3:-0}"
  echo "[fetch] $BASE/bootstrap/artifact/$name"
  mkdir -p "$(dirname "$dest")"
  if ! curl -fL --retry 3 --retry-delay 1 --connect-timeout 15 --max-time 300 \
      "$BASE/bootstrap/artifact/$name" -o "$dest.tmp"; then
    rm -f "$dest.tmp"
    if [[ "$optional" == "1" ]]; then
      echo "  (可选) $name 不可用，跳过"
      return 0
    fi
    echo "拉取失败: $name"
    return 1
  fi
  mv -f "$dest.tmp" "$dest"
  chmod +x "$dest" 2>/dev/null || true
  cp -f "$dest" "$INSTALL_DIR/artifacts/$(basename "$dest")" 2>/dev/null || true
}

# 统一包内节点侧二进制（与主控同一套，角色由参数区分）
fetch pathweaver-agent      "$INSTALL_DIR/bin/pathweaver-agent"
fetch pathweaver-netd       "$INSTALL_DIR/bin/pathweaver-netd"
fetch pathweaver-updater    "$INSTALL_DIR/bin/pathweaver-updater"
fetch pathweaver-cli        "$INSTALL_DIR/bin/pathweaver-cli" 1
fetch pathweaver-controller "$INSTALL_DIR/bin/pathweaver-controller" 1
# 缓存 install 脚本供本节点继续做父节点；链式首装不再 exec 它（避免旧脚本自拷贝退出）
fetch install-node.sh       "$INSTALL_DIR/artifacts/install-node.sh" 1

# 交互：从真实终端读入（curl|bash 时 stdin 是管道）
if [[ ! -t 0 ]] && [[ -r /dev/tty ]]; then
  exec </dev/tty
fi
ask() { local p="$1" v="$2" s="${3:-0}" x=""; if [[ "$s" == "1" ]]; then read -rsp "$p" x; echo; else read -rp "$p" x; fi; printf -v "$v" '%%s' "$x"; }
detect_public_ip() {
  local ip="" u
  for u in https://api.ipify.org https://ifconfig.me/ip https://icanhazip.com; do
    ip=$(curl -fsS --connect-timeout 3 --max-time 5 "$u" 2>/dev/null | tr -d ' \t\r\n' || true)
    if [[ "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then printf '%%s' "$ip"; return 0; fi
  done
  return 1
}
detect_lan_ip() {
  ip -4 -o addr show scope global 2>/dev/null | awk '{print $4}' | cut -d/ -f1 | head -n1
}

if [[ "$NAME" == "node" || -z "$NAME" ]]; then
  ask "节点名称 [node]: " _n
  NAME=${_n:-node}
fi
echo ""
echo "本节点是否有公网 IP？"
echo "  - 有：下级可通过公网接入本节点"
echo "  - 无：填写内网 IP（同网段或可路由到本机的设备可接入）"
while true; do
  ask "有公网 IP？ [y/N]: " _ans
  _ans_l=$(printf '%%s' "$_ans" | tr '[:upper:]' '[:lower:]')
  case "$_ans_l" in
    y|yes) HAS_PUBLIC=1; ADDR_TYPE=public; break ;;
    n|no|"") HAS_PUBLIC=0; ADDR_TYPE=lan; break ;;
    *) echo "请输入 y 或 n" ;;
  esac
done
if [[ "$HAS_PUBLIC" == "1" ]]; then
  echo "正在探测公网 IP..."
  DET=$(detect_public_ip || true)
  ask "本节点公网地址 ${DET:+[$DET] }（回车确认或手动输入）: " _a
  ADVERTISE_ADDR=${_a:-$DET}
else
  DET=$(detect_lan_ip || true)
  echo "探测到内网 IP: ${DET:-无}"
  ask "本节点内网地址 ${DET:+[$DET] }（回车确认或手动输入）: " _a
  ADVERTISE_ADDR=${_a:-$DET}
fi
[[ -n "$ADVERTISE_ADDR" ]] || { echo "必须填写本机可达地址"; exit 1; }

echo "[enroll] 向父节点注册 (advertise=$ADVERTISE_ADDR type=$ADDR_TYPE)..."
WG_PRIV=$( (wg genkey) 2>/dev/null || openssl rand -base64 32 )
WG_PUB=$( (echo "$WG_PRIV" | wg pubkey) 2>/dev/null || echo "$WG_PRIV" )
umask 077; echo "$WG_PRIV" > "$INSTALL_DIR/data/wg_private.key"
RESP=$(
  PW_BASE="$BASE" PW_TOKEN="$TOKEN" PW_NAME="$NAME" PW_WG_PUB="$WG_PUB" PW_WG_PRIV="$WG_PRIV" PW_VER="%s" \
  PW_ADV_ADDR="$ADVERTISE_ADDR" PW_ADDR_TYPE="$ADDR_TYPE" \
  python3 - <<'PY'
import json, os, urllib.request
payload = {
  "token": os.environ["PW_TOKEN"],
  "node_name": os.environ["PW_NAME"],
  "wg_public_key": os.environ["PW_WG_PUB"],
  "wg_private_key": os.environ["PW_WG_PRIV"],
  "identity_public_key": os.environ["PW_WG_PUB"],
  "agent_version": os.environ.get("PW_VER", "0.1.0"),
  "protocol_version": 1,
  "advertise_address": os.environ.get("PW_ADV_ADDR", ""),
  "address_type": os.environ.get("PW_ADDR_TYPE", "lan"),
  "has_public_ip": os.environ.get("PW_ADDR_TYPE", "lan") == "public",
}
req = urllib.request.Request(
  os.environ["PW_BASE"].rstrip("/") + "/bootstrap/enroll",
  data=json.dumps(payload).encode(),
  headers={"Content-Type": "application/json"},
  method="POST",
)
print(urllib.request.urlopen(req, timeout=60).read().decode())
PY
)
NODE_ID=$(printf '%%s' "$RESP" | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')
[[ -n "$NODE_ID" ]] || { echo "enroll failed: $RESP"; exit 1; }
echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
echo "$ADVERTISE_ADDR" > "$INSTALL_DIR/data/advertise_address"
echo "$ADDR_TYPE" > "$INSTALL_DIR/data/address_type"
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
