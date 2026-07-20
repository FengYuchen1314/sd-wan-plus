# PathWeaver 部署指南

## 方式一：Docker 部署（推荐）

```bash
# 构建镜像
docker build -t pathweaver:latest .

# 运行
docker run -d \
  --name pathweaver \
  --restart unless-stopped \
  --network host \
  --cap-add NET_ADMIN \
  -v /opt/pathweaver/data:/opt/pathweaver/data \
  pathweaver:latest
```

容器启动后，访问 `https://<服务器IP>:8443`

## 方式二：手动部署 (Linux x86_64)

### 前置依赖
- Debian 12+ / Ubuntu 22.04+
- Rust 工具链 (rustup)
- Node.js 18+ 和 npm
- protobuf-compiler
- wireguard-tools, nftables

### 构建

```bash
git clone https://github.com/FengYuchen1314/sd-wan-plus.git
cd sd-wan-plus
bash build-release.sh
```

### 安装

```bash
sudo mkdir -p /opt/pathweaver/{bin,web,data}
sudo cp target/release/pathweaver-controller /opt/pathweaver/bin/
sudo cp -r web/dist/* /opt/pathweaver/web/
```

### 创建 systemd 服务

```ini
# /etc/systemd/system/pathweaver.service
[Unit]
Description=PathWeaver Controller
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/pathweaver/bin/pathweaver-controller
Environment=PW_STATIC_DIR=/opt/pathweaver/web
Environment=RUST_LOG=info
WorkingDirectory=/opt/pathweaver
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now pathweaver
```

### 防火墙

```bash
sudo ufw allow 8443/tcp    # Web UI
sudo ufw allow 8444/tcp    # 节点服务端口
sudo ufw allow 30000:30999/udp  # WireGuard 端口池
```

### 验证

```bash
curl http://localhost:8443/api/health
# {"status":"ok","version":"0.1.0"}
```

浏览器访问 `http://<服务器IP>:8443`，使用 admin / admin123 登录。

### 生产环境注意事项

1. 务必修改默认密码
2. 建议配置 Nginx 反向代理 + Let's Encrypt HTTPS
3. SQLite 数据库位置：`/opt/pathweaver/pathweaver.db`，建议定期备份
4. 日志查看：`journalctl -u pathweaver -f`
