use sqlx::SqlitePool;
use sqlx::sqlite::SqliteConnectOptions;
use pathweaver_core::models::*;
use pathweaver_core::error::{PathWeaverError, Result};
use uuid::Uuid;
use chrono::Utc;

pub struct Database {
    pool: SqlitePool,
}

impl Database {
    pub async fn open(path: &str) -> std::result::Result<Self, sqlx::Error> {
        let pool = if path == "sqlite::memory:" || path.contains(":memory:") {
            SqlitePool::connect("sqlite::memory:").await?
        } else {
            let db_path = path.strip_prefix("sqlite:///")
                .or_else(|| path.strip_prefix("sqlite://"))
                .or_else(|| path.strip_prefix("sqlite:"))
                .unwrap_or(path);

            let opts = SqliteConnectOptions::new()
                .filename(db_path)
                .create_if_missing(true);

            SqlitePool::connect_with(opts).await?
        };
        Ok(Self { pool })
    }

    pub async fn run_migrations(&self) -> std::result::Result<(), sqlx::Error> {
        let migration_sql = include_str!("../../../migrations/001_initial.sql");
        sqlx::query(migration_sql).execute(&self.pool).await?;
        Ok(())
    }

    pub fn pool(&self) -> &SqlitePool {
        &self.pool
    }
}

pub async fn create_admin(
    pool: &SqlitePool,
    username: &str,
    password_hash: &str,
) -> Result<Admin> {
    let id = Uuid::new_v4().to_string();
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO admin (id, username, password_hash, created_at, updated_at)
           VALUES (?, ?, ?, ?, ?)"#,
    )
    .bind(&id)
    .bind(username)
    .bind(password_hash)
    .bind(&now)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(Admin {
        id: Uuid::parse_str(&id).unwrap(),
        username: username.to_string(),
        password_hash: password_hash.to_string(),
        created_at: Utc::now(),
        updated_at: Utc::now(),
    })
}

pub async fn get_admin_by_username(
    pool: &SqlitePool,
    username: &str,
) -> Result<Option<Admin>> {
    let row = sqlx::query_as::<_, AdminRow>(
        r#"SELECT id, username, password_hash, created_at, updated_at
           FROM admin WHERE username = ?"#,
    )
    .bind(username)
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(|r| r.into()))
}

pub async fn create_session(
    pool: &SqlitePool,
    admin_id: &Uuid,
    token: &str,
    ip_address: &str,
    user_agent: &str,
    expires_hours: i64,
) -> Result<Session> {
    let id = Uuid::new_v4().to_string();
    let now = Utc::now();
    let expires = now + chrono::Duration::hours(expires_hours);

    sqlx::query(
        r#"INSERT INTO sessions (id, admin_id, token, ip_address, user_agent,
           created_at, expires_at, revoked)
           VALUES (?, ?, ?, ?, ?, ?, ?, 0)"#,
    )
    .bind(&id)
    .bind(&admin_id.to_string())
    .bind(token)
    .bind(ip_address)
    .bind(user_agent)
    .bind(&now.to_rfc3339())
    .bind(&expires.to_rfc3339())
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(Session {
        id: Uuid::parse_str(&id).unwrap(),
        admin_id: *admin_id,
        token: token.to_string(),
        ip_address: ip_address.to_string(),
        user_agent: user_agent.to_string(),
        created_at: now,
        expires_at: expires,
        revoked: false,
    })
}

pub async fn get_session_by_token(
    pool: &SqlitePool,
    token: &str,
) -> Result<Option<Session>> {
    let row = sqlx::query_as::<_, SessionRow>(
        r#"SELECT id, admin_id, token, ip_address, user_agent,
           created_at, expires_at, revoked
           FROM sessions WHERE token = ?"#,
    )
    .bind(token)
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(|r| r.into()))
}

