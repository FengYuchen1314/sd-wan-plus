# PathWeaver 最终开发计划

## 单用户 · Go 后端 · Vue 控制台 · 递归部署 · 图状 WireGuard SD-WAN

------

# 1. 项目最终定义

PathWeaver 是一套自托管 SD-WAN 组网与控制平台。

系统由一台具有公网访问能力的**控制机**开始部署。控制机安装完成后提供唯一的 Web 控制台。管理员通过控制台选择任意已有节点作为父节点，生成一条一次性安装命令，让新节点主动连接该父节点。

父节点负责向新节点传输 PathWeaver 安装文件、Agent、更新制品和初始配置。新节点接入后，也可以继续作为下一级父节点，为更多节点提供安装和控制中继。

节点不断递归加入后，形成：

1. 一棵用于安装、管理和更新传递的**控制树**；
2. 一张可以增加额外链路的 **WireGuard 数据图**；
3. 一套由控制机统一编译和下发的**流量路径策略**。

系统只有控制机运行 Web UI。普通节点不运行管理面板，只运行 Go Agent、网络服务和更新服务。

------

# 2. 固定产品边界

## 2.1 用户模型

V1 只支持单用户。

控制机安装时要求管理员输入密码：

- 不开放用户注册；
- 不实现组织；
- 不实现租户；
- 不实现 RBAC；
- 不实现多管理员；
- 不实现邀请用户。

管理员使用密码登录控制机 Web UI。

密码必须使用 Argon2id 哈希保存，不保存明文。

登录方式：

```text
用户名固定：admin
密码：安装时设置
```

Web 登录采用安全 Session Cookie：

- HttpOnly；
- SameSite；
- HTTPS 时启用 Secure；
- 支持退出登录；
- 支持修改密码；
- 支持撤销全部 Session。

------

## 2.2 控制机

控制机必须：

- 具有管理员可以访问的公网地址或域名；
- 能够访问 GitHub 下载首个安装脚本和发布制品；
- 运行唯一的 Web UI；
- 保存全网拓扑数据库；
- 保存管理员密码；
- 编译全网 WireGuard 配置；
- 编译流量路径策略；
- 签发节点接入凭证；
- 发起全网更新；
- 保存审计日志。

控制机同时也是第一个 PathWeaver 节点，可以参与 WireGuard 数据面。

------

## 2.3 普通节点

普通节点只运行：

```text
pathweaver-agent
pathweaver-netd
pathweaver-updater
```

普通节点不运行：

- Web 管理面板；
- 管理员登录接口；
- 独立数据库后台；
- Node.js；
- 前端服务。

普通节点可以：

- 主动连接上级节点；
- 接收下级节点连接；
- 中继控制消息；
- 中继安装文件；
- 缓存更新制品；
- 参与 WireGuard 转发；
- 作为下一个新节点的父节点。

------

# 3. 最终架构

```text
┌───────────────────────────────────────────────────────────┐
│                    控制机 Web UI                           │
│ React + TypeScript                                        │
│                                                           │
│ 登录 · 节点 · 拓扑 · 链路 · 路径策略 · 安装 · 更新          │
└──────────────────────────┬────────────────────────────────┘
                           │ REST + WebSocket
┌──────────────────────────▼────────────────────────────────┐
│             pathweaver-controller（仅控制机）              │
│                                                           │
│ 单用户认证 · 节点注册 · 控制树 · 数据图                     │
│ WireGuard 配置编译 · 路径策略编译 · 更新编排                │
│ SQLite · 制品缓存 · 审计日志                               │
└──────────────────────────┬────────────────────────────────┘
                           │ 控制消息向下传递
                  ┌────────▼────────┐
                  │  控制机 Agent    │
                  └────────┬────────┘
                           │
                安装 / 控制 / 制品传递
                           │
          ┌────────────────▼────────────────┐
          │              节点 A             │
          │ Agent · netd · updater          │
          └───────────┬──────────────┬──────┘
                      │              │
            ┌─────────▼──────┐  ┌────▼───────────┐
            │     节点 B      │  │     节点 C      │
            └─────────┬──────┘  └────────────────┘
                      │
               ┌──────▼──────┐
               │    节点 D    │
               └─────────────┘
```

控制树可以是：

```text
控制机
├── 节点 A
│   ├── 节点 B
│   │   └── 节点 D
│   └── 节点 C
└── 节点 E
```

WireGuard 数据图可以另外增加交叉链路：

```text
控制机 ─ A ─ B ─ D
   │      │   ╲
   E ─────C────D
```

控制树和 WireGuard 数据图必须分开保存。

增加 WireGuard 交叉链路，不改变节点原来的管理父节点。

------

# 4. Go + Vue 技术方案

## 4.1 后端

| 部分       | 技术                                           |
| ---------- | ---------------------------------------------- |
| 语言       | Go 1.22+                                       |
| Web API    | chi + gorilla/websocket                        |
| 节点控制面 | HTTP bootstrap / DesiredState 拉取；可扩展 gRPC |
| 数据库     | SQLite（modernc.org/sqlite）                   |
| 序列化     | encoding/json + Protobuf 协议定义              |
| CLI        | pathweaver-cli                                 |
| 密码哈希   | Argon2id（golang.org/x/crypto）                |
| 本地 IPC   | Unix Domain Socket（netd JSON）                |
| 网络配置   | ip / wg / nftables（结构化 DesiredState）      |
| 安装包签名 | Ed25519                                        |
| 制品校验   | SHA-256                                        |

