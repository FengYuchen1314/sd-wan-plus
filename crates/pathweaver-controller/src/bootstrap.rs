use axum::{
    Json, extract::State,
    response::IntoResponse, http::StatusCode,
};
use std::sync::Arc;
use serde::{Deserialize, Serialize};
use serde_json::json;
use crate::AppState;
use crate::enrollment::{EnrollmentService, EnrollmentResult};

#[derive(Debug, Deserialize)]
pub struct BootstrapEnrollRequest {
    pub token: String,
    pub node_name: String,
    pub wg_public_key: String,
    pub identity_public_key: String,
    pub agent_version: String,
    pub protocol_version: u32,
}

#[derive(Debug, Serialize)]
pub struct BootstrapEnrollResponse {
    pub node_id: String,
    pub overlay_ipv4: String,
    pub overlay_ipv6: Option<String>,
    pub network_id: String,
    pub parent_wg_public_key: String,
    pub parent_wg_endpoint: String,
    pub recovery_endpoint: String,
}

pub async fn handle_bootstrap_enroll(
    State(state): State<Arc<AppState>>,
    Json(req): Json<BootstrapEnrollRequest>,
) -> impl IntoResponse {
    let service = EnrollmentService::new(state);

    match service.process_enrollment(
        &req.token,
        &req.node_name,
        &req.wg_public_key,
        &req.identity_public_key,
        &req.agent_version,
        req.protocol_version,
    )
    .await
    {
        Ok(result) => {
            let resp = BootstrapEnrollResponse {
                node_id: result.node_id.to_string(),
                overlay_ipv4: result.overlay_ipv4.to_string(),
                overlay_ipv6: result.overlay_ipv6,
                network_id: result.network_id.to_string(),
                parent_wg_public_key: result.parent_wg_public_key,
                parent_wg_endpoint: result.parent_wg_endpoint,
                recovery_endpoint: result.recovery_endpoint,
            };
            (StatusCode::OK, Json(resp)).into_response()
        }
        Err(e) => {
            let (status, msg) = map_enrollment_error(e);
            (status, Json(json!({ "error": msg }))).into_response()
        }
    }
}

fn map_enrollment_error(err: pathweaver_core::error::PathWeaverError) -> (StatusCode, String) {
    use pathweaver_core::error::PathWeaverError;
    match err {
        PathWeaverError::TokenExpired => (StatusCode::FORBIDDEN, "Token expired".into()),
        PathWeaverError::TokenAlreadyUsed => (StatusCode::FORBIDDEN, "Token already used".into()),
        PathWeaverError::TokenRevoked => (StatusCode::FORBIDDEN, "Token revoked".into()),
        PathWeaverError::InvalidToken(msg) => (StatusCode::BAD_REQUEST, msg),
        PathWeaverError::ParentNotFound(msg) => (StatusCode::NOT_FOUND, msg),
        _ => (StatusCode::INTERNAL_SERVER_ERROR, err.to_string()),
    }
}

pub async fn handle_install_script() -> impl IntoResponse {
    let script = include_str!(concat!(env!("CARGO_MANIFEST_DIR"), "/../../installer/install-node.sh"));
    (StatusCode::OK, [("content-type", "text/x-shellscript")], script)
}

pub async fn handle_artifact(
    axum::extract::Query(params): axum::extract::Query<std::collections::HashMap<String, String>>,
) -> impl IntoResponse {
    let _artifact = params.get("name").cloned().unwrap_or_default();
    let _token = params.get("token").cloned().unwrap_or_default();
    (StatusCode::OK, "artifact data placeholder")
}

pub async fn handle_manifest() -> impl IntoResponse {
    let manifest = serde_json::json!({
        "product_version": "0.1.0",
        "protocol_version": 1,
        "target_architecture": "x86_64",
        "min_compatible_version": "0.1.0",
        "git_commit": "dev",
        "build_time": "2026-01-01T00:00:00Z",
        "allow_downgrade": false,
    });
    (StatusCode::OK, Json(manifest))
}