pub async fn revoke_session(pool: &SqlitePool, token: &str) -> Result<()> {
    sqlx::query("UPDATE sessions SET revoked = 1 WHERE token = ?")
        .bind(token)
        .execute(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn revoke_all_sessions(pool: &SqlitePool, admin_id: &Uuid) -> Result<()> {
    sqlx::query("UPDATE sessions SET revoked = 1 WHERE admin_id = ?")
        .bind(&admin_id.to_string())
        .execute(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn insert_node(
    pool: &SqlitePool,
    node: &Node,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO nodes (id, display_name, overlay_ipv4, overlay_ipv6,
           wg_public_key, wg_private_key_encrypted,
           identity_public_key, identity_private_key_encrypted,
           control_parent_id, node_service_port,
           wg_port_range_start, wg_port_range_end, is_controller,
           agent_version, protocol_version,
           desired_generation, active_generation,
           created_at, updated_at, last_seen_at, last_handshake_at,
           enrollment_token_id)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&node.id.to_string())
    .bind(&node.display_name)
    .bind(&node.overlay_ipv4.to_string())
    .bind(&node.overlay_ipv6)
    .bind(&node.wg_public_key)
    .bind(&node.wg_private_key_encrypted)
    .bind(&node.identity_public_key)
    .bind(&node.identity_private_key_encrypted)
    .bind(&node.control_parent_id.map(|id| id.to_string()))
    .bind(node.node_service_port as i64)
    .bind(node.wg_port_range_start as i64)
    .bind(node.wg_port_range_end as i64)
    .bind(node.is_controller as i64)
    .bind(&node.agent_version)
    .bind(node.protocol_version as i64)
    .bind(node.desired_generation.map(|g| g as i64))
    .bind(node.active_generation.map(|g| g as i64))
    .bind(&now)
    .bind(&now)
    .bind(&node.last_seen_at.map(|t| t.to_rfc3339()))
    .bind(&node.last_handshake_at.map(|t| t.to_rfc3339()))
    .bind(&node.enrollment_token_id.map(|id| id.to_string()))
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_node_by_id(
    pool: &SqlitePool,
    node_id: &NodeId,
) -> Result<Option<Node>> {
    let row = sqlx::query_as::<_, NodeRow>(
        "SELECT * FROM nodes WHERE id = ?",
    )
    .bind(&node_id.to_string())
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(|r| node_from_row(r)))
}

pub async fn list_all_nodes(pool: &SqlitePool) -> Result<Vec<Node>> {
    let rows = sqlx::query_as::<_, NodeRow>("SELECT * FROM nodes ORDER BY created_at")
        .fetch_all(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(node_from_row).collect())
}

pub async fn update_node_display_name(
    pool: &SqlitePool,
    node_id: &NodeId,
    display_name: &str,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    sqlx::query("UPDATE nodes SET display_name = ?, updated_at = ? WHERE id = ?")
        .bind(display_name)
        .bind(&now)
        .bind(&node_id.to_string())
        .execute(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn insert_wireguard_link(
    pool: &SqlitePool,
    link: &WireGuardLink,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO wireguard_links (id, node_a, node_b, initiator_node_id,
           listener_node_id, listener_address, listener_port,
           interface_name_a, interface_name_b, enabled, admin_weight,
           last_handshake_a, last_handshake_b, status, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&link.id.to_string())
    .bind(&link.node_a.to_string())
    .bind(&link.node_b.to_string())
    .bind(&link.initiator_node_id.to_string())
    .bind(&link.listener_node_id.to_string())
    .bind(&link.listener_address)
    .bind(link.listener_port as i64)
    .bind(&link.interface_name_a)
    .bind(&link.interface_name_b)
    .bind(link.enabled as i64)
    .bind(link.admin_weight as i64)
    .bind(&link.last_handshake_a.map(|t| t.to_rfc3339()))
    .bind(&link.last_handshake_b.map(|t| t.to_rfc3339()))
    .bind(link_status_str(&link.status))
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn insert_enrollment_token(
    pool: &SqlitePool,
    token: &EnrollmentToken,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO enrollment_tokens (id, token, network_id, parent_node_id,
           suggested_node_name, allowed_install_mode, expires_at, used_at, revoked, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&token.id.to_string())
    .bind(&token.token)
    .bind(&token.network_id.to_string())
    .bind(&token.parent_node_id.to_string())
    .bind(&token.suggested_node_name)
    .bind(&token.allowed_install_mode)
    .bind(&token.expires_at.to_rfc3339())
    .bind(&token.used_at.map(|t| t.to_rfc3339()))
    .bind(token.revoked as i64)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn mark_token_used(
    pool: &SqlitePool,
    token: &str,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    sqlx::query(
        "UPDATE enrollment_tokens SET used_at = ? WHERE token = ?",
    )
    .bind(&now)
    .bind(token)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn insert_audit_log(
    pool: &SqlitePool,
    log: &AuditLog,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO audit_logs (id, admin_id, action, resource_type, resource_id, details, ip_address, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&log.id.to_string())
    .bind(&log.admin_id.map(|id| id.to_string()))
    .bind(&log.action)
    .bind(&log.resource_type)
    .bind(&log.resource_id)
    .bind(&log.details)
    .bind(&log.ip_address)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

fn link_status_str(s: &LinkStatus) -> &'static str {
    match s {
        LinkStatus::Pending => "Pending",
        LinkStatus::Prepared => "Prepared",
        LinkStatus::Active => "Active",
        LinkStatus::Failed => "Failed",
        LinkStatus::Disabled => "Disabled",
    }
}

#[derive(sqlx::FromRow)]
struct AdminRow {
    id: String,
    username: String,
    password_hash: String,
    created_at: String,
    updated_at: String,
}

impl From<AdminRow> for Admin {
    fn from(r: AdminRow) -> Self {
        Self {
            id: Uuid::parse_str(&r.id).unwrap(),
            username: r.username,
            password_hash: r.password_hash,
            created_at: chrono::DateTime::parse_from_rfc3339(&r.created_at).unwrap().with_timezone(&Utc),
            updated_at: chrono::DateTime::parse_from_rfc3339(&r.updated_at).unwrap().with_timezone(&Utc),
        }
    }
}

#[derive(sqlx::FromRow)]
struct SessionRow {
    id: String,
    admin_id: String,
    token: String,
    ip_address: String,
    user_agent: String,
    created_at: String,
    expires_at: String,
    revoked: i64,
}

impl From<SessionRow> for Session {
    fn from(r: SessionRow) -> Self {
        Self {
            id: Uuid::parse_str(&r.id).unwrap(),
            admin_id: Uuid::parse_str(&r.admin_id).unwrap(),
            token: r.token,
            ip_address: r.ip_address,
            user_agent: r.user_agent,
            created_at: chrono::DateTime::parse_from_rfc3339(&r.created_at).unwrap().with_timezone(&Utc),
            expires_at: chrono::DateTime::parse_from_rfc3339(&r.expires_at).unwrap().with_timezone(&Utc),
            revoked: r.revoked != 0,
        }
    }
}

#[derive(sqlx::FromRow)]
#[allow(dead_code)]
struct NodeRow {
    id: String,
    display_name: String,
    overlay_ipv4: String,
    overlay_ipv6: Option<String>,
    wg_public_key: String,
    wg_private_key_encrypted: String,
    identity_public_key: String,
    identity_private_key_encrypted: String,
    control_parent_id: Option<String>,
    node_service_port: i64,
    wg_port_range_start: i64,
    wg_port_range_end: i64,
    is_controller: i64,
    agent_version: Option<String>,
    protocol_version: i64,
    desired_generation: Option<i64>,
    active_generation: Option<i64>,
    created_at: String,
    updated_at: String,
    last_seen_at: Option<String>,
    last_handshake_at: Option<String>,
    enrollment_token_id: Option<String>,
}

fn parse_optional_datetime(s: &Option<String>) -> Option<chrono::DateTime<Utc>> {
    s.as_ref().and_then(|s| chrono::DateTime::parse_from_rfc3339(s).ok().map(|dt| dt.with_timezone(&Utc)))
}

fn parse_datetime(s: &str) -> chrono::DateTime<Utc> {
    chrono::DateTime::parse_from_rfc3339(s).unwrap().with_timezone(&Utc)
}

fn node_from_row(r: NodeRow) -> Node {
    Node {
        id: Uuid::parse_str(&r.id).unwrap(),
        display_name: r.display_name,
        overlay_ipv4: r.overlay_ipv4.parse().unwrap(),
        overlay_ipv6: r.overlay_ipv6,
        wg_public_key: r.wg_public_key,
        wg_private_key_encrypted: r.wg_private_key_encrypted,
        identity_public_key: r.identity_public_key,
        identity_private_key_encrypted: r.identity_private_key_encrypted,
        control_parent_id: r.control_parent_id.map(|id| Uuid::parse_str(&id).unwrap()),
        node_service_port: r.node_service_port as u16,
        wg_port_range_start: r.wg_port_range_start as u16,
        wg_port_range_end: r.wg_port_range_end as u16,
        is_controller: r.is_controller != 0,
        agent_version: r.agent_version,
        protocol_version: r.protocol_version as u32,
        desired_generation: r.desired_generation.map(|g| g as u64),
        active_generation: r.active_generation.map(|g| g as u64),
        created_at: parse_datetime(&r.created_at),
        updated_at: parse_datetime(&r.updated_at),
        last_seen_at: parse_optional_datetime(&r.last_seen_at),
        last_handshake_at: parse_optional_datetime(&r.last_handshake_at),
        enrollment_token_id: r.enrollment_token_id.map(|id| Uuid::parse_str(&id).unwrap()),
    }
}

pub async fn get_enrollment_token(
    pool: &SqlitePool,
    token: &str,
) -> Result<Option<EnrollmentToken>> {
    let row = sqlx::query_as::<_, TokenRow>(
        r#"SELECT id, token, network_id, parent_node_id,
           suggested_node_name, allowed_install_mode,
           expires_at, used_at, revoked, created_at
           FROM enrollment_tokens WHERE token = ?"#,
    )
    .bind(token)
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(token_from_row))
}

pub async fn revoke_token(pool: &SqlitePool, token: &str) -> Result<()> {
    sqlx::query("UPDATE enrollment_tokens SET revoked = 1 WHERE token = ?")
        .bind(token)
        .execute(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn get_node_by_display_name(
    pool: &SqlitePool,
    name: &str,
) -> Result<Option<Node>> {
    let row = sqlx::query_as::<_, NodeRow>(
        "SELECT * FROM nodes WHERE display_name = ?",
    )
    .bind(name)
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(node_from_row))
}

pub async fn update_node_heartbeat(
    pool: &SqlitePool,
    node_id: &NodeId,
    agent_version: &str,
    protocol_version: u32,
    active_generation: Option<u64>,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    sqlx::query(
        r#"UPDATE nodes SET agent_version = ?, protocol_version = ?,
           active_generation = ?, last_seen_at = ? WHERE id = ?"#,
    )
    .bind(agent_version)
    .bind(protocol_version as i64)
    .bind(active_generation.map(|g| g as i64))
    .bind(&now)
    .bind(&node_id.to_string())
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn insert_control_relation(
    pool: &SqlitePool,
    relation: &ControlRelation,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO control_relations (id, parent_id, child_id, enrolled_at, enrollment_token_id)
           VALUES (?, ?, ?, ?, ?)"#,
    )
    .bind(&relation.id.to_string())
    .bind(&relation.parent_id.to_string())
    .bind(&relation.child_id.to_string())
    .bind(&now)
    .bind(&relation.enrollment_token_id.to_string())
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_children_of_node(
    pool: &SqlitePool,
    parent_id: &NodeId,
) -> Result<Vec<NodeId>> {
    let rows = sqlx::query_as::<_, (String,)>(
        "SELECT child_id FROM control_relations WHERE parent_id = ?",
    )
    .bind(&parent_id.to_string())
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(|(id,)| Uuid::parse_str(&id).unwrap()).collect())
}

pub async fn get_control_ancestors(
    pool: &SqlitePool,
    node_id: &NodeId,
) -> Result<Vec<NodeId>> {
    let mut ancestors = Vec::new();
    let mut current = *node_id;

    loop {
        let row = sqlx::query_as::<_, (Option<String>,)>(
            "SELECT control_parent_id FROM nodes WHERE id = ?",
        )
        .bind(&current.to_string())
        .fetch_optional(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

        match row {
            Some((Some(parent_id),)) => {
                let pid = Uuid::parse_str(&parent_id).unwrap();
                ancestors.push(pid);
                current = pid;
            }
            _ => break,
        }
    }

    Ok(ancestors)
}

pub async fn insert_artifact(
    pool: &SqlitePool,
    artifact: &Artifact,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO artifacts (id, name, version, architecture,
           sha256, size_bytes, manifest_id, created_at)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&artifact.id.to_string())
    .bind(&artifact.name)
    .bind(&artifact.version)
    .bind(&artifact.architecture)
    .bind(&artifact.sha256)
    .bind(artifact.size_bytes as i64)
    .bind(&artifact.manifest_id.map(|id| id.to_string()))
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_artifacts_by_version(
    pool: &SqlitePool,
    version: &str,
) -> Result<Vec<Artifact>> {
    let rows = sqlx::query_as::<_, ArtifactRow>(
        r#"SELECT id, name, version, architecture, sha256, size_bytes, manifest_id, created_at
           FROM artifacts WHERE version = ?"#,
    )
    .bind(version)
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(artifact_from_row).collect())
}

pub async fn create_network(
    pool: &SqlitePool,
    network: &Network,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO networks (id, name, overlay_ipv4_cidr, overlay_ipv6_cidr,
           ipv6_enabled, created_at)
           VALUES (?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&network.id.to_string())
    .bind(&network.name)
    .bind(&network.overlay_ipv4_cidr)
    .bind(&network.overlay_ipv6_cidr)
    .bind(network.ipv6_enabled as i64)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_network(
    pool: &SqlitePool,
    network_id: &NetworkId,
) -> Result<Option<Network>> {
    let row = sqlx::query_as::<_, NetworkRow>(
        r#"SELECT id, name, overlay_ipv4_cidr, overlay_ipv6_cidr,
           ipv6_enabled, created_at FROM networks WHERE id = ?"#,
    )
    .bind(&network_id.to_string())
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(network_from_row))
}

pub async fn insert_heartbeat(
    pool: &SqlitePool,
    hb: &Heartbeat,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    let snapshots_json = serde_json::to_string(&hb.wireguard_snapshots)
        .map_err(|e| PathWeaverError::SerializationError(e.to_string()))?;

    sqlx::query(
        r#"INSERT INTO heartbeats (id, node_id, agent_version, protocol_version,
           active_generation, wireguard_snapshots, received_at)
           VALUES (?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&hb.id.to_string())
    .bind(&hb.node_id.to_string())
    .bind(&hb.agent_version)
    .bind(hb.protocol_version as i64)
    .bind(hb.active_generation.map(|g| g as i64))
    .bind(&snapshots_json)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn insert_config_revision(
    pool: &SqlitePool,
    rev: &ConfigRevision,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO config_revisions (id, generation, reason, status, created_at, completed_at)
           VALUES (?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&rev.id.to_string())
    .bind(rev.generation as i64)
    .bind(&rev.reason)
    .bind(config_revision_status_str(&rev.status))
    .bind(&now)
    .bind(&rev.completed_at.map(|t| t.to_rfc3339()))
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

fn config_revision_status_str(s: &ConfigRevisionStatus) -> &'static str {
    match s {
        ConfigRevisionStatus::Draft => "Draft",
        ConfigRevisionStatus::Pending => "Pending",
        ConfigRevisionStatus::Dispatching => "Dispatching",
        ConfigRevisionStatus::DispatchingCompleted => "DispatchingCompleted",
        ConfigRevisionStatus::RolledBack => "RolledBack",
        ConfigRevisionStatus::Failed => "Failed",
    }
}

#[derive(sqlx::FromRow)]
struct TokenRow {
    id: String,
    token: String,
    network_id: String,
    parent_node_id: String,
    suggested_node_name: String,
    allowed_install_mode: String,
    expires_at: String,
    used_at: Option<String>,
    revoked: i64,
    created_at: String,
}

fn token_from_row(r: TokenRow) -> EnrollmentToken {
    EnrollmentToken {
        id: Uuid::parse_str(&r.id).unwrap(),
        token: r.token,
        network_id: Uuid::parse_str(&r.network_id).unwrap(),
        parent_node_id: Uuid::parse_str(&r.parent_node_id).unwrap(),
        suggested_node_name: r.suggested_node_name,
        allowed_install_mode: r.allowed_install_mode,
        expires_at: parse_datetime(&r.expires_at),
        used_at: parse_optional_datetime(&r.used_at),
        revoked: r.revoked != 0,
        created_at: parse_datetime(&r.created_at),
    }
}

#[derive(sqlx::FromRow)]
struct ArtifactRow {
    id: String,
    name: String,
    version: String,
    architecture: String,
    sha256: String,
    size_bytes: i64,
    manifest_id: Option<String>,
    created_at: String,
}

fn artifact_from_row(r: ArtifactRow) -> Artifact {
    Artifact {
        id: Uuid::parse_str(&r.id).unwrap(),
        name: r.name,
        version: r.version,
        architecture: r.architecture,
        sha256: r.sha256,
        size_bytes: r.size_bytes as u64,
        manifest_id: r.manifest_id.map(|id| Uuid::parse_str(&id).unwrap()),
        created_at: parse_datetime(&r.created_at),
    }
}

#[derive(sqlx::FromRow)]
struct NetworkRow {
    id: String,
    name: String,
    overlay_ipv4_cidr: String,
    overlay_ipv6_cidr: Option<String>,
    ipv6_enabled: i64,
    created_at: String,
}

fn network_from_row(r: NetworkRow) -> Network {
    Network {
        id: Uuid::parse_str(&r.id).unwrap(),
        name: r.name,
        overlay_ipv4_cidr: r.overlay_ipv4_cidr,
        overlay_ipv6_cidr: r.overlay_ipv6_cidr,
        ipv6_enabled: r.ipv6_enabled != 0,
        created_at: parse_datetime(&r.created_at),
    }
}

pub async fn insert_traffic_policy(
    pool: &SqlitePool,
    policy: &TrafficPolicy,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO traffic_policies (id, name, priority, enabled, description, created_at, updated_at)
           VALUES (?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&policy.id.to_string())
    .bind(&policy.name)
    .bind(policy.priority as i64)
    .bind(policy.enabled as i64)
    .bind(&policy.description)
    .bind(&now)
    .bind(&now)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn list_traffic_policies(pool: &SqlitePool) -> Result<Vec<TrafficPolicy>> {
    let rows = sqlx::query_as::<_, PolicyRow>(
        r#"SELECT id, name, priority, enabled, description, created_at, updated_at
           FROM traffic_policies ORDER BY priority"#,
    )
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(policy_from_row).collect())
}

pub async fn insert_policy_match(
    pool: &SqlitePool,
    m: &TrafficPolicyMatch,
) -> Result<()> {
    sqlx::query(
        r#"INSERT INTO traffic_policy_matches (id, policy_id, source_node_id, source_node_group,
           source_cidr, destination_node_id, destination_cidr, protocol,
           destination_port_start, destination_port_end)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&m.id.to_string())
    .bind(&m.policy_id.to_string())
    .bind(&m.source_node_id.map(|id| id.to_string()))
    .bind(&m.source_node_group)
    .bind(&m.source_cidr)
    .bind(&m.destination_node_id.map(|id| id.to_string()))
    .bind(&m.destination_cidr)
    .bind(match_protocol_db(&m.protocol))
    .bind(m.destination_port_start.map(|p| p as i64))
    .bind(m.destination_port_end.map(|p| p as i64))
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_policy_matches(
    pool: &SqlitePool,
    policy_id: &PolicyId,
) -> Result<Vec<TrafficPolicyMatch>> {
    let rows = sqlx::query_as::<_, MatchRow>(
        r#"SELECT id, policy_id, source_node_id, source_node_group,
           source_cidr, destination_node_id, destination_cidr, protocol,
           destination_port_start, destination_port_end
           FROM traffic_policy_matches WHERE policy_id = ?"#,
    )
    .bind(&policy_id.to_string())
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(match_from_row).collect())
}

pub async fn insert_policy_path(
    pool: &SqlitePool,
    path: &TrafficPolicyPath,
) -> Result<()> {
    sqlx::query(
        r#"INSERT INTO traffic_policy_paths (id, policy_id, hop_order, node_id)
           VALUES (?, ?, ?, ?)"#,
    )
    .bind(&path.id.to_string())
    .bind(&path.policy_id.to_string())
    .bind(path.hop_order as i64)
    .bind(&path.node_id.to_string())
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_policy_paths(
    pool: &SqlitePool,
    policy_id: &PolicyId,
) -> Result<Vec<TrafficPolicyPath>> {
    let rows = sqlx::query_as::<_, PathRow>(
        r#"SELECT id, policy_id, hop_order, node_id
           FROM traffic_policy_paths WHERE policy_id = ? ORDER BY hop_order"#,
    )
    .bind(&policy_id.to_string())
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(path_from_row).collect())
}

pub async fn insert_policy_config(
    pool: &SqlitePool,
    config: &TrafficPolicyConfig,
) -> Result<()> {
    sqlx::query(
        r#"INSERT INTO traffic_policy_configs (id, policy_id, egress_node_id,
           egress_nat, return_path_type, failover_enabled, failover_path_id)
           VALUES (?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&config.id.to_string())
    .bind(&config.policy_id.to_string())
    .bind(&config.egress_node_id.map(|id| id.to_string()))
    .bind(config.egress_nat as i64)
    .bind(return_path_str(&config.return_path_type))
    .bind(config.failover_enabled as i64)
    .bind(&config.failover_path_id.map(|id| id.to_string()))
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_policy_config(
    pool: &SqlitePool,
    policy_id: &PolicyId,
) -> Result<Option<TrafficPolicyConfig>> {
    let row = sqlx::query_as::<_, ConfigRow>(
        r#"SELECT id, policy_id, egress_node_id, egress_nat,
           return_path_type, failover_enabled, failover_path_id
           FROM traffic_policy_configs WHERE policy_id = ?"#,
    )
    .bind(&policy_id.to_string())
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(config_from_row))
}

pub async fn list_wireguard_links(pool: &SqlitePool) -> Result<Vec<WireGuardLink>> {
    let rows = sqlx::query_as::<_, LinkRow>(
        r#"SELECT id, node_a, node_b, initiator_node_id, listener_node_id,
           listener_address, listener_port, interface_name_a, interface_name_b,
           enabled, admin_weight, last_handshake_a, last_handshake_b, status, created_at
           FROM wireguard_links"#,
    )
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(link_from_row).collect())
}

fn match_protocol_db(p: &MatchProtocol) -> &'static str {
    match p {
        MatchProtocol::Any => "Any",
        MatchProtocol::Tcp => "TCP",
        MatchProtocol::Udp => "UDP",
        MatchProtocol::Icmp => "ICMP",
    }
}

fn return_path_str(r: &ReturnPathType) -> &'static str {
    match r {
        ReturnPathType::Symmetric => "Symmetric",
        ReturnPathType::Independent => "Independent",
    }
}
#[derive(sqlx::FromRow)]
struct PolicyRow {
    id: String,
    name: String,
    priority: i64,
    enabled: i64,
    description: Option<String>,
    created_at: String,
    updated_at: String,
}

fn policy_from_row(r: PolicyRow) -> TrafficPolicy {
    TrafficPolicy {
        id: Uuid::parse_str(&r.id).unwrap(),
        name: r.name,
        priority: r.priority as u32,
        enabled: r.enabled != 0,
        description: r.description,
        created_at: parse_datetime(&r.created_at),
        updated_at: parse_datetime(&r.updated_at),
    }
}

#[derive(sqlx::FromRow)]
struct MatchRow {
    id: String,
    policy_id: String,
    source_node_id: Option<String>,
    source_node_group: Option<String>,
    source_cidr: Option<String>,
    destination_node_id: Option<String>,
    destination_cidr: Option<String>,
    protocol: String,
    destination_port_start: Option<i64>,
    destination_port_end: Option<i64>,
}

fn match_from_row(r: MatchRow) -> TrafficPolicyMatch {
    TrafficPolicyMatch {
        id: Uuid::parse_str(&r.id).unwrap(),
        policy_id: Uuid::parse_str(&r.policy_id).unwrap(),
        source_node_id: r.source_node_id.map(|id| Uuid::parse_str(&id).unwrap()),
        source_node_group: r.source_node_group,
        source_cidr: r.source_cidr,
        destination_node_id: r.destination_node_id.map(|id| Uuid::parse_str(&id).unwrap()),
        destination_cidr: r.destination_cidr,
        protocol: match r.protocol.as_str() {
            "TCP" => MatchProtocol::Tcp,
            "UDP" => MatchProtocol::Udp,
            "ICMP" => MatchProtocol::Icmp,
            _ => MatchProtocol::Any,
        },
        destination_port_start: r.destination_port_start.map(|p| p as u16),
        destination_port_end: r.destination_port_end.map(|p| p as u16),
    }
}

#[derive(sqlx::FromRow)]
struct PathRow {
    id: String,
    policy_id: String,
    hop_order: i64,
    node_id: String,
}

fn path_from_row(r: PathRow) -> TrafficPolicyPath {
    TrafficPolicyPath {
        id: Uuid::parse_str(&r.id).unwrap(),
        policy_id: Uuid::parse_str(&r.policy_id).unwrap(),
        hop_order: r.hop_order as u32,
        node_id: Uuid::parse_str(&r.node_id).unwrap(),
    }
}

#[derive(sqlx::FromRow)]
struct ConfigRow {
    id: String,
    policy_id: String,
    egress_node_id: Option<String>,
    egress_nat: i64,
    return_path_type: String,
    failover_enabled: i64,
    failover_path_id: Option<String>,
}

fn config_from_row(r: ConfigRow) -> TrafficPolicyConfig {
    TrafficPolicyConfig {
        id: Uuid::parse_str(&r.id).unwrap(),
        policy_id: Uuid::parse_str(&r.policy_id).unwrap(),
        egress_node_id: r.egress_node_id.map(|id| Uuid::parse_str(&id).unwrap()),
        egress_nat: r.egress_nat != 0,
        return_path_type: match r.return_path_type.as_str() {
            "Independent" => ReturnPathType::Independent,
            _ => ReturnPathType::Symmetric,
        },
        failover_enabled: r.failover_enabled != 0,
        failover_path_id: r.failover_path_id.map(|id| Uuid::parse_str(&id).unwrap()),
    }
}

#[derive(sqlx::FromRow)]
struct LinkRow {
    id: String,
    node_a: String,
    node_b: String,
    initiator_node_id: String,
    listener_node_id: String,
    listener_address: String,
    listener_port: i64,
    interface_name_a: String,
    interface_name_b: String,
    enabled: i64,
    admin_weight: i64,
    last_handshake_a: Option<String>,
    last_handshake_b: Option<String>,
    status: String,
    created_at: String,
}

fn link_from_row(r: LinkRow) -> WireGuardLink {
    WireGuardLink {
        id: Uuid::parse_str(&r.id).unwrap(),
        node_a: Uuid::parse_str(&r.node_a).unwrap(),
        node_b: Uuid::parse_str(&r.node_b).unwrap(),
        initiator_node_id: Uuid::parse_str(&r.initiator_node_id).unwrap(),
        listener_node_id: Uuid::parse_str(&r.listener_node_id).unwrap(),
        listener_address: r.listener_address,
        listener_port: r.listener_port as u16,
        interface_name_a: r.interface_name_a,
        interface_name_b: r.interface_name_b,
        enabled: r.enabled != 0,
        admin_weight: r.admin_weight as u32,
        last_handshake_a: parse_optional_datetime(&r.last_handshake_a),
        last_handshake_b: parse_optional_datetime(&r.last_handshake_b),
        status: match r.status.as_str() {
            "Prepared" => LinkStatus::Prepared,
            "Active" => LinkStatus::Active,
            "Failed" => LinkStatus::Failed,
            "Disabled" => LinkStatus::Disabled,
            _ => LinkStatus::Pending,
        },
        created_at: parse_datetime(&r.created_at),
    }
}

pub async fn insert_update_job(
    pool: &SqlitePool,
    job: &UpdateJob,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();

    sqlx::query(
        r#"INSERT INTO update_jobs (id, target_version, manifest_sha256, status, created_at, completed_at)
           VALUES (?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&job.id.to_string())
    .bind(&job.target_version)
    .bind(&job.manifest_sha256)
    .bind(update_job_status_str(&job.status))
    .bind(&now)
    .bind(&job.completed_at.map(|t| t.to_rfc3339()))
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_update_job(
    pool: &SqlitePool,
    job_id: &UpdateJobId,
) -> Result<Option<UpdateJob>> {
    let row = sqlx::query_as::<_, UpdateJobRow>(
        r#"SELECT id, target_version, manifest_sha256, status, created_at, completed_at
           FROM update_jobs WHERE id = ?"#,
    )
    .bind(&job_id.to_string())
    .fetch_optional(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(row.map(update_job_from_row))
}

pub async fn list_update_jobs(pool: &SqlitePool) -> Result<Vec<UpdateJob>> {
    let rows = sqlx::query_as::<_, UpdateJobRow>(
        "SELECT id, target_version, manifest_sha256, status, created_at, completed_at
         FROM update_jobs ORDER BY created_at DESC",
    )
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(update_job_from_row).collect())
}

pub async fn update_job_status(
    pool: &SqlitePool,
    job_id: &UpdateJobId,
    status: &UpdateJobStatus,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    sqlx::query("UPDATE update_jobs SET status = ?, completed_at = ? WHERE id = ?")
        .bind(update_job_status_str(status))
        .bind(&now)
        .bind(&job_id.to_string())
        .execute(pool)
        .await
        .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn insert_update_target(
    pool: &SqlitePool,
    target: &UpdateTarget,
) -> Result<()> {
    sqlx::query(
        r#"INSERT INTO update_targets (id, update_job_id, node_id, depth, status, started_at, completed_at, error_message)
           VALUES (?, ?, ?, ?, ?, ?, ?, ?)"#,
    )
    .bind(&target.id.to_string())
    .bind(&target.update_job_id.to_string())
    .bind(&target.node_id.to_string())
    .bind(target.depth as i64)
    .bind(update_target_status_str(&target.status))
    .bind(&target.started_at.map(|t| t.to_rfc3339()))
    .bind(&target.completed_at.map(|t| t.to_rfc3339()))
    .bind(&target.error_message)
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(())
}

pub async fn get_update_targets(
    pool: &SqlitePool,
    job_id: &UpdateJobId,
) -> Result<Vec<UpdateTarget>> {
    let rows = sqlx::query_as::<_, UpdateTargetRow>(
        r#"SELECT id, update_job_id, node_id, depth, status, started_at, completed_at, error_message
           FROM update_targets WHERE update_job_id = ? ORDER BY depth DESC"#,
    )
    .bind(&job_id.to_string())
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    Ok(rows.into_iter().map(update_target_from_row).collect())
}

pub async fn update_target_status(
    pool: &SqlitePool,
    target_id: &Uuid,
    status: &UpdateTargetStatus,
    error_message: Option<&str>,
) -> Result<()> {
    let now = Utc::now().to_rfc3339();
    sqlx::query(
        r#"UPDATE update_targets SET status = ?, error_message = ?,
           completed_at = CASE WHEN ? IN ('Completed','DownloadFailed','SignatureInvalid','HealthCheckFailed','RolledBack')
           THEN ? ELSE completed_at END WHERE id = ?"#,
    )
    .bind(update_target_status_str(status))
    .bind(error_message)
    .bind(update_target_status_str(status))
    .bind(&now)
    .bind(&target_id.to_string())
    .execute(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;
    Ok(())
}

pub async fn get_nodes_by_depth(
    pool: &SqlitePool,
) -> Result<Vec<(NodeId, u32)>> {
    let nodes = list_all_nodes(pool).await?;
    let control_rels: Vec<(String, String)> = sqlx::query_as::<_, (String, String)>(
        "SELECT parent_id, child_id FROM control_relations",
    )
    .fetch_all(pool)
    .await
    .map_err(|e| PathWeaverError::DatabaseError(e.to_string()))?;

    let mut children: std::collections::HashMap<NodeId, Vec<NodeId>> = std::collections::HashMap::new();
    let mut has_parent: std::collections::HashSet<NodeId> = std::collections::HashSet::new();

    for (parent, child) in &control_rels {
        let pid = Uuid::parse_str(parent).unwrap();
        let cid = Uuid::parse_str(child).unwrap();
        children.entry(pid).or_default().push(cid);
        has_parent.insert(cid);
    }

    let roots: Vec<NodeId> = nodes.iter()
        .filter(|n| !has_parent.contains(&n.id))
        .map(|n| n.id)
        .collect();

    let mut depths = std::collections::HashMap::new();
    for root in roots {
        assign_depth(root, 0, &children, &mut depths);
    }

    for node in &nodes {
        depths.entry(node.id).or_insert(0);
    }

    let mut result: Vec<_> = depths.into_iter().collect();
    result.sort_by_key(|(_, d)| std::cmp::Reverse(*d));
    Ok(result)
}

fn assign_depth(
    node_id: NodeId,
    depth: u32,
    children: &std::collections::HashMap<NodeId, Vec<NodeId>>,
    depths: &mut std::collections::HashMap<NodeId, u32>,
) {
    depths.insert(node_id, depth);
    if let Some(kids) = children.get(&node_id) {
        for kid in kids {
            assign_depth(*kid, depth + 1, children, depths);
        }
    }
}

fn update_job_status_str(s: &UpdateJobStatus) -> &'static str {
    match s {
        UpdateJobStatus::Created => "Created",
        UpdateJobStatus::Downloading => "Downloading",
        UpdateJobStatus::DistributingArtifacts => "DistributingArtifacts",
        UpdateJobStatus::Installing => "Installing",
        UpdateJobStatus::Completed => "Completed",
        UpdateJobStatus::Failed => "Failed",
        UpdateJobStatus::RollingBack => "RollingBack",
        UpdateJobStatus::RolledBack => "RolledBack",
    }
}

fn update_target_status_str(s: &UpdateTargetStatus) -> &'static str {
    match s {
        UpdateTargetStatus::Waiting => "Waiting",
        UpdateTargetStatus::Prefetching => "Prefetching",
        UpdateTargetStatus::Verifying => "Verifying",
        UpdateTargetStatus::Staged => "Staged",
        UpdateTargetStatus::Installing => "Installing",
        UpdateTargetStatus::Restarting => "Restarting",
        UpdateTargetStatus::HealthChecking => "HealthChecking",
        UpdateTargetStatus::Completed => "Completed",
        UpdateTargetStatus::DownloadFailed => "DownloadFailed",
        UpdateTargetStatus::SignatureInvalid => "SignatureInvalid",
        UpdateTargetStatus::InstallFailed => "InstallFailed",
        UpdateTargetStatus::HealthCheckFailed => "HealthCheckFailed",
        UpdateTargetStatus::RollingBack => "RollingBack",
        UpdateTargetStatus::RolledBack => "RolledBack",
    }
}

#[derive(sqlx::FromRow)]
struct UpdateJobRow {
    id: String,
    target_version: String,
    manifest_sha256: String,
    status: String,
    created_at: String,
    completed_at: Option<String>,
}

fn update_job_from_row(r: UpdateJobRow) -> UpdateJob {
    UpdateJob {
        id: Uuid::parse_str(&r.id).unwrap(),
        target_version: r.target_version,
        manifest_sha256: r.manifest_sha256,
        status: match r.status.as_str() {
            "Downloading" => UpdateJobStatus::Downloading,
            "DistributingArtifacts" => UpdateJobStatus::DistributingArtifacts,
            "Installing" => UpdateJobStatus::Installing,
            "Completed" => UpdateJobStatus::Completed,
            "Failed" => UpdateJobStatus::Failed,
            "RollingBack" => UpdateJobStatus::RollingBack,
            "RolledBack" => UpdateJobStatus::RolledBack,
            _ => UpdateJobStatus::Created,
        },
        created_at: parse_datetime(&r.created_at),
        completed_at: parse_optional_datetime(&r.completed_at),
    }
}

#[derive(sqlx::FromRow)]
struct UpdateTargetRow {
    id: String,
    update_job_id: String,
    node_id: String,
    depth: i64,
    status: String,
    started_at: Option<String>,
    completed_at: Option<String>,
    error_message: Option<String>,
}

fn update_target_from_row(r: UpdateTargetRow) -> UpdateTarget {
    UpdateTarget {
        id: Uuid::parse_str(&r.id).unwrap(),
        update_job_id: Uuid::parse_str(&r.update_job_id).unwrap(),
        node_id: Uuid::parse_str(&r.node_id).unwrap(),
        depth: r.depth as u32,
        status: match r.status.as_str() {
            "Prefetching" => UpdateTargetStatus::Prefetching,
            "Verifying" => UpdateTargetStatus::Verifying,
            "Staged" => UpdateTargetStatus::Staged,
            "Installing" => UpdateTargetStatus::Installing,
            "Restarting" => UpdateTargetStatus::Restarting,
            "HealthChecking" => UpdateTargetStatus::HealthChecking,
            "Completed" => UpdateTargetStatus::Completed,
            "DownloadFailed" => UpdateTargetStatus::DownloadFailed,
            "SignatureInvalid" => UpdateTargetStatus::SignatureInvalid,
            "InstallFailed" => UpdateTargetStatus::InstallFailed,
            "HealthCheckFailed" => UpdateTargetStatus::HealthCheckFailed,
            "RollingBack" => UpdateTargetStatus::RollingBack,
            "RolledBack" => UpdateTargetStatus::RolledBack,
            _ => UpdateTargetStatus::Waiting,
        },
        started_at: parse_optional_datetime(&r.started_at),
        completed_at: parse_optional_datetime(&r.completed_at),
        error_message: r.error_message,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_create_and_get_admin() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let admin = create_admin(db.pool(), "admin", "hashed_password")
            .await
            .unwrap();
        assert_eq!(admin.username, "admin");

        let fetched = get_admin_by_username(db.pool(), "admin")
            .await
            .unwrap()
            .unwrap();
        assert_eq!(fetched.password_hash, "hashed_password");
    }

    #[tokio::test]
    async fn test_session_lifecycle() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let admin = create_admin(db.pool(), "admin", "hash").await.unwrap();

        let session = create_session(
            db.pool(), &admin.id, "session_token",
            "127.0.0.1", "test-agent", 24,
        )
        .await
        .unwrap();

        assert_eq!(session.token, "session_token");
        assert!(!session.revoked);

        revoke_session(db.pool(), "session_token").await.unwrap();

        let revoked = get_session_by_token(db.pool(), "session_token")
            .await
            .unwrap()
            .unwrap();
        assert!(revoked.revoked);
    }

    #[tokio::test]
    async fn test_enrollment_token_lifecycle() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let network = Network {
            id: Uuid::new_v4(),
            name: "test-net".into(),
            overlay_ipv4_cidr: "10.250.0.0/16".into(),
            overlay_ipv6_cidr: None,
            ipv6_enabled: false,
            created_at: chrono::Utc::now(),
        };
        create_network(db.pool(), &network).await.unwrap();

        let parent_id = make_node_entry(db.pool(), "parent-node", None).await;

        let token = EnrollmentToken {
            id: Uuid::new_v4(),
            token: "test-token-123".into(),
            network_id: network.id,
            parent_node_id: parent_id,
            suggested_node_name: "test-node".into(),
            allowed_install_mode: "node".into(),
            expires_at: chrono::Utc::now() + chrono::Duration::minutes(10),
            used_at: None,
            revoked: false,
            created_at: chrono::Utc::now(),
        };

        insert_enrollment_token(db.pool(), &token).await.unwrap();

        let fetched = get_enrollment_token(db.pool(), "test-token-123")
            .await.unwrap().unwrap();
        assert_eq!(fetched.token, "test-token-123");
        assert!(!fetched.revoked);

        revoke_token(db.pool(), "test-token-123").await.unwrap();

        let revoked = get_enrollment_token(db.pool(), "test-token-123")
            .await.unwrap().unwrap();
        assert!(revoked.revoked);
    }

    #[tokio::test]
    async fn test_node_heartbeat_update() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let node = Node {
            id: Uuid::new_v4(),
            display_name: "test-node".into(),
            overlay_ipv4: "10.250.0.1".parse().unwrap(),
            overlay_ipv6: None,
            wg_public_key: "wg-key".into(),
            wg_private_key_encrypted: "wg-priv".into(),
            identity_public_key: "id-pub".into(),
            identity_private_key_encrypted: "id-priv".into(),
            control_parent_id: None,
            node_service_port: 8444,
            wg_port_range_start: 30000,
            wg_port_range_end: 30999,
            is_controller: false,
            agent_version: None,
            protocol_version: 1,
            desired_generation: None,
            active_generation: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
            last_seen_at: None,
            last_handshake_at: None,
            enrollment_token_id: None,
        };
        insert_node(db.pool(), &node).await.unwrap();

        update_node_heartbeat(db.pool(), &node.id, "0.2.0", 2, Some(5)).await.unwrap();

        let updated = get_node_by_id(db.pool(), &node.id).await.unwrap().unwrap();
        assert_eq!(updated.agent_version, Some("0.2.0".into()));
        assert_eq!(updated.protocol_version, 2);
        assert_eq!(updated.active_generation, Some(5));
        assert!(updated.last_seen_at.is_some());
    }

    #[tokio::test]
    async fn test_control_ancestors() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let root = make_node_entry(db.pool(), "root", None).await;
        let a = make_node_entry(db.pool(), "node-a", Some(root)).await;
        let b = make_node_entry(db.pool(), "node-b", Some(a)).await;

        let ancestors = get_control_ancestors(db.pool(), &b).await.unwrap();
        assert_eq!(ancestors.len(), 2);
        assert_eq!(ancestors[0], a);
        assert_eq!(ancestors[1], root);
    }

    async fn make_node_entry(pool: &SqlitePool, name: &str, parent: Option<Uuid>) -> Uuid {
        let id = Uuid::new_v4();
        let node = Node {
            id,
            display_name: name.into(),
            overlay_ipv4: "10.250.0.1".parse().unwrap(),
            overlay_ipv6: None,
            wg_public_key: String::new(),
            wg_private_key_encrypted: String::new(),
            identity_public_key: String::new(),
            identity_private_key_encrypted: String::new(),
            control_parent_id: parent,
            node_service_port: 8444,
            wg_port_range_start: 30000,
            wg_port_range_end: 30999,
            is_controller: false,
            agent_version: None,
            protocol_version: 1,
            desired_generation: None,
            active_generation: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
            last_seen_at: None,
            last_handshake_at: None,
            enrollment_token_id: None,
        };
        insert_node(pool, &node).await.unwrap();
        id
    }

    #[tokio::test]
    async fn test_policy_crud() {
        let db = Database::open("sqlite::memory:").await.unwrap();
        db.run_migrations().await.unwrap();

        let node_id = make_node_entry(db.pool(), "test-node", None).await;

        let policy = TrafficPolicy {
            id: Uuid::new_v4(),
            name: "test-policy".into(),
            priority: 10,
            enabled: true,
            description: Some("test".into()),
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
        };

        insert_traffic_policy(db.pool(), &policy).await.unwrap();

        let policies = list_traffic_policies(db.pool()).await.unwrap();
        assert_eq!(policies.len(), 1);
        assert_eq!(policies[0].name, "test-policy");

        let mc = TrafficPolicyMatch {
            id: Uuid::new_v4(),
            policy_id: policy.id,
            source_node_id: Some(node_id),
            source_node_group: None,
            source_cidr: Some("10.0.0.0/24".into()),
            destination_node_id: Some(node_id),
            destination_cidr: Some("192.168.1.0/24".into()),
            protocol: MatchProtocol::Tcp,
            destination_port_start: Some(443),
            destination_port_end: Some(443),
        };
        insert_policy_match(db.pool(), &mc).await.unwrap();

        let matches = get_policy_matches(db.pool(), &policy.id).await.unwrap();
        assert_eq!(matches.len(), 1);

        let path = TrafficPolicyPath {
            id: Uuid::new_v4(),
            policy_id: policy.id,
            hop_order: 0,
            node_id,
        };
        insert_policy_path(db.pool(), &path).await.unwrap();
        let paths = get_policy_paths(db.pool(), &policy.id).await.unwrap();
        assert_eq!(paths.len(), 1);
    }
}