## 4.2 前端

```text
Vue 3
TypeScript
Vite
Pinia
Vue Router
Cytoscape.js
```

前端只在构建时依赖 Node.js。

正式安装包内只包含编译后的静态文件，由 Go Controller 托管。生产服务器不安装 Node.js。

------

# 5. 进程划分

## 5.1 pathweaver-controller

只运行在控制机。

职责：

- 单用户登录；
- Web UI；
- 节点管理；
- 节点重命名；
- 控制树管理；
- WireGuard 数据图管理；
- 接入 Token 签发；
- 路径策略配置；
- 全网配置编译；
- 配置发布；
- 更新编排；
- 制品缓存；
- 状态聚合；
- 审计记录。

------

## 5.2 pathweaver-agent

控制机和所有普通节点均运行。

职责：

- 保存节点身份；
- 保存 WireGuard 密钥；
- 连接上级节点；
- 接收和转发控制消息；
- 为下级节点提供接入入口；
- 接收期望配置；
- 调用 netd；
- 上报节点状态；
- 执行链路探测；
- 缓存安装和更新制品；
- 将下级节点消息递归转发给控制机。

------

## 5.3 pathweaver-netd

拥有网络管理权限。

职责：

- 创建 WireGuard 接口；
- 设置 WireGuard Peer；
- 设置 Endpoint；
- 设置监听端口；
- 配置 Overlay 地址；
- 配置路由表；
- 配置 `ip rule`；
- 配置 nftables；
- 开启转发；
- 设置出口 NAT；
- 读取握手和流量状态；
- 执行网络配置回滚。

netd 不接受任意 Shell 命令，只接受结构化配置。

------

## 5.4 pathweaver-updater

独立于 Agent。

职责：

- 接收更新任务；
- 下载或从父节点领取制品；
- 校验签名；
- 校验哈希；
- Stage 新版本；
- 原子切换版本；
- 重启服务；
- 执行健康检查；
- 失败后自动回滚。

Updater 独立存在，是为了保证 Agent 自身更新失败时仍有恢复能力。

------

# 6. 端口模型

控制机安装时要求用户设置以下内容：

| 参数                   | 用途                       |
| ---------------------- | -------------------------- |
| Web 管理端口           | 控制机 UI 和浏览器 API     |
| 节点服务端口           | 安装、控制中继、制品传输   |
| WireGuard UDP 端口范围 | 自动为新增链路分配端口     |
| 控制机公网地址         | 浏览器访问和首个节点接入   |
| 管理员密码             | Web 登录                   |
| Overlay IPv4 网段      | 默认可用预设值             |
| TLS 模式               | 域名证书、已有证书或自签名 |

建议默认值：

```text
Web 管理端口：14301/TCP
节点服务端口：14302/TCP
WireGuard 端口池：14303-14399/UDP
Overlay 网段：10.250.0.0/16
```

每个普通节点安装时保存：

```text
节点服务端口
WireGuard UDP 端口池
可供其他节点访问的地址列表
上级节点地址
上级节点服务端口
恢复地址
```

端口不是临时猜测值，必须进入数据库长期保存。

------

# 7. 控制机首次安装

## 7.1 安装方式

控制机通过 GitHub 脚本安装：

```bash
curl -fsSL https://raw.githubusercontent.com/<owner>/pathweaver/main/install-controller.sh \
  -o /tmp/pathweaver-install.sh

sudo bash /tmp/pathweaver-install.sh
```

正式版本必须支持安装器签名或 SHA-256 校验，不能长期使用无校验的 `curl | bash`。

------

## 7.2 安装过程交互

安装器依次询问：

```text
1. 控制机名称
2. 管理员密码
3. Web UI 监听端口
4. 节点服务端口
5. WireGuard UDP 端口范围
6. 控制机公网 IP 或域名
7. Overlay IPv4 地址池
8. 是否启用 IPv6 Overlay
9. TLS 配置
10. 是否允许系统自动配置防火墙
```

安装器完成：

1. 检查 Linux 和 systemd；
2. 检查 CPU 架构；
3. 下载签名 Release；
4. 安装 Controller；
5. 安装 Agent；
6. 安装 netd；
7. 安装 updater；
8. 创建 SQLite；
9. 初始化管理员密码；
10. 创建节点身份；
11. 创建 WireGuard 密钥；
12. 创建系统服务；
13. 启动控制机；
14. 输出 Web UI 地址。

------

# 8. 新节点递归接入

## 8.1 核心操作

管理员在控制机 UI 中打开：

```text
接入新节点
```

然后：

1. 从现有节点列表中选择父节点；
2. 选择父节点可供新节点访问的地址；
3. 系统自动填充父节点服务端口；
4. 设置新节点名称；
5. 设置 Token 有效时间；
6. 生成一次性安装命令。

