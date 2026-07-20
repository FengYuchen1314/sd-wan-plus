#!/bin/bash
set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[1;33m'; NC='\033[0m'
log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
err()  { echo -e "${RED}[ERROR]${NC} $1"; }

if [ "$(id -u)" -ne 0 ]; then err "请使用 root: sudo bash install.sh"; exit 1; fi

ARCH=$(uname -m); case "$ARCH" in x86_64) ARCH="x86_64";; aarch64) ARCH="aarch64";; *) err "不支持的架构: $ARCH"; exit 1;; esac

# ─── 端口工具 (仅本地 127.0.0.1 检测) ───

port_is_free() {
    local port=$1 proto=$2
    case "$proto" in
        tcp) ss -tlnH 2>/dev/null | awk '{print $4}' | grep -qE ":(127\.0\.0\.1|0\.0\.0\.0|\*|\[::\]):$port$" && return 1 ;;
        udp) ss -ulnH 2>/dev/null | awk '{print $4}' | grep -qE ":(127\.0\.0\.1|0\.0\.0\.0|\*|\[::\]):$port$" && return 1 ;;
    esac
    return 0
}

find_free_port() {
    local start=$1 proto=$2 max=${3:-200}
    local port=$start
    for ((i=0; i<max; i++)); do
        [ $port -gt 65535 ] && port=1024
        if port_is_free $port "$proto"; then echo $port; return 0; fi
        port=$((port + 1))
    done
    return 1
}

find_free_port_range() {
    local start=$1 count=$2 max=${3:-60}
    local port=$start
    for ((i=0; i<max; i++)); do
        [ $((port + count)) -gt 65535 ] && port=1024
        local ok=1
        for ((j=0; j<count; j++)); do
            port_is_free $((port + j)) udp || { ok=0; break; }
        done
        if [ $ok -eq 1 ]; then echo $port; return 0; fi
        port=$((port + 20))
    done
    return 1
}

# ─── 获取所有 IP ───
get_all_ips() {
    ip -o -4 addr show 2>/dev/null | awk '{print $4}' | cut -d/ -f1
}

# ─── 判断 IP 是否公网地址 ───
is_public_ip() {
    local ip=$1
    # RFC1918 私有地址范围
    if echo "$ip" | grep -qE '^(10\.|172\.(1[6-9]|2[0-9]|3[01])\.|192\.168\.|127\.|0\.)'; then return 1; fi
    # CGNAT 范围 100.64.0.0/10
    if echo "$ip" | grep -qE '^100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.'; then return 1; fi
    return 0
}

# ─── 尝试获取公网可达 IP ───
find_public_address() {
    # 先查本地网卡是否有公网IP
    for ip in $(get_all_ips); do
        if is_public_ip "$ip"; then
            log "检测到公网IP: $ip (本地网卡直连)"
            echo "$ip"
            return 0
        fi
    done

    # 本机没有公网IP，尝试用外部服务获取出口IP
    local ext_ip=$(curl -s --max-time 5 ifconfig.me 2>/dev/null \
        || curl -s --max-time 5 icanhazip.com 2>/dev/null \
        || curl -s --max-time 5 api.ipify.org 2>/dev/null \
        || echo "")

    if [ -z "$ext_ip" ]; then
        # 完全无法联网或只有内网
        warn "无法获取任何公网地址"
        warn "此机器无法作为控制机（没有公网可达 IP）"
        warn "它只能作为子节点加入已有 SD-WAN 网络"
        echo ""
        return 1
    fi

    # 有出口IP，但本机网卡上没有——NAT
    warn "本机网卡未绑定公网IP"
    warn "出口IP: $ext_ip (可能是NAT/端口映射/CGNAT)"
    warn ""
    warn "如果这是云服务器，云厂商的安全组/防火墙规则会影响端口可达性"
    warn "如果是 NAT VPS，请先配置端口映射后再继续"
    echo ""
    echo "$ext_ip"
    return 0
}

# ─── 开始 ───
echo ""
echo -e "${CYAN}══════════════════════════════════════${NC}"
echo -e "${CYAN}  PathWeaver 控制机安装${NC}"
echo -e "${CYAN}══════════════════════════════════════${NC}"
echo ""

log "检测网络环境..."
PUBLIC_IP=$(find_public_address) || {
    echo ""
    warn "此机器是内网节点，无法直装控制机"
    warn "请先在另一台有公网IP的机器上安装控制机"
    warn "然后在控制机面板中为此机器生成子节点安装命令"
    exit 1
}

log "检测可用端口..."

WEB_PORT=$(find_free_port 8443 tcp) || { err "无可用 TCP 端口"; exit 1; }
NODE_PORT=$(find_free_port $((WEB_PORT + 1)) tcp) || { err "无可用 TCP 端口"; exit 1; }
WG_START=$(find_free_port_range 30000 50) || WG_START=$(find_free_port_range 20000 50)
WG_START=${WG_START:-$(find_free_port_range 10000 50)}
[ -z "$WG_START" ] && { err "无足够连续 UDP 端口"; exit 1; }
WG_END=$((WG_START + 49))

ADMIN_PASSWORD=$(head -c 14 /dev/urandom | base64 | tr -dc 'A-Za-z0-9' | head -c 14)

