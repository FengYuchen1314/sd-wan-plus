#!/bin/bash
# PathWeaver 统一安装脚本 — 主控与子节点使用同一安装包，仅参数不同。
# 用法:
#   控制机: sudo bash install.sh --role controller
#   子节点: sudo bash install.sh --role node --parent-url http://PARENT:14302 --token TOKEN [--name NAME]
set -euo pipefail

ROLE=""
PARENT_URL=""
TOKEN=""
NODE_NAME="node"
CTRL_NAME="controller"
WEB_PORT=14301
NODE_PORT=14302
WG_START=14303
WG_END=14399
PUBLIC_ADDR=""
OVERLAY="10.250.0.0/16"
ADMIN_PASS=""
INSTALL_DIR=/opt/pathweaver
NONINTERACTIVE=0

usage() {
  cat <<EOF
PathWeaver 统一安装包

  sudo bash install.sh --role controller [选项]
  sudo bash install.sh --role node --parent-url URL --token TOKEN [选项]

控制机选项:
  --name NAME              控制机显示名 (默认 controller)
  --password PASS          管理员密码 (否则交互输入)
  --public-address ADDR    公网 IP/域名 (必填，或交互输入)
  --web-port N             默认 14301
  --node-port N            默认 14302
  --wg-start N             默认 14303
  --wg-end N               默认 14399
  --overlay CIDR           默认 10.250.0.0/16
  --noninteractive         非交互（需提供 --password 与 --public-address）

子节点选项:
  --parent-url URL         父节点 bootstrap 地址，如 http://10.0.0.1:14302
  --token TOKEN            一次性接入 Token
  --name NAME              节点显示名
  --node-port N            本机节点服务端口 (默认 14302)

说明:
  - 主控与子节点二进制完全相同，角色由 --role 决定
  - 子节点安装时全部文件只从 --parent-url 拉取（链式安装），不访问 GitHub
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --role) ROLE="$2"; shift 2 ;;
    --parent-url) PARENT_URL="$2"; shift 2 ;;
    --token) TOKEN="$2"; shift 2 ;;
    --name) NODE_NAME="$2"; CTRL_NAME="$2"; shift 2 ;;
    --password) ADMIN_PASS="$2"; shift 2 ;;
    --public-address) PUBLIC_ADDR="$2"; shift 2 ;;
    --web-port) WEB_PORT="$2"; shift 2 ;;
    --node-port) NODE_PORT="$2"; shift 2 ;;
    --wg-start) WG_START="$2"; shift 2 ;;
    --wg-end) WG_END="$2"; shift 2 ;;
    --overlay) OVERLAY="$2"; shift 2 ;;
    --noninteractive) NONINTERACTIVE=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "未知参数: $1"; usage; exit 1 ;;
  esac
done

if [[ $EUID -ne 0 ]]; then
  echo "请使用 root 或 sudo 运行"
  exit 1
fi
if ! command -v systemctl >/dev/null; then
  echo "需要 systemd"
  exit 1
fi
if [[ -z "$ROLE" ]]; then
  echo "必须指定 --role controller|node"
  usage
  exit 1
fi

# curl|bash 时 stdin 是管道，交互 read 会立刻 EOF 退出；改从终端读入
ensure_tty() {
  if [[ -t 0 ]]; then
    return 0
  fi
  if [[ -r /dev/tty ]]; then
    exec </dev/tty
    return 0
  fi
  return 1
}

ask() {
  # ask "提示" VAR_NAME [secret]
  local prompt=$1
  local __var=$2
  local secret=${3:-0}
  local __val=""
  if [[ "$secret" == "1" ]]; then
    read -rsp "$prompt" __val
    echo
  else
    read -rp "$prompt" __val
  fi
  printf -v "$__var" '%s' "$__val"
}

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
# 安装包根目录：脚本旁、解压目录、或链式安装已拉取到的 /opt/pathweaver
PKG_ROOT="$SCRIPT_DIR"
if [[ -x "$INSTALL_DIR/bin/pathweaver-agent" ]]; then
  PKG_ROOT="$INSTALL_DIR"
elif [[ -x "$SCRIPT_DIR/bin/pathweaver-agent" ]]; then
  PKG_ROOT="$SCRIPT_DIR"
elif [[ -x "$SCRIPT_DIR/pathweaver/bin/pathweaver-agent" ]]; then
  PKG_ROOT="$SCRIPT_DIR/pathweaver"