最初的第一个普通节点选择控制机作为父节点。

后续节点可以选择任何已经在线的节点作为父节点。

------

## 8.2 安装命令示例

```bash
curl -fsSL \
  "https://203.0.113.10:8444/bootstrap/install.sh?token=<ONE_TIME_TOKEN>" \
  -o /tmp/pathweaver-node-install.sh

sudo bash /tmp/pathweaver-node-install.sh
```

命令中的地址是所选父节点的可达地址，而不是永远指向控制机。

例如：

```text
节点 A 从控制机安装
节点 B 从节点 A 安装
节点 C 从节点 A 安装
节点 D 从节点 B 安装
```

新节点不要求直接访问 GitHub，也不要求直接访问控制机。

------

## 8.3 父节点传输文件

所有已激活节点必须缓存当前版本的：

```text
节点安装器
Agent
netd
updater
WireGuard userspace 回退制品
Release Manifest
Release 签名
```

新节点请求安装时：

1. 父节点检查 Token；
2. 父节点从本地缓存返回安装器；
3. 如果本地没有制品，则向自己的父节点请求；
4. 制品逐级向上查找；
5. 下载完成后父节点本地缓存；
6. 父节点再向新节点传输。

因此可以形成：

```text
控制机
  ↓
节点 A
  ↓
节点 B
  ↓
节点 D
```

节点 D 的安装文件可以沿控制树逐级传递，而不需要节点 D 访问控制机或 GitHub。

------

## 8.4 一次性 Token

Token 必须包含：

```text
network_id
parent_node_id
suggested_node_name
expires_at
nonce
allowed_install_mode
controller_signature
```

安全规则：

- 默认十分钟有效；
- 只能使用一次；
- 绑定指定父节点；
- 使用后立即失效；
- 可以在 UI 手动撤销；
- 父节点能够用控制机公钥验证签名；
- Token 不能用于管理控制台登录；
- Token 不能执行任意命令。

------

## 8.5 新节点安装过程

```text
1. 从父节点下载安装器
2. 校验制品签名
3. 安装 Agent、netd、updater
4. 生成节点身份私钥
5. 生成 WireGuard 私钥
6. 向父节点提交注册请求
7. 父节点逐级中继至控制机
8. 控制机分配节点 UUID 和 Overlay IP
9. 控制机生成父子 WireGuard 链路配置
10. 父节点先配置被动 Peer
11. 新节点配置父节点 Endpoint
12. 新节点主动发起 WireGuard 握手
13. Overlay 建立
14. Agent 切换到 Overlay 控制通道
15. 执行链路探测
16. UI 显示接入完成
```

------

# 9. WireGuard 链路语义

必须避免使用容易误解的“单向隧道”说法。

WireGuard 链路建立后，流量可以双向传输。

所谓“单向发起握手”是：

- 一端保存对方固定 Endpoint；
- 另一端不保存固定 Endpoint；
- 有 Endpoint 的一端负责主动发起握手；
- 被动端通过收到的数据包学习对端地址。

------

## 9.1 初始父子链路

新节点 B 加入父节点 A：

### B 的配置

```text
Peer PublicKey：A
Endpoint：A 的可达 IP:A 的链路监听端口
PersistentKeepalive：25
```

### A 的配置

```text
Peer PublicKey：B
Endpoint：不设置
PersistentKeepalive：0
```

因此：

```text
B 主动发起握手
A 被动接收并学习 B 的 Endpoint
```

节点加入后，数据仍然可以双向传输。

------

# 10. 为什么每条链路使用独立 WireGuard 接口

PathWeaver 要支持管理员为不同流量指定不同路径。

如果所有 Peer 都放在同一个 WireGuard 接口里，WireGuard 的 AllowedIPs 会同时承担 Peer 选择功能，很难让相同目标流量根据不同策略走不同下一跳。

因此最终设计为：

> 每一条逻辑链路对应一组独立 WireGuard 接口。

例如：

```text
节点 A 与节点 B：
pwl-ab

节点 A 与节点 C：
pwl-ac

节点 B 与节点 D：
pwl-bd
```

每个链路接口只包含一个 Peer。

节点自身具有固定 Overlay 身份地址，放在 Dummy 接口：

```text
pw-lo
10.250.0.x/32
```

WireGuard 链路接口用于传输，`pw-lo` 地址代表节点身份。

这样可以实现：

- 一条链路一个明确出口；
- 不同路由表选择不同 WireGuard 链路；
- 同一目标可以按不同策略走不同路径；
- 不依赖单个 `wg0` 的 AllowedIPs 选路；
- 更容易执行显式路径控制；
- 更容易统计每条链路流量。

------

# 11. 新增任意 WireGuard 链路

## 11.1 UI 操作

管理员在拓扑图中选择两个节点，例如：

```text
节点 B
节点 C
```

系统弹出“建立链路”窗口。

窗口自动显示：

```text
节点 B：
- 节点名称
- Overlay IP
- WireGuard 公钥
- 可用 UDP 端口范围
- 已保存可达地址

节点 C：
- 节点名称
- Overlay IP
- WireGuard 公钥
- 可用 UDP 端口范围
- 已保存可达地址
```

