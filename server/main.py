#!/usr/bin/env python3
"""
PathWeaver SD-WAN Controller
Single-file server: FastAPI + SQLite + React frontend
"""
import os, sys, json, hashlib, secrets, time, uuid
from datetime import datetime, timezone, timedelta
from pathlib import Path
from functools import wraps
from contextlib import contextmanager

import bcrypt
import uvicorn
from fastapi import FastAPI, Request, HTTPException, Query
from fastapi.responses import JSONResponse, FileResponse, HTMLResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
import sqlite3

# ─── Config ───
WEB_PORT   = int(os.getenv("PW_WEB_PORT", "8443"))
NODE_PORT  = int(os.getenv("PW_NODE_PORT", "8444"))
WG_START   = int(os.getenv("PW_WG_PORT_START", "30000"))
WG_END     = int(os.getenv("PW_WG_PORT_END", "30999"))
PUBLIC_IP  = os.getenv("PW_PUBLIC_ADDRESS", "127.0.0.1")
DB_PATH    = os.getenv("PW_DB_PATH", os.path.join(Path(__file__).parent.parent.absolute(), "pathweaver.db"))
STATIC_DIR = os.getenv("PW_STATIC_DIR", "")

app = FastAPI(title="PathWeaver", version="0.1.0")
app.add_middleware(CORSMiddleware, allow_origins=["*"], allow_methods=["*"], allow_headers=["*"])

# ─── Database ───
DB = None

def get_db() -> sqlite3.Connection:
    global DB
    if DB is None:
        DB = sqlite3.connect(DB_PATH, check_same_thread=False)
        DB.row_factory = sqlite3.Row
        DB.execute("PRAGMA journal_mode=WAL")
        DB.execute("PRAGMA foreign_keys=ON")
        init_db(DB)
    return DB

def init_db(db):
    db.executescript("""
        CREATE TABLE IF NOT EXISTS admin (
            id TEXT PRIMARY KEY, username TEXT UNIQUE NOT NULL, password_hash TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
            id TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL, admin_id TEXT NOT NULL,
            created_at TEXT NOT NULL, expires_at TEXT NOT NULL, revoked INTEGER DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS networks (
            id TEXT PRIMARY KEY, name TEXT NOT NULL, overlay_cidr TEXT NOT NULL,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS nodes (
            id TEXT PRIMARY KEY, display_name TEXT NOT NULL, overlay_ip TEXT NOT NULL,
            wg_pubkey TEXT NOT NULL, wg_privkey TEXT NOT NULL,
            identity_pubkey TEXT NOT NULL, identity_privkey TEXT NOT NULL,
            control_parent_id TEXT REFERENCES nodes(id),
            node_port INTEGER DEFAULT 8444,
            wg_port_start INTEGER DEFAULT 30000, wg_port_end INTEGER DEFAULT 30999,
            is_controller INTEGER DEFAULT 0, agent_version TEXT,
            protocol_version INTEGER DEFAULT 1,
            created_at TEXT NOT NULL, updated_at TEXT NOT NULL,
            last_seen_at TEXT
        );
        CREATE TABLE IF NOT EXISTS enrollment_tokens (
            id TEXT PRIMARY KEY, token TEXT UNIQUE NOT NULL,
            parent_node_id TEXT NOT NULL REFERENCES nodes(id),
            node_name TEXT NOT NULL, expires_at TEXT NOT NULL,
            used_at TEXT, revoked INTEGER DEFAULT 0,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS wireguard_links (
            id TEXT PRIMARY KEY, node_a TEXT NOT NULL, node_b TEXT NOT NULL,
            initiator_id TEXT NOT NULL, listener_id TEXT NOT NULL,
            listener_ip TEXT NOT NULL, listener_port INTEGER NOT NULL,
            iface_a TEXT NOT NULL, iface_b TEXT NOT NULL,
            enabled INTEGER DEFAULT 1, status TEXT DEFAULT 'Active',
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS traffic_policies (
            id TEXT PRIMARY KEY, name TEXT NOT NULL, priority INTEGER DEFAULT 10,
            enabled INTEGER DEFAULT 1, description TEXT,
            match_src TEXT, match_dst TEXT, match_proto TEXT DEFAULT 'Any',
            match_port INTEGER,
            path_nodes TEXT NOT NULL DEFAULT '[]',
            egress_nat INTEGER DEFAULT 0,
            return_path TEXT DEFAULT 'Symmetric',
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS update_jobs (
            id TEXT PRIMARY KEY, target_version TEXT NOT NULL,
            manifest_sha256 TEXT NOT NULL,
            status TEXT DEFAULT 'Created',
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS update_targets (
            id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES update_jobs(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            depth INTEGER DEFAULT 0, status TEXT DEFAULT 'Waiting',
            error_message TEXT
        );
    """)
    db.commit()

