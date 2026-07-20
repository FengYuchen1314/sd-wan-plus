use serde::{Deserialize, Serialize};
use uuid::Uuid;

#[derive(Debug, Deserialize)]
pub struct EnrollRequest {
    pub token: String,
    pub node_name: String,
    pub wg_public_key: String,
    pub identity_public_key: String,
    pub agent_version: String,
    pub protocol_version: u32,
}

#[derive(Debug, Serialize)]
pub struct EnrollResponse {
    pub node_id: String,
    pub overlay_ipv4: String,
    pub overlay_ipv6: Option<String>,
    pub network_id: String,
    pub parent_wg_public_key: String,
    pub parent_wg_endpoint: String,
    pub recovery_endpoint: String,
}

#[derive(Debug, Deserialize)]
pub struct CommitEnrollRequest {
    pub node_id: String,
    pub token: String,
    pub overlay_reachable: bool,
}

#[derive(Debug, Serialize)]
pub struct CommitEnrollResponse {
    pub success: bool,
    pub recovery_endpoint: String,
}

#[derive(Debug, Serialize)]
pub struct ArtifactListResponse {
    pub artifacts: Vec<ArtifactInfo>,
}

#[derive(Debug, Serialize)]
pub struct ArtifactInfo {
    pub artifact_id: String,
    pub name: String,
    pub version: String,
    pub sha256: String,
    pub size_bytes: u64,
}