管理员设置：

1. 哪一端主动发起；
2. 另一端可被访问的 IP 或域名；
3. 可选链路名称；
4. 可选链路权重；
5. 是否立即启用。

系统自动完成：

- 为被访问端分配空闲 UDP 端口；
- 填充双方公钥；
- 创建链路 ID；
- 创建两端独立 WireGuard 接口；
- 先配置被动端；
- 再配置主动端；
- 触发握手；
- 测试 Overlay 连通；
- 将链路添加到拓扑图。

------

## 11.2 示例

管理员选择：

```text
主动端：节点 B
被访问端：节点 C
```

用户只需填写：

```text
节点 B 可以访问的节点 C 地址：
192.0.2.30
```

系统已知节点 C 的可用 WireGuard 端口池，并自动分配：

```text
节点 C WireGuard 监听端口：30008/UDP
```

生成配置：

```text
B：
Endpoint = 192.0.2.30:30008
PersistentKeepalive = 25

C：
不设置 B 的固定 Endpoint
```

这种新增链路只改变 WireGuard 数据图，不改变 B、C 的控制父子关系。

------

# 12. 节点重命名

节点必须具有两个名称：

```text
immutable_node_id
display_name
```

`immutable_node_id`：

- 创建后永不改变；
- 用于数据库关系；
- 用于证书；
- 用于控制协议；
- 用于配置版本。

`display_name`：

- 可以在 UI 中随时修改；
- 用于拓扑展示；
- 不影响节点身份；
- 不重新生成密钥；
- 不改变 Overlay IP；
- 不影响 WireGuard 链路。

禁止使用节点名称作为数据库主键。

------

# 13. 控制树和数据图

## 13.1 控制树

控制树负责：

- 新节点安装；
- 注册消息中继；
- 心跳中继；
- 配置中继；
- 制品中继；
- 更新状态中继；
- 故障恢复。

每个普通节点只有一个控制父节点。

控制树必须无环。

------

## 13.2 WireGuard 数据图

WireGuard 数据图负责：

- 节点间业务流量；
- Overlay 控制流量；
- 多跳转发；
- 显式路径；
- 外部网段转发；
- 出口节点访问。

一个节点可以拥有多条 WireGuard 链路。

WireGuard 数据图允许出现环，但路径编译结果本身不能出现转发环。

------

# 14. 流量路径控制

控制面板必须提供“路径策略”页面。

管理员可以决定特定流量经过哪些节点。

------

## 14.1 策略匹配条件

V1 支持以下匹配字段：

```text
源节点
源节点组
源 IP/CIDR
目标节点
目标 IP/CIDR
协议：TCP / UDP / ICMP / 任意
目标端口
目标端口范围
```

V1 不直接识别：

```text
应用名称
域名规则
进程名称
TLS SNI
网站类别
```

域名和应用识别可以在后续版本增加，V1 先完成稳定的 IP、协议和端口路径控制。

------

## 14.2 策略动作

每条策略可以设置：

```text
显式节点路径
出口节点
是否出口 NAT
返回路径
优先级
故障备用路径
启用或禁用
```

示例：

```text
匹配：
源节点 = 上海 NAS
目标 CIDR = 192.168.100.0/24
协议 = 任意

路径：
上海 NAS
→ 腾讯云
→ 上海 IX
→ 日本节点
→ 荷兰节点
```

另一个示例：

```text
匹配：
源节点 = 上海 NAS
目标 CIDR = 0.0.0.0/0
协议 = TCP
目标端口 = 443

路径：
上海 NAS
→ 腾讯云
→ 上海 IX
→ 日本出口

出口 NAT：
启用
```

------

## 14.3 路径编辑器

UI 中采用图形化方式：

1. 选择源节点；
2. 选择目标节点、CIDR 或互联网；
3. 选择匹配协议和端口；
4. 在拓扑图上依次点击路径节点；
5. 系统验证每两个相邻节点之间已有 WireGuard 链路；
6. 设置返回路径；
7. 保存；
8. Controller 编译并预览配置；
9. 用户确认后发布。

如果路径中某一段不存在链路，UI 必须提示：

```text
节点 B 与节点 C 之间没有直接 WireGuard 链路
```

并提供“建立链路”入口。

------

## 14.4 路径实现方式

每条链路使用独立 WireGuard 接口。

Controller 根据策略生成：

```text
nftables 流量标记
ip rule
独立路由表
下一跳 WireGuard 接口
出口 NAT 规则
```

处理流程：

```text
流量进入节点
    ↓
nftables 根据源、目标、协议、端口匹配
    ↓
设置 fwmark
    ↓
ip rule 选择对应路由表
    ↓
路由表选择指定 WireGuard 链路接口
    ↓
传递到下一节点
```

中间节点会收到同一份路径规则，因此能够继续把流量传向下一个指定节点。

------

## 14.5 返回路径

每条策略必须明确返回路径。

支持：

### 对称返回

```text
A → B → C → D
D → C → B → A
```

这是默认模式。

### 独立返回路径

管理员可以另行设置：

```text
去程：A → B → C → D
回程：D → E → A
```

系统必须验证：

