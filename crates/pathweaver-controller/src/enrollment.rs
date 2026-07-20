use std::sync::Arc;
use uuid::Uuid;
use chrono::Utc;
use pathweaver_core::models::*;
use pathweaver_core::error::{PathWeaverError, Result};
use pathweaver_security::crypto;
use crate::AppState;

pub struct EnrollmentService {
    state: Arc<AppState>,
}

impl EnrollmentService {
    pub fn new(state: Arc<AppState>) -> Self {
        Self { state }
    }

    pub async fn process_enrollment(
        &self,
        token_str: &str,
        node_name: &str,
        wg_public_key: &str,
        identity_public_key: &str,
        agent_version: &str,
        protocol_version: u32,
    ) -> Result<EnrollmentResult> {
        let pool = self.state.db.pool();

        let token = pathweaver_storage::db::get_enrollment_token(pool, token_str)
            .await?
            .ok_or_else(|| PathWeaverError::InvalidToken("token not found".into()))?;

        if token.revoked {
            return Err(PathWeaverError::TokenRevoked);
        }
        if token.used_at.is_some() {
            return Err(PathWeaverError::TokenAlreadyUsed);
        }
        if Utc::now() > token.expires_at {
            return Err(PathWeaverError::TokenExpired);
        }

        let parent = pathweaver_storage::db::get_node_by_id(pool, &token.parent_node_id)
            .await?
            .ok_or_else(|| PathWeaverError::ParentNotFound(token.parent_node_id.to_string()))?;

        let network = pathweaver_storage::db::get_network(pool, &token.network_id)
            .await?
            .ok_or_else(|| PathWeaverError::InternalError("network not found".into()))?;

        let node_id = Uuid::new_v4();

        let overlay_ipv4 = allocate_overlay_ip(pool, &network).await?;

        let (wg_private_plain, _wg_public) = crypto::generate_wireguard_keypair();
        let (identity_private_plain, _identity_public) = crypto::generate_identity_keypair();

        let wg_private_encrypted = wg_private_plain;
        let identity_private_encrypted = identity_private_plain;

        let node = Node {
            id: node_id,
            display_name: node_name.to_string(),
            overlay_ipv4,
            overlay_ipv6: None,
            wg_public_key: wg_public_key.to_string(),
            wg_private_key_encrypted: wg_private_encrypted,
            identity_public_key: identity_public_key.to_string(),
            identity_private_key_encrypted: identity_private_encrypted,
            control_parent_id: Some(token.parent_node_id),
            node_service_port: parent.node_service_port,
            wg_port_range_start: 30000,
            wg_port_range_end: 30999,
            is_controller: false,
            agent_version: Some(agent_version.to_string()),
            protocol_version,
            desired_generation: None,
            active_generation: None,
            created_at: Utc::now(),
            updated_at: Utc::now(),
            last_seen_at: None,
            last_handshake_at: None,
            enrollment_token_id: Some(token.id),
        };

        pathweaver_storage::db::insert_node(pool, &node).await?;

        pathweaver_storage::db::mark_token_used(pool, token_str).await?;

        let relation = ControlRelation {
            id: Uuid::new_v4(),
            parent_id: token.parent_node_id,
            child_id: node_id,
            enrolled_at: Utc::now(),
            enrollment_token_id: token.id,
        };
        pathweaver_storage::db::insert_control_relation(pool, &relation).await?;

        let recovery_endpoint = format!("{}:{}", parent.overlay_ipv4, parent.node_service_port);

        Ok(EnrollmentResult {
            node_id,
            overlay_ipv4,
            overlay_ipv6: None,
            network_id: token.network_id,
            parent_wg_public_key: parent.wg_public_key.clone(),
            parent_wg_endpoint: format!("{}:{}", parent.overlay_ipv4, 30000),
            recovery_endpoint,
        })
    }
}

pub struct EnrollmentResult {
    pub node_id: NodeId,
    pub overlay_ipv4: std::net::Ipv4Addr,
    pub overlay_ipv6: Option<String>,
    pub network_id: NetworkId,
    pub parent_wg_public_key: String,
    pub parent_wg_endpoint: String,
    pub recovery_endpoint: String,
}

async fn allocate_overlay_ip(
    pool: &sqlx::SqlitePool,
    network: &Network,
) -> Result<std::net::Ipv4Addr> {
    let nodes = pathweaver_storage::db::list_all_nodes(pool).await?;

    let base = network.overlay_ipv4_cidr
        .split('/')
        .next()
        .unwrap_or("10.250.0.0")
        .parse::<std::net::Ipv4Addr>()
        .map_err(|e| PathWeaverError::InternalError(format!("invalid overlay CIDR: {e}")))?;

    let base_u32 = u32::from(base);
    let used: std::collections::HashSet<u32> = nodes
        .iter()
        .map(|n| u32::from(n.overlay_ipv4))
        .collect();

    for offset in 1u32..65535 {
        let candidate = base_u32 + offset;
        if !used.contains(&candidate) {
            return Ok(std::net::Ipv4Addr::from(candidate));
        }
    }

    Err(PathWeaverError::InternalError("overlay IP pool exhausted".into()))
}
