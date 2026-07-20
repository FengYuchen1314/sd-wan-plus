# PathWeaver

自托管 SD-WAN 组网与控制平台。单用户 · 递归部署 · 图状 WireGuard 网络。

## 架构

```
控制机 Web UI (React + TypeScript)
         │ REST + WebSocket
pathweaver-controller (Python/FastAPI)
         │ 控制消息向下传递
    控制机 Agent
         │
 ┌───────┴───────┐
 │   节点 A       │
 │ Agent·netd    │
 └───┬───────┬───┘
     │       │
 ┌───┴──┐ ┌──┴───┐
 │节点 B │ │节点 C │
 └───┬──┘ └──────┘
     │
 ┌───┴──┐
 │节点 D │
 └──────┘
```

- **控制树**：安装、管理和更新传递（树状，无环）
- **数据图**：WireGuard 链路，任意两节点可建立直连（图状，支持交叉链路）
- **路径策略**：控制机统一编译分发的流量路径控制（nftables + ip rule + 多路由表）

## 快速开始

### 环境要求

- **控制机**：Linux (Debian 12+/Ubuntu 22.04+)，systemd，公网可达地址
- **普通节点**：Linux (Debian 12+/Ubuntu 22.04+)，systemd
- **Python** 3.10+
- 可选：Node.js 20+ (仅构建前端)

### 1. 安装控制机

```bash
# 从 GitHub 下载安装脚本
curl -fsSL https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/install.sh -o /tmp/pw-install.sh

# 检查脚本内容后执行
sudo bash /tmp/pw-install.sh
```

安装过程将引导你设置：
- 控制机名称
- 管理员密码（Argon2id 哈希存储）
- Web 管理端口（默认 8443）
- 节点服务端口（默认 8444）
- WireGuard UDP 端口范围（默认 30000-30999）
- 控制机公网地址
- Overlay IPv4 网段

安装完成后访问 `http://<控制机IP>:8443` 登录管理面板。

### 2. 接入新节点

1. 登录 Web UI → "接入新节点"
2. 选择父节点 → 输入新节点名称 → 生成安装命令
3. 在目标机器上执行安装命令

```bash
# 示例：从节点 A 安装新节点 B（不需要访问控制机或 GitHub）
curl -fsSL "http://<节点A的IP>:8444/bootstrap/install.sh?token=<TOKEN>" | sudo bash
```

新节点将从父节点获取安装文件、依赖和初始配置，形成多级链式部署。

### 3. 建立 WireGuard 链路

在拓扑图中选择两个节点 → "建立链路" → 系统自动分配端口、填充公钥 → 握手建立。

### 4. 配置路径策略

1. "路径策略" → "新建策略"
2. 设置匹配条件（源 CIDR、目标 CIDR、协议、端口）
3. 按序选择路径节点
4. 系统自动验证相邻节点链路存在
5. "配置预览" → 确认 → "发布"

## 开发

```bash
# 安装 Python 依赖
pip install -r server/requirements.txt

# 启动后端
PW_STATIC_DIR=web/dist python server/main.py

# 前端开发
cd web && npm install && npm run dev

# 前端构建
cd web && npm run build
```

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `PW_WEB_PORT` | `8443` | Web 管理端口 |
| `PW_NODE_PORT` | `8444` | 节点服务端口 |
| `PW_WG_PORT_START` | `30000` | WireGuard 端口池起始 |
| `PW_WG_PORT_END` | `30999` | WireGuard 端口池结束 |
| `PW_PUBLIC_ADDRESS` | `127.0.0.1` | 控制机公网地址 |
| `PW_DB_PATH` | `./pathweaver.db` | SQLite 数据库路径 |
| `PW_STATIC_DIR` | 空 | 前端静态文件目录 |
| `PW_OVERLAY_CIDR` | `10.250.0.0/16` | Overlay 网段 |
| `PW_SESSION_HOURS` | `48` | 会话过期时间 |
| `PW_MAX_LOGIN_ATTEMPTS` | `5` | 登录限速次数 |
| `PW_LOGIN_WINDOW_SEC` | `300` | 登录限速窗口(秒) |
| `PW_TLS` | 空 | 设为 `1` 启用 Cookie Secure 标志 |
| `PW_CONTROLLER` | `1` | 设为 `1` 启动依赖缓存 |

