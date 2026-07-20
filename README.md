# PathWeaver

自托管 SD-WAN 组网与控制平台。单用户 · 递归部署 · 图状 WireGuard 网络。

**技术栈**：Go（controller / agent / netd / updater）+ Vue 3 + TypeScript 控制台。

## 控制机一键安装

在 Debian 12+ / Ubuntu 22.04+（x86_64 或 arm64）上执行：

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
```

交互时会：

1. 自动探测公网 IP，回车确认或手动改写  
2. 询问管理员密码、名称、端口等  

非交互示例：

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp" -- \
    --public-address YOUR_PUBLIC_IP \
    --password 'YOUR_PASSWORD' \
    --name controller; rm -f "$tmp"
```

安装完成后访问：`http://公网IP:14301`（用户名固定 `admin`）。

默认端口：Web **14301**、节点服务 **14302**、WireGuard **14303–14399**。

## 完全卸载（换机前）

一键清除服务、数据目录、`pw-lo` / `pwl-*` 接口：

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/uninstall.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp"; rm -f "$tmp"
```

非交互（不询问确认）：

```bash
tmp=$(mktemp) && curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/uninstall.sh?$(date +%s)" \
  -o "$tmp" && sudo bash "$tmp" -- --yes; rm -f "$tmp"
```

## 子节点接入（链式）

在控制台「接入」复制安装命令后执行。交互时会询问：

1. **本节点是否有公网 IP**  
2. **有**：自动探测公网 IP，确认或手改（供下级接入）  
3. **无**：探测内网 IP，确认或手改（仅同网段 / 可路由设备可接入本节点）

也可直接：

```bash
curl -fsSL "http://PARENT_IP:14302/bootstrap/install.sh?token=TOKEN" -o /tmp/pw-node.sh
sudo bash /tmp/pw-node.sh
```

或统一安装包：

```bash
sudo bash install.sh --role node \
  --parent-url http://PARENT_IP:14302 \
  --token ONE_TIME_TOKEN \
  --name node-a \
  --has-public-ip no \
  --advertise-address 192.168.1.10
```

子节点**全部文件只从父节点拉取**，不访问 GitHub、不必直连控制机。

## 防火墙

控制机 / 有公网的父节点建议放行：

```bash
sudo ufw allow 14301/tcp
sudo ufw allow 14302/tcp
sudo ufw allow 14303:14399/udp
```

## 架构

```
控制机 Web UI (Vue 3)
         │ REST + WebSocket
    pathweaver-controller (Go)
         │
    控制机 Agent · netd · updater
         │
    控制树递归部署 / WireGuard 数据图
```

- **控制树**：安装、管理和更新传递（树状，无环）
- **数据图**：WireGuard 链路，可交叉直连（图状）
- **Overlay 互通**：沿已有 WG 链路最短路径转发（不自动建全网状链路）

## 一键更新

控制机 UI「更新中心」：制品沿控制树预分发 → 叶子优先安装。

## WireGuard

数据面使用嵌入 **pathweaver-netd** 的 **wireguard-go**（userspace），由主控编译 desired state 动态调控各节点链路；不依赖内核 WG 模块。

握手语义见 [docs/WIREGUARD.md](docs/WIREGUARD.md)：主动端单向发起 + Keepalive；被动端 ListenPort，由协议动态学习对端。

## 环境要求

- 控制机 / 节点：Linux（Debian 12+ / Ubuntu 22.04+），systemd
- 构建（可选）：Go 1.22+，Node.js 20+（仅构建前端）

## 从源码构建

```bash
go build -o bin/pathweaver-controller ./cmd/pathweaver-controller
go build -o bin/pathweaver-agent ./cmd/pathweaver-agent
go build -o bin/pathweaver-netd ./cmd/pathweaver-netd
go build -o bin/pathweaver-updater ./cmd/pathweaver-updater
go build -o bin/pathweaver-cli ./cmd/pathweaver-cli

cd web && npm install && npm run build && cd ..
bash packaging/build-package.sh
```

Windows 开发可使用 `build.ps1`。

## 本地开发（控制机）

```bash
mkdir -p data
set PW_DATA_DIR=./data
set PW_PUBLIC_ADDRESS=127.0.0.1
./bin/pathweaver-controller --bootstrap --password 'changeme123' --name controller

set PW_STATIC_DIR=web/dist
./bin/pathweaver-controller

cd web && npm run dev
```

访问 `http://127.0.0.1:14301`（或 Vite `5173`）。

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `PW_WEB_PORT` | 14301 | Web 管理端口 |
| `PW_NODE_PORT` | 14302 | 节点服务端口（bootstrap/制品） |
| `PW_WG_PORT_START/END` | 14303/14399 | WG 端口池 |
| `PW_PUBLIC_ADDRESS` | 127.0.0.1 | 公网地址 |
| `PW_DB_PATH` | ./data/pathweaver.db | SQLite |
| `PW_STATIC_DIR` | | 前端静态目录 |
| `PW_OVERLAY_CIDR` | 10.250.0.0/16 | Overlay 网段 |
| `PW_TLS` | | `1` 启用 Cookie Secure |
| `PW_SERVE_CHILDREN` | 1 | 节点是否对下级提供制品代理（控制机本地 Agent 设 0） |

## 进程

| 进程 | 角色 |
|------|------|
| pathweaver-controller | 仅控制机：API、UI、编排 |
| pathweaver-agent | 全节点：中继、拉配置、调 netd |
| pathweaver-netd | 结构化配网（WG / 路由 / nft） |
| pathweaver-updater | 制品 Stage、原子切换、回滚 |

## License

MIT
