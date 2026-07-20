use axum::{
    Json, extract::{State, Path, Query},
    response::IntoResponse, http::StatusCode,
};
use std::sync::Arc;
use serde::{Deserialize, Serialize};
use serde_json::json;
use uuid::Uuid;
use chrono::Utc;
use pathweaver_storage;
use pathweaver_security;
use pathweaver_core::models::*;
use crate::AppState;

pub async fn health_check() -> impl IntoResponse {
    Json(json!({ "status": "ok", "version": env!("CARGO_PKG_VERSION") }))
}

#[derive(Debug, Deserialize)]
pub struct LoginRequest {
    pub username: String,
    pub password: String,
}

#[derive(Debug, Serialize)]
pub struct LoginResponse {
    pub token: String,
    pub message: String,
}

pub async fn login(
    State(state): State<Arc<AppState>>,
    Json(req): Json<LoginRequest>,
) -> impl IntoResponse {
    let pool = state.db.pool();

    match pathweaver_storage::db::get_admin_by_username(pool, &req.username).await {
        Ok(Some(admin)) => {
            match pathweaver_security::auth::verify_password(&req.password, &admin.password_hash) {
                Ok(true) => {
                    let token = pathweaver_security::crypto::generate_random_token(32);
                    match pathweaver_storage::db::create_session(
                        pool, &admin.id, &token, "127.0.0.1", "api", 24,
                    ).await {
                        Ok(_) => (StatusCode::OK, Json(LoginResponse {
                            token,
                            message: "Login successful".into(),
                        })).into_response(),
                        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
                            Json(json!({ "error": e.to_string() }))).into_response(),
                    }
                }
                _ => (StatusCode::UNAUTHORIZED,
                    Json(json!({ "error": "Invalid credentials" }))).into_response(),
            }
        }
        Ok(None) => (StatusCode::UNAUTHORIZED,
            Json(json!({ "error": "Invalid credentials" }))).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }
}

pub async fn list_nodes(
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    match pathweaver_storage::db::list_all_nodes(state.db.pool()).await {
        Ok(nodes) => (StatusCode::OK, Json(nodes)).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }
}

#[derive(Debug, Deserialize)]
pub struct RenameRequest {
    pub display_name: String,
}

pub async fn rename_node(
    State(state): State<Arc<AppState>>,
    Path(node_id_str): Path<String>,
    Json(req): Json<RenameRequest>,
) -> impl IntoResponse {
    let node_id = match Uuid::parse_str(&node_id_str) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({ "error": format!("invalid node ID: {e}") }))).into_response(),
    };

    let pool = state.db.pool();

    match pathweaver_storage::db::get_node_by_display_name(pool, &req.display_name).await {
        Ok(Some(_)) => {
            return (StatusCode::CONFLICT,
                Json(json!({ "error": "Display name already exists" }))).into_response();
        }
        Ok(None) => {}
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }

    match pathweaver_storage::db::update_node_display_name(pool, &node_id, &req.display_name).await {
        Ok(_) => (StatusCode::OK, Json(json!({ "status": "renamed" }))).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }
}

#[derive(Debug, Deserialize)]
pub struct CreateTokenRequest {
    pub parent_node_id: String,
    pub suggested_node_name: String,
    pub ttl_seconds: Option<u64>,
}

#[derive(Debug, Serialize)]
pub struct TokenResponse {
    pub token_id: String,
    pub token: String,
    pub install_command: String,
    pub expires_at: String,
}