def now(): return datetime.now(timezone.utc).isoformat()
def uid():  return str(uuid.uuid4())
def token(): return secrets.token_hex(32)

# ─── Auth middleware ───
def auth_required(f):
    @wraps(f)
    async def wrapper(req: Request, *args, **kwargs):
        token_cookie = req.cookies.get("pw_token") or req.headers.get("Authorization", "").replace("Bearer ", "")
        if not token_cookie:
            raise HTTPException(401, "Not authenticated")
        db = get_db()
        sess = db.execute("SELECT * FROM sessions WHERE token=? AND revoked=0", (token_cookie,)).fetchone()
        if not sess or sess["expires_at"] < now():
            raise HTTPException(401, "Session expired")
        return await f(req, *args, **kwargs)
    return wrapper

# ─── Auth API ───
@app.post("/api/auth/login")
async def login(req: Request):
    data = await req.json()
    username = data.get("username", "admin")
    password = data.get("password", "")
    db = get_db()
    admin = db.execute("SELECT * FROM admin WHERE username=?", (username,)).fetchone()
    if not admin:
        hash_pw = bcrypt.hashpw(password.encode(), bcrypt.gensalt()).decode()
        db.execute("INSERT INTO admin (id, username, password_hash, created_at) VALUES (?,?,?,?)",
                   (uid(), username, hash_pw, now()))
        db.commit()
        admin = db.execute("SELECT * FROM admin WHERE username=?", (username,)).fetchone()

    if not bcrypt.checkpw(password.encode(), admin["password_hash"].encode()):
        raise HTTPException(401, "Invalid password")

    tok = token()
    db.execute("INSERT INTO sessions (id, token, admin_id, created_at, expires_at, revoked) VALUES (?,?,?,?,?,0)",
               (uid(), tok, admin["id"], now(), (datetime.now(timezone.utc) + timedelta(hours=48)).isoformat()))
    db.commit()
    resp = JSONResponse({"token": tok, "message": "ok"})
    resp.set_cookie("pw_token", tok, httponly=True, samesite="lax", max_age=172800)
    return resp

@app.post("/api/auth/logout")
async def logout(req: Request):
    tok = req.cookies.get("pw_token", "")
    db = get_db()
    db.execute("UPDATE sessions SET revoked=1 WHERE token=?", (tok,))
    db.commit()
    resp = JSONResponse({"status": "ok"})
    resp.delete_cookie("pw_token")
    return resp

# ─── Node API ───
@app.get("/api/nodes")
async def list_nodes():
    db = get_db()
    rows = db.execute("SELECT * FROM nodes ORDER BY is_controller DESC").fetchall()
    return [dict(r) for r in rows]

@app.put("/api/nodes/{node_id}/rename")
async def rename_node(node_id: str, req: Request):
    data = await req.json()
    name = data.get("display_name", "")
    db = get_db()
    db.execute("UPDATE nodes SET display_name=?, updated_at=? WHERE id=?",
               (name, now(), node_id))
    db.commit()
    return {"status": "ok"}

# ─── Enrollment ───
@app.post("/api/enrollment/tokens")
async def create_token(req: Request):
    data = await req.json()
    pid = data["parent_node_id"]
    name = data.get("suggested_node_name", "new-node")
    ttl = data.get("ttl_seconds", 600)
    db = get_db()
    parent = db.execute("SELECT * FROM nodes WHERE id=?", (pid,)).fetchone()
    if not parent:
        raise HTTPException(404, "Parent node not found")

    tok = token()
    exp = (datetime.now(timezone.utc) + timedelta(seconds=ttl)).isoformat()
    tid = uid()
    db.execute("INSERT INTO enrollment_tokens (id, token, parent_node_id, node_name, expires_at, created_at) VALUES (?,?,?,?,?,?)",
               (tid, tok, pid, name, exp, now()))
    db.commit()

    # 安装命令指向父节点的可达地址
    parent_addr = parent.get("overlay_ip", PUBLIC_IP)
    parent_web_port = WEB_PORT  # 父节点监听同一个Web端口
    cmd = f"curl -fsSL 'http://{parent_addr}:{parent_web_port}/bootstrap/install.sh?token={tok}' | sudo bash"
    return {"token_id": tid, "token": tok, "install_command": cmd, "expires_at": exp}