- 每一段链路存在；
- 没有转发环；
- 出口和入口配置一致；
- NAT 场景返回流量可以正确关联。

------

## 14.6 路径优先级

策略按优先级匹配：

```text
数值越小，优先级越高
```

系统必须阻止明显冲突，例如：

- 完全相同匹配条件但不同路径；
- 同一优先级的重叠规则；
- 返回路径不存在；
- 路径包含重复节点；
- 目标网段无法到达；
- 出口节点未开启 NAT。

------

# 15. 配置编译与发布

Controller 不向节点发送 Shell 命令。

Controller 生成完整期望状态：

```text
NodeDesiredState {
    generation
    node_id

    overlay_identity
    wireguard_links
    listen_ports
    peers

    nftables_rules
    policy_rules
    route_tables
    forwarding_settings
    nat_rules

    control_parent
    recovery_endpoint

    config_hash
}
```

节点收到后执行：

```text
Prepare
→ 校验
→ 激活
→ 健康检查
→ 确认
```

------

## 15.1 发布状态

```text
PENDING
DISPATCHED
PREPARING
PREPARED
ACTIVATING
ACTIVE
```

失败状态：

```text
PREPARE_FAILED
ACTIVATE_FAILED
VERIFY_FAILED
ROLLING_BACK
ROLLED_BACK
```

UI 必须显示每台节点当前阶段和失败原因。

------

## 15.2 发布顺序

新增父子链路：

```text
1. 被动端 Prepare
2. 被动端 Activate
3. 主动端 Prepare
4. 主动端 Activate
5. 主动端发起握手
6. 执行链路探测
7. 链路标记为 Active
```

新增路径策略：

```text
1. 最终出口节点
2. 中间节点，由终点向源节点发布
3. 源节点最后发布
4. 执行路径探测
```

删除路径时顺序相反，先停止源节点发送新流量。

------

# 16. 节点状态显示

UI 必须分开显示：

| 状态               | 含义                       |
| ------------------ | -------------------------- |
| Agent 在线         | 控制消息正常               |
| 配置一致           | 节点配置版本与控制机一致   |
| WireGuard 接口正常 | 链路接口创建成功           |
| 最近握手           | WireGuard 最近成功握手时间 |
| Overlay 可达       | 应用层探测成功             |
| 路径可用           | 显式路径端到端探测成功     |
| 更新一致           | 节点运行目标版本           |

不能只显示一个“在线/离线”。

一个节点可能：

```text
Agent 在线
配置一致
WireGuard 接口正常
最近没有握手
Overlay 不可达
```

UI 必须准确显示这种状态。

------

# 17. 链路和路径探测

## 17.1 相邻链路探测

主动握手端负责执行标准探测。

结果包括：

```text
成功次数
失败次数
丢包率
最小 RTT
中位 RTT
P95 RTT
抖动
最近探测时间
数据是否过期
错误代码
```

------

## 17.2 端到端路径探测

管理员选择两个节点时，可以点击：

```text
测试路径
```

系统：

1. 根据路径策略确定节点序列；
2. 从源节点向目标 Overlay IP 发起应用层 Echo；
3. 流量按照当前策略经过中间节点；
4. 返回实际端到端 RTT；
5. 同时显示各相邻链路历史 RTT；
6. 显示估算 RTT 和实测 RTT。

------

# 18. 更新自动扩散

## 18.1 更新来源

控制机负责从 GitHub 获取新版本。

普通节点不需要访问 GitHub。

更新流程只能从控制机 UI 发起：

```text
检查更新
→ 下载 Release
→ 校验签名
→ 创建全网更新任务
```

------

## 18.2 制品预分发

更新不能直接让父节点先重启。

必须先执行预分发：

```text
1. 控制机下载并缓存制品
2. 控制机把制品传给直接子节点
3. 子节点校验并缓存
4. 子节点继续传给自己的子节点
5. 所有在线节点先完成 Stage
6. Stage 完成后才开始安装
```

制品传播方式：

```text
控制机
  ↓
节点 A
  ↓
节点 B
  ↓
节点 D
```

每个节点下载一次，然后可以为多个子节点提供缓存。

------

## 18.3 更新安装顺序

安装顺序从控制树叶子开始：

```text
最深层叶子节点
→ 中间节点
→ 控制机直接子节点
→ 控制机最后
```

例如：

```text
先更新 D
再更新 B、C
再更新 A
最后更新控制机
```

这样可以避免父节点提前重启导致子节点失去更新通道。

------

## 18.4 更新状态机

```text
WAITING
PREFETCHING
VERIFYING
STAGED
INSTALLING
RESTARTING
HEALTH_CHECKING
COMPLETED
```

失败状态：

```text
DOWNLOAD_FAILED
SIGNATURE_INVALID
INSTALL_FAILED
HEALTH_CHECK_FAILED
ROLLING_BACK
ROLLED_BACK
```

------

## 18.5 更新安全

每个 Release 包含：

```text
manifest.json
manifest.sig
controller
agent
netd
updater
pathweaver-cli
frontend/
database-migrations/
userspace-wireguard/
```

Manifest 包含：

```text
产品版本
协议版本
目标架构
每个文件 SHA-256
最低兼容版本
Git Commit
构建时间
是否允许降级
```