elif [[ -x "$SCRIPT_DIR/../bin/pathweaver-agent" ]]; then
  PKG_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
fi

need_bins=(pathweaver-agent pathweaver-netd pathweaver-updater pathweaver-cli)
if [[ "$ROLE" == "controller" ]]; then
  need_bins+=(pathweaver-controller)
fi
for b in "${need_bins[@]}"; do
  if [[ ! -f "$PKG_ROOT/bin/$b" ]]; then
    echo "缺少二进制: $PKG_ROOT/bin/$b （请使用 GitHub Release 安装包，或确认父节点制品缓存完整）"
    exit 1
  fi
done

export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get install -y curl ca-certificates wireguard-tools nftables iproute2 sqlite3 python3

install_common_files() {
  mkdir -p "$INSTALL_DIR"/{bin,web,data,artifacts,releases}
  cp -f "$PKG_ROOT/bin/"* "$INSTALL_DIR/bin/"
  chmod +x "$INSTALL_DIR/bin/"*
  if [[ -d "$PKG_ROOT/web" ]]; then
    mkdir -p "$INSTALL_DIR/web"
    cp -a "$PKG_ROOT/web/." "$INSTALL_DIR/web/"
  fi
  # 制品缓存：本包全部二进制，便于作为父节点向下游分发
  cp -f "$INSTALL_DIR/bin/"* "$INSTALL_DIR/artifacts/"
  cp -f "$PKG_ROOT/install.sh" "$INSTALL_DIR/artifacts/install-node.sh" 2>/dev/null || true
}

write_unit() {
  local name="$1" content="$2"
  echo "$content" > "/etc/systemd/system/$name"
}

