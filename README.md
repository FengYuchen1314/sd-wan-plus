# PathWeaver

自托管 SD-WAN 组网与控制平台。单用户 · 递归部署 · 图状 WireGuard 网络。

**技术栈**：Go（controller / agent / netd / updater）+ Vue 3 + TypeScript 控制台。

## 控制机一键安装

在 Debian 12+ / Ubuntu 22.04+（x86_64 或 arm64）上执行（**带防缓存参数**，避免命中旧脚本）：

```bash
curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  | sudo bash
```

成功时应先看到一行：`[pathweaver] install-controller.sh rev=...`，随后交互询问密码、公网地址等。

非交互示例：

```bash
curl -fsSL -H 'Cache-Control: no-cache' -H 'Pragma: no-cache' \
  "https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/scripts/install-controller.sh?$(date +%s)" \
  | sudo bash -s -- \
    --public-address YOUR_PUBLIC_IP \
    --password 'YOUR_PASSWORD' \
    --name controller
```

安装完成后访问：`http://YOUR_PUBLIC_IP:14301`（用户名固定 `admin`）。

默认端口：Web **14301**、节点服务 **14302**、WireGuard **14303–14399**。

> 安装包由 GitHub Actions 在每次 push `master` 时自动编译发布；主控与子节点使用**同一包**，仅 `--role` 参数不同。

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
- **路径策略**：nftables + ip rule + 多路由表

## 子节点接入（链式）

在控制台「接入」复制命令，或在已解压的统一安装包目录执行：

```bash
sudo bash install.sh --role node \
  --parent-url http://PARENT_IP:14302 \
  --token ONE_TIME_TOKEN \
  --name node-a
```

也可直接管道父节点生成的 bootstrap：

```bash
curl -fsSL "http://PARENT_IP:14302/bootstrap/install.sh?token=TOKEN" | sudo bash
```

子节点**全部文件只从父节点拉取**，不访问 GitHub、不必直连控制机。

## 一键更新

控制机 UI「更新中心」创建并启动：制品沿控制树预分发 → 叶子优先安装。

## WireGuard

见 [docs/WIREGUARD.md](docs/WIREGUARD.md)：主动端单向发起握手 + Keepalive；被动端无固定 Endpoint，由内核动态学习维护。

## 环境要求

- 控制机 / 节点：Linux（Debian 12+ / Ubuntu 22.04+），systemd
- 构建（可选）：Go 1.22+，Node.js 20+（仅构建前端）

## 从源码构建

```bash
# 后端
go build -o bin/pathweaver-controller ./cmd/pathweaver-controller
go build -o bin/pathweaver-agent ./cmd/pathweaver-agent
go build -o bin/pathweaver-netd ./cmd/pathweaver-netd
go build -o bin/pathweaver-updater ./cmd/pathweaver-updater
go build -o bin/pathweaver-cli ./cmd/pathweaver-cli

# 前端
cd web && npm install && npm run build && cd ..

# 打统一安装包
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