@app.post("/bootstrap/enroll")
async def bootstrap_enroll(req: Request):
    data = await req.json()
    tok_val = data["token"]
    name = data.get("node_name", "new-node")
    wg_pub = data.get("wg_public_key", "")
    id_pub = data.get("identity_public_key", "")
    agent_ver = data.get("agent_version", "0.1.0")
    proto_ver = data.get("protocol_version", 1)

    db = get_db()
    tok = db.execute("SELECT * FROM enrollment_tokens WHERE token=?", (tok_val,)).fetchone()
    if not tok: raise HTTPException(404, "Token not found")
    if tok["revoked"]: raise HTTPException(403, "Token revoked")
    if tok["used_at"]: raise HTTPException(403, "Token already used")
    if tok["expires_at"] < now(): raise HTTPException(403, "Token expired")

    parent = db.execute("SELECT * FROM nodes WHERE id=?", (tok["parent_node_id"],)).fetchone()
    if not parent: raise HTTPException(404, "Parent not found")

    # Allocate overlay IP
    existing = db.execute("SELECT overlay_ip FROM nodes").fetchall()
    used = set(r["overlay_ip"] for r in existing)
    base = parent["overlay_ip"].rsplit(".", 1)[0]
    new_ip = None
    for i in range(2, 255):
        ip = f"{base}.{i}"
        if ip not in used:
            new_ip = ip
            break
    if not new_ip: raise HTTPException(500, "Overlay IP pool exhausted")

    nid = uid()
    wg_priv = secrets.token_hex(32)
    id_priv = secrets.token_hex(32)
    db.execute("""INSERT INTO nodes (id, display_name, overlay_ip,
        wg_pubkey, wg_privkey, identity_pubkey, identity_privkey,
        control_parent_id, node_port, wg_port_start, wg_port_end,
        is_controller, agent_version, protocol_version,
        created_at, updated_at, last_seen_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?)""",
        (nid, name, new_ip, wg_pub, wg_priv, id_pub, id_priv,
         parent["id"], NODE_PORT, WG_START, WG_END,
         agent_ver, proto_ver, now(), now(), now()))
    db.execute("UPDATE enrollment_tokens SET used_at=? WHERE id=?", (now(), tok["id"]))
    db.commit()

    # 使用父节点的可达地址 (不是控制机公网IP)
    parent_addr = parent.get("overlay_ip", PUBLIC_IP)
    parent_web = parent.get("node_port", WEB_PORT)  # 子节点也是Web服务端口

    return {
        "node_id": nid, "overlay_ipv4": new_ip,
        "parent_wg_public_key": parent["wg_pubkey"],
        "parent_wg_endpoint": f"{parent['overlay_ip']}:{WG_START}",
        "recovery_endpoint": f"{parent['overlay_ip']}:{NODE_PORT}",
    }

# ─── WG Links ───
@app.get("/api/links")
async def list_links():
    db = get_db()
    return [dict(r) for r in db.execute("SELECT * FROM wireguard_links").fetchall()]

@app.post("/api/links")
async def create_link(req: Request):
    data = await req.json()
    aid = data["initiator_node_id"]
    bid = data["listener_node_id"]
    lip = data.get("listener_address", "")
    lport = data.get("listener_port", WG_START)

    db = get_db()
    a = db.execute("SELECT * FROM nodes WHERE id=?", (aid,)).fetchone()
    b = db.execute("SELECT * FROM nodes WHERE id=?", (bid,)).fetchone()
    if not a or not b: raise HTTPException(404, "Node not found")

    lid = uid()
    ifa = f"pwl-{a['display_name'][:8]}"
    ifb = f"pwl-{b['display_name'][:8]}"
    db.execute("""INSERT INTO wireguard_links (id, node_a, node_b, initiator_id, listener_id,
        listener_ip, listener_port, iface_a, iface_b, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?)""",
        (lid, aid, bid, aid, bid, lip or b["overlay_ip"], lport, ifa, ifb, now()))
    db.commit()
    return {"link_id": lid, "interface_initiator": ifa, "interface_listener": ifb,
            "listener_port": lport, "status": "Active"}

