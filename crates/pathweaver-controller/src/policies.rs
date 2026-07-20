use axum::{
    Json, extract::{State, Path},
    response::IntoResponse, http::StatusCode,
};
use std::sync::Arc;
use std::collections::HashMap;
use serde::{Deserialize, Serialize};
use serde_json::json;
use uuid::Uuid;
use pathweaver_storage;
use pathweaver_core::models::*;
use pathweaver_core::error::{PathWeaverError, Result};
use pathweaver_topology;
use pathweaver_routing;
use crate::AppState;

#[derive(Debug, Deserialize)]
pub struct CreatePolicyRequest {
    pub name: String,
    pub priority: u32,
    pub description: Option<String>,
    pub matches: Vec<MatchDef>,
    pub path_node_ids: Vec<String>,
    pub egress_node_id: Option<String>,
    pub egress_nat: Option<bool>,
    pub return_path_type: Option<String>,
    pub failover_enabled: Option<bool>,
}

#[derive(Debug, Deserialize)]
pub struct MatchDef {
    pub source_node_id: Option<String>,
    pub source_cidr: Option<String>,
    pub destination_node_id: Option<String>,
    pub destination_cidr: Option<String>,
    pub protocol: Option<String>,
    pub destination_port: Option<u16>,
}

#[derive(Debug, Serialize)]
pub struct PolicyDetail {
    pub policy: TrafficPolicy,
    pub matches: Vec<TrafficPolicyMatch>,
    pub path: Vec<TrafficPolicyPath>,
    pub config: Option<TrafficPolicyConfig>,
}

pub async fn create_policy(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreatePolicyRequest>,
) -> impl IntoResponse {
    match do_create_policy(&state, req).await {
        Ok(detail) => (StatusCode::CREATED, Json(detail)).into_response(),
        Err(e) => (StatusCode::BAD_REQUEST, Json(json!({"error": e.to_string()}))).into_response(),
    }
}

async fn do_create_policy(
    state: &Arc<AppState>,
    req: CreatePolicyRequest,
) -> Result<PolicyDetail> {
    let pool = state.db.pool();

    let policy_id = Uuid::new_v4();
    let policy = TrafficPolicy {
        id: policy_id,
        name: req.name,
        priority: req.priority,
        enabled: true,
        description: req.description,
        created_at: chrono::Utc::now(),
        updated_at: chrono::Utc::now(),
    };

    let matches: Vec<TrafficPolicyMatch> = req.matches.iter().map(|m| {
        TrafficPolicyMatch {
            id: Uuid::new_v4(),
            policy_id,
            source_node_id: m.source_node_id.as_ref().and_then(|s| Uuid::parse_str(s).ok()),
            source_node_group: None,
            source_cidr: m.source_cidr.clone(),
            destination_node_id: m.destination_node_id.as_ref().and_then(|s| Uuid::parse_str(s).ok()),
            destination_cidr: m.destination_cidr.clone(),
            protocol: match m.protocol.as_deref() {
                Some("TCP") | Some("tcp") => MatchProtocol::Tcp,
                Some("UDP") | Some("udp") => MatchProtocol::Udp,
                Some("ICMP") | Some("icmp") => MatchProtocol::Icmp,
                _ => MatchProtocol::Any,
            },
            destination_port_start: m.destination_port,
            destination_port_end: m.destination_port,
        }
    }).collect();

    let path: Vec<TrafficPolicyPath> = req.path_node_ids.iter().enumerate().map(|(i, node_id_str)| {
        TrafficPolicyPath {
            id: Uuid::new_v4(),
            policy_id,
            hop_order: i as u32,
            node_id: Uuid::parse_str(node_id_str).unwrap_or_else(|_| Uuid::nil()),
        }
    }).collect();

    let config = TrafficPolicyConfig {
        id: Uuid::new_v4(),
        policy_id,
        egress_node_id: req.egress_node_id.as_ref().and_then(|s| Uuid::parse_str(s).ok()),
        egress_nat: req.egress_nat.unwrap_or(false),
        return_path_type: match req.return_path_type.as_deref() {
            Some("Independent") | Some("independent") => ReturnPathType::Independent,
            _ => ReturnPathType::Symmetric,
        },
        failover_enabled: req.failover_enabled.unwrap_or(false),
        failover_path_id: None,
    };

    pathweaver_storage::db::insert_traffic_policy(pool, &policy).await?;

    for m in &matches {
        pathweaver_storage::db::insert_policy_match(pool, m).await?;
    }

    for p in &path {
        pathweaver_storage::db::insert_policy_path(pool, p).await?;
    }

    pathweaver_storage::db::insert_policy_config(pool, &config).await?;

    Ok(PolicyDetail {
        policy,
        matches,
        path,
        config: Some(config),
    })
}

