# WireGuard 握手语义（V1）

PathWeaver 每条逻辑链路使用**独立 WireGuard 接口**，并区分主动端 / 被动端。

## 单向发起握手

| 角色 | 配置 | 行为 |
|------|------|------|
| **主动端 (initiator)** | `Endpoint = 被动端IP:端口`，`PersistentKeepalive = 25` | 主动发起握手并保活 |
| **被动端 (listener)** | 仅 `ListenPort`，**不写 Endpoint**，Keepalive=0 | 等待对端；内核学习地址 |

实现位置：

- 编译：[`internal/routing/compile.go`](../internal/routing/compile.go)
- 下发应用：[`internal/netd/netd.go`](../internal/netd/netd.go)

## 动态维护 Endpoint

被动端不配置固定对端地址。WireGuard 内核在收到主动端握手/数据包后，会**自动学习并更新 peer endpoint**（随 NAT/IP 变化动态维护）。

主动端通过 `PersistentKeepalive` 维持映射，使被动端持续能看到最新来源地址。

数据面仍是双向的；“单向”仅指**谁发起握手**。
