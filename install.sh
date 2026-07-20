#!/bin/bash
set -euo pipefail

# PathWeaver SD-WAN 一键安装脚本
# 在 Linux (Debian 12+ / Ubuntu 22.04+) 上运行:
#   curl -fsSL https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/install.sh | sudo bash

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${RED}[WARN]${NC} $1"; }

if [ "$(id -u)" -ne 0 ]; then
    warn "请使用 root 权限运行: sudo bash install.sh"
    exit 1
fi

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="x86_64" ;;
    aarch64) ARCH="aarch64" ;;
    *)       warn "不支持的架构: $ARCH"; exit 1 ;;
esac

log "PathWeaver SD-WAN 一键安装"
log "架构: $ARCH"

# 安装系统依赖
log "安装系统依赖..."
apt-get update -qq
apt-get install -y -qq curl wireguard-tools nftables sqlite3

# 创建目录
INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR"/{bin,web,data}

# 下载预编译二进制
REPO="https://github.com/FengYuchen1314/sd-wan-plus"
log "下载 PathWeaver..."

# 尝试下载 release，如果不存在则从源码编译
download_binary() {
    curl -fsSL "$REPO/releases/latest/download/pathweaver-controller-$ARCH" \
        -o "$INSTALL_DIR/bin/pathweaver-controller" 2>/dev/null && return 0
    return 1
}

if download_binary; then
    chmod +x "$INSTALL_DIR/bin/pathweaver-controller"
    log "已下载预编译版本"
else
    warn "未找到预编译版本，将从源码编译..."
    log "安装 Rust 工具链..."
    if ! command -v cargo &> /dev/null; then
        curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
        source "$HOME/.cargo/env"
    fi

    log "安装 Node.js..."
    if ! command -v node &> /dev/null; then
        curl -fsSL https://deb.nodesource.com/setup_20.x | bash -
        apt-get install -y nodejs
    fi

    apt-get install -y -qq pkg-config libssl-dev protobuf-compiler

    log "克隆仓库..."
    BUILD_DIR=$(mktemp -d)
    git clone "$REPO" "$BUILD_DIR"
    cd "$BUILD_DIR"

    log "构建前端..."
    cd web && npm install && npm run build && cd ..

    log "构建后端 (约5-10分钟)..."
    cargo build --release -p pathweaver-controller

    cp target/release/pathweaver-controller "$INSTALL_DIR/bin/"
    cp -r web/dist/* "$INSTALL_DIR/web/"
    rm -rf "$BUILD_DIR"
fi

# 配置
log "配置 PathWeaver..."
cat > "$INSTALL_DIR/.env" << EOF
PW_STATIC_DIR=$INSTALL_DIR/web
RUST_LOG=info
EOF

# systemd 服务
log "创建 systemd 服务..."
cat > /etc/systemd/system/pathweaver.service << EOF
[Unit]
Description=PathWeaver SD-WAN Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-controller
Environment=PW_STATIC_DIR=$INSTALL_DIR/web
Environment=RUST_LOG=info
WorkingDirectory=$INSTALL_DIR
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# 防火墙
log "配置防火墙..."
if command -v ufw &> /dev/null; then
    ufw allow 8443/tcp comment "PathWeaver Web UI"
fi

# 启动
log "启动服务..."
systemctl daemon-reload
systemctl enable pathweaver
systemctl start pathweaver

sleep 3

# 检查状态
if systemctl is-active --quiet pathweaver; then
    IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')
    echo ""
    echo -e "${CYAN}========================================${NC}"
    echo -e "${GREEN}  PathWeaver 安装完成！${NC}"
    echo -e "${CYAN}========================================${NC}"
    echo ""
    echo "  Web 管理面板:  http://$IP:8443"
    echo "  用户名:        admin"
    echo "  密码:          admin123"
    echo "  (首次登录后请立即修改密码)"
    echo ""
    echo "  管理命令:"
    echo "    systemctl status pathweaver  查看状态"
    echo "    journalctl -u pathweaver -f  查看日志"
    echo "    systemctl restart pathweaver 重启服务"
    echo -e "${CYAN}========================================${NC}"
else
    warn "服务启动失败，请查看日志: journalctl -u pathweaver -n 50"
    exit 1
fi
