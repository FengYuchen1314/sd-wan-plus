# PathWeaver 部署指南

## 控制机一键安装

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
```

安装时会自动探测公网 IP，可回车确认或手动修改。

## 删除节点（面板）

交互顺序（参考旧版面板约定，非其实现）：

1. 在「节点」页点 **删除** → 先展示影响（相邻链路数、控制树下级）。
2. **控制机不可删**；仍有控制树下级时拒绝，需先删下级。
3. 确认后从控制面移除该节点与相邻 WG 链路，并 **自动发布** 新配置。
4. 再到该设备执行本机卸载（`scripts/uninstall.sh`）；需要清数据时加 `--purge` / `--yes`。

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

云厂商安全组也需放行 **UDP 14303–14399**（监听端 / 父节点）。

### 数据面（wireguard-go）

- Overlay 由各节点上的 **pathweaver-netd** 内嵌 **wireguard-go**（userspace）维护，**不依赖**内核 WireGuard 模块或 `wireguard-tools`。
- 主控通过「发布配置」/ enroll·建链路后的自动发布，向每个节点下发 desired state；agent 调用 netd 热更新本机各条 `pwl-*` 链路。
- 密钥由 `pathweaver-cli wg-keypair` 生成。

### 验证

```bash
curl -sS http://127.0.0.1:14301/api/health
# 各节点：
ip -br addr show pw-lo
ip -br link | grep pwl
ip route | grep -E '10\.250\.|pwl-'
# 用控制台显示的 overlay IP：
ping -c 3 -I <本机overlay> <对端overlay>
```

升级主控/节点后请重启 `pathweaver` 与 `pathweaver-netd`，再在控制台**发布一次配置**（会自动把旧单向链路修成双侧 Listen+Endpoint）。

若 ping 出现 `Destination address required` / `Destination Host Unreachable`：

```bash
# 两边
ip -br link | grep pwl
ip route | grep 10.250
journalctl -u pathweaver-netd -u pathweaver-agent -n 40 --no-pager
# 防火墙放行 UDP 14303-14399；子节点须能访问父节点该端口
```

## systemd

```bash
journalctl -u pathweaver -f
```

## 备份

备份 `/opt/pathweaver/data/`（含数据库与密钥）。
