use axum::{
    Json, extract::{State, Path},
    response::IntoResponse, http::StatusCode,
};
use std::sync::Arc;
use serde::{Deserialize, Serialize};
use serde_json::json;
use uuid::Uuid;
use pathweaver_storage;
use pathweaver_core::models::*;
use crate::AppState;
use crate::wireguard::WireGuardCompiler;

#[derive(Debug, Deserialize)]
pub struct CreateLinkRequest {
    pub initiator_node_id: String,
    pub listener_node_id: String,
    pub listener_address: String,
    pub listener_port: Option<u16>,
    pub link_name: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct LinkResponse {
    pub link_id: String,
    pub interface_initiator: String,
    pub interface_listener: String,
    pub listener_port: u16,
    pub status: String,
}

pub async fn create_link(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreateLinkRequest>,
) -> impl IntoResponse {
    let initiator_id = match Uuid::parse_str(&req.initiator_node_id) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({ "error": format!("invalid initiator node ID: {e}") }))).into_response(),
    };

    let listener_id = match Uuid::parse_str(&req.listener_node_id) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({ "error": format!("invalid listener node ID: {e}") }))).into_response(),
    };

    let pool = state.db.pool();

    let initiator = match pathweaver_storage::db::get_node_by_id(pool, &initiator_id).await {
        Ok(Some(n)) => n,
        Ok(None) => return (StatusCode::NOT_FOUND,
            Json(json!({ "error": "Initiator node not found" }))).into_response(),
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    };

    let listener = match pathweaver_storage::db::get_node_by_id(pool, &listener_id).await {
        Ok(Some(n)) => n,
        Ok(None) => return (StatusCode::NOT_FOUND,
            Json(json!({ "error": "Listener node not found" }))).into_response(),
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    };

    let listener_port = req.listener_port.unwrap_or_else(|| {
        let start = listener.wg_port_range_start;
        start + 1
    });

    let link = match WireGuardCompiler::compile_cross_link(
        &initiator,
        &listener,
        &req.listener_address,
        listener_port,
        req.link_name.as_deref(),
    ) {
        Ok(l) => l,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({ "error": e.to_string() }))).into_response(),
    };

    if let Err(e) = pathweaver_storage::db::insert_wireguard_link(pool, &link).await {
        return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response();
    }

    (StatusCode::CREATED, Json(LinkResponse {
        link_id: link.id.to_string(),
        interface_initiator: link.interface_name_a.clone(),
        interface_listener: link.interface_name_b.clone(),
        listener_port: link.listener_port,
        status: "Pending".to_string(),
    })).into_response()
}

pub async fn list_links(
    State(_state): State<Arc<AppState>>,
) -> impl IntoResponse {
    (StatusCode::OK, Json(json!({ "links": [] }))).into_response()
}

pub async fn get_link(
    State(_state): State<Arc<AppState>>,
    Path(_link_id): Path<String>,
) -> impl IntoResponse {
    (StatusCode::OK, Json(json!({ "status": "not found" }))).into_response()
}

pub async fn delete_link(
    State(_state): State<Arc<AppState>>,
    Path(_link_id): Path<String>,
) -> impl IntoResponse {
    (StatusCode::OK, Json(json!({ "status": "deleted" }))).into_response()
}
