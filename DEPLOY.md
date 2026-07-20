# PathWeaver 部署指南

## 控制机一键安装

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
```

安装时会自动探测公网 IP，可回车确认或手动修改。

## 完全卸载

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/uninstall.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
```

## 子节点

控制台「接入」复制命令后执行。安装时需选择：

- **有公网 IP**：探测/确认公网地址（供下级接入）
- **无公网 IP**：填写内网 IP（仅可达该地址的设备可作下级）

非交互示例：

```bash
sudo bash install.sh --role node \
  --parent-url http://PARENT:14302 --token TOKEN --name node-a \
  --has-public-ip no --advertise-address 192.168.1.10 --noninteractive
```

### 防火墙

```bash
sudo ufw allow 14301/tcp
sudo ufw allow 14302/tcp
sudo ufw allow 14303:14399/udp
```

### 验证

```bash
curl http://localhost:14301/api/health
```

## systemd

```bash
journalctl -u pathweaver -f
```

## 备份

备份 `/opt/pathweaver/data/`（含数据库与密钥）。