所有节点内置 Release 公钥。

Updater 必须同时验证：

```text
Ed25519 签名
SHA-256 哈希
CPU 架构
协议兼容性
版本升级路径
```

------

## 18.6 原子升级和回滚

目录结构：

```text
/opt/pathweaver/
├── current -> releases/1.2.0
├── releases/
│   ├── 1.1.0/
│   └── 1.2.0/
└── artifacts/
```

更新过程：

```text
1. 解压到新的 releases 目录
2. 验证文件
3. 执行兼容性检查
4. 切换 current 软链接
5. 重启服务
6. 检查 Agent、netd 和 Overlay
7. 成功则确认
8. 失败则切回旧软链接
9. 重启旧版本
```

更新不得删除：

```text
节点身份密钥
WireGuard 私钥
Overlay IP
节点 UUID
本地数据库
父节点配置
Last Known Good 网络配置
```

------

## 18.7 离线节点

离线节点不能永久阻塞全网任务。

离线节点恢复后：

1. 向父节点报告当前版本；
2. 父节点发现版本低于目标版本；
3. 继续发送原更新任务；
4. 节点从最近父节点缓存领取制品；
5. 完成更新；
6. 向控制机报告结果。

------

# 19. 控制通信

## 19.1 Bootstrap 通道

新节点尚未建立 WireGuard 时，通过父节点的公网或可达地址通信。

用途仅限：

- 下载安装器；
- 节点注册；
- 获取初始配置；
- 报告初始错误；
- Overlay 故障恢复。

------

## 19.2 Overlay 控制通道

WireGuard 建立后，Agent 切换到父节点 Overlay IP。

正常操作全部走 Overlay：

- 心跳；
- 配置同步；
- 状态上报；
- 更新任务；
- 制品传输；
- 探测任务；
- 日志和诊断。

Bootstrap 地址保留为 Recovery Endpoint。

------

## 19.3 控制消息中继

节点之间使用双向 gRPC Stream。

控制消息格式：

```text
RelayEnvelope {
    message_id
    source_node_id
    target_node_id
    network_id
    ttl
    trace_id
    payload_type
    payload
}
```

要求：

- TTL 防止循环；
- Message ID 防止重复执行；
- 控制父节点不能修改原始发送者身份；
- 大型制品使用独立 Artifact Stream；
- 心跳不能被更新下载阻塞；
- 控制消息支持断线重连；
- 关键任务支持幂等重试。

------

# 20. 数据库核心表

```text
admin
sessions

networks
nodes
node_addresses
node_port_pools
node_identities
node_certificates

control_relations
wireguard_links
wireguard_link_endpoints

traffic_policies
traffic_policy_matches
traffic_policy_paths
traffic_policy_hops

config_revisions
node_desired_configs
config_rollout_nodes

heartbeats
wireguard_snapshots
probe_jobs
probe_results

enrollment_tokens
artifacts
artifact_cache_records

update_jobs
update_targets

audit_logs
```

节点表重要字段：

```text
id
display_name
overlay_ipv4
overlay_ipv6
wg_public_key

control_parent_id
node_service_port
wg_port_range_start
wg_port_range_end

agent_version
protocol_version
desired_generation
active_generation
last_seen_at
```

链路表重要字段：

```text
id
node_a
node_b

initiator_node_id
listener_node_id
listener_address
listener_port

interface_name_a
interface_name_b

enabled
admin_weight
created_at
```

------

# 21. Web UI 页面

## 21.1 登录页

只有：

```text
管理员密码
登录按钮
```

------

## 21.2 Dashboard

显示：

- 节点总数；
- Agent 在线数；
- Overlay 可达数；
- WireGuard 链路数；
- 失败链路数；
- 路径策略数；
- 配置一致率；
- 当前软件版本；
- 更新状态；
- 最近错误。

------

## 21.3 节点页面

支持：

- 查看节点；
- 重命名；
- 查看 Overlay IP；
- 查看控制父节点；
- 查看端口；
- 查看已保存可达地址；
- 查看 WireGuard 链路；
- 查看版本；
- 查看最近握手；
- 查看配置状态；
- 设置节点可达地址；
- 设置 WireGuard 端口池；
- 生成以该节点为父节点的接入命令。

------

## 21.4 拓扑页面

同时展示：

- 控制树；
- WireGuard 链路；
- 链路主动端；
- 链路监听端；
- WireGuard 监听端口；
- RTT；
- 握手状态；
- 路径策略。

控制树和数据链路使用不同颜色或线型。

支持选择两个节点：

```text
建立 WireGuard 链路
测试现有路径
查看两个节点之间的路径
创建流量策略
```

------

## 21.5 接入页面

操作：

```text
选择父节点
选择父节点可达地址
自动填充父节点服务端口
输入新节点名称
设置 Token 有效期
生成命令
复制命令
```

------

## 21.6 链路页面

操作：

```text
选择两个节点
选择主动端
选择被访问端
系统自动分配 WireGuard 端口
用户填写被访问端 IP
保存并部署
查看握手
执行探测
禁用链路
删除链路
```

------

## 21.7 路径策略页面