pub async fn create_token(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreateTokenRequest>,
) -> impl IntoResponse {
    let parent_node_id = match Uuid::parse_str(&req.parent_node_id) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({ "error": format!("invalid parent node ID: {e}") }))).into_response(),
    };

    let pool = state.db.pool();

    let parent = match pathweaver_storage::db::get_node_by_id(pool, &parent_node_id).await {
        Ok(Some(n)) => n,
        Ok(None) => return (StatusCode::NOT_FOUND,
            Json(json!({ "error": "Parent node not found" }))).into_response(),
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    };

    let ttl = req.ttl_seconds.unwrap_or(600);
    let token_value = pathweaver_security::crypto::generate_random_token(32);
    let now = Utc::now();
    let expires_at = now + chrono::Duration::seconds(ttl as i64);

    let token = EnrollmentToken {
        id: Uuid::new_v4(),
        token: token_value.clone(),
        network_id: state.network_id,
        parent_node_id,
        suggested_node_name: req.suggested_node_name.clone(),
        allowed_install_mode: "node".to_string(),
        expires_at,
        used_at: None,
        revoked: false,
        created_at: now,
    };

    if let Err(e) = pathweaver_storage::db::insert_enrollment_token(pool, &token).await {
        return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response();
    }

    let parent_addr = format!("{}:{}", parent.overlay_ipv4, parent.node_service_port);
    let install_command = format!(
        "curl -fsSL \"https://{}/bootstrap/install.sh?token={}\" -o /tmp/pw-install.sh && sudo bash /tmp/pw-install.sh",
        parent_addr, token_value
    );

    (StatusCode::CREATED, Json(TokenResponse {
        token_id: token.id.to_string(),
        token: token_value,
        install_command,
        expires_at: expires_at.to_rfc3339(),
    })).into_response()
}

pub async fn list_tokens(
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    (StatusCode::OK, Json(json!({ "tokens": [] }))).into_response()
}

pub async fn revoke_token(
    State(state): State<Arc<AppState>>,
    Path(token_id_str): Path<String>,
) -> impl IntoResponse {
    let pool = state.db.pool();
    match pathweaver_storage::db::revoke_token(pool, &token_id_str).await {
        Ok(_) => (StatusCode::OK, Json(json!({ "status": "revoked" }))).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }
}

#[derive(Debug, Deserialize)]
pub struct InstallCommandQuery {
    pub token: String,
}

pub async fn get_install_command(
    State(state): State<Arc<AppState>>,
    Path(_token_path): Path<String>,
    Query(query): Query<InstallCommandQuery>,
) -> impl IntoResponse {
    let pool = state.db.pool();
    match pathweaver_storage::db::get_enrollment_token(pool, &query.token).await {
        Ok(Some(token)) => {
            if token.revoked {
                return (StatusCode::FORBIDDEN,
                    Json(json!({ "error": "Token revoked" }))).into_response();
            }
            if token.used_at.is_some() {
                return (StatusCode::FORBIDDEN,
                    Json(json!({ "error": "Token already used" }))).into_response();
            }
            if Utc::now() > token.expires_at {
                return (StatusCode::FORBIDDEN,
                    Json(json!({ "error": "Token expired" }))).into_response();
            }

            let parent = match pathweaver_storage::db::get_node_by_id(pool, &token.parent_node_id).await {
                Ok(Some(n)) => n,
                _ => return (StatusCode::NOT_FOUND,
                    Json(json!({ "error": "Parent node not found" }))).into_response(),
            };

            let command = format!(
                "#!/bin/bash\n# PathWeaver Node Installer\nTOKEN=\"{}\"\nPARENT_ADDR=\"{}\"\nPARENT_PORT=\"{}\"\ncurl -fsSL \"https://{}:{}/bootstrap/install.sh?token={}\" -o /tmp/pw-install.sh && sudo bash /tmp/pw-install.sh {}",
                token.token,
                parent.overlay_ipv4,
                parent.node_service_port,
                parent.overlay_ipv4,
                parent.node_service_port,
                token.token,
                token.token
            );

            (StatusCode::OK, Json(json!({
                "command": command,
                "parent_address": parent.overlay_ipv4.to_string(),
                "parent_port": parent.node_service_port,
                "token": token.token,
            }))).into_response()
        }
        Ok(None) => (StatusCode::NOT_FOUND,
            Json(json!({ "error": "Token not found" }))).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({ "error": e.to_string() }))).into_response(),
    }
}
