#!/bin/bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; }

if [ "$(id -u)" -ne 0 ]; then err "请使用 root 权限: sudo bash install.sh"; exit 1; fi

ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH="x86_64";; aarch64) ARCH="aarch64";; *) err "不支持的架构: $ARCH"; exit 1;; esac

log "PathWeaver SD-WAN 一键安装 — $ARCH"

# ─── 端口工具函数 ───

port_is_free() {
    local port=$1 proto=$2
    if ss -"${proto:0:1}"ln 2>/dev/null | grep -qE "[[:space:]]$port[[:space:]]"; then return 1; fi
    return 0
}

find_free_port() {
    local start=$1 proto=$2 limit=${3:-100}
    local port=$start
    for ((i=0; i<limit; i++)); do
        [ $port -gt 65535 ] && port=1024
        if port_is_free $port "$proto"; then echo $port; return 0; fi
        port=$((port + 1))
    done
    return 1
}

find_free_port_range() {
    local start=$1 count=$2 limit=${3:-40}
    local port=$start
    for ((i=0; i<limit; i++)); do
        [ $((port + count)) -gt 65535 ] && port=1024
        local ok=1
        for ((j=0; j<count; j++)); do
            if ! port_is_free $((port + j)) udp; then ok=0; break; fi
        done
        if [ $ok -eq 1 ]; then echo $port; return 0; fi
        port=$((port + 20))
    done
    return 1
}

get_public_ip() {
    curl -s --max-time 5 ifconfig.me 2>/dev/null \
      || curl -s --max-time 5 icanhazip.com 2>/dev/null \
      || curl -s --max-time 5 api.ipify.org 2>/dev/null \
      || echo ""
}

# 外部端口可达性验证：启动临时 HTTP 监听，用公网服务回连检测
verify_port_reachable() {
    local port=$1 pubip=$2

    log "  验证端口 $port 公网可达性..."

    python3 -c "
import socket, sys
s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
s.bind(('0.0.0.0', $port))
s.listen(1)
s.settimeout(8)
try:
    conn, addr = s.accept()
    conn.sendall(b'HTTP/1.1 200 OK\r\nContent-Length: 6\r\n\r\nPW-OK\r\n')
    conn.close()
except:
    pass
s.close()
" & local PID=$!

    sleep 1

    local reachable=1
    # 方法1: 用 curl 的公网反射检测
    local RESULT=$(curl -s --max-time 6 "https://portchecker.co/check" \
        -H "Content-Type: application/x-www-form-urlencoded" \
        -d "target=$pubip&port=$port" 2>/dev/null || true)
    if echo "$RESULT" | grep -qi "open\|success\|reachable"; then
        reachable=0
    fi

    # 方法2: 用另一公网服务
    if [ $reachable -ne 0 ]; then
        local R2=$(curl -s --max-time 6 "https://api.portcheckers.com/$pubip/$port" 2>/dev/null || true)
        if echo "$R2" | grep -qi '"open":true\|"reachable":true'; then reachable=0; fi
    fi

    # 方法3: 用 yougetsignal 的端口检测
    if [ $reachable -ne 0 ]; then
        local R3=$(curl -s --max-time 8 "https://ports.yougetsignal.com/check-port.php" \
            -d "remoteAddress=$pubip&portNumber=$port" 2>/dev/null || true)
        if echo "$R3" | grep -qi "open"; then reachable=0; fi
    fi

    # 方法4: 用 ping.pe 做 TCP 检测
    if [ $reachable -ne 0 ]; then
        local R4=$(curl -s --max-time 8 "https://ping.pe/$pubip:$port" 2>/dev/null || true)
        if echo "$R4" | grep -qi "success\|connected"; then reachable=0; fi
    fi

    kill $PID 2>/dev/null; wait $PID 2>/dev/null

    if [ $reachable -eq 0 ]; then
        log "  端口 $port ✓ 公网可达"
        return 0
    else
        warn "  端口 $port ✗ 公网不可达 (NAT/防火墙可能阻止了入站连接)"
        return 1
    fi
}

# ─── 检测 NAT ───
PUBLIC_IP=$(get_public_ip)
LOCAL_IPS=$(hostname -I 2>/dev/null || ip addr show 2>/dev/null | grep 'inet ' | awk '{print $2}' | cut -d/ -f1)