操作：

```text
设置匹配条件
在拓扑上选择节点路径
设置返回路径
设置出口 NAT
设置优先级
验证
预览配置
发布
```

------

## 21.8 配置发布页面

显示：

- 配置 Revision；
- 配置原因；
- 节点期望版本；
- 节点当前版本；
- Prepare 状态；
- Activate 状态；
- 回滚状态；
- 失败原因；
- 配置 Diff。

------

## 21.9 更新中心

显示：

- GitHub 最新版本；
- 当前控制机版本；
- 各节点版本；
- 制品下载进度；
- 制品缓存位置；
- Stage 状态；
- 安装顺序；
- 更新结果；
- 回滚结果；
- 离线待更新节点。

------

# 22. 仓库结构

```text
pathweaver/
├── go.mod
├── go.sum
├── cmd/
│   ├── pathweaver-controller/
│   ├── pathweaver-agent/
│   ├── pathweaver-netd/
│   ├── pathweaver-updater/
│   └── pathweaver-cli/
├── internal/
│   ├── core/
│   ├── storage/
│   ├── security/
│   ├── topology/
│   ├── routing/
│   ├── controller/
│   ├── agent/
│   ├── netd/
│   ├── updater/
│   └── relay/
├── api/proto/
├── web/                 # Vue 3 + TypeScript
├── migrations/
├── installer/
├── packaging/systemd/
├── tests/
└── docs/
```

------

# 23. 开发阶段

## Phase 0：冻结协议和数据模型

首先完成：

- Controller、Agent、netd、updater 边界；
- Node 数据模型；
- 控制树数据模型；
- WireGuard 链路模型；
- 每链路独立接口模型；
- 接入 Token；
- 配置状态机；
- 更新状态机；
- 路径策略模型；
- Protobuf；
- SQLite migration；
- 错误码；
- 威胁模型。

本阶段禁止先做复杂 UI。

------

## Phase 1：控制机安装和单用户登录

实现：

- GitHub 控制机安装脚本；
- 交互式端口配置；
- 管理员密码；
- SQLite；
- Controller；
- 控制机 Agent；
- 基础 Web 登录；
- systemd 服务；
- Release 校验。

验收：

- 全新 Debian/Ubuntu 可完成控制机安装；
- 可以使用密码登录；
- 重启后配置和密码不丢失；
- UI 只能在控制机访问。

------

## Phase 2：父子节点递归接入

实现：

- 节点服务端口；
- 一次性 Token；
- 父节点安装器服务；
- 制品缓存；
- 新节点安装；
- 注册消息递归中继；
- 节点身份；
- 节点列表；
- 节点重命名。

验收：

```text
控制机安装 A
A 安装 B
B 安装 C
```

C 不访问控制机和 GitHub，也可以成功出现在控制机 UI。

------

## Phase 3：父子 WireGuard 链路

实现：

- `pw-lo` Overlay 身份接口；
- 每链路独立 WireGuard 接口；
- UDP 端口池；
- 父节点被动 Peer；
- 子节点主动 Endpoint；
- Keepalive；
- Overlay 地址；
- 链路状态；
- 网络配置回滚。

验收：

- B 主动连接 A；
- A 不保存 B 的固定 Endpoint；
- 双向 Overlay 流量可用；
- 每条链路有独立接口；
- 节点重启后自动恢复。

------

## Phase 4：控制树和三层传递

测试：

```text
控制机 → A → B → C
```

实现：

- Overlay 控制通道；
- Recovery 通道；
- 控制消息中继；
- 心跳中继；
- 配置中继；
- Artifact 中继；
- TTL；
- 消息去重；
- 断线重连。

验收：

- 控制机可以管理 C；
- C 的消息经 B、A 到达控制机；
- A、B 不需要 Web UI；
- Overlay 故障时可以走 Recovery。

------

## Phase 5：任意节点新增链路

实现：

- 拓扑图双节点选择；
- 主动端选择；
- 被访问端选择；
- 自动分配目标 UDP 端口；
- 用户输入目标可达 IP；
- 两阶段配置发布；
- 握手测试；
- 删除和禁用链路。

验收：

- 可以在已有树状网络中增加交叉链路；
- 增加链路不改变控制父节点；
- 用户不需要手动填写公钥和端口；
- 用户只需指定主动端和被访问端 IP。

------

## Phase 6：流量路径控制

实现：

- 匹配条件；
- 显式节点序列；
- 返回路径；
- 每链路路由；
- nftables mark；
- `ip rule`；
- 多路由表；
- 出口 NAT；
- 冲突检查；
- 路径探测；
- 配置预览。

验收：

- 同一节点的不同目标流量可以走不同路径；
- TCP/UDP 端口可以匹配；
- 路径中断不会形成转发环；
- 显式路径与 UI 展示一致；
- 出口节点可以处理默认路由和 NAT。

------

## Phase 7：完整 UI

实现：

- Dashboard；
- 节点页；
- 拓扑页；
- 接入页；
- 链路页；
- 路径策略页；
- 配置发布页；
- 更新中心；
- WebSocket 实时状态；
- 可读错误信息。

------

## Phase 8：自动扩散更新