@app.delete("/api/links/{link_id}")
async def delete_link(link_id: str):
    db = get_db()
    db.execute("DELETE FROM wireguard_links WHERE id=?", (link_id,))
    db.commit()
    return {"status": "deleted"}

# ─── Policies ───
@app.get("/api/policies")
async def list_policies():
    db = get_db()
    rows = db.execute("SELECT * FROM traffic_policies ORDER BY priority").fetchall()
    result = []
    for r in rows:
        d = dict(r)
        d["path_nodes"] = json.loads(d.get("path_nodes", "[]"))
        d["matches"] = [{
            "id": d["id"],
            "policy_id": d["id"],
            "source_cidr": d.get("match_src"),
            "destination_cidr": d.get("match_dst"),
            "protocol": d.get("match_proto", "Any"),
            "destination_port_start": d.get("match_port"),
        }]
        d["path"] = [{"id": uid(), "policy_id": d["id"], "hop_order": i, "node_id": nid}
                     for i, nid in enumerate(d["path_nodes"])]
        d["config"] = {"egress_nat": bool(d.get("egress_nat")), "return_path_type": d.get("return_path", "Symmetric")}
        result.append({"policy": {k: v for k, v in d.items() if k in ("id", "name", "priority", "enabled", "description")},
                       "matches": d["matches"], "path": d["path"], "config": d["config"]})
    return result

@app.post("/api/policies")
async def create_policy(req: Request):
    data = await req.json()
    pid = uid()
    m = (data.get("matches") or [{}])[0]
    db = get_db()
    db.execute("""INSERT INTO traffic_policies (id, name, priority, enabled, description,
        match_src, match_dst, match_proto, match_port,
        path_nodes, egress_nat, return_path, created_at)
        VALUES (?,?,?,1,?,?,?,?,?,?,?,?,?)""",
        (pid, data["name"], data.get("priority", 10), data.get("description", ""),
         m.get("source_cidr"), m.get("destination_cidr"),
         m.get("protocol", "Any"), m.get("destination_port"),
         json.dumps(data.get("path_node_ids", [])),
         1 if data.get("egress_nat") else 0,
         data.get("return_path_type", "Symmetric"),
         now()))
    db.commit()

    p = dict(db.execute("SELECT * FROM traffic_policies WHERE id=?", (pid,)).fetchone())
    p["path_nodes"] = json.loads(p.get("path_nodes", "[]"))
    return {"policy": {k: v for k, v in p.items() if k in ("id", "name", "priority", "enabled", "description")},
            "matches": [{"id": pid, "policy_id": pid, "source_cidr": p.get("match_src"),
                         "destination_cidr": p.get("match_dst"),
                         "protocol": p.get("match_proto", "Any"),
                         "destination_port_start": p.get("match_port")}],
            "path": [{"id": uid(), "policy_id": pid, "hop_order": i, "node_id": nid}
                     for i, nid in enumerate(p["path_nodes"])],
            "config": {"egress_nat": bool(p.get("egress_nat")), "return_path_type": p.get("return_path", "Symmetric")}}

@app.get("/api/config/preview")
async def config_preview():
    db = get_db()
    nodes = [dict(r) for r in db.execute("SELECT * FROM nodes").fetchall()]
    links = [dict(r) for r in db.execute("SELECT * FROM wireguard_links").fetchall()]
    policies = [dict(r) for r in db.execute("SELECT * FROM traffic_policies WHERE enabled=1").fetchall()]

    errors = []
    configs = []
    for node in nodes:
        node_links = [l for l in links if l["node_a"] == node["id"] or l["node_b"] == node["id"]]
        node_policies = []
        for pol in policies:
            path = json.loads(pol.get("path_nodes", "[]"))
            if node["id"] in path:
                node_policies.append({
                    "fwmak": 1000 + path.index(node["id"]),
                    "table_id": 100 + path.index(node["id"]),
                    "next_hop": path[path.index(node["id"]) + 1] if path.index(node["id"]) + 1 < len(path) else None,
                    "egress_nat": bool(pol.get("egress_nat")),
                })
        configs.append({
            "node_id": node["id"], "display_name": node["display_name"],
            "overlay_ip": node["overlay_ip"],
            "wireguard_links": [{"link_id": l["id"], "interface_name": l["iface_a"] if l["node_a"] == node["id"] else l["iface_b"]} for l in node_links],
            "policy_routing_rules": node_policies,
            "route_tables": [],
            "nat_rules": [{"rule_type": "Masquerade"} for p in node_policies if p["egress_nat"]] or [],
        })

    for pol in policies:
        path = json.loads(pol.get("path_nodes", "[]"))
        for i in range(len(path) - 1):
            if not any((l["node_a"] == path[i] and l["node_b"] == path[i+1]) or
                       (l["node_a"] == path[i+1] and l["node_b"] == path[i])
                       for l in links):
                errors.append(f"Policy '{pol['name']}': no link between nodes")

    return {"generation": int(time.time()), "node_configs": configs, "validation_errors": errors}