NAT_DETECTED=0
if [ -z "$PUBLIC_IP" ]; then
    PUBLIC_IP=$(echo "$LOCAL_IPS" | awk '{print $1}')
    warn "无法获取公网IP，将使用本地IP: $PUBLIC_IP"
    NAT_DETECTED=1
else
    local_is_public=0
    for lip in $LOCAL_IPS; do
        [ "$lip" = "$PUBLIC_IP" ] && local_is_public=1
    done
    if [ $local_is_public -eq 0 ]; then
        NAT_DETECTED=1
        warn "检测到 NAT 环境: 内网IP ≠ 公网IP ($PUBLIC_IP)"
    else
        log "公网IP直连: $PUBLIC_IP"
    fi
fi

# ─── 端口分配 ───
echo ""
echo -e "${CYAN}═══ 端口自动检测 ═══${NC}"
echo ""

log "检测可用端口..."

WEB_PORT=$(find_free_port 8443 tcp)
[ -z "$WEB_PORT" ] && { err "无可用 TCP 端口"; exit 1; }

NODE_PORT=$(find_free_port $((WEB_PORT + 1)) tcp)
[ -z "$NODE_PORT" ] && { err "无可用 TCP 端口"; exit 1; }

WG_START=$(find_free_port_range 30000 50)
[ -z "$WG_START" ] && WG_START=$(find_free_port_range 20000 50)
[ -z "$WG_START" ] && WG_START=$(find_free_port_range 10000 50)
[ -z "$WG_START" ] && { err "无足够连续 UDP 端口"; exit 1; }
WG_END=$((WG_START + 49))

# ─── 端口可达性验证 ───
echo ""
echo -e "${CYAN}═══ 端口可达性验证 ═══${NC}"
echo ""

REACHABLE_WEB=1; REACHABLE_NODE=1

if [ $NAT_DETECTED -eq 1 ] || [ -n "$PUBLIC_IP" ]; then
    verify_port_reachable "$WEB_PORT" "$PUBLIC_IP" && REACHABLE_WEB=0
    verify_port_reachable "$NODE_PORT" "$PUBLIC_IP" && REACHABLE_NODE=0
else
    log "未检测到 NAT，跳过外部可达性验证"
fi

# ─── 管理员密码 ───
ADMIN_PASSWORD=$(head -c 12 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 14)

# ─── 汇总 ───
echo ""
echo -e "${CYAN}══════════════════════════════════${NC}"
echo -e "${CYAN}       配置汇总${NC}"
echo -e "${CYAN}══════════════════════════════════${NC}"
echo -e "  公网 IP:       ${GREEN}$PUBLIC_IP${NC}"
echo -e "  Web 端口:      ${GREEN}$WEB_PORT/TCP${NC}  $([ $REACHABLE_WEB -eq 0 ] && echo '✓ 公网可达' || echo '✗ 需确认')"
echo -e "  节点端口:      ${GREEN}$NODE_PORT/TCP${NC}  $([ $REACHABLE_NODE -eq 0 ] && echo '✓ 公网可达' || echo '✗ 需确认')"
echo -e "  WG 端口池:     ${GREEN}$WG_START-$WG_END/UDP${NC}"
echo -e "  管理员密码:    ${GREEN}$ADMIN_PASSWORD${NC}"
echo -e "${CYAN}══════════════════════════════════${NC}"
echo ""

if [ $REACHABLE_WEB -ne 0 ] || [ $REACHABLE_NODE -ne 0 ]; then
    warn "部分端口公网可达性未通过验证。"
    warn "如果你在使用云服务器，请检查安全组/防火墙规则是否放行这些端口"
    warn "如果使用 NAT VPS，请确保已做端口映射"
    echo ""
    read -rp "仍然继续安装？[Y/n] " yn
    [ "$yn" = "n" ] || [ "$yn" = "N" ] && exit 0
fi

# ─── 安装依赖 ───
log "安装系统依赖..."
if command -v apt-get &>/dev/null; then
    apt-get update -qq && apt-get install -y -qq curl wireguard-tools nftables sqlite3 python3 netcat-openbsd
elif command -v yum &>/dev/null; then
    yum install -y -q curl wireguard-tools nftables sqlite python3 nmap-ncat
fi

# ─── 下载 / 编译 ───
INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR"/{bin,web,data}
REPO="https://github.com/FengYuchen1314/sd-wan-plus"

