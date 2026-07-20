# PathWeaver

自托管 SD-WAN 组网与控制平台。单用户 · 递归部署 · 图状 WireGuard 网络。

**技术栈**：Go（controller / agent / netd / updater）+ Vue 3 + TypeScript 控制台。

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

## 环境要求

- 控制机 / 节点：Linux（Debian 12+ / Ubuntu 22.04+），systemd
- 构建：Go 1.22+，Node.js 20+（仅构建前端）

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
```

Windows 开发可使用 `build.ps1`。

## 本地开发（控制机）

```bash
# 初始化数据库
mkdir -p data
set PW_DATA_DIR=./data
set PW_PUBLIC_ADDRESS=127.0.0.1
./bin/pathweaver-controller --bootstrap --password 'changeme123' --name controller

# 启动（另开终端可跑 netd + agent）
set PW_STATIC_DIR=web/dist
./bin/pathweaver-controller

# 前端热更新
cd web && npm run dev
```

访问 `http://127.0.0.1:8443`（或 Vite `5173`）。用户名固定 `admin`。

## 生产安装

```bash
sudo bash installer/install-controller.sh
```

默认端口：Web `8443`、节点服务 `8444`、WG `30000-30999`、Overlay `10.250.0.0/16`。

## 接入新节点

1. Web UI →「接入」
2. 选择父节点 → 生成一次性命令
3. 在目标机执行（从父节点拉制品，无需 GitHub）

## 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `PW_WEB_PORT` | 8443 | Web 管理端口 |
| `PW_NODE_PORT` | 8444 | 节点服务端口 |
| `PW_WG_PORT_START/END` | 30000/30999 | WG 端口池 |
| `PW_PUBLIC_ADDRESS` | 127.0.0.1 | 公网地址 |
| `PW_DB_PATH` | ./data/pathweaver.db | SQLite |
| `PW_STATIC_DIR` | | 前端静态目录 |
| `PW_OVERLAY_CIDR` | 10.250.0.0/16 | Overlay 网段 |
| `PW_TLS` | | `1` 启用 Cookie Secure |

## 进程

| 进程 | 角色 |
|------|------|
| pathweaver-controller | 仅控制机：API、UI、编排 |
| pathweaver-agent | 全节点：中继、拉配置、调 netd |
| pathweaver-netd | 结构化配网（WG / 路由 / nft） |
| pathweaver-updater | 制品 Stage、原子切换、回滚 |

## License

MIT
