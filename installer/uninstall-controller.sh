#!/bin/bash
set -e
echo "PathWeaver 控制机卸载"
read -p "确认卸载？[y/N] " -r
[[ "$REPLY" =~ ^[Yy]$ ]] || exit 0
systemctl stop pathweaver pathweaver-agent pathweaver-netd pathweaver-updater 2>/dev/null || true
systemctl disable pathweaver pathweaver-agent pathweaver-netd pathweaver-updater 2>/dev/null || true
rm -f /etc/systemd/system/pathweaver*.service
systemctl daemon-reload
rm -rf /opt/pathweaver
for iface in $(ip link show 2>/dev/null | grep -oE 'pwl-[^:]+|pw-lo' | sort -u); do
  ip link delete "$iface" 2>/dev/null || true
done
echo "完成"
