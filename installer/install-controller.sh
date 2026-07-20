#!/bin/bash
# PathWeaver controller installer (Debian/Ubuntu)
set -euo pipefail

INSTALL_DIR=/opt/pathweaver
REPO_URL="${PW_REPO_URL:-https://github.com/FengYuchen1314/sd-wan-plus}"

echo "============================================"
echo "  PathWeaver 控制机安装"
echo "============================================"

if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 或 sudo 运行"
  exit 1
fi

if ! command -v systemctl >/dev/null; then
  echo "需要 systemd"
  exit 1
fi

read -rp "控制机名称 [controller]: " CTRL_NAME
CTRL_NAME=${CTRL_NAME:-controller}
while true; do
  read -rsp "管理员密码: " ADMIN_PASS; echo
  read -rsp "确认密码: " ADMIN_PASS2; echo
  [[ "$ADMIN_PASS" == "$ADMIN_PASS2" ]] && [[ ${#ADMIN_PASS} -ge 8 ]] && break
  echo "密码不一致或短于 8 位，请重试"
done
read -rp "Web 管理端口 [8443]: " WEB_PORT
WEB_PORT=${WEB_PORT:-8443}
read -rp "节点服务端口 [8444]: " NODE_PORT
NODE_PORT=${NODE_PORT:-8444}
read -rp "WireGuard 端口起始 [30000]: " WG_START
WG_START=${WG_START:-30000}
read -rp "WireGuard 端口结束 [30999]: " WG_END
WG_END=${WG_END:-30999}
read -rp "控制机公网地址: " PUBLIC_ADDR
[[ -z "$PUBLIC_ADDR" ]] && { echo "公网地址必填"; exit 1; }
read -rp "Overlay IPv4 网段 [10.250.0.0/16]: " OVERLAY
OVERLAY=${OVERLAY:-10.250.0.0/16}

apt-get update -y
apt-get install -y curl ca-certificates wireguard-tools nftables iproute2

mkdir -p "$INSTALL_DIR"/{bin,web,data,artifacts,releases}
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "不支持的架构: $ARCH"; exit 1 ;;
esac

# Prefer local release bundle if present next to script
SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
if [[ -f "$SCRIPT_DIR/../bin/pathweaver-controller" ]]; then
  cp "$SCRIPT_DIR/../bin/pathweaver-"* "$INSTALL_DIR/bin/" || true
  [[ -d "$SCRIPT_DIR/../web/dist" ]] && cp -r "$SCRIPT_DIR/../web/dist/"* "$INSTALL_DIR/web/" || true
else
  echo "请将预构建二进制放入 /opt/pathweaver/bin 或从源码构建后重试"
  echo "构建: go build -o bin/ ./cmd/..."
fi

chmod +x "$INSTALL_DIR/bin/"* 2>/dev/null || true

export PW_DATA_DIR="$INSTALL_DIR/data"
export PW_DB_PATH="$INSTALL_DIR/data/pathweaver.db"
export PW_WEB_PORT="$WEB_PORT"
export PW_NODE_PORT="$NODE_PORT"
export PW_WG_PORT_START="$WG_START"
export PW_WG_PORT_END="$WG_END"
export PW_PUBLIC_ADDRESS="$PUBLIC_ADDR"
export PW_OVERLAY_CIDR="$OVERLAY"
export PW_STATIC_DIR="$INSTALL_DIR/web"

"$INSTALL_DIR/bin/pathweaver-controller" --bootstrap --password "$ADMIN_PASS" --name "$CTRL_NAME"

cat > /etc/systemd/system/pathweaver.service <<EOF
[Unit]
Description=PathWeaver Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
Environment=PW_DATA_DIR=$INSTALL_DIR/data
Environment=PW_DB_PATH=$INSTALL_DIR/data/pathweaver.db
Environment=PW_WEB_PORT=$WEB_PORT
Environment=PW_NODE_PORT=$NODE_PORT
Environment=PW_WG_PORT_START=$WG_START
Environment=PW_WG_PORT_END=$WG_END
Environment=PW_PUBLIC_ADDRESS=$PUBLIC_ADDR
Environment=PW_OVERLAY_CIDR=$OVERLAY
Environment=PW_STATIC_DIR=$INSTALL_DIR/web
ExecStart=$INSTALL_DIR/bin/pathweaver-controller
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-netd.service <<EOF
[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=$INSTALL_DIR/bin/pathweaver-netd
Restart=always
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-agent.service <<EOF
[Unit]
Description=PathWeaver agent (controller node)
After=pathweaver.service pathweaver-netd.service
[Service]
Environment=PW_DATA_DIR=$INSTALL_DIR/data
Environment=PW_PARENT_URL=http://127.0.0.1:$WEB_PORT
EnvironmentFile=-$INSTALL_DIR/data/agent.env
ExecStart=$INSTALL_DIR/bin/pathweaver-agent
Restart=always
[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-updater.service <<EOF
[Unit]
Description=PathWeaver updater
[Service]
Environment=PW_ROOT=$INSTALL_DIR
ExecStart=$INSTALL_DIR/bin/pathweaver-updater
Restart=always
[Install]
WantedBy=multi-user.target
EOF

# Write controller node id for local agent after bootstrap
NODE_ID=$(sqlite3 "$INSTALL_DIR/data/pathweaver.db" "SELECT id FROM nodes WHERE is_controller=1 LIMIT 1;" 2>/dev/null || true)
if [[ -n "${NODE_ID:-}" ]]; then
  echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
  echo "PW_NODE_ID=$NODE_ID" > "$INSTALL_DIR/data/agent.env"
fi

systemctl daemon-reload
systemctl enable --now pathweaver pathweaver-netd pathweaver-agent pathweaver-updater

echo ""
echo "安装完成。访问: http://$PUBLIC_ADDR:$WEB_PORT"
echo "用户名: admin"
