#!/bin/bash
set -euo pipefail

GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; }

[ "$(id -u)" -ne 0 ] && { err "请用 root 运行: sudo bash install.sh"; exit 1; }

port_free() {
    ss -tlnH 2>/dev/null | awk '{print $4}' | grep -qE ":${1}$" && return 1
    return 0
}
find_port() {
    local p=${1:-8443}
    for ((i=0; i<200; i++)); do
        [ $p -gt 65535 ] && p=1024
        port_free $p && { echo $p; return 0; }
        p=$((p + 1))
    done
    return 1
}
find_range() {
    local s=${1:-30000} n=${2:-50}
    local p=$s
    for ((i=0; i<60; i++)); do
        [ $((p + n)) -gt 65535 ] && p=1024
        local ok=1
        for ((j=0; j<n; j++)); do
            port_free $((p + j)) 2>/dev/null || { ok=0; break; }
        done
        [ $ok -eq 1 ] && { echo $p; return 0; }
        p=$((p + 20))
    done
    return 1
}

log "PathWeaver SD-WAN 控制机安装"

PUBLIC_IP=$(curl -s --max-time 5 ifconfig.me 2>/dev/null || curl -s --max-time 5 icanhazip.com 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')

WEB_PORT=$(find_port 8443) || { err "无可用 TCP 端口"; exit 1; }
WG_START=$(find_range 30000 50) || WG_START=$(find_range 20000 50)
[ -z "$WG_START" ] && { err "无可用连续 UDP 端口"; exit 1; }
PASS=$(head -c 12 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 14)

echo ""
echo -e "${CYAN}══════════════════════════════════${NC}"
echo -e "  公网地址:  ${GREEN}$PUBLIC_IP${NC}"
echo -e "  Web 端口:  ${GREEN}$WEB_PORT${NC}"
echo -e "  WG 端口池: ${GREEN}$WG_START-$((WG_START + 49))${NC}"
echo -e "  管理员密码: ${GREEN}$PASS${NC}"
echo -e "${CYAN}══════════════════════════════════${NC}"
echo ""
read -rp "确认继续? [Y/n] " yn; [ "$yn" = "n" ] && exit 0

log "安装系统依赖..."
export DEBIAN_FRONTEND=noninteractive
echo "  更新软件包列表..."
apt-get update -qq -o Acquire::Retries=2 -o Acquire::http::Timeout=15 -o Acquire::https::Timeout=15 || warn "apt update 失败，继续尝试安装"
echo "  安装: python3 python3-pip python3-venv curl wireguard-tools nftables"
apt-get install -y -qq --no-install-recommends python3 python3-pip python3-venv curl wireguard-tools nftables || {
    err "系统依赖安装失败，请检查网络和 apt 源"
    exit 1
}

INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR/deps_cache"

log "下载 PathWeaver..."
REPO="https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master"
curl -fsSL "$REPO/server/main.py" -o "$INSTALL_DIR/main.py"
curl -fsSL "$REPO/server/requirements.txt" -o "$INSTALL_DIR/requirements.txt"

log "安装 Python 环境..."
cd "$INSTALL_DIR"
python3 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt -q

log "缓存 Python 依赖 (子节点将从此处链式下载)..."
pip download -d "$INSTALL_DIR/deps_cache" --only-binary :all: \
    --platform manylinux2014_x86_64 --python-version 311 \
    fastapi "uvicorn[standard]" argon2-cffi cryptography python-multipart websockets 2>/dev/null || \
pip download -d "$INSTALL_DIR/deps_cache" fastapi uvicorn argon2-cffi cryptography python-multipart websockets 2>/dev/null || true
log "依赖缓存: $(find "$INSTALL_DIR/deps_cache" -name '*.whl' | wc -l) 个包"

cat > "$INSTALL_DIR/.env" << EOF
PW_WEB_PORT=$WEB_PORT
PW_WG_PORT_START=$WG_START
PW_WG_PORT_END=$((WG_START + 49))
PW_PUBLIC_ADDRESS=$PUBLIC_IP
PW_INITIAL_ADMIN_PASSWORD=$PASS
EOF

cat > /etc/systemd/system/pathweaver.service << EOF
[Unit]
Description=PathWeaver SD-WAN Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/.venv/bin/python3 $INSTALL_DIR/main.py
EnvironmentFile=$INSTALL_DIR/.env
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

for tool in ufw nft iptables; do
    command -v $tool &>/dev/null || continue
    case $tool in
        ufw) ufw allow $WEB_PORT/tcp comment 'PathWeaver' 2>/dev/null || true
             ufw allow $WG_START:$((WG_START + 49))/udp comment 'PathWeaver WG' 2>/dev/null || true;;
        nft) nft add table inet pathweaver 2>/dev/null || true
             nft add chain inet pathweaver input '{ type filter hook input priority 0; }' 2>/dev/null || true
             nft add rule inet pathweaver input tcp dport $WEB_PORT accept 2>/dev/null || true
             nft add rule inet pathweaver input udp dport $WG_START-$((WG_START + 49)) accept 2>/dev/null || true;;
    esac
    break
done

systemctl daemon-reload
systemctl enable pathweaver
systemctl start pathweaver
sleep 3

if systemctl is-active --quiet pathweaver; then
    echo ""
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo -e "${GREEN}          PathWeaver 安装完成${NC}"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo -e "  ${GREEN}面板: http://$PUBLIC_IP:$WEB_PORT${NC}"
    echo -e "  用户名: ${GREEN}admin${NC}"
    echo -e "  密码:   ${GREEN}$PASS${NC}"
    echo ""
    echo -e "  日志: journalctl -u pathweaver -f"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
else
    err "启动失败，查看日志: journalctl -u pathweaver -n 50"
    exit 1
fi