log "下载 PathWeaver..."
if curl -fsSL "$REPO/releases/latest/download/pathweaver-controller-$ARCH" \
    -o "$INSTALL_DIR/bin/pathweaver-controller" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/bin/pathweaver-controller"
    log "已下载预编译版本"
else
    warn "无预编译版本，从源码构建 (约10分钟)..."
    command -v cargo &>/dev/null || { curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y; source "$HOME/.cargo/env"; }
    command -v node &>/dev/null || { curl -fsSL https://deb.nodesource.com/setup_20.x | bash -; apt-get install -y nodejs; }
    apt-get install -y -qq pkg-config libssl-dev protobuf-compiler 2>/dev/null || true

    TMP=$(mktemp -d)
    git clone "$REPO" "$TMP"
    cd "$TMP"
    cd web && npm install && npm run build && cd ..
    cargo build --release -p pathweaver-controller
    cp target/release/pathweaver-controller "$INSTALL_DIR/bin/"
    cp -r web/dist/* "$INSTALL_DIR/web/"
    rm -rf "$TMP"
fi

# ─── 配置 ───
log "写入配置..."
cat > "$INSTALL_DIR/.env" << EOF
PW_STATIC_DIR=$INSTALL_DIR/web
PW_WEB_PORT=$WEB_PORT
PW_NODE_PORT=$NODE_PORT
PW_WG_PORT_START=$WG_START
PW_WG_PORT_END=$WG_END
PW_PUBLIC_ADDRESS=$PUBLIC_IP
PW_OVERLAY_CIDR=10.250.0.0/16
RUST_LOG=info
EOF

# ─── systemd ───
log "创建 systemd 服务..."
cat > /etc/systemd/system/pathweaver.service << EOF
[Unit]
Description=PathWeaver SD-WAN Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-controller
EnvironmentFile=$INSTALL_DIR/.env
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5
AmbientCapabilities=CAP_NET_BIND_SERVICE

[Install]
WantedBy=multi-user.target
EOF

# ─── 防火墙 ───
log "配置防火墙规则..."
for tool in ufw firewall-cmd nft iptables; do
    command -v $tool &>/dev/null || continue
    case $tool in
        ufw)
            ufw allow $WEB_PORT/tcp comment 'PW-Web' 2>/dev/null || true
            ufw allow $NODE_PORT/tcp comment 'PW-Node' 2>/dev/null || true
            ufw allow $WG_START:$WG_END/udp comment 'PW-WG' 2>/dev/null || true
            ;;
        firewall-cmd)
            firewall-cmd --permanent --add-port=$WEB_PORT/tcp --add-port=$NODE_PORT/tcp --add-port=$WG_START-$WG_END/udp 2>/dev/null || true
            firewall-cmd --reload 2>/dev/null || true
            ;;
        nft)
            nft add table inet pathweaver 2>/dev/null || true
            nft add chain inet pathweaver input '{ type filter hook input priority 0; }' 2>/dev/null || true
            nft add rule inet pathweaver input tcp dport $WEB_PORT accept 2>/dev/null || true
            nft add rule inet pathweaver input tcp dport $NODE_PORT accept 2>/dev/null || true
            nft add rule inet pathweaver input udp dport $WG_START-$WG_END accept 2>/dev/null || true
            ;;
        iptables)
            iptables -A INPUT -p tcp --dport $WEB_PORT -j ACCEPT 2>/dev/null || true
            iptables -A INPUT -p tcp --dport $NODE_PORT -j ACCEPT 2>/dev/null || true
            iptables -A INPUT -p udp --dport $WG_START:$WG_END -j ACCEPT 2>/dev/null || true
            ;;
    esac
    break
done

# ─── 启动 ───
log "启动 PathWeaver..."
systemctl daemon-reload
systemctl enable pathweaver
systemctl start pathweaver
sleep 4

if systemctl is-active --quiet pathweaver; then
    echo ""
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo -e "${GREEN}         PathWeaver 安装完成！${NC}"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo ""
    echo -e "  ${GREEN}Web 面板:   http://$PUBLIC_IP:$WEB_PORT${NC}"
    echo -e "  用户名:     admin"
    echo -e "  密码:       ${GREEN}$ADMIN_PASSWORD${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠ 请立即保存密码并登录修改${NC}"
    echo ""
    echo "  管理:"
    echo "    journalctl -u pathweaver -f      查看日志"
    echo "    systemctl restart pathweaver      重启"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
else
    err "启动失败: journalctl -u pathweaver -n 50"
    exit 1
fi
