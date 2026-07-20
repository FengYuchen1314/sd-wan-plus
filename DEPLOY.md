# PathWeaver 部署指南

## 源码构建部署

### 前置

- Debian 12+ / Ubuntu 22.04+
- Go 1.22+
- Node.js 20+（构建前端）
- wireguard-tools, nftables, iproute2

### 构建

```bash
git clone https://github.com/FengYuchen1314/sd-wan-plus.git
cd sd-wan-plus
bash build-release.sh   # 或 Windows: .\build.ps1
```

### 安装

```bash
sudo bash installer/install-controller.sh
```

### 防火墙

```bash
sudo ufw allow 8443/tcp
sudo ufw allow 8444/tcp
sudo ufw allow 30000:30999/udp
```

### 验证

```bash
curl http://localhost:8443/api/health
# {"status":"ok","version":"0.1.0"}
```

## systemd

单元文件见 `packaging/systemd/`。

```bash
journalctl -u pathweaver -f
```

## 备份

```bash
cp /opt/pathweaver/data/pathweaver.db /opt/pathweaver/data/pathweaver.db.bak.$(date +%Y%m%d)
```