## 安全特性

- **密码**：Argon2id 哈希（time_cost=3, memory_cost=65536）
- **会话**：HttpOnly + SameSite + Secure(HTTPS) Cookie，48h 过期
- **密钥**：WireGuard 私钥 Fernet 加密存储
- **Token**：HMAC-SHA256 签名 + nonce，一次性，绑定父节点
- **速率限制**：登录 5次/5分钟 按 IP 限速
- **审计日志**：所有 CUD 操作全量记录
- **配置发布**：Prepare → Activate → Verify 状态机，支持回滚

## API 概览

| 端点 | 方法 | 说明 |
|------|------|------|
| `/api/auth/login` | POST | 管理员登录 |
| `/api/auth/logout` | POST | 退出登录 |
| `/api/auth/me` | GET | 当前用户信息 |
| `/api/auth/change-password` | POST | 修改密码 |
| `/api/auth/revoke-sessions` | POST | 撤销其他会话 |
| `/api/nodes` | GET | 节点列表 |
| `/api/nodes/{id}` | PUT | 更新节点（地址、端口池） |
| `/api/nodes/{id}/rename` | PUT | 重命名节点 |
| `/api/links` | GET/POST | 链路管理 |
| `/api/links/{id}` | PUT/DELETE | 更新/删除链路 |
| `/api/policies` | GET/POST | 策略管理 |
| `/api/policies/{id}` | PUT/DELETE | 更新/删除策略 |
| `/api/enrollment/tokens` | GET/POST | 接入 Token 管理 |
| `/api/enrollment/tokens/{id}/revoke` | POST | 撤销 Token |
| `/api/config/preview` | GET | 配置预览 |
| `/api/config/publish` | POST | 发布配置 |
| `/api/config/revisions` | GET | 配置版本历史 |
| `/api/updates` | GET/POST | 更新作业管理 |
| `/api/updates/{id}/start` | POST | 启动更新 |
| `/api/updates/{id}/rollback` | POST | 回滚更新 |
| `/api/audit-logs` | GET | 审计日志 |
| `/api/health` | GET | 健康检查 |
| `/ws` | WS | WebSocket 实时推送 |

---

## 完整卸载脚本

### 卸载控制机

```bash
#!/bin/bash
# PathWeaver 控制机卸载脚本
set -e

echo "============================================"
echo "  PathWeaver 控制机卸载"
echo "============================================"
echo ""

# 确认操作
read -p "确认卸载 PathWeaver 控制机？这将删除所有配置、密钥和数据库。[y/N] " -r
if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
    echo "已取消"
    exit 0
fi

echo ""
echo "[1/6] 停止服务..."
systemctl stop pathweaver 2>/dev/null || true
systemctl stop pathweaver-agent 2>/dev/null || true
systemctl stop pathweaver-netd 2>/dev/null || true
systemctl stop pathweaver-updater 2>/dev/null || true

echo "[2/6] 禁用服务..."
systemctl disable pathweaver 2>/dev/null || true
systemctl disable pathweaver-agent 2>/dev/null || true
systemctl disable pathweaver-netd 2>/dev/null || true
systemctl disable pathweaver-updater 2>/dev/null || true

echo "[3/6] 移除 systemd 单元文件..."
rm -f /etc/systemd/system/pathweaver.service
rm -f /etc/systemd/system/pathweaver-agent.service
rm -f /etc/systemd/system/pathweaver-netd.service
rm -f /etc/systemd/system/pathweaver-updater.service
systemctl daemon-reload

echo "[4/6] 删除应用目录..."
rm -rf /opt/pathweaver

echo "[5/6] 清理残留的网络接口..."
# 移除所有 pw-* WireGuard 接口
for iface in $(ip link show 2>/dev/null | grep -oP 'pw\w+-\w+' | sort -u); do
    echo "  移除接口: $iface"
    ip link delete "$iface" 2>/dev/null || true
done

# 移除 pw-lo dummy 接口
ip link delete pw-lo 2>/dev/null || true

# 清理 nftables 规则
if command -v nft &>/dev/null; then
    nft list tables 2>/dev/null | grep -i pathweaver | while read table; do
        table_name=$(echo "$table" | awk '{print $2}')
        echo "  移除 nftables 表: $table_name"
        nft delete table inet "$table_name" 2>/dev/null || true
    done
fi

echo "[6/6] 清理完成"
echo ""
echo "PathWeaver 控制机已完全卸载。"
echo "以下内容已被删除："
echo "  - 所有 systemd 服务单元"
echo "  - /opt/pathweaver 目录（程序、配置、密钥、数据库）"
echo "  - 所有 WireGuard 接口（pw-*）"
echo "  - nftables 规则"
```

