# WireGuard 握手语义（V1）

PathWeaver 每条逻辑链路使用**独立 WireGuard 接口**（userspace **wireguard-go**，由 `pathweaver-netd` 内嵌维护），并区分主动端 / 被动端。

主控编译各节点 desired state 并发布；agent 调用 netd，对每条 `pwl-*` 链路创建或热更新 `device.Device`（不依赖内核 WireGuard 模块）。

## 默认：单向发起握手

**无论公网还是内网，默认都是单向。** 只有管理员在建链时显式勾选「双向互拨」，且两端都有公网可达地址时，才会配置双侧 Endpoint。

| 角色 | 配置 | 行为 |
|------|------|------|
| **主动端 (initiator)** | `Endpoint = 被动端IP:端口`，`PersistentKeepalive = 25`，通常不 Listen | 主动发起握手并保活 |
| **被动端 (listener)** | 仅 `ListenPort`，**不写 Endpoint**，Keepalive=0 | 等待对端；协议学习地址 |

典型场景：

- **入网**：子节点 → 父节点（单向）
- **两台公网**：默认仍单向；需要互拨时在控制台勾选「双向互拨」
- **内网后期连其它公网**：CreateLink，内网节点作主动端，目标公网作被动端（单向）

实现位置：

- 编译：[`internal/routing/compile.go`](../internal/routing/compile.go)
- 下发应用：[`internal/netd/netd.go`](../internal/netd/netd.go)、[`internal/netd/wg_userspace.go`](../internal/netd/wg_userspace.go)
- 建链 / 发布修复：[`internal/controller/nodes_handlers.go`](../internal/controller/nodes_handlers.go)、[`internal/controller/policy_handlers.go`](../internal/controller/policy_handlers.go)

## 可选：双向互拨（仅双公网）

链路字段 `bidirectional=true` 时：

- 两端都 Listen
- 两端都写对端公网 `Endpoint`
- 两端都 `PersistentKeepalive=25`

内网 / NAT 节点**不要**开双向；反向 Endpoint 写私网地址会导致公网侧拨不通。

## 动态维护 Endpoint

被动端不配置固定对端地址（单向模式）。收到主动端握手/数据包后，会**自动学习并更新 peer endpoint**（随 NAT/IP 变化动态维护）。

主动端通过 `PersistentKeepalive` 维持映射，使被动端持续能看到最新来源地址。

数据面仍是双向的；“单向”仅指**谁发起握手**。