# ─── Updates ───
@app.get("/api/updates")
async def list_updates():
    db = get_db()
    return [dict(r) for r in db.execute("SELECT * FROM update_jobs ORDER BY created_at DESC").fetchall()]

@app.post("/api/updates")
async def create_update(req: Request):
    data = await req.json()
    jid = uid()
    db = get_db()
    db.execute("INSERT INTO update_jobs (id, target_version, manifest_sha256, created_at) VALUES (?,?,?,?)",
               (jid, data["target_version"], data["manifest_sha256"], now()))

    # Leaf-first ordering: deeper nodes first
    nodes = db.execute("SELECT * FROM nodes WHERE is_controller=0").fetchall()
    for n in nodes:
        tid = uid()
        depth = 0
        parent_id = n["control_parent_id"]
        while parent_id:
            depth += 1
            p = db.execute("SELECT control_parent_id FROM nodes WHERE id=?", (parent_id,)).fetchone()
            parent_id = p["control_parent_id"] if p else None
        db.execute("INSERT INTO update_targets (id, job_id, node_id, depth) VALUES (?,?,?,?)",
                   (tid, jid, n["id"], depth))

    # Controller node last
    ctrl = db.execute("SELECT id FROM nodes WHERE is_controller=1").fetchone()
    if ctrl:
        db.execute("INSERT INTO update_targets (id, job_id, node_id, depth) VALUES (?,?,?,?)",
                   (uid(), jid, ctrl["id"], -1))

    db.commit()
    job = dict(db.execute("SELECT * FROM update_jobs WHERE id=?", (jid,)).fetchone())
    targets = [dict(r) for r in db.execute("SELECT * FROM update_targets WHERE job_id=?", (jid,)).fetchall()]
    return {"job": job, "targets": targets,
            "summary": {"total": len(targets), "completed": 0, "failed": 0, "waiting": len(targets), "in_progress": 0}}

@app.post("/api/updates/{job_id}/start")
async def start_update(job_id: str):
    db = get_db()
    db.execute("UPDATE update_jobs SET status='Installing' WHERE id=?", (job_id,))
    db.execute("UPDATE update_targets SET status='Installing' WHERE job_id=?", (job_id,))
    db.commit()
    return {"status": "started"}

@app.post("/api/updates/{job_id}/rollback")
async def rollback_update(job_id: str):
    db = get_db()
    db.execute("UPDATE update_jobs SET status='RolledBack' WHERE id=?", (job_id,))
    db.execute("UPDATE update_targets SET status='RolledBack' WHERE job_id=?", (job_id,))
    db.commit()
    return {"status": "rolled_back"}

# ─── Health ───
@app.get("/api/health")
async def health():
    return {"status": "ok", "version": "0.1.0"}

# ─── Bootstrap: 父节点缓存 + 依赖分发 ───
DEP_CACHE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "deps_cache")
os.makedirs(DEP_CACHE, exist_ok=True)

def download_deps():
    """预下载 Python 包到本地缓存, 供子节点下载"""
    import subprocess
    pkgs = ["fastapi", "uvicorn[standard]", "bcrypt", "python-multipart"]
    for pkg in pkgs:
        name = pkg.replace("[standard]", "") + "-*.whl"
        existing = list(Path(DEP_CACHE).glob(f"{pkg.replace('[standard]','')}-*"))
        if existing:
            print(f"  [cached] {pkg}")
            continue
        print(f"  [download] {pkg}")
        subprocess.run([sys.executable, "-m", "pip", "download", pkg, "-d", DEP_CACHE,
                        "--only-binary", ":all:", "--platform", "manylinux2014_x86_64",
                        "--python-version", "311"],
                       capture_output=True, timeout=60)

@app.get("/bootstrap/deps/{filename}")
async def serve_dep(filename: str):
    """子节点从此端点下载 Python 依赖"""
    path = os.path.join(DEP_CACHE, filename)
    if not os.path.isfile(path): return JSONResponse({"error": "not found"}, 404)
    return FileResponse(path, media_type="application/octet-stream")

@app.get("/bootstrap/deps/list")
async def list_deps():
    """子节点获取依赖包列表"""
    files = sorted(f.name for f in Path(DEP_CACHE).glob("*.whl"))
    return JSONResponse(files)

@app.get("/bootstrap/server.py")
async def serve_server_py():
    """子节点下载控制面主程序"""
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "main.py")
    return FileResponse(path, media_type="text/x-python")

# ─── Bootstrap install script ───
@app.get("/bootstrap/install.sh")
async def bootstrap_install(token: str = "", parent_ip: str = None, parent_port: int = None):
    # 父节点地址: 优先从URL参数取，回退到请求方IP
    if not parent_ip:
        pip = PUBLIC_IP
    else:
        pip = parent_ip
    ppt = parent_port or WEB_PORT
    script = f"""#!/bin/bash
set -e
echo "PathWeaver Node Installer"
echo "父节点: {pip}:{ppt}"

PARENT="http://{pip}:{ppt}"
TOKEN="{token}"

mkdir -p /opt/pathweaver/{{bin,etc,deps}}
apt-get update -qq && apt-get install -y -qq python3 python3-venv curl wireguard-tools nftables 2>/dev/null || true

# 从父节点下载 Python 依赖包 (链式传播: 父节点已缓存)
echo "从父节点下载依赖包..."
mkdir -p /opt/pathweaver/deps
DEP_LIST=$(curl -sf "$PARENT/bootstrap/deps/list" 2>/dev/null || echo "")
if [ -n "$DEP_LIST" ]; then
    echo "$DEP_LIST" | while read f; do
        [ -z "$f" ] && continue
        echo "  -> $f"
        curl -sf "$PARENT/bootstrap/deps/$f" -o "/opt/pathweaver/deps/$f"
    done
fi

# 下载服务器程序
echo "下载服务器程序..."
curl -sf "$PARENT/bootstrap/server.py" -o /opt/pathweaver/main.py

# 安装 Python 依赖 (离线, 从本地缓存)
python3 -m venv /opt/pathweaver/.venv
source /opt/pathweaver/.venv/bin/activate
pip install --no-index --find-links=/opt/pathweaver/deps /opt/pathweaver/deps/*.whl 2>/dev/null || \
    pip install --no-index --find-links=/opt/pathweaver/deps fastapi uvicorn bcrypt python-multipart 2>/dev/null || true

# 生成密钥
WG_PRIV=$(wg genkey 2>/dev/null || head -c32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9+/' | head -c44)
WG_PUB=$(echo "$WG_PRIV" | wg pubkey 2>/dev/null || echo "auto-wg-$(head -c16 /dev/urandom | base64)")
ID_PRIV=$(head -c32 /dev/urandom | base64)
ID_PUB=$(echo -n "$ID_PRIV" | sha256sum | cut -d' ' -f1)

# 注册到父节点
echo "注册节点到父节点..."
RESP=$(curl -sf $PARENT/bootstrap/enroll \\
    -H 'Content-Type: application/json' \\
    -d '{{"token":"$TOKEN","node_name":"$(hostname)","wg_public_key":"'$WG_PUB'","identity_public_key":"'$ID_PUB'","agent_version":"0.1.0","protocol_version":1}}')

NODE_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['node_id'])")
OVERLAY_IP=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['overlay_ipv4'])")
PARENT_WG=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['parent_wg_public_key'])")
PARENT_EP=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['parent_wg_endpoint'])")

# 保存配置 (包括父节点信息)
cat > /opt/pathweaver/etc/node.conf << EOFCONF
NODE_ID=$NODE_ID
OVERLAY_IP=$OVERLAY_IP
WG_PRIV=$WG_PRIV
PARENT_WG_PUB=$PARENT_WG
PARENT_ENDPOINT=$PARENT_EP
PW_WEB_PORT={ppt}
EOF

# 启动本节点服务 (使其可作为下一级父节点)
cat > /etc/systemd/system/pathweaver.service << EOF
[Unit]
Description=PathWeaver Node
After=network.target

[Service]
Type=simple
ExecStart=/opt/pathweaver/.venv/bin/python3 /opt/pathweaver/main.py
Environment=PW_WEB_PORT={ppt}
Environment=PW_PUBLIC_ADDRESS=$OVERLAY_IP
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable pathweaver
systemctl start pathweaver

echo ""
echo "=========================================取==========================================[32m  节点入网成功![0m"
echo "=========================================取=========================================="
echo "  节点 ID:     $NODE_ID"
echo "  Overlay IP:  $OVERLAY_IP"
echo "  父节点:      {pip}:{ppt}"
echo "  本节点已启动服务，可接入下一级子节点"
echo "=========================================取=========================================="

# 清理安装脚本自身
rm -f /tmp/pw-install.sh"""
    return HTMLResponse(script, media_type="text/plain")

