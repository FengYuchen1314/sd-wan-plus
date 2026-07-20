# PathWeaver 部署指南

## 源码构建部署

### 使用 GitHub Release 安装包（推荐）

控制机一键安装（防缓存）：

```bash
curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  | sudo bash
```

或手动下载 `pathweaver-linux-amd64.tar.gz` / `pathweaver-linux-arm64.tar.gz` 后：

```bash
tar -xzf pathweaver-linux-amd64.tar.gz && cd pathweaver-*
sudo bash install.sh --role controller --public-address YOUR_IP --password 'secret'
```

子节点：

```bash
sudo bash install.sh --role node --parent-url http://PARENT:14302 --token TOKEN --name node-a
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

单元文件见 `packaging/systemd/`。

```bash
journalctl -u pathweaver -f
```

## 备份

```bash
cp /opt/pathweaver/data/pathweaver.db /opt/pathweaver/data/pathweaver.db.bak.$(date +%Y%m%d)
```
