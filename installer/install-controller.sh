#!/bin/bash
set -euo pipefail

# PathWeaver Controller Installer
# This script installs the PathWeaver controller on a fresh Debian/Ubuntu system.

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Check root
if [ "$(id -u)" -ne 0 ]; then
    log_error "This script must be run as root"
fi

# Check Linux and systemd
if [ ! -d /run/systemd/system ]; then
    log_error "systemd is required"
fi

ARCH=$(uname -m)
case "$ARCH" in
    x86_64)  ARCH="x86_64" ;;
    aarch64) ARCH="aarch64" ;;
    *)       log_error "Unsupported architecture: $ARCH" ;;
esac

log_info "PathWeaver Controller Installer"
log_info "Architecture: $ARCH"

# Collect configuration
echo ""
echo "=== Controller Configuration ==="

read -p "Controller name [controller]: " CONTROLLER_NAME
CONTROLLER_NAME=${CONTROLLER_NAME:-controller}

read -sp "Admin password: " ADMIN_PASSWORD
echo ""

read -p "Web UI port [8443]: " WEB_PORT
WEB_PORT=${WEB_PORT:-8443}

read -p "Node service port [8444]: " NODE_SERVICE_PORT
NODE_SERVICE_PORT=${NODE_SERVICE_PORT:-8444}

read -p "WireGuard UDP port range start [30000]: " WG_PORT_START
WG_PORT_START=${WG_PORT_START:-30000}

read -p "WireGuard UDP port range end [30999]: " WG_PORT_END
WG_PORT_END=${WG_PORT_END:-30999}

read -p "Public IP or domain: " PUBLIC_ADDRESS
if [ -z "$PUBLIC_ADDRESS" ]; then
    log_error "Public address is required"
fi

read -p "Overlay IPv4 CIDR [10.250.0.0/16]: " OVERLAY_CIDR
OVERLAY_CIDR=${OVERLAY_CIDR:-10.250.0.0/16}

read -p "Enable IPv6 overlay? (y/N): " IPV6_ENABLED
IPV6_ENABLED=${IPV6_ENABLED:-n}

read -p "TLS mode (auto-generated/existing/domain) [auto-generated]: " TLS_MODE
TLS_MODE=${TLS_MODE:-auto-generated}

read -p "Auto configure firewall? (Y/n): " AUTO_FIREWALL
AUTO_FIREWALL=${AUTO_FIREWALL:-y}

echo ""
log_info "Configuration complete"

# Install dependencies
log_info "Installing dependencies..."
apt-get update -qq
apt-get install -y -qq curl wireguard-tools nftables sqlite3

# Create directories
INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR"/{bin,etc,data,certs,releases,artifacts}
mkdir -p /var/log/pathweaver

# Download release artifacts
RELEASE_URL="https://github.com/FengYuchen1314/pathweaver/releases/latest/download"
log_info "Downloading PathWeaver..."

for component in pathweaver-controller pathweaver-agent pathweaver-netd pathweaver-updater pathweaver-cli; do
    curl -fsSL "$RELEASE_URL/$component-$ARCH" -o "$INSTALL_DIR/bin/$component" || log_warn "Could not download $component"
    chmod +x "$INSTALL_DIR/bin/$component" 2>/dev/null || true
done

# Generate initial config
log_info "Initializing configuration..."
cat > "$INSTALL_DIR/etc/controller.toml" << EOF
controller_name = "$CONTROLLER_NAME"
web_port = $WEB_PORT
node_service_port = $NODE_SERVICE_PORT
wg_port_range_start = $WG_PORT_START
wg_port_range_end = $WG_PORT_END
public_address = "$PUBLIC_ADDRESS"
overlay_ipv4_cidr = "$OVERLAY_CIDR"
ipv6_enabled = $([ "$IPV6_ENABLED" = "y" ] && echo "true" || echo "false")
tls_mode = "$TLS_MODE"
auto_configure_firewall = $([ "$AUTO_FIREWALL" = "y" ] && echo "true" || echo "false")
EOF

# Initialize database and admin password
log_info "Initializing database..."
"$INSTALL_DIR/bin/pathweaver-cli" init-controller \
    --name "$CONTROLLER_NAME" \
    --admin-password "$ADMIN_PASSWORD" \
    --web-port "$WEB_PORT" \
    --node-service-port "$NODE_SERVICE_PORT" \
    --wg-port-start "$WG_PORT_START" \
    --wg-port-end "$WG_PORT_END" \
    --public-address "$PUBLIC_ADDRESS" \
    --overlay-ipv4-cidr "$OVERLAY_CIDR" \
    --ipv6-enabled "$([ "$IPV6_ENABLED" = "y" ] && echo "true" || echo "false")" \
    --tls-mode "$TLS_MODE" || log_warn "CLI initialization failed, starting with defaults"

# Install systemd services
log_info "Installing systemd services..."

cat > /etc/systemd/system/pathweaver-controller.service << EOF
[Unit]
Description=PathWeaver Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-controller
Restart=always
RestartSec=5
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-agent.service << EOF
[Unit]
Description=PathWeaver Agent
After=network-online.target pathweaver-controller.service
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-agent
Restart=always
RestartSec=5
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-netd.service << EOF
[Unit]
Description=PathWeaver Network Daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-netd
Restart=always
RestartSec=3
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

cat > /etc/systemd/system/pathweaver-updater.service << EOF
[Unit]
Description=PathWeaver Updater
After=network-online.target

[Service]
Type=simple
ExecStart=$INSTALL_DIR/bin/pathweaver-updater
Restart=always
RestartSec=10
WorkingDirectory=$INSTALL_DIR

[Install]
WantedBy=multi-user.target
EOF

# Configure firewall
if [ "$AUTO_FIREWALL" = "y" ]; then
    log_info "Configuring firewall..."
    if command -v ufw &> /dev/null; then
        ufw allow "$WEB_PORT/tcp" comment "PathWeaver Web UI"
        ufw allow "$NODE_SERVICE_PORT/tcp" comment "PathWeaver Node Service"
        ufw allow "$WG_PORT_START:$WG_PORT_END/udp" comment "PathWeaver WireGuard"
    elif command -v nft &> /dev/null; then
        nft add table inet pathweaver 2>/dev/null || true
        nft add chain inet pathweaver input '{ type filter hook input priority 0; }' 2>/dev/null || true
        nft add rule inet pathweaver input tcp dport "$WEB_PORT" accept 2>/dev/null || true
        nft add rule inet pathweaver input tcp dport "$NODE_SERVICE_PORT" accept 2>/dev/null || true
        nft add rule inet pathweaver input udp dport "$WG_PORT_START-$WG_PORT_END" accept 2>/dev/null || true
    fi
fi

# Enable and start services
log_info "Enabling services..."
systemctl daemon-reload
systemctl enable pathweaver-controller pathweaver-agent pathweaver-netd pathweaver-updater
systemctl start pathweaver-controller

log_info ""
log_info "=============================="
log_info "Installation complete!"
log_info "Web UI: https://$PUBLIC_ADDRESS:$WEB_PORT"
log_info "Username: admin"
log_info "=============================="
