#!/bin/bash
set -euo pipefail

# PathWeaver Node Installer
# This script is served by parent nodes for new node enrollment.

TOKEN="$1"
if [ -z "$TOKEN" ]; then
    echo "Usage: $0 <enrollment_token>"
    exit 1
fi

# Detect parent address from the script URL or config
PARENT_ADDR="${PARENT_ADDR:-}"
PARENT_PORT="${PARENT_PORT:-8444}"

log_info() { echo "[INFO] $1"; }
log_error() { echo "[ERROR] $1"; exit 1; }

if [ "$(id -u)" -ne 0 ]; then
    log_error "Must be run as root"
fi

log_info "PathWeaver Node Installer"

ARCH=$(uname -m)
INSTALL_DIR="/opt/pathweaver"
mkdir -p "$INSTALL_DIR"/{bin,etc,artifacts}

# Download components from parent node
log_info "Downloading from parent node..."

for component in pathweaver-agent pathweaver-netd pathweaver-updater; do
    curl -fsSL "https://${PARENT_ADDR}:${PARENT_PORT}/bootstrap/artifacts/${component}-${ARCH}?token=${TOKEN}" \
        -o "$INSTALL_DIR/bin/$component" || log_error "Failed to download $component"
    chmod +x "$INSTALL_DIR/bin/$component"
done

# Download installer manifest
curl -fsSL "https://${PARENT_ADDR}:${PARENT_PORT}/bootstrap/manifest?token=${TOKEN}" \
    -o "$INSTALL_DIR/manifest.json" || log_error "Failed to download manifest"

# Generate identity
log_info "Generating node identity..."

# Initialize agent with token
"$INSTALL_DIR/bin/pathweaver-agent" --enroll --token "$TOKEN" --parent "$PARENT_ADDR:$PARENT_PORT"

# Install systemd services
cat > /etc/systemd/system/pathweaver-agent.service << EOF
[Unit]
Description=PathWeaver Agent
After=network-online.target
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

systemctl daemon-reload
systemctl enable pathweaver-agent pathweaver-netd pathweaver-updater
systemctl start pathweaver-agent

# Clean up installer
rm -f /tmp/pathweaver-node-install.sh

log_info "Node installation complete"