pub async fn list_policies(
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    let pool = state.db.pool();

    let policies = match pathweaver_storage::db::list_traffic_policies(pool).await {
        Ok(p) => p,
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    };

    let mut details = Vec::new();
    for policy in policies {
        let matches = pathweaver_storage::db::get_policy_matches(pool, &policy.id).await.unwrap_or_default();
        let path = pathweaver_storage::db::get_policy_paths(pool, &policy.id).await.unwrap_or_default();
        let config = pathweaver_storage::db::get_policy_config(pool, &policy.id).await.unwrap_or(None);

        details.push(PolicyDetail {
            policy,
            matches,
            path,
            config,
        });
    }

    (StatusCode::OK, Json(details)).into_response()
}

#[derive(Debug, Serialize)]
pub struct ConfigPreview {
    pub generation: u64,
    pub node_configs: Vec<NodeConfigPreview>,
    pub validation_errors: Vec<String>,
}

#[derive(Debug, Serialize)]
pub struct NodeConfigPreview {
    pub node_id: String,
    pub display_name: String,
    pub overlay_ip: String,
    pub wireguard_links: Vec<WireGuardLinkConfig>,
    pub policy_routing_rules: Vec<PolicyRoutingRule>,
    pub route_tables: Vec<RouteTable>,
    pub nat_rules: Vec<NatRule>,
    pub nftables_rules: Vec<String>,
}

pub async fn preview_config(
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    match do_preview_config(&state).await {
        Ok(preview) => (StatusCode::OK, Json(preview)).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    }
}

async fn do_preview_config(state: &Arc<AppState>) -> Result<ConfigPreview> {
    let pool = state.db.pool();

    let nodes = pathweaver_storage::db::list_all_nodes(pool).await?;
    let links = pathweaver_storage::db::list_wireguard_links(pool).await?;
    let policies = pathweaver_storage::db::list_traffic_policies(pool).await?;
    let enabled_policies: Vec<_> = policies.into_iter().filter(|p| p.enabled).collect();

    let mut validation_errors = Vec::new();
    let mut path_map: HashMap<PolicyId, Vec<TrafficPolicyPath>> = HashMap::new();
    let mut match_map: HashMap<PolicyId, Vec<TrafficPolicyMatch>> = HashMap::new();

    for policy in &enabled_policies {
        let paths = pathweaver_storage::db::get_policy_paths(pool, &policy.id).await?;

        if let Err(e) = pathweaver_topology::validate_wireguard_path(
            &paths.iter().map(|p| p.node_id).collect::<Vec<_>>(),
            &links,
        ) {
            validation_errors.push(format!("Policy '{}': {e}", policy.name));
        }

        let conflicts = pathweaver_topology::check_path_conflicts(
            &enabled_policies,
            &match_map.values().flatten().cloned().collect::<Vec<_>>(),
        );
        for (p1, p2) in &conflicts {
            validation_errors.push(format!(
                "Conflict between policies '{}' and '{}' at priority {}",
                p1.name, p2.name, p1.priority
            ));
        }

        let matches = pathweaver_storage::db::get_policy_matches(pool, &policy.id).await?;
        match_map.insert(policy.id, matches);
        path_map.insert(policy.id, paths);
    }

    let generation = chrono::Utc::now().timestamp() as u64;
    let mut node_configs = Vec::new();

    for node in &nodes {
        let desired = pathweaver_routing::compile_desired_state(
            node.id, generation, node, &links,
            &enabled_policies, &path_map, &match_map,
        );

        let wg_links = links.iter()
            .filter(|l| l.node_a == node.id || l.node_b == node.id)
            .map(|link| {
                WireGuardLinkConfig {
                    link_id: link.id,
                    interface_name: if link.node_a == node.id {
                        link.interface_name_a.clone()
                    } else {
                        link.interface_name_b.clone()
                    },
                    listen_port: if link.listener_node_id == node.id { link.listener_port } else { 0 },
                    peer_public_key: String::new(),
                    peer_endpoint: if link.initiator_node_id == node.id {
                        Some(format!("{}:{}", link.listener_address, link.listener_port))
                    } else { None },
                    persistent_keepalive: if link.initiator_node_id == node.id { 25 } else { 0 },
                    is_initiator: link.initiator_node_id == node.id,
                    node_private_key: String::new(),
                    overlay_ip: node.overlay_ipv4.to_string(),
                    peer_overlay_ip: String::new(),
                }
            })
            .collect();

        match desired {
            Ok(ds) => {
                node_configs.push(NodeConfigPreview {
                    node_id: node.id.to_string(),
                    display_name: node.display_name.clone(),
                    overlay_ip: node.overlay_ipv4.to_string(),
                    wireguard_links: wg_links,
                    policy_routing_rules: ds.policy_routing_rules,
                    route_tables: ds.route_tables,
                    nat_rules: ds.nat_rules,
                    nftables_rules: ds.nftables_rules,
                });
            }
            Err(e) => {
                validation_errors.push(format!("Node '{}': {e}", node.display_name));
            }
        }
    }

    Ok(ConfigPreview {
        generation,
        node_configs,
        validation_errors,
    })
}