install_controller() {
  echo "============================================"
  echo "  PathWeaver 控制机安装（统一安装包）"
  echo "============================================"

  if [[ "$NONINTERACTIVE" != "1" ]]; then
    if ! ensure_tty; then
      echo "当前无法交互（例如 curl|bash 且无终端）。请改用："
      echo "  curl -fsSL ... | sudo bash -s -- --public-address IP --password 'SECRET'"
      exit 1
    fi
    if [[ -z "$ADMIN_PASS" ]]; then
      while true; do
        ask "管理员密码: " ADMIN_PASS 1
        ask "确认密码: " ADMIN_PASS2 1
        [[ "$ADMIN_PASS" == "$ADMIN_PASS2" ]] && [[ ${#ADMIN_PASS} -ge 8 ]] && break
        echo "密码不一致或短于 8 位"
      done
    fi
    if [[ -z "$PUBLIC_ADDR" ]]; then
      ask "控制机公网地址: " PUBLIC_ADDR
    fi
    ask "控制机名称 [$CTRL_NAME]: " _n
    CTRL_NAME=${_n:-$CTRL_NAME}
    ask "Web 端口 [$WEB_PORT]: " _p
    WEB_PORT=${_p:-$WEB_PORT}
    ask "节点服务端口 [$NODE_PORT]: " _p
    NODE_PORT=${_p:-$NODE_PORT}
    ask "WG 起始 [$WG_START]: " _p
    WG_START=${_p:-$WG_START}
    ask "WG 结束 [$WG_END]: " _p
    WG_END=${_p:-$WG_END}
    ask "Overlay [$OVERLAY]: " _p
    OVERLAY=${_p:-$OVERLAY}
  fi

  [[ -n "$ADMIN_PASS" ]] || { echo "需要 --password"; exit 1; }
  [[ -n "$PUBLIC_ADDR" ]] || { echo "需要 --public-address"; exit 1; }

  install_common_files

  export PW_DATA_DIR="$INSTALL_DIR/data"
  export PW_DB_PATH="$INSTALL_DIR/data/pathweaver.db"
  export PW_KEY_FILE="$INSTALL_DIR/data/.pathweaver.key"
  export PW_WEB_PORT="$WEB_PORT"
  export PW_NODE_PORT="$NODE_PORT"
  export PW_WG_PORT_START="$WG_START"
  export PW_WG_PORT_END="$WG_END"
  export PW_PUBLIC_ADDRESS="$PUBLIC_ADDR"
  export PW_OVERLAY_CIDR="$OVERLAY"
  export PW_STATIC_DIR="$INSTALL_DIR/web"

  # bootstrap 必须在 INSTALL_DIR 下跑，避免相对路径写到解压临时目录
  cd "$INSTALL_DIR"
  "$INSTALL_DIR/bin/pathweaver-controller" --bootstrap --password "$ADMIN_PASS" --name "$CTRL_NAME"

  write_unit pathweaver.service "[Unit]
Description=PathWeaver Controller
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
Environment=PW_DATA_DIR=$INSTALL_DIR/data
Environment=PW_DB_PATH=$INSTALL_DIR/data/pathweaver.db
Environment=PW_KEY_FILE=$INSTALL_DIR/data/.pathweaver.key
Environment=PW_WEB_PORT=$WEB_PORT
Environment=PW_NODE_PORT=$NODE_PORT
Environment=PW_WG_PORT_START=$WG_START
Environment=PW_WG_PORT_END=$WG_END
Environment=PW_PUBLIC_ADDRESS=$PUBLIC_ADDR
Environment=PW_OVERLAY_CIDR=$OVERLAY
Environment=PW_STATIC_DIR=$INSTALL_DIR/web
Environment=PW_ARTIFACT_DIR=$INSTALL_DIR/artifacts
ExecStart=$INSTALL_DIR/bin/pathweaver-controller
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target"

  write_unit pathweaver-netd.service "[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=$INSTALL_DIR/bin/pathweaver-netd
Restart=always
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
[Install]
WantedBy=multi-user.target"

  NODE_ID=""
  if [[ -f "$INSTALL_DIR/data/node_id" ]]; then
    NODE_ID=$(tr -d ' \t\r\n' < "$INSTALL_DIR/data/node_id" || true)
  fi
  if [[ -z "${NODE_ID}" ]] && command -v sqlite3 >/dev/null 2>&1; then
    NODE_ID=$(sqlite3 "$INSTALL_DIR/data/pathweaver.db" "SELECT id FROM nodes WHERE is_controller=1 LIMIT 1;" 2>/dev/null || true)
  fi
  if [[ -z "${NODE_ID}" ]] && command -v python3 >/dev/null 2>&1; then
    NODE_ID=$(PW_DB="$INSTALL_DIR/data/pathweaver.db" python3 - <<'PY' 2>/dev/null || true
import os, sqlite3
db=sqlite3.connect(os.environ["PW_DB"])
row=db.execute("SELECT id FROM nodes WHERE is_controller=1 LIMIT 1").fetchone()
print(row[0] if row else "")
PY
)
  fi
  if [[ -z "${NODE_ID}" ]]; then
    echo "错误: 无法读取控制机 node_id（agent 将无法上线）。请检查 $INSTALL_DIR/data/node_id"
    exit 1
  fi
  echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
  cat > "$INSTALL_DIR/data/agent.env" <<EOF
PW_NODE_ID=$NODE_ID
PW_PARENT_URL=http://127.0.0.1:$WEB_PORT
PW_SERVE_CHILDREN=0
EOF
  echo "本机 Agent node_id=$NODE_ID"

  write_unit pathweaver-agent.service "[Unit]
Description=PathWeaver agent (controller node)
After=pathweaver.service pathweaver-netd.service
[Service]
Environment=PW_DATA_DIR=$INSTALL_DIR/data
Environment=PW_ARTIFACT_DIR=$INSTALL_DIR/artifacts
Environment=PW_ROOT=$INSTALL_DIR
Environment=PW_SERVE_CHILDREN=0
Environment=PW_NODE_PORT=$NODE_PORT
EnvironmentFile=-$INSTALL_DIR/data/agent.env
ExecStart=$INSTALL_DIR/bin/pathweaver-agent
Restart=always
[Install]
WantedBy=multi-user.target"

  write_unit pathweaver-updater.service "[Unit]
Description=PathWeaver updater
[Service]
Environment=PW_ROOT=$INSTALL_DIR
Environment=PW_ARTIFACT_DIR=$INSTALL_DIR/artifacts
ExecStart=$INSTALL_DIR/bin/pathweaver-updater
Restart=always
[Install]
WantedBy=multi-user.target"

  systemctl daemon-reload
  systemctl enable --now pathweaver pathweaver-netd pathweaver-agent pathweaver-updater
  echo ""
  echo "控制机安装完成: http://$PUBLIC_ADDR:$WEB_PORT  用户 admin"
}

install_node() {
  echo "============================================"
  echo "  PathWeaver 子节点安装（统一安装包 / 链式）"
  echo "============================================"

  [[ -n "$PARENT_URL" ]] || { echo "子节点需要 --parent-url"; exit 1; }
  [[ -n "$TOKEN" ]] || { echo "子节点需要 --token"; exit 1; }
  PARENT_URL="${PARENT_URL%/}"

  # 若当前目录已是完整包则本地安装；否则强制从父节点拉齐制品
  install_common_files

  echo "[chain] 校验/刷新制品（仅从父节点）..."
  for b in pathweaver-agent pathweaver-netd pathweaver-updater pathweaver-cli; do
    echo "  fetch $b"
    curl -fsSL "$PARENT_URL/bootstrap/artifact/$b" -o "$INSTALL_DIR/bin/$b"
    chmod +x "$INSTALL_DIR/bin/$b"
    cp -f "$INSTALL_DIR/bin/$b" "$INSTALL_DIR/artifacts/$b"
  done
  # 统一安装脚本也缓存，便于继续作为下一级父节点
  curl -fsSL "$PARENT_URL/bootstrap/artifact/install-node.sh" -o "$INSTALL_DIR/artifacts/install-node.sh" 2>/dev/null || \
    cp -f "$PKG_ROOT/install.sh" "$INSTALL_DIR/artifacts/install-node.sh" || true

  WG_PRIV=$( (wg genkey) 2>/dev/null || openssl rand -base64 32 )
  WG_PUB=$( (echo "$WG_PRIV" | wg pubkey) 2>/dev/null || echo "$WG_PRIV" )
  RESP=$(curl -fsSL -X POST "$PARENT_URL/bootstrap/enroll" -H 'Content-Type: application/json' \
    -d "{\"token\":\"$TOKEN\",\"node_name\":\"$NODE_NAME\",\"wg_public_key\":\"$WG_PUB\",\"identity_public_key\":\"$WG_PUB\",\"agent_version\":\"0.1.0\",\"protocol_version\":1}")
  echo "$RESP" > "$INSTALL_DIR/data/enroll.json"
  NODE_ID=$(printf '%s' "$RESP" | sed -n 's/.*"node_id":"\([^"]*\)".*/\1/p')
  [[ -n "$NODE_ID" ]] || { echo "enroll 失败: $RESP"; exit 1; }
  echo "$NODE_ID" > "$INSTALL_DIR/data/node_id"
  umask 077; echo "$WG_PRIV" > "$INSTALL_DIR/data/wg_private.key"

  cat > "$INSTALL_DIR/data/agent.env" <<EOF
PW_NODE_ID=$NODE_ID
PW_PARENT_URL=$PARENT_URL
PW_SERVE_CHILDREN=1
PW_NODE_PORT=$NODE_PORT
EOF

  write_unit pathweaver-netd.service "[Unit]
Description=PathWeaver netd
After=network-online.target
[Service]
ExecStart=$INSTALL_DIR/bin/pathweaver-netd
Restart=always
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
[Install]
WantedBy=multi-user.target"

  write_unit pathweaver-agent.service "[Unit]
Description=PathWeaver agent
After=pathweaver-netd.service
[Service]
Environment=PW_DATA_DIR=$INSTALL_DIR/data
Environment=PW_ARTIFACT_DIR=$INSTALL_DIR/artifacts
Environment=PW_ROOT=$INSTALL_DIR
Environment=PW_NODE_PORT=$NODE_PORT
Environment=PW_SERVE_CHILDREN=1
EnvironmentFile=-$INSTALL_DIR/data/agent.env
ExecStart=$INSTALL_DIR/bin/pathweaver-agent
Restart=always
[Install]
WantedBy=multi-user.target"

  write_unit pathweaver-updater.service "[Unit]
Description=PathWeaver updater
[Service]
Environment=PW_ROOT=$INSTALL_DIR
Environment=PW_ARTIFACT_DIR=$INSTALL_DIR/artifacts
EnvironmentFile=-$INSTALL_DIR/data/agent.env
ExecStart=$INSTALL_DIR/bin/pathweaver-updater
Restart=always
[Install]
WantedBy=multi-user.target"

  systemctl daemon-reload
  systemctl enable --now pathweaver-netd pathweaver-agent pathweaver-updater

  curl -fsSL -X POST "$PARENT_URL/bootstrap/commit" -H 'Content-Type: application/json' \
    -d "{\"node_id\":\"$NODE_ID\",\"token\":\"$TOKEN\",\"overlay_reachable\":true}" || true

  echo "子节点安装完成 node_id=$NODE_ID（制品仅来自 $PARENT_URL）"
}

case "$ROLE" in
  controller) install_controller ;;
  node) install_node ;;
  *) echo "role 必须是 controller 或 node"; exit 1 ;;
esac