# ─── 汇总 ───
echo ""
echo -e "${CYAN}══════════════════════════════════════${NC}"
echo -e "${CYAN}       安装配置确认${NC}"
echo -e "${CYAN}══════════════════════════════════════${NC}"
echo ""
echo -e "  公网地址:      ${GREEN}$PUBLIC_IP${NC}"
echo -e "  Web 管理端口:  ${GREEN}$WEB_PORT/TCP${NC}"
echo -e "  节点服务端口:  ${GREEN}$NODE_PORT/TCP${NC}"
echo -e "  WG 端口池:     ${GREEN}$WG_START-$WG_END/UDP${NC}"
echo -e "  管理员密码:    ${GREEN}$ADMIN_PASSWORD${NC}"
echo ""
echo -e "  ${YELLOW}请确认以上端口已在防火墙/安全组中放行${NC}"
echo -e "  ${YELLOW}NAT 机器请提前配置端口映射${NC}"
echo ""

read -rp "确认继续安装？[Y/n] " yn
[ "$yn" = "n" ] || [ "$yn" = "N" ] && exit 0

# ─── 安装依赖 ───
log "安装系统依赖..."
if command -v apt-get &>/dev/null; then
    apt-get update -qq && apt-get install -y -qq curl wireguard-tools nftables sqlite3
elif command -v yum &>/dev/null; then
    yum install -y -q curl wireguard-tools nftables sqlite
fi

# ─── 下载 ───
INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR"/{bin,web,data}
REPO="https://github.com/FengYuchen1314/sd-wan-plus"

log "安装 PathWeaver..."
if curl -fsSL "$REPO/releases/latest/download/pathweaver-controller-$ARCH" \
    -o "$INSTALL_DIR/bin/pathweaver-controller" 2>/dev/null; then
    chmod +x "$INSTALL_DIR/bin/pathweaver-controller"
else
    warn "无预编译版本，从源码构建..."
    command -v cargo &>/dev/null || { curl --proto '=https' -sSf https://sh.rustup.rs | sh -s -- -y; source "$HOME/.cargo/env"; }
    command -v node &>/dev/null || { curl -fsSL https://deb.nodesource.com/setup_20.x | bash -; apt-get install -y nodejs; }
    apt-get install -y -qq pkg-config libssl-dev protobuf-compiler 2>/dev/null || true
    TMP=$(mktemp -d); git clone "$REPO" "$TMP"; cd "$TMP"
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
RUST_LOG=info
EOF

# ─── systemd ───
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

[Install]
WantedBy=multi-user.target
EOF

# ─── 防火墙 ───
log "尝试配置防火墙..."
for tool in ufw firewall-cmd nft iptables; do
    command -v $tool &>/dev/null || continue
    case $tool in
        ufw)
            ufw allow $WEB_PORT/tcp comment 'PW-Web' 2>/dev/null
            ufw allow $NODE_PORT/tcp comment 'PW-Node' 2>/dev/null
            ufw allow $WG_START:$WG_END/udp comment 'PW-WG' 2>/dev/null
            ;;
        firewall-cmd)
            firewall-cmd --permanent --add-port=$WEB_PORT/tcp --add-port=$NODE_PORT/tcp \
                --add-port=$WG_START-$WG_END/udp 2>/dev/null
            firewall-cmd --reload 2>/dev/null
            ;;
        nft)
            nft add table inet pathweaver 2>/dev/null || true
            nft add chain inet pathweaver input '{ type filter hook input priority 0; }' 2>/dev/null || true
            nft add rule inet pathweaver input tcp dport $WEB_PORT accept 2>/dev/null || true
            nft add rule inet pathweaver input tcp dport $NODE_PORT accept 2>/dev/null || true
            nft add rule inet pathweaver input udp dport $WG_START-$WG_END accept 2>/dev/null || true
            ;;
        iptables)
            iptables -A INPUT -p tcp --dport $WEB_PORT -j ACCEPT 2>/dev/null
            iptables -A INPUT -p tcp --dport $NODE_PORT -j ACCEPT 2>/dev/null
            iptables -A INPUT -p udp --dport $WG_START:$WG_END -j ACCEPT 2>/dev/null
            ;;
    esac
    break
done

# ─── 启动 ───
log "启动服务..."
systemctl daemon-reload
systemctl enable pathweaver
systemctl start pathweaver
sleep 4

if systemctl is-active --quiet pathweaver; then
    echo ""
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo -e "${GREEN}     PathWeaver 安装完成${NC}"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
    echo ""
    echo -e "  ${GREEN}面板地址: http://$PUBLIC_IP:$WEB_PORT${NC}"
    echo -e "  用户名:   admin"
    echo -e "  密码:     ${GREEN}$ADMIN_PASSWORD${NC}"
    echo ""
    echo -e "  ${YELLOW}⚠ 请立即登录并修改密码${NC}"
    echo -e "  ${YELLOW}⚠ 添加子节点前，在「接入新节点」页面生成安装命令${NC}"
    echo ""
    echo -e "  日志: journalctl -u pathweaver -f"
    echo -e "${CYAN}════════════════════════════════════════${NC}"
else
    err "启动失败: journalctl -u pathweaver -n 50"
    exit 1
fi