### 卸载普通节点

```bash
#!/bin/bash
# PathWeaver 普通节点卸载脚本
set -e

echo "============================================"
echo "  PathWeaver 节点卸载"
echo "============================================"
echo ""

read -p "确认卸载此 PathWeaver 节点？[y/N] " -r
if [[ ! "$REPLY" =~ ^[Yy]$ ]]; then
    echo "已取消"
    exit 0
fi

echo ""
echo "[1/5] 停止服务..."
systemctl stop pathweaver 2>/dev/null || true
systemctl stop pathweaver-agent 2>/dev/null || true
systemctl stop pathweaver-netd 2>/dev/null || true
systemctl stop pathweaver-updater 2>/dev/null || true

echo "[2/5] 禁用服务..."
systemctl disable pathweaver 2>/dev/null || true
systemctl disable pathweaver-agent 2>/dev/null || true
systemctl disable pathweaver-netd 2>/dev/null || true
systemctl disable pathweaver-updater 2>/dev/null || true

echo "[3/5] 移除 systemd 单元..."
rm -f /etc/systemd/system/pathweaver.service
rm -f /etc/systemd/system/pathweaver-agent.service
rm -f /etc/systemd/system/pathweaver-netd.service
rm -f /etc/systemd/system/pathweaver-updater.service
systemctl daemon-reload

echo "[4/5] 删除应用目录..."
rm -rf /opt/pathweaver

echo "[5/5] 清理网络接口..."
for iface in $(ip link show 2>/dev/null | grep -oP 'pw\w+-\w+' | sort -u); do
    echo "  移除接口: $iface"
    ip link delete "$iface" 2>/dev/null || true
done
ip link delete pw-lo 2>/dev/null || true

echo ""
echo "PathWeaver 节点已完全卸载。"
```

### 一键卸载命令

```bash
# 控制机
curl -fsSL https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/installer/uninstall-controller.sh | sudo bash

# 普通节点
curl -fsSL https://raw.githubusercontent.com/FengYuchen1314/sd-wan-plus/master/installer/uninstall-node.sh | sudo bash
```

## 数据库备份与恢复

```bash
# 备份（控制机上执行）
cp /opt/pathweaver/pathweaver.db /opt/pathweaver/pathweaver.db.bak.$(date +%Y%m%d_%H%M%S)

# 也可以通过 SQLite 在线备份
sqlite3 /opt/pathweaver/pathweaver.db ".backup /tmp/pathweaver_backup.db"

# 恢复
systemctl stop pathweaver
cp /tmp/pathweaver_backup.db /opt/pathweaver/pathweaver.db
systemctl start pathweaver
```

## License

MIT