实现：

- GitHub Release 检查；
- Controller 下载；
- Release 签名；
- 制品内容寻址；
- 控制树预分发；
- 节点缓存；
- 分块传输；
- 断点续传；
- Stage；
- 叶子优先更新；
- 控制机最后更新；
- 健康检查；
- 自动回滚；
- 离线节点续传。

验收：

- 一次操作更新全网；
- 普通节点不访问 GitHub；
- 深层节点从直接父节点获取制品；
- 更新失败自动恢复；
- 节点恢复在线后继续原任务。

------

## Phase 9：生产加固

实现：

- Agent 与 netd 权限分离；
- mTLS；
- 节点证书轮换；
- 登录限速；
- CSRF；
- Session 管理；
- 审计日志；
- 数据库备份；
- 配置备份；
- 故障注入；
- arm64；
- Debian 12/13；
- Ubuntu 22.04/24.04；
- N/N-1 协议兼容；
- 长时间运行测试。

------

# 24. 必须测试的场景

```text
1. 控制机首次安装
2. 管理员密码登录
3. 控制机生成 A 的安装命令
4. A 生成 B 的安装命令
5. B 生成 C 的安装命令
6. C 不访问 GitHub也能安装
7. 子节点主动连接父节点
8. 父节点动态学习子节点 Endpoint
9. 任意两个节点新增交叉链路
10. 节点重命名
11. 显式多跳路径
12. TCP/UDP 端口分流
13. 出口 NAT
14. 对称返回路径
15. 独立返回路径
16. 中间节点重启
17. 控制机重启
18. Overlay 故障后 Recovery
19. 配置发布失败回滚
20. 制品递归传输
21. 全网叶子优先更新
22. 控制机最后更新
23. 更新制品被篡改
24. 更新失败回滚
25. 离线节点恢复后续更
```

------

# 25. V1 不做的功能

V1 明确不实现：

- 多用户；
- 多租户；
- RBAC；
- 多控制机高可用；
- Windows；
- macOS；
- 移动端；
- 双 NAT 自动打洞；
- DERP/TURN 数据中继；
- BGP；
- OSPF；
- 域名分流；
- 应用识别；
- TLS SNI 识别；
- 自动根据 RTT 高频切换路径；
- 云厂商自动开防火墙；
- 任意用户 Shell Hook；
- 插件市场。

------

# 26. 最终验收标准

项目只有满足以下全部条件才算 V1 完成：

-  控制机可通过 GitHub 脚本交互安装；
-  安装时可以设置密码和端口；
-  系统只有一个管理员；
-  Web UI 只运行在控制机；
-  控制机可以生成新节点安装命令；
-  可以选择任意已有节点作为父节点；
-  新节点从父节点获取安装文件；
-  文件可以沿多级控制树传递；
-  普通节点不需要访问 GitHub；
-  节点接入后可以继续接入下一级节点；
-  每条 WireGuard 链路使用独立接口；
-  子节点默认主动向父节点发起握手；
-  被动端不需要保存主动端固定 IP；
-  可以在 UI 中重命名节点；
-  可以选择两个节点新增链路；
-  系统自动填充公钥和端口；
-  用户只需输入被访问端 IP 或域名；
-  新增链路不改变控制父子关系；
-  可以配置不同流量的显式节点路径；
-  支持源、目标、协议和端口匹配；
-  支持返回路径；
-  支持出口 NAT；
-  配置经过 Prepare、Activate 和 Verify；
-  配置失败能够恢复 Last Known Good；
-  控制机只需从 GitHub下载一次更新制品；
-  更新制品自动沿控制树扩散；
-  所有节点安装前先完成 Stage；
-  更新按叶子到根顺序执行；
-  控制机最后更新；
-  更新失败能够自动回滚；
-  离线节点恢复后自动继续更新；
-  节点身份、密钥、Overlay IP 和拓扑不会因更新丢失。

------

# 27. 给编码 AI 的执行要求

编码时必须遵守：

1. 先实现协议、状态机和数据模型，再实现 UI。
2. 不得使用节点名称作为唯一身份。
3. 不得把控制树和 WireGuard 数据图合并成同一个模型。
4. 不得把所有 Peer 塞进单一 WireGuard 接口。
5. 每条数据链路必须可以被独立路由和统计。
6. 不得通过下发任意 Shell 命令修改节点。
7. 网络配置必须通过结构化 Desired State。
8. 所有操作必须幂等。
9. 所有配置修改必须可以回滚。
10. 所有安装和更新制品必须校验签名。
11. 普通节点不得依赖 GitHub。
12. 普通节点不得运行 Web UI。
13. 父节点不得拥有全网管理员权限。
14. WireGuard 私钥必须只保存在本机。
15. 更新必须先预分发，再分批安装。
16. 每完成一个 Phase，必须先通过对应集成测试，再开始下一阶段。
17. 不允许以 Mock 或 TODO 代替核心网络功能。
18. 不允许先做一个只能展示静态拓扑的前端原型冒充完成。
19. V1 优先保证递归接入、链路扩张、路径控制和更新可靠性。
20. 任何架构变更必须同步修改协议文档、数据库迁移和测试。