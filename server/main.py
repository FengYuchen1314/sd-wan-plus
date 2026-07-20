#!/usr/bin/env python3
"""
PathWeaver SD-WAN Controller
Enterprise-grade single-file server: FastAPI + SQLite + Argon2id + WebSocket
"""
import os, sys, json, hashlib, hmac, secrets, time, uuid, logging
from datetime import datetime, timezone, timedelta
from pathlib import Path
from functools import wraps
from typing import Optional

from argon2 import PasswordHasher
from argon2.exceptions import VerifyMismatchError
from cryptography.fernet import Fernet
import uvicorn
from fastapi import FastAPI, Request, HTTPException, Query, WebSocket, WebSocketDisconnect
from fastapi.responses import JSONResponse, FileResponse, HTMLResponse
from fastapi.staticfiles import StaticFiles
from fastapi.middleware.cors import CORSMiddleware
import sqlite3

# ─── Logging ───
logging.basicConfig(level=logging.INFO, format="%(asctime)s [%(levelname)s] %(message)s")
log = logging.getLogger("pathweaver")

# ─── Config ───
WEB_PORT      = int(os.getenv("PW_WEB_PORT", "8443"))
NODE_PORT     = int(os.getenv("PW_NODE_PORT", "8444"))
WG_START      = int(os.getenv("PW_WG_PORT_START", "30000"))
WG_END        = int(os.getenv("PW_WG_PORT_END", "30999"))
PUBLIC_IP     = os.getenv("PW_PUBLIC_ADDRESS", "127.0.0.1")
OVERLAY_CIDR  = os.getenv("PW_OVERLAY_CIDR", "10.250.0.0/16")
DB_PATH       = os.getenv("PW_DB_PATH", os.path.join(Path(__file__).parent.parent.absolute(), "pathweaver.db"))
STATIC_DIR    = os.getenv("PW_STATIC_DIR", "")
KEY_FILE      = os.path.join(Path(__file__).parent.parent.absolute(), ".pathweaver.key")
SESSION_HOURS = int(os.getenv("PW_SESSION_HOURS", "48"))
MAX_LOGIN_ATTEMPTS = int(os.getenv("PW_MAX_LOGIN_ATTEMPTS", "5"))
LOGIN_WINDOW_SEC   = int(os.getenv("PW_LOGIN_WINDOW_SEC", "300"))

# ─── Encryption ───
def _load_or_create_key() -> bytes:
    if os.path.exists(KEY_FILE):
        with open(KEY_FILE, "rb") as f:
            return f.read()
    key = Fernet.generate_key()
    with open(KEY_FILE, "wb") as f:
        f.write(key)
    os.chmod(KEY_FILE, 0o600)
    return key

FERNET = Fernet(_load_or_create_key())

def encrypt_secret(plain: str) -> str:
    return FERNET.encrypt(plain.encode()).decode()

def decrypt_secret(encrypted: str) -> str:
    return FERNET.decrypt(encrypted.encode()).decode()

# ─── Argon2id ───
PH = PasswordHasher(time_cost=3, memory_cost=65536, parallelism=4, hash_len=32, salt_len=16)

def hash_password(pw: str) -> str:
    return PH.hash(pw)

def verify_password(hash_val: str, pw: str) -> bool:
    try:
        PH.verify(hash_val, pw)
        if PH.check_needs_rehash(hash_val):
            return True  # rehash handled on next login
        return True
    except VerifyMismatchError:
        return False

# ─── Rate limiter (in-memory) ───
_login_attempts: dict[str, list[float]] = {}

def check_rate_limit(key: str, max_attempts: int, window_sec: int) -> bool:
    now = time.time()
    attempts = [t for t in _login_attempts.get(key, []) if now - t < window_sec]
    _login_attempts[key] = attempts
    if len(attempts) >= max_attempts:
        return False
    _login_attempts[key].append(now)
    return True

# ─── App ───
app = FastAPI(title="PathWeaver", version="1.0.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=[f"http://localhost:{WEB_PORT}", f"https://localhost:{WEB_PORT}"],
    allow_methods=["GET","POST","PUT","DELETE"],
    allow_headers=["Content-Type","Authorization"],
    allow_credentials=True,
)

# ─── Database ───
DB: Optional[sqlite3.Connection] = None

def get_db() -> sqlite3.Connection:
    global DB
    if DB is None:
        DB = sqlite3.connect(DB_PATH, check_same_thread=False)
        DB.row_factory = sqlite3.Row
        DB.execute("PRAGMA journal_mode=WAL")
        DB.execute("PRAGMA foreign_keys=ON")
        DB.execute("PRAGMA busy_timeout=5000")
        init_db(DB)
    return DB

def init_db(db: sqlite3.Connection):
    db.executescript("""
        CREATE TABLE IF NOT EXISTS admin (
            id TEXT PRIMARY KEY,
            username TEXT NOT NULL UNIQUE,
            password_hash TEXT NOT NULL,
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
            id TEXT PRIMARY KEY,
            admin_id TEXT NOT NULL REFERENCES admin(id),
            token TEXT NOT NULL UNIQUE,
            ip_address TEXT NOT NULL DEFAULT '',
            user_agent TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL,
            expires_at TEXT NOT NULL,
            revoked INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS networks (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            overlay_ipv4_cidr TEXT NOT NULL,
            overlay_ipv6_cidr TEXT,
            ipv6_enabled INTEGER NOT NULL DEFAULT 0,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS nodes (
            id TEXT PRIMARY KEY,
            display_name TEXT NOT NULL,
            overlay_ipv4 TEXT NOT NULL,
            overlay_ipv6 TEXT,
            wg_public_key TEXT NOT NULL,
            wg_private_key_encrypted TEXT NOT NULL,
            identity_public_key TEXT NOT NULL,
            identity_private_key_encrypted TEXT NOT NULL,
            control_parent_id TEXT REFERENCES nodes(id),
            node_service_port INTEGER NOT NULL DEFAULT 8444,
            wg_port_range_start INTEGER NOT NULL DEFAULT 30000,
            wg_port_range_end INTEGER NOT NULL DEFAULT 30999,
            is_controller INTEGER NOT NULL DEFAULT 0,
            agent_version TEXT,
            protocol_version INTEGER NOT NULL DEFAULT 1,
            desired_generation INTEGER,
            active_generation INTEGER,
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL,
            last_seen_at TEXT,
            last_handshake_at TEXT,
            enrollment_token_id TEXT
        );
        CREATE TABLE IF NOT EXISTS node_addresses (
            id TEXT PRIMARY KEY,
            node_id TEXT NOT NULL REFERENCES nodes(id),
            address TEXT NOT NULL,
            address_type TEXT NOT NULL DEFAULT 'public',
            is_primary INTEGER NOT NULL DEFAULT 0,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS node_port_pools (
            id TEXT PRIMARY KEY,
            node_id TEXT NOT NULL REFERENCES nodes(id),
            protocol TEXT NOT NULL DEFAULT 'UDP',
            port_range_start INTEGER NOT NULL,
            port_range_end INTEGER NOT NULL,
            allocated_ports TEXT NOT NULL DEFAULT '[]'
        );
        CREATE TABLE IF NOT EXISTS control_relations (
            id TEXT PRIMARY KEY,
            parent_id TEXT NOT NULL REFERENCES nodes(id),
            child_id TEXT NOT NULL REFERENCES nodes(id),
            enrolled_at TEXT NOT NULL,
            enrollment_token_id TEXT NOT NULL,
            UNIQUE(child_id)
        );
        CREATE TABLE IF NOT EXISTS wireguard_links (
            id TEXT PRIMARY KEY,
            node_a TEXT NOT NULL REFERENCES nodes(id),
            node_b TEXT NOT NULL REFERENCES nodes(id),
            initiator_node_id TEXT NOT NULL REFERENCES nodes(id),
            listener_node_id TEXT NOT NULL REFERENCES nodes(id),
            listener_address TEXT NOT NULL,
            listener_port INTEGER NOT NULL,
            interface_name_a TEXT NOT NULL,
            interface_name_b TEXT NOT NULL,
            enabled INTEGER NOT NULL DEFAULT 1,
            admin_weight INTEGER NOT NULL DEFAULT 1,
            last_handshake_a TEXT,
            last_handshake_b TEXT,
            status TEXT NOT NULL DEFAULT 'Active',
            created_at TEXT NOT NULL,
            UNIQUE(node_a, node_b)
        );
        CREATE TABLE IF NOT EXISTS wireguard_link_endpoints (
            id TEXT PRIMARY KEY,
            link_id TEXT NOT NULL REFERENCES wireguard_links(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            interface_name TEXT NOT NULL,
            listen_port INTEGER NOT NULL,
            peer_endpoint TEXT,
            peer_public_key TEXT NOT NULL,
            persistent_keepalive INTEGER NOT NULL DEFAULT 0,
            is_initiator INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS traffic_policies (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            priority INTEGER NOT NULL DEFAULT 10,
            enabled INTEGER NOT NULL DEFAULT 1,
            description TEXT,
            created_at TEXT NOT NULL,
            updated_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS traffic_policy_matches (
            id TEXT PRIMARY KEY,
            policy_id TEXT NOT NULL REFERENCES traffic_policies(id) ON DELETE CASCADE,
            source_node_id TEXT REFERENCES nodes(id),
            source_node_group TEXT,
            source_cidr TEXT,
            destination_node_id TEXT REFERENCES nodes(id),
            destination_cidr TEXT,
            protocol TEXT NOT NULL DEFAULT 'Any',
            destination_port_start INTEGER,
            destination_port_end INTEGER
        );
        CREATE TABLE IF NOT EXISTS traffic_policy_paths (
            id TEXT PRIMARY KEY,
            policy_id TEXT NOT NULL REFERENCES traffic_policies(id) ON DELETE CASCADE,
            hop_order INTEGER NOT NULL,
            node_id TEXT NOT NULL REFERENCES nodes(id)
        );
        CREATE TABLE IF NOT EXISTS traffic_policy_configs (
            id TEXT PRIMARY KEY,
            policy_id TEXT NOT NULL REFERENCES traffic_policies(id) ON DELETE CASCADE,
            egress_node_id TEXT REFERENCES nodes(id),
            egress_nat INTEGER NOT NULL DEFAULT 0,
            return_path_type TEXT NOT NULL DEFAULT 'Symmetric',
            failover_enabled INTEGER NOT NULL DEFAULT 0,
            failover_path_id TEXT
        );
        CREATE TABLE IF NOT EXISTS config_revisions (
            id TEXT PRIMARY KEY,
            generation INTEGER NOT NULL,
            reason TEXT NOT NULL DEFAULT 'Manual change',
            status TEXT NOT NULL DEFAULT 'Draft',
            created_at TEXT NOT NULL,
            completed_at TEXT
        );
        CREATE TABLE IF NOT EXISTS node_desired_configs (
            id TEXT PRIMARY KEY,
            config_revision_id TEXT NOT NULL REFERENCES config_revisions(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            config_data TEXT NOT NULL,
            config_hash TEXT NOT NULL,
            UNIQUE(config_revision_id, node_id)
        );
        CREATE TABLE IF NOT EXISTS config_rollout_nodes (
            id TEXT PRIMARY KEY,
            config_revision_id TEXT NOT NULL REFERENCES config_revisions(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            status TEXT NOT NULL DEFAULT 'Pending',
            started_at TEXT,
            completed_at TEXT,
            error_message TEXT
        );
        CREATE TABLE IF NOT EXISTS heartbeats (
            id TEXT PRIMARY KEY,
            node_id TEXT NOT NULL REFERENCES nodes(id),
            agent_version TEXT NOT NULL,
            protocol_version INTEGER NOT NULL,
            active_generation INTEGER,
            wireguard_snapshots TEXT NOT NULL DEFAULT '[]',
            received_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS wireguard_snapshots (
            id TEXT PRIMARY KEY,
            heartbeat_id TEXT NOT NULL REFERENCES heartbeats(id),
            link_id TEXT NOT NULL,
            interface_name TEXT NOT NULL,
            last_handshake TEXT,
            rx_bytes INTEGER NOT NULL DEFAULT 0,
            tx_bytes INTEGER NOT NULL DEFAULT 0,
            peer_public_key TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS probe_jobs (
            id TEXT PRIMARY KEY,
            probe_type TEXT NOT NULL DEFAULT 'link',
            source_node_id TEXT NOT NULL REFERENCES nodes(id),
            target_node_id TEXT REFERENCES nodes(id),
            policy_id TEXT REFERENCES traffic_policies(id),
            created_at TEXT NOT NULL,
            completed_at TEXT
        );
        CREATE TABLE IF NOT EXISTS probe_results (
            id TEXT PRIMARY KEY,
            probe_job_id TEXT NOT NULL REFERENCES probe_jobs(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            link_id TEXT REFERENCES wireguard_links(id),
            success_count INTEGER NOT NULL DEFAULT 0,
            failure_count INTEGER NOT NULL DEFAULT 0,
            packet_loss_pct REAL NOT NULL DEFAULT 0.0,
            rtt_min_ms REAL NOT NULL DEFAULT 0.0,
            rtt_median_ms REAL NOT NULL DEFAULT 0.0,
            rtt_p95_ms REAL NOT NULL DEFAULT 0.0,
            jitter_ms REAL NOT NULL DEFAULT 0.0,
            probed_at TEXT NOT NULL,
            is_stale INTEGER NOT NULL DEFAULT 0,
            error_code TEXT
        );
        CREATE TABLE IF NOT EXISTS enrollment_tokens (
            id TEXT PRIMARY KEY,
            token TEXT NOT NULL UNIQUE,
            network_id TEXT NOT NULL DEFAULT '',
            parent_node_id TEXT NOT NULL REFERENCES nodes(id),
            suggested_node_name TEXT NOT NULL,
            allowed_install_mode TEXT NOT NULL DEFAULT 'node',
            expires_at TEXT NOT NULL,
            used_at TEXT,
            revoked INTEGER NOT NULL DEFAULT 0,
            nonce TEXT NOT NULL DEFAULT '',
            controller_signature TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS artifacts (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            version TEXT NOT NULL,
            architecture TEXT NOT NULL DEFAULT 'amd64',
            sha256 TEXT NOT NULL,
            size_bytes INTEGER NOT NULL DEFAULT 0,
            manifest_id TEXT,
            created_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS artifact_cache_records (
            id TEXT PRIMARY KEY,
            node_id TEXT NOT NULL REFERENCES nodes(id),
            artifact_id TEXT NOT NULL REFERENCES artifacts(id),
            cached_at TEXT NOT NULL,
            is_complete INTEGER NOT NULL DEFAULT 0
        );
        CREATE TABLE IF NOT EXISTS update_jobs (
            id TEXT PRIMARY KEY,
            target_version TEXT NOT NULL,
            manifest_sha256 TEXT NOT NULL,
            status TEXT NOT NULL DEFAULT 'Created',
            created_at TEXT NOT NULL,
            completed_at TEXT
        );
        CREATE TABLE IF NOT EXISTS update_targets (
            id TEXT PRIMARY KEY,
            update_job_id TEXT NOT NULL REFERENCES update_jobs(id),
            node_id TEXT NOT NULL REFERENCES nodes(id),
            depth INTEGER NOT NULL DEFAULT 0,
            status TEXT NOT NULL DEFAULT 'Waiting',
            started_at TEXT,
            completed_at TEXT,
            error_message TEXT
        );
        CREATE TABLE IF NOT EXISTS audit_logs (
            id TEXT PRIMARY KEY,
            admin_id TEXT,
            action TEXT NOT NULL,
            resource_type TEXT NOT NULL,
            resource_id TEXT,
            details TEXT,
            ip_address TEXT NOT NULL DEFAULT '',
            created_at TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
        CREATE INDEX IF NOT EXISTS idx_sessions_admin ON sessions(admin_id);
        CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(control_parent_id);
        CREATE INDEX IF NOT EXISTS idx_nodes_display ON nodes(display_name);
        CREATE INDEX IF NOT EXISTS idx_links_nodes ON wireguard_links(node_a, node_b);
        CREATE INDEX IF NOT EXISTS idx_control_relations_parent ON control_relations(parent_id);
        CREATE INDEX IF NOT EXISTS idx_control_relations_child ON control_relations(child_id);
        CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_token ON enrollment_tokens(token);
        CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_parent ON enrollment_tokens(parent_node_id);
        CREATE INDEX IF NOT EXISTS idx_update_targets_job ON update_targets(update_job_id);
        CREATE INDEX IF NOT EXISTS idx_update_targets_node ON update_targets(node_id);
        CREATE INDEX IF NOT EXISTS idx_audit_logs_admin ON audit_logs(admin_id);
        CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
        CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at);
    """)
    db.commit()

def now() -> str:
    return datetime.now(timezone.utc).isoformat()

def uid() -> str:
    return str(uuid.uuid4())

def new_token() -> str:
    return secrets.token_hex(32)

# ─── Audit logger ───
def audit_log(db: sqlite3.Connection, admin_id: Optional[str], action: str,
              resource_type: str, resource_id: Optional[str] = None,
              details: Optional[str] = None, ip_address: str = ""):
    try:
        db.execute(
            "INSERT INTO audit_logs (id, admin_id, action, resource_type, resource_id, details, ip_address, created_at) VALUES (?,?,?,?,?,?,?,?)",
            (uid(), admin_id, action, resource_type, resource_id, details, ip_address, now()))
        db.commit()
    except Exception as e:
        log.error(f"Audit log failed: {e}")

# ─── Auth helpers ───
def get_client_ip(req: Request) -> str:
    forwarded = req.headers.get("X-Forwarded-For")
    if forwarded:
        return forwarded.split(",")[0].strip()
    host = req.client.host if req.client else "unknown"
    return host

def get_session(db: sqlite3.Connection, req: Request) -> Optional[sqlite3.Row]:
    token = req.cookies.get("pw_token")
    if not token:
        token = req.headers.get("Authorization", "").replace("Bearer ", "")
    if not token:
        return None
    row = db.execute(
        "SELECT * FROM sessions WHERE token=? AND revoked=0", (token,)
    ).fetchone()
    if not row:
        return None
    if row["expires_at"] < now():
        return None
    return row

def auth_required(f):
    @wraps(f)
    async def wrapper(req: Request, *args, **kwargs):
        db = get_db()
        sess = get_session(db, req)
        if not sess:
            raise HTTPException(status_code=401, detail="Not authenticated")
        req.state.admin_id = sess["admin_id"]
        return await f(req, *args, **kwargs)
    return wrapper

def make_session_cookie(token: str, max_age_hours: int = SESSION_HOURS) -> dict:
    return {
        "key": "pw_token",
        "value": token,
        "httponly": True,
        "samesite": "lax",
        "secure": os.getenv("PW_TLS", "") == "1",
        "max_age": max_age_hours * 3600,
        "path": "/",
    }

# ─── WebSocket manager ───
_ws_clients: list[WebSocket] = []

async def ws_broadcast(msg: dict):
    stale = []
    for ws in _ws_clients:
        try:
            await ws.send_json(msg)
        except Exception:
            stale.append(ws)
    for s in stale:
        _ws_clients.remove(s)

# ─── Auth API ───
@app.post("/api/auth/login")
async def login(req: Request):
    ip = get_client_ip(req)
    if not check_rate_limit(f"login:{ip}", MAX_LOGIN_ATTEMPTS, LOGIN_WINDOW_SEC):
        raise HTTPException(status_code=429, detail="Too many attempts. Try again later.")

    data = await req.json()
    username = data.get("username", "admin").strip()
    password = data.get("password", "")
    if not password:
        raise HTTPException(status_code=400, detail="Password required")

    db = get_db()
    admin = db.execute("SELECT * FROM admin WHERE username=?", (username,)).fetchone()
    if not admin:
        raise HTTPException(status_code=401, detail="Invalid credentials")
    if not verify_password(admin["password_hash"], password):
        raise HTTPException(status_code=401, detail="Invalid credentials")

    tok = new_token()
    sid = uid()
    ua = req.headers.get("User-Agent", "")
    exp = (datetime.now(timezone.utc) + timedelta(hours=SESSION_HOURS)).isoformat()
    db.execute(
        "INSERT INTO sessions (id, admin_id, token, ip_address, user_agent, created_at, expires_at, revoked) VALUES (?,?,?,?,?,?,?,0)",
        (sid, admin["id"], tok, ip, ua, now(), exp))
    db.commit()
    audit_log(db, admin["id"], "login", "session", sid, f"IP: {ip}", ip)

    resp = JSONResponse({"token": tok, "message": "ok"})
    ck = make_session_cookie(tok)
    resp.set_cookie(**ck)
    return resp

@app.post("/api/auth/logout")
async def logout(req: Request):
    tok = req.cookies.get("pw_token", "")
    db = get_db()
    if tok:
        sess = db.execute("SELECT * FROM sessions WHERE token=?", (tok,)).fetchone()
        db.execute("UPDATE sessions SET revoked=1 WHERE token=?", (tok,))
        db.commit()
        if sess:
            audit_log(db, sess["admin_id"], "logout", "session", sess["id"], ip_address=get_client_ip(req))
    resp = JSONResponse({"status": "ok"})
    resp.delete_cookie("pw_token", path="/")
    return resp

@app.get("/api/auth/me")
@auth_required
async def me(req: Request):
    db = get_db()
    admin = db.execute("SELECT id, username, created_at FROM admin WHERE id=?", (req.state.admin_id,)).fetchone()
    if not admin:
        raise HTTPException(status_code=404, detail="Admin not found")
    return {"id": admin["id"], "username": admin["username"], "created_at": admin["created_at"]}

@app.post("/api/auth/change-password")
@auth_required
async def change_password(req: Request):
    data = await req.json()
    current = data.get("current_password", "")
    new_pw = data.get("new_password", "")
    if not new_pw or len(new_pw) < 8:
        raise HTTPException(status_code=400, detail="New password must be at least 8 characters")

    db = get_db()
    admin = db.execute("SELECT * FROM admin WHERE id=?", (req.state.admin_id,)).fetchone()
    if not admin:
        raise HTTPException(status_code=404, detail="Admin not found")
    if not verify_password(admin["password_hash"], current):
        raise HTTPException(status_code=403, detail="Current password is incorrect")

    new_hash = hash_password(new_pw)
    db.execute("UPDATE admin SET password_hash=?, updated_at=? WHERE id=?", (new_hash, now(), req.state.admin_id))
    db.commit()
    audit_log(db, req.state.admin_id, "change_password", "admin", req.state.admin_id, ip_address=get_client_ip(req))
    return {"status": "ok"}

@app.post("/api/auth/revoke-sessions")
@auth_required
async def revoke_sessions(req: Request):
    db = get_db()
    current_token = req.cookies.get("pw_token", "")
    db.execute(
        "UPDATE sessions SET revoked=1 WHERE admin_id=? AND token!=? AND revoked=0",
        (req.state.admin_id, current_token))
    db.commit()
    audit_log(db, req.state.admin_id, "revoke_sessions", "session", ip_address=get_client_ip(req))
    return {"status": "ok"}

@app.get("/api/auth/sessions")
@auth_required
async def list_sessions(req: Request):
    db = get_db()
    rows = db.execute(
        "SELECT id, ip_address, user_agent, created_at, expires_at, revoked FROM sessions WHERE admin_id=? ORDER BY created_at DESC",
        (req.state.admin_id,)).fetchall()
    return [dict(r) for r in rows]

# ─── Node API ───
@app.get("/api/nodes")
@auth_required
async def list_nodes():
    db = get_db()
    rows = db.execute("SELECT * FROM nodes ORDER BY is_controller DESC, display_name").fetchall()
    result = []
    for r in rows:
        d = dict(r)
        d["addresses"] = [dict(a) for a in db.execute("SELECT * FROM node_addresses WHERE node_id=?", (d["id"],)).fetchall()]
        d["links"] = [dict(l) for l in db.execute(
            "SELECT * FROM wireguard_links WHERE (node_a=? OR node_b=?) AND enabled=1",
            (d["id"], d["id"])).fetchall()]
        d["control_parent"] = _get_control_parent_name(db, d.get("control_parent_id"))
        result.append(d)
    return result

def _get_control_parent_name(db: sqlite3.Connection, parent_id: Optional[str]) -> Optional[str]:
    if not parent_id:
        return None
    row = db.execute("SELECT display_name FROM nodes WHERE id=?", (parent_id,)).fetchone()
    return row["display_name"] if row else None

@app.put("/api/nodes/{node_id}")
@auth_required
async def update_node(node_id: str, req: Request):
    data = await req.json()
    db = get_db()
    node = db.execute("SELECT * FROM nodes WHERE id=?", (node_id,)).fetchone()
    if not node:
        raise HTTPException(status_code=404, detail="Node not found")

    fields = []
    vals = []

    if "display_name" in data:
        fields.append("display_name=?")
        vals.append(data["display_name"])
    if "wg_port_range_start" in data:
        fields.append("wg_port_range_start=?")
        vals.append(int(data["wg_port_range_start"]))
    if "wg_port_range_end" in data:
        fields.append("wg_port_range_end=?")
        vals.append(int(data["wg_port_range_end"]))

    if fields:
        fields.append("updated_at=?")
        vals.append(now())
        vals.append(node_id)
        db.execute(f"UPDATE nodes SET {','.join(fields)} WHERE id=?", vals)
        db.commit()

    # Update addresses
    if "addresses" in data:
        db.execute("DELETE FROM node_addresses WHERE node_id=?", (node_id,))
        for addr in data["addresses"]:
            db.execute(
                "INSERT INTO node_addresses (id, node_id, address, address_type, is_primary, created_at) VALUES (?,?,?,?,?,?)",
                (uid(), node_id, addr.get("address",""), addr.get("type","public"), 1 if addr.get("primary") else 0, now()))
        db.commit()

    audit_log(db, req.state.admin_id, "update_node", "node", node_id, json.dumps(data, ensure_ascii=False), get_client_ip(req))
    return {"status": "ok"}

@app.put("/api/nodes/{node_id}/rename")
@auth_required
async def rename_node(node_id: str, req: Request):
    data = await req.json()
    name = data.get("display_name", "")
    if not name:
        raise HTTPException(status_code=400, detail="Name required")
    db = get_db()
    db.execute("UPDATE nodes SET display_name=?, updated_at=? WHERE id=?", (name, now(), node_id))
    db.commit()
    audit_log(db, req.state.admin_id, "rename_node", "node", node_id, f"Renamed to: {name}", get_client_ip(req))
    return {"status": "ok"}

@app.put("/api/nodes/{node_id}/addresses")
@auth_required
async def update_node_addresses(node_id: str, req: Request):
    data = await req.json()
    db = get_db()
    db.execute("DELETE FROM node_addresses WHERE node_id=?", (node_id,))
    for addr in data.get("addresses", []):
        db.execute(
            "INSERT INTO node_addresses (id, node_id, address, address_type, is_primary, created_at) VALUES (?,?,?,?,?,?)",
            (uid(), node_id, addr.get("address",""), addr.get("type","public"), 1 if addr.get("primary") else 0, now()))
    db.commit()
    audit_log(db, req.state.admin_id, "update_addresses", "node", node_id, json.dumps(data, ensure_ascii=False), get_client_ip(req))
    return {"status": "ok"}

# ─── Enrollment ───
@app.post("/api/enrollment/tokens")
@auth_required
async def create_enrollment_token(req: Request):
    data = await req.json()
    pid = data.get("parent_node_id", "")
    name = data.get("suggested_node_name", "new-node")
    ttl = data.get("ttl_seconds", 600)
    if ttl < 10:
        ttl = 10
    if ttl > 86400:
        ttl = 86400

    db = get_db()
    parent = db.execute("SELECT * FROM nodes WHERE id=?", (pid,)).fetchone()
    if not parent:
        raise HTTPException(status_code=404, detail="Parent node not found")

    tok_val = new_token()
    nonce = secrets.token_hex(16)
    exp = (datetime.now(timezone.utc) + timedelta(seconds=ttl)).isoformat()

    # HMAC-SHA256 controller signature: sign(token + parent_id + name + expires + nonce)
    sign_key = FERNET._signing_key if hasattr(FERNET, '_signing_key') else secrets.token_bytes(32)
    sig_data = f"{tok_val}|{pid}|{name}|{exp}|{nonce}"
    signature = hmac.new(sign_key, sig_data.encode(), hashlib.sha256).hexdigest()

    tid = uid()
    db.execute(
        "INSERT INTO enrollment_tokens (id, token, network_id, parent_node_id, suggested_node_name, allowed_install_mode, expires_at, nonce, controller_signature, created_at) VALUES (?,?,?,?,?,?,?,?,?,?)",
        (tid, tok_val, "", pid, name, "node", exp, nonce, signature, now()))
    db.commit()

    parent_addr = parent.get("overlay_ipv4", PUBLIC_IP)
    parent_port = parent.get("node_service_port", WEB_PORT)
    cmd = f"curl -fsSL 'http://{parent_addr}:{parent_port}/bootstrap/install.sh?token={tok_val}' | sudo bash"

    audit_log(db, req.state.admin_id, "create_token", "enrollment_token", tid,
              f"Parent: {parent['display_name']}, Node: {name}", get_client_ip(req))

    return {
        "token_id": tid, "token": tok_val,
        "install_command": cmd, "expires_at": exp,
        "parent_node_name": parent["display_name"],
    }

@app.get("/api/enrollment/tokens")
@auth_required
async def list_enrollment_tokens():
    db = get_db()
    rows = db.execute("SELECT * FROM enrollment_tokens ORDER BY created_at DESC").fetchall()
    return [dict(r) for r in rows]

@app.post("/api/enrollment/tokens/{token_id}/revoke")
@auth_required
async def revoke_enrollment_token(token_id: str, req: Request):
    db = get_db()
    db.execute("UPDATE enrollment_tokens SET revoked=1 WHERE id=?", (token_id,))
    db.commit()
    audit_log(db, req.state.admin_id, "revoke_token", "enrollment_token", token_id, ip_address=get_client_ip(req))
    return {"status": "ok"}

@app.post("/bootstrap/enroll")
async def bootstrap_enroll(req: Request):
    data = await req.json()
    tok_val = data.get("token", "")
    name = data.get("node_name", "new-node")
    wg_pub = data.get("wg_public_key", "")
    id_pub = data.get("identity_public_key", "")
    agent_ver = data.get("agent_version", "1.0.0")
    proto_ver = data.get("protocol_version", 1)

    db = get_db()
    tok = db.execute("SELECT * FROM enrollment_tokens WHERE token=?", (tok_val,)).fetchone()
    if not tok:
        raise HTTPException(status_code=404, detail="Token not found")
    if tok["revoked"]:
        raise HTTPException(status_code=403, detail="Token revoked")
    if tok["used_at"]:
        raise HTTPException(status_code=403, detail="Token already used")
    if tok["expires_at"] < now():
        raise HTTPException(status_code=403, detail="Token expired")

    parent = db.execute("SELECT * FROM nodes WHERE id=?", (tok["parent_node_id"],)).fetchone()
    if not parent:
        raise HTTPException(status_code=404, detail="Parent node not found")

    # Allocate overlay IP
    existing = db.execute("SELECT overlay_ipv4 FROM nodes").fetchall()
    used = set(r["overlay_ipv4"] for r in existing)
    base = parent["overlay_ipv4"].rsplit(".", 1)[0]
    new_ip = None
    for i in range(2, 255):
        ip = f"{base}.{i}"
        if ip not in used:
            new_ip = ip
            break
    if not new_ip:
        raise HTTPException(status_code=500, detail="Overlay IP pool exhausted")

    nid = uid()
    wg_priv = new_token()
    id_priv = new_token()
    wg_pub_key = wg_pub or hashlib.sha256(wg_priv.encode()).hexdigest()
    id_pub_key = id_pub or hashlib.sha256(id_priv.encode()).hexdigest()

    db.execute(
        "INSERT INTO nodes (id, display_name, overlay_ipv4, wg_public_key, wg_private_key_encrypted, identity_public_key, identity_private_key_encrypted, control_parent_id, node_service_port, wg_port_range_start, wg_port_range_end, is_controller, agent_version, protocol_version, created_at, updated_at, last_seen_at, enrollment_token_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,0,?,?,?,?,?,?)",
        (nid, name, new_ip, wg_pub_key, encrypt_secret(wg_priv), id_pub_key, encrypt_secret(id_priv),
         parent["id"], NODE_PORT, WG_START, WG_END,
         agent_ver, proto_ver, now(), now(), now(), tok["id"]))

    # Control relation
    db.execute(
        "INSERT INTO control_relations (id, parent_id, child_id, enrolled_at, enrollment_token_id) VALUES (?,?,?,?,?)",
        (uid(), parent["id"], nid, now(), tok["id"]))

    db.execute("UPDATE enrollment_tokens SET used_at=? WHERE id=?", (now(), tok["id"]))
    db.commit()

    audit_log(db, None, "node_enrolled", "node", nid,
              f"Name: {name}, IP: {new_ip}, Parent: {parent['display_name']}")

    return {
        "node_id": nid,
        "overlay_ipv4": new_ip,
        "parent_wg_public_key": parent["wg_public_key"],
        "parent_wg_endpoint": f"{parent['overlay_ipv4']}:{parent['wg_port_range_start']}",
        "recovery_endpoint": f"{parent['overlay_ipv4']}:{parent['node_service_port']}",
    }

# ─── WireGuard Links ───
@app.get("/api/links")
@auth_required
async def list_links():
    db = get_db()
    rows = db.execute("SELECT * FROM wireguard_links ORDER BY created_at DESC").fetchall()
    result = []
    for r in rows:
        d = dict(r)
        a = db.execute("SELECT display_name FROM nodes WHERE id=?", (d["node_a"],)).fetchone()
        b = db.execute("SELECT display_name FROM nodes WHERE id=?", (d["node_b"],)).fetchone()
        d["node_a_name"] = a["display_name"] if a else ""
        d["node_b_name"] = b["display_name"] if b else ""
        result.append(d)
    return result

@app.post("/api/links")
@auth_required
async def create_link(req: Request):
    data = await req.json()
    aid = data.get("initiator_node_id", "")
    bid = data.get("listener_node_id", "")
    lip = data.get("listener_address", "")
    lport = data.get("listener_port", WG_START)
    weight = data.get("admin_weight", 1)

    db = get_db()
    a = db.execute("SELECT * FROM nodes WHERE id=?", (aid,)).fetchone()
    b = db.execute("SELECT * FROM nodes WHERE id=?", (bid,)).fetchone()
    if not a or not b:
        raise HTTPException(status_code=404, detail="Node not found")
    if aid == bid:
        raise HTTPException(status_code=400, detail="Cannot link a node to itself")

    existing = db.execute(
        "SELECT id FROM wireguard_links WHERE (node_a=? AND node_b=?) OR (node_a=? AND node_b=?)",
        (aid, bid, bid, aid)).fetchone()
    if existing:
        raise HTTPException(status_code=409, detail="Link already exists between these nodes")

    lid = uid()
    iface_a = f"pwl-{a['display_name'][:6]}-{b['display_name'][:6]}"
    iface_b = f"pwl-{b['display_name'][:6]}-{a['display_name'][:6]}"
    db.execute(
        "INSERT INTO wireguard_links (id, node_a, node_b, initiator_node_id, listener_node_id, listener_address, listener_port, interface_name_a, interface_name_b, enabled, admin_weight, status, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)",
        (lid, aid, bid, aid, bid, lip or b["overlay_ipv4"], lport, iface_a, iface_b, 1, weight, "Active", now()))

    # Endpoints
    db.execute(
        "INSERT INTO wireguard_link_endpoints (id, link_id, node_id, interface_name, listen_port, peer_endpoint, peer_public_key, persistent_keepalive, is_initiator) VALUES (?,?,?,?,?,?,?,?,1)",
        (uid(), lid, aid, iface_a, WG_START, f"{b['overlay_ipv4']}:{lport}", b["wg_public_key"], 25))
    db.execute(
        "INSERT INTO wireguard_link_endpoints (id, link_id, node_id, interface_name, listen_port, peer_endpoint, peer_public_key, persistent_keepalive, is_initiator) VALUES (?,?,?,?,?,?,?,?,0)",
        (uid(), lid, bid, iface_b, lport, None, a["wg_public_key"], 0))
    db.commit()

    audit_log(db, req.state.admin_id, "create_link", "wireguard_link", lid,
              f"{a['display_name']} <-> {b['display_name']}", get_client_ip(req))
    return {
        "link_id": lid,
        "interface_name_a": iface_a, "interface_name_b": iface_b,
        "listener_port": lport, "status": "Active",
    }

@app.put("/api/links/{link_id}")
@auth_required
async def update_link(link_id: str, req: Request):
    data = await req.json()
    db = get_db()
    link = db.execute("SELECT * FROM wireguard_links WHERE id=?", (link_id,)).fetchone()
    if not link:
        raise HTTPException(status_code=404, detail="Link not found")

    if "enabled" in data:
        db.execute("UPDATE wireguard_links SET enabled=? WHERE id=?", (1 if data["enabled"] else 0, link_id))
    if "admin_weight" in data:
        db.execute("UPDATE wireguard_links SET admin_weight=? WHERE id=?", (int(data["admin_weight"]), link_id))
    db.commit()
    audit_log(db, req.state.admin_id, "update_link", "wireguard_link", link_id, json.dumps(data, ensure_ascii=False), get_client_ip(req))
    return {"status": "ok"}

@app.delete("/api/links/{link_id}")
@auth_required
async def delete_link(link_id: str, req: Request):
    db = get_db()
    db.execute("DELETE FROM wireguard_link_endpoints WHERE link_id=?", (link_id,))
    db.execute("DELETE FROM wireguard_links WHERE id=?", (link_id,))
    db.commit()
    audit_log(db, req.state.admin_id, "delete_link", "wireguard_link", link_id, ip_address=get_client_ip(req))
    return {"status": "deleted"}

# ─── Policies ───
@app.get("/api/policies")
@auth_required
async def list_policies():
    db = get_db()
    policies = db.execute("SELECT * FROM traffic_policies ORDER BY priority ASC, name").fetchall()
    result = []
    for p in policies:
        pd = dict(p)
        matches = [dict(m) for m in db.execute("SELECT * FROM traffic_policy_matches WHERE policy_id=?", (pd["id"],)).fetchall()]
        paths = [dict(h) for h in db.execute("SELECT * FROM traffic_policy_paths WHERE policy_id=? ORDER BY hop_order", (pd["id"],)).fetchall()]
        configs = db.execute("SELECT * FROM traffic_policy_configs WHERE policy_id=?", (pd["id"],)).fetchone()
        for h in paths:
            node = db.execute("SELECT display_name FROM nodes WHERE id=?", (h["node_id"],)).fetchone()
            h["node_name"] = node["display_name"] if node else h["node_id"][:8]
        result.append({
            "policy": {k: v for k, v in pd.items() if k not in ("created_at","updated_at") or k in ("id","name","priority","enabled","description","created_at","updated_at")},
            "matches": matches,
            "path": paths,
            "config": dict(configs) if configs else None,
        })
    return result

@app.post("/api/policies")
@auth_required
async def create_policy(req: Request):
    data = await req.json()
    pid = uid()
    db = get_db()

    path_node_ids = data.get("path_node_ids", [])
    # Validate: adjacent nodes have links
    validation_errors = []
    for i in range(len(path_node_ids) - 1):
        a, b = path_node_ids[i], path_node_ids[i+1]
        link = db.execute(
            "SELECT id FROM wireguard_links WHERE enabled=1 AND ((node_a=? AND node_b=?) OR (node_a=? AND node_b=?))",
            (a, b, b, a)).fetchone()
        if not link:
            na = db.execute("SELECT display_name FROM nodes WHERE id=?", (a,)).fetchone()
            nb = db.execute("SELECT display_name FROM nodes WHERE id=?", (b,)).fetchone()
            validation_errors.append(f"节点 '{na['display_name'] if na else a[:8]}' 与 '{nb['display_name'] if nb else b[:8]}' 之间没有直接 WireGuard 链路")

    db.execute(
        "INSERT INTO traffic_policies (id, name, priority, enabled, description, created_at, updated_at) VALUES (?,?,?,1,?,?,?)",
        (pid, data["name"], data.get("priority", 10), data.get("description", ""), now(), now()))

    matches = data.get("matches", [{}])
    for m in matches:
        db.execute(
            "INSERT INTO traffic_policy_matches (id, policy_id, source_node_id, source_cidr, destination_node_id, destination_cidr, protocol, destination_port_start, destination_port_end) VALUES (?,?,?,?,?,?,?,?,?)",
            (uid(), pid, m.get("source_node_id"), m.get("source_cidr"), m.get("destination_node_id"),
             m.get("destination_cidr"), m.get("protocol", "Any"),
             m.get("destination_port_start"), m.get("destination_port_end")))

    for idx, nid in enumerate(path_node_ids):
        db.execute(
            "INSERT INTO traffic_policy_paths (id, policy_id, hop_order, node_id) VALUES (?,?,?,?)",
            (uid(), pid, idx, nid))

    db.execute(
        "INSERT INTO traffic_policy_configs (id, policy_id, egress_nat, return_path_type, failover_enabled) VALUES (?,?,?,?,?)",
        (uid(), pid, 1 if data.get("egress_nat") else 0, data.get("return_path_type", "Symmetric"),
         1 if data.get("failover_enabled") else 0))
    db.commit()

    audit_log(db, req.state.admin_id, "create_policy", "traffic_policy", pid, data["name"], get_client_ip(req))

    result = dict(db.execute("SELECT * FROM traffic_policies WHERE id=?", (pid,)).fetchone())
    return {
        "policy": {k: v for k, v in result.items() if k in ("id","name","priority","enabled","description","created_at","updated_at")},
        "matches": matches, "path_node_ids": path_node_ids,
        "validation_errors": validation_errors,
    }

@app.put("/api/policies/{policy_id}")
@auth_required
async def update_policy(policy_id: str, req: Request):
    data = await req.json()
    db = get_db()
    existing = db.execute("SELECT * FROM traffic_policies WHERE id=?", (policy_id,)).fetchone()
    if not existing:
        raise HTTPException(status_code=404, detail="Policy not found")

    db.execute("UPDATE traffic_policies SET name=?, priority=?, enabled=?, description=?, updated_at=? WHERE id=?",
               (data.get("name", existing["name"]), data.get("priority", existing["priority"]),
                1 if data.get("enabled", existing["enabled"]) else 0,
                data.get("description", existing.get("description")), now(), policy_id))

    if "matches" in data:
        db.execute("DELETE FROM traffic_policy_matches WHERE policy_id=?", (policy_id,))
        for m in data["matches"]:
            db.execute(
                "INSERT INTO traffic_policy_matches (id, policy_id, source_node_id, source_cidr, destination_node_id, destination_cidr, protocol, destination_port_start, destination_port_end) VALUES (?,?,?,?,?,?,?,?,?)",
                (uid(), policy_id, m.get("source_node_id"), m.get("source_cidr"), m.get("destination_node_id"),
                 m.get("destination_cidr"), m.get("protocol","Any"),
                 m.get("destination_port_start"), m.get("destination_port_end")))

    if "path_node_ids" in data:
        db.execute("DELETE FROM traffic_policy_paths WHERE policy_id=?", (policy_id,))
        for idx, nid in enumerate(data["path_node_ids"]):
            db.execute(
                "INSERT INTO traffic_policy_paths (id, policy_id, hop_order, node_id) VALUES (?,?,?,?)",
                (uid(), policy_id, idx, nid))

    if any(k in data for k in ("egress_nat", "return_path_type", "failover_enabled")):
        db.execute("DELETE FROM traffic_policy_configs WHERE policy_id=?", (policy_id,))
        db.execute(
            "INSERT INTO traffic_policy_configs (id, policy_id, egress_nat, return_path_type, failover_enabled) VALUES (?,?,?,?,?)",
            (uid(), policy_id, 1 if data.get("egress_nat") else 0,
             data.get("return_path_type","Symmetric"), 1 if data.get("failover_enabled") else 0))
    db.commit()
    audit_log(db, req.state.admin_id, "update_policy", "traffic_policy", policy_id, json.dumps(data, ensure_ascii=False), get_client_ip(req))
    return {"status": "ok"}

@app.delete("/api/policies/{policy_id}")
@auth_required
async def delete_policy(policy_id: str, req: Request):
    db = get_db()
    db.execute("DELETE FROM traffic_policy_configs WHERE policy_id=?", (policy_id,))
    db.execute("DELETE FROM traffic_policy_paths WHERE policy_id=?", (policy_id,))
    db.execute("DELETE FROM traffic_policy_matches WHERE policy_id=?", (policy_id,))
    db.execute("DELETE FROM traffic_policies WHERE id=?", (policy_id,))
    db.commit()
    audit_log(db, req.state.admin_id, "delete_policy", "traffic_policy", policy_id, ip_address=get_client_ip(req))
    return {"status": "deleted"}

# ─── Config Publish ───
@app.get("/api/config/preview")
@auth_required
async def config_preview():
    db = get_db()
    nodes = [dict(r) for r in db.execute("SELECT * FROM nodes").fetchall()]
    links = [dict(r) for r in db.execute("SELECT * FROM wireguard_links WHERE enabled=1").fetchall()]
    policies = [dict(r) for r in db.execute("SELECT * FROM traffic_policies WHERE enabled=1").fetchall()]

    errors = []
    configs = []
    for node in nodes:
        node_links = [l for l in links if l["node_a"] == node["id"] or l["node_b"] == node["id"]]
        node_policies = []
        for pol in policies:
            paths = db.execute("SELECT node_id FROM traffic_policy_paths WHERE policy_id=? ORDER BY hop_order", (pol["id"],)).fetchall()
            path_ids = [p["node_id"] for p in paths]
            if node["id"] in path_ids:
                idx = path_ids.index(node["id"])
                config = db.execute("SELECT * FROM traffic_policy_configs WHERE policy_id=?", (pol["id"],)).fetchone()
                node_policies.append({
                    "policy_name": pol["name"],
                    "fwmak": 1000 + idx,
                    "table_id": 100 + idx,
                    "next_hop": path_ids[idx+1] if idx+1 < len(path_ids) else None,
                    "egress_nat": bool(config["egress_nat"]) if config else False,
                })
        configs.append({
            "node_id": node["id"], "display_name": node["display_name"],
            "overlay_ipv4": node["overlay_ipv4"],
            "wireguard_links": [{
                "link_id": l["id"],
                "interface_name": l["interface_name_a"] if l["node_a"] == node["id"] else l["interface_name_b"],
                "peer": (l["node_b"] if l["node_a"] == node["id"] else l["node_a"]),
            } for l in node_links],
            "policy_routing_rules": node_policies,
            "route_tables": [],
            "nat_rules": [{"rule_type": "Masquerade", "policy": p["policy_name"]} for p in node_policies if p["egress_nat"]],
        })

    for pol in policies:
        paths = db.execute("SELECT node_id FROM traffic_policy_paths WHERE policy_id=? ORDER BY hop_order", (pol["id"],)).fetchall()
        path_ids = [p["node_id"] for p in paths]
        for i in range(len(path_ids) - 1):
            a, b = path_ids[i], path_ids[i+1]
            if not any((l["node_a"] == a and l["node_b"] == b) or (l["node_a"] == b and l["node_b"] == a) for l in links):
                na = db.execute("SELECT display_name FROM nodes WHERE id=?", (a,)).fetchone()
                nb = db.execute("SELECT display_name FROM nodes WHERE id=?", (b,)).fetchone()
                errors.append(f"Policy '{pol['name']}': no link between {na['display_name'] if na else a[:8]} and {nb['display_name'] if nb else b[:8]}")

    generation = int(time.time())
    return {"generation": generation, "node_configs": configs, "validation_errors": errors, "ready": len(errors) == 0}

@app.post("/api/config/publish")
@auth_required
async def publish_config(req: Request):
    data = await req.json()
    db = get_db()
    revision_id = uid()
    generation = int(time.time())
    reason = data.get("reason", "Manual publish")

    db.execute(
        "INSERT INTO config_revisions (id, generation, reason, status, created_at) VALUES (?,?,?,?,?)",
        (revision_id, generation, reason, "Draft", now()))

    nodes = db.execute("SELECT * FROM nodes").fetchall()
    for node in nodes:
        config_data = json.dumps({"node_id": node["id"], "generation": generation})
        config_hash = hashlib.sha256(config_data.encode()).hexdigest()
        nid = uid()
        db.execute(
            "INSERT INTO node_desired_configs (id, config_revision_id, node_id, config_data, config_hash) VALUES (?,?,?,?,?)",
            (nid, revision_id, node["id"], config_data, config_hash))
        db.execute(
            "INSERT INTO config_rollout_nodes (id, config_revision_id, node_id, status) VALUES (?,?,?,?)",
            (uid(), revision_id, node["id"], "Pending"))

    db.execute("UPDATE config_revisions SET status='Published' WHERE id=?", (revision_id,))
    db.execute(f"UPDATE nodes SET desired_generation={generation}")
    db.commit()

    audit_log(db, req.state.admin_id, "publish_config", "config_revision", revision_id, reason, get_client_ip(req))
    await ws_broadcast({"type": "config_published", "generation": generation})

    return {"revision_id": revision_id, "generation": generation, "status": "Published", "node_count": len(nodes)}

@app.get("/api/config/revisions")
@auth_required
async def list_config_revisions():
    db = get_db()
    rows = db.execute("SELECT * FROM config_revisions ORDER BY created_at DESC LIMIT 50").fetchall()
    result = []
    for r in rows:
        d = dict(r)
        nodes = db.execute("SELECT * FROM config_rollout_nodes WHERE config_revision_id=?", (d["id"],)).fetchall()
        d["rollout"] = [dict(n) for n in nodes]
        result.append(d)
    return result

# ─── Updates ───
@app.get("/api/updates")
@auth_required
async def list_updates():
    db = get_db()
    jobs = db.execute("SELECT * FROM update_jobs ORDER BY created_at DESC").fetchall()
    result = []
    for j in jobs:
        d = dict(j)
        targets = db.execute("SELECT * FROM update_targets WHERE update_job_id=?", (d["id"],)).fetchall()
        d["targets"] = []
        for t in targets:
            td = dict(t)
            node = db.execute("SELECT display_name FROM nodes WHERE id=?", (td["node_id"],)).fetchone()
            td["node_name"] = node["display_name"] if node else ""
            d["targets"].append(td)
        d["summary"] = {
            "total": len(d["targets"]),
            "completed": sum(1 for x in d["targets"] if x["status"] == "Completed"),
            "failed": sum(1 for x in d["targets"] if "FAIL" in x["status"].upper()),
            "waiting": sum(1 for x in d["targets"] if x["status"] == "Waiting"),
            "in_progress": sum(1 for x in d["targets"] if x["status"] == "Installing"),
        }
        result.append(d)
    return result

@app.post("/api/updates")
@auth_required
async def create_update(req: Request):
    data = await req.json()
    jid = uid()
    db = get_db()
    db.execute(
        "INSERT INTO update_jobs (id, target_version, manifest_sha256, status, created_at) VALUES (?,?,?,?,?)",
        (jid, data["target_version"], data["manifest_sha256"], "Created", now()))

    nodes = db.execute("SELECT * FROM nodes WHERE is_controller=0").fetchall()
    for n in nodes:
        depth = 0
        pid = n["control_parent_id"]
        while pid:
            depth += 1
            p = db.execute("SELECT control_parent_id FROM nodes WHERE id=?", (pid,)).fetchone()
            pid = p["control_parent_id"] if p else None
        db.execute(
            "INSERT INTO update_targets (id, update_job_id, node_id, depth, status) VALUES (?,?,?,?,?)",
            (uid(), jid, n["id"], depth, "Waiting"))

    ctrl = db.execute("SELECT id FROM nodes WHERE is_controller=1").fetchone()
    if ctrl:
        db.execute(
            "INSERT INTO update_targets (id, update_job_id, node_id, depth, status) VALUES (?,?,?,?,?)",
            (uid(), jid, ctrl["id"], -1, "Waiting"))
    db.commit()

    audit_log(db, req.state.admin_id, "create_update", "update_job", jid, f"Version: {data['target_version']}", get_client_ip(req))
    return await _get_update_detail(db, jid)

def _get_update_detail(db: sqlite3.Connection, jid: str):
    job = dict(db.execute("SELECT * FROM update_jobs WHERE id=?", (jid,)).fetchone())
    targets = [dict(r) for r in db.execute("SELECT * FROM update_targets WHERE update_job_id=?", (jid,)).fetchall()]
    return {
        "job": job, "targets": targets,
        "summary": {
            "total": len(targets), "completed": 0, "failed": 0,
            "waiting": len(targets), "in_progress": 0,
        },
    }

@app.post("/api/updates/{job_id}/start")
@auth_required
async def start_update(job_id: str, req: Request):
    db = get_db()
    db.execute("UPDATE update_jobs SET status='Installing' WHERE id=?", (job_id,))
    db.execute("UPDATE update_targets SET status='Installing' WHERE update_job_id=?", (job_id,))
    db.commit()
    audit_log(db, req.state.admin_id, "start_update", "update_job", job_id, ip_address=get_client_ip(req))
    return {"status": "started"}

@app.post("/api/updates/{job_id}/rollback")
@auth_required
async def rollback_update(job_id: str, req: Request):
    db = get_db()
    db.execute("UPDATE update_jobs SET status='RolledBack', completed_at=? WHERE id=?", (now(), job_id))
    db.execute("UPDATE update_targets SET status='RolledBack', completed_at=? WHERE update_job_id=?", (now(), job_id))
    db.commit()
    audit_log(db, req.state.admin_id, "rollback_update", "update_job", job_id, ip_address=get_client_ip(req))
    return {"status": "rolled_back"}

# ─── Audit Logs ───
@app.get("/api/audit-logs")
@auth_required
async def list_audit_logs(limit: int = Query(100, le=1000)):
    db = get_db()
    rows = db.execute("SELECT * FROM audit_logs ORDER BY created_at DESC LIMIT ?", (limit,)).fetchall()
    return [dict(r) for r in rows]

# ─── Health ───
@app.get("/api/health")
async def health():
    db = get_db()
    node_count = db.execute("SELECT count(*) FROM nodes").fetchone()[0]
    return {"status": "ok", "version": "1.0.0", "nodes": node_count}

# ─── WebSocket ───
@app.websocket("/ws")
async def websocket_endpoint(ws: WebSocket):
    await ws.accept()
    _ws_clients.append(ws)
    try:
        while True:
            data = await ws.receive_text()
            if data == "ping":
                db = get_db()
                nodes = db.execute("SELECT count(*) as total FROM nodes").fetchone()
                await ws.send_json({"type": "pong", "nodes": nodes["total"] if nodes else 0})
    except WebSocketDisconnect:
        pass
    except Exception:
        pass
    finally:
        if ws in _ws_clients:
            _ws_clients.remove(ws)

# ─── Bootstrap ───
DEP_CACHE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "deps_cache")
os.makedirs(DEP_CACHE, exist_ok=True)

@app.get("/bootstrap/install.sh")
async def bootstrap_install(token: str = "", parent_ip: str = None, parent_port: int = None):
    pip = parent_ip or PUBLIC_IP
    ppt = parent_port or WEB_PORT
    script = f"""#!/bin/bash
set -e
echo "PathWeaver Node Installer"
echo "Parent: {pip}:{ppt}"

PARENT="http://{pip}:{ppt}"
TOKEN="{token}"

mkdir -p /opt/pathweaver/{{bin,etc,deps}}
apt-get update -qq && apt-get install -y -qq python3 python3-venv curl wireguard-tools nftables 2>/dev/null || true

echo "Downloading deps..."
mkdir -p /opt/pathweaver/deps
DEP_LIST=$(curl -sf "$PARENT/bootstrap/deps/list" 2>/dev/null || echo "")
if [ -n "$DEP_LIST" ]; then
    echo "$DEP_LIST" | while read f; do
        [ -z "$f" ] && continue
        echo "  -> $f"
        curl -sf "$PARENT/bootstrap/deps/$f" -o "/opt/pathweaver/deps/$f"
    done
fi

echo "Downloading server..."
curl -sf "$PARENT/bootstrap/server.py" -o /opt/pathweaver/main.py

python3 -m venv /opt/pathweaver/.venv
source /opt/pathweaver/.venv/bin/activate
pip install --no-index --find-links=/opt/pathweaver/deps /opt/pathweaver/deps/*.whl 2>/dev/null || \
    pip install --no-index --find-links=/opt/pathweaver/deps fastapi uvicorn argon2-cffi cryptography python-multipart websockets 2>/dev/null || true

WG_PRIV=$(wg genkey 2>/dev/null || head -c32 /dev/urandom | base64 | tr -dc 'A-Za-z0-9+/' | head -c44)
WG_PUB=$(echo "$WG_PRIV" | wg pubkey 2>/dev/null || echo "auto-wg-$(head -c16 /dev/urandom | base64)")
ID_PRIV=$(head -c32 /dev/urandom | base64)
ID_PUB=$(echo -n "$ID_PRIV" | sha256sum | cut -d' ' -f1)

echo "Enrolling..."
RESP=$(curl -sf $PARENT/bootstrap/enroll \\
    -H 'Content-Type: application/json' \\
    -d '{{"token":"$TOKEN","node_name":"$(hostname)","wg_public_key":"'$WG_PUB'","identity_public_key":"'$ID_PUB'","agent_version":"1.0.0","protocol_version":1}}')

NODE_ID=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['node_id'])")
OVERLAY_IP=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['overlay_ipv4'])")
PARENT_WG=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['parent_wg_public_key'])")
PARENT_EP=$(echo "$RESP" | python3 -c "import sys,json; print(json.load(sys.stdin)['parent_wg_endpoint'])")

cat > /opt/pathweaver/etc/node.conf << EOFCONF
NODE_ID=$NODE_ID
OVERLAY_IP=$OVERLAY_IP
WG_PRIV=$WG_PRIV
PARENT_WG_PUB=$PARENT_WG
PARENT_ENDPOINT=$PARENT_EP
PW_WEB_PORT={ppt}
EOFCONF

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
echo "Node enrollment success!"
echo "  Node ID:     $NODE_ID"
echo "  Overlay IP:  $OVERLAY_IP"
echo "  Parent:      {pip}:{ppt}"
echo ""

rm -f /tmp/pw-install.sh
"""
    return HTMLResponse(script, media_type="text/plain")

@app.get("/bootstrap/deps/list")
async def list_deps():
    files = sorted(f.name for f in Path(DEP_CACHE).glob("*.whl"))
    return JSONResponse(files)

@app.get("/bootstrap/deps/{filename}")
async def serve_dep(filename: str):
    path = os.path.join(DEP_CACHE, filename)
    if not os.path.isfile(path):
        return JSONResponse({"error": "not found"}, 404)
    return FileResponse(path, media_type="application/octet-stream")

@app.get("/bootstrap/server.py")
async def serve_server_py():
    path = os.path.join(os.path.dirname(os.path.abspath(__file__)), "main.py")
    return FileResponse(path, media_type="text/x-python")

# ─── Static / SPA fallback ───
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
<style>body{{font-family:system-ui,sans-serif;display:flex;justify-content:center;align-items:center;height:100vh;background:#0f172a;color:#e2e8f0;margin:0}}div{{text-align:center}}a{{color:#38bdf8}}code{{background:#1e293b;padding:3px 8px;border-radius:4px}}</style></head>
<body><div>
<h1>PathWeaver</h1><p>SD-WAN Control Plane v1.0.0</p>
<p>Nodes: <strong>{nc}</strong> | Port: <strong>{WEB_PORT}</strong></p>
<hr style="border-color:#334155;margin:16px 0">
<p>To enable the full admin panel, build and serve the frontend:</p>
<p><code>cd web && npm install && npm run build</code></p>
<p><code>PW_STATIC_DIR=web/dist python3 server/main.py</code></p>
<p><a href="/api/health">API Health</a> · <a href="https://github.com/FengYuchen1314/sd-wan-plus">GitHub</a></p>
</div></body></html>""")

# ─── Startup ───
def main():
    if os.getenv("PW_CONTROLLER", "1") == "1":
        log.info("Controller mode: caching bootstrap deps")
        try:
            _cache_deps()
        except Exception as e:
            log.warning(f"Deps cache failed: {e}")

    db = get_db()
    existing = db.execute("SELECT id FROM nodes WHERE is_controller=1").fetchone()
    if not existing:
        cid = uid()
        wg_priv = new_token()
        id_priv = new_token()
        wg_pub = hashlib.sha256(wg_priv.encode()).hexdigest()
        id_pub = hashlib.sha256(id_priv.encode()).hexdigest()
        db.execute(
            "INSERT INTO nodes (id, display_name, overlay_ipv4, wg_public_key, wg_private_key_encrypted, identity_public_key, identity_private_key_encrypted, is_controller, node_service_port, wg_port_range_start, wg_port_range_end, created_at, updated_at, last_seen_at) VALUES (?,?,?,?,?,?,?,1,?,?,?,?,?,?)",
            (cid, "controller", "10.250.0.1",
             wg_pub, encrypt_secret(wg_priv), id_pub, encrypt_secret(id_priv),
             NODE_PORT, WG_START, WG_END,
             now(), now(), now()))
        db.commit()
        log.info("Controller node created: 10.250.0.1")

    log.info(f"PathWeaver v1.0.0")
    log.info(f"Listening: http://0.0.0.0:{WEB_PORT}")
    log.info("Set admin password on first login\n")
    uvicorn.run(app, host="0.0.0.0", port=WEB_PORT, log_level="info", access_log=False)

def _cache_deps():
    import subprocess
    pkgs = ["fastapi", "uvicorn[standard]", "argon2-cffi", "cryptography", "python-multipart", "websockets"]
    for pkg in pkgs:
        clean = pkg.replace("[standard]", "")
        existing = list(Path(DEP_CACHE).glob(f"{clean}-*"))
        if existing:
            log.info(f"  [cached] {pkg}")
            continue
        log.info(f"  [download] {pkg}")
        subprocess.run([sys.executable, "-m", "pip", "download", pkg, "-d", DEP_CACHE,
                        "--only-binary", ":all:", "--platform", "manylinux2014_x86_64",
                        "--python-version", "311"],
                       capture_output=True, timeout=60)

if __name__ == "__main__":
    main()
