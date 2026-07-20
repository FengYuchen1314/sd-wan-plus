use axum::{
    extract::Request,
    middleware::Next,
    response::Response,
    http::StatusCode,
};
use std::sync::Arc;
use crate::AppState;
use pathweaver_storage;
use pathweaver_core::models::AuditLog;
use uuid::Uuid;

pub async fn audit_middleware(
    state: Arc<AppState>,
    req: Request,
    next: Next,
) -> Result<Response, StatusCode> {
    let method = req.method().clone();
    let uri = req.uri().clone();
    let ip = req.headers()
        .get("x-forwarded-for")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("unknown")
        .to_string();

    let response = next.run(req).await;

    let log = AuditLog {
        id: Uuid::new_v4(),
        admin_id: None,
        action: format!("{} {}", method, uri.path()),
        resource_type: "http_request".to_string(),
        resource_id: None,
        details: Some(format!("status={}", response.status().as_u16())),
        ip_address: ip,
        created_at: chrono::Utc::now(),
    };

    let _ = pathweaver_storage::db::insert_audit_log(state.db.pool(), &log).await;

    Ok(response)
}

pub async fn auth_required(
    state: Arc<AppState>,
    mut req: Request,
    next: Next,
) -> Result<Response, StatusCode> {
    let cookie_header = req.headers()
        .get("cookie")
        .and_then(|v| v.to_str().ok())
        .unwrap_or("");

    let session_token = extract_session_cookie(cookie_header);

    if let Some(token) = session_token {
        match pathweaver_storage::db::get_session_by_token(state.db.pool(), &token).await {
            Ok(Some(session)) if !session.revoked => {
                if chrono::Utc::now() < session.expires_at {
                    req.extensions_mut().insert(session);
                    return Ok(next.run(req).await);
                }
            }
            _ => {}
        }
    }

    Err(StatusCode::UNAUTHORIZED)
}

fn extract_session_cookie(cookie_header: &str) -> Option<String> {
    for part in cookie_header.split(';') {
        let part = part.trim();
        if let Some(value) = part.strip_prefix("pw_session=") {
            return Some(value.to_string());
        }
    }
    None
}