# ─── Static Files ───
if STATIC_DIR and os.path.isdir(STATIC_DIR):
    @app.get("/{full_path:path}")
    async def serve_frontend(full_path: str = ""):
        file_path = os.path.join(STATIC_DIR, full_path or "index.html")
        if os.path.isfile(file_path):
            return FileResponse(file_path)
        return FileResponse(os.path.join(STATIC_DIR, "index.html"))
else:
    @app.get("/")
    async def root():
        db = get_db()
        nc = db.execute("SELECT count(*) FROM nodes").fetchone()[0]
        return HTMLResponse(f"""<!DOCTYPE html>
<html><head><title>PathWeaver</title><meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<style>body{{font-family:sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;
background:#0f172a;color:#e2e8f0;margin:0}}div{{text-align:center}}
a{{color:#38bdf8}}code{{background:#1e293b;padding:3px 8px;border-radius:4px}}</style></head>
<body><div>
<h1>🛰️ PathWeaver</h1><p>SD-WAN 控制平面 v0.1.0</p>
<p>节点: <strong>{nc}</strong> | 端口: <strong>{WEB_PORT}</strong></p>
<hr style="border-color:#334155;margin:16px 0">
<p>部署前端静态文件以启用完整管理面板：</p>
<p><code>cd web && npm install && npm run build</code></p>
<p><code>PW_STATIC_DIR=web/dist python3 server/main.py</code></p>
<p><a href="/api/health">API 健康检查</a> · <a href="https://github.com/FengYuchen1314/sd-wan-plus">GitHub</a></p>
</div></body></html>""")

# ─── Start ───
def main():
    # 控制机: 下载并缓存 Python 依赖 (供子节点链式下载)
    if os.getenv("PW_CONTROLLER", "1") == "1":
        print("  [controller] caching python deps...")
        try: download_deps()
        except Exception as e: print(f"  [warn] deps cache failed: {e}")

    # Ensure controller node exists on first run
    db = get_db()
    existing = db.execute("SELECT id FROM nodes WHERE is_controller=1").fetchone()
    if not existing:
        cid = uid()
        wg_priv = secrets.token_hex(32)
        wg_pub = secrets.token_hex(32)
        id_priv = secrets.token_hex(32)
        id_pub = hashlib.sha256(id_priv.encode()).hexdigest()
        db.execute("""INSERT INTO nodes (id, display_name, overlay_ip,
            wg_pubkey, wg_privkey, identity_pubkey, identity_privkey,
            is_controller, node_port, wg_port_start, wg_port_end,
            created_at, updated_at, last_seen_at)
            VALUES (?,?,?,?,?,?,?,1,?,?,?,?,?,?)""",
            (cid, "controller", "10.250.0.1",
             wg_pub, wg_priv, id_pub, id_priv,
             NODE_PORT, WG_START, WG_END,
             now(), now(), now()))
        db.commit()
        print(f"  Controller node created: 10.250.0.1")

    print(f"\n  PathWeaver v0.1.0")
    print(f"  Listening: http://0.0.0.0:{WEB_PORT}")
    print(f"  Admin password: first login sets it\n")
    uvicorn.run(app, host="0.0.0.0", port=WEB_PORT, log_level="info", access_log=False)

if __name__ == "__main__":
    main()
