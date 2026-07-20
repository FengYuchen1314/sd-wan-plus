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
use pathweaver_core::error::Result;
use crate::AppState;

#[derive(Debug, Deserialize)]
pub struct CreateUpdateRequest {
    pub target_version: String,
    pub manifest_sha256: String,
}

#[derive(Debug, Serialize)]
pub struct UpdateStatus {
    pub job: UpdateJob,
    pub targets: Vec<UpdateTarget>,
    pub summary: UpdateSummary,
}

#[derive(Debug, Serialize)]
pub struct UpdateSummary {
    pub total: usize,
    pub completed: usize,
    pub failed: usize,
    pub waiting: usize,
    pub in_progress: usize,
}

pub async fn create_update_job(
    State(state): State<Arc<AppState>>,
    Json(req): Json<CreateUpdateRequest>,
) -> impl IntoResponse {
    match do_create_update(&state, req).await {
        Ok(status) => (StatusCode::CREATED, Json(status)).into_response(),
        Err(e) => (StatusCode::BAD_REQUEST,
            Json(json!({"error": e.to_string()}))).into_response(),
    }
}

async fn do_create_update(
    state: &Arc<AppState>,
    req: CreateUpdateRequest,
) -> Result<UpdateStatus> {
    let pool = state.db.pool();

    let job_id = Uuid::new_v4();
    let job = UpdateJob {
        id: job_id,
        target_version: req.target_version,
        manifest_sha256: req.manifest_sha256,
        status: UpdateJobStatus::Created,
        created_at: chrono::Utc::now(),
        completed_at: None,
    };

    pathweaver_storage::db::insert_update_job(pool, &job).await?;

    let nodes_by_depth = pathweaver_storage::db::get_nodes_by_depth(pool).await?;

    let mut targets = Vec::new();
    for (node_id, depth) in &nodes_by_depth {
        let target = UpdateTarget {
            id: Uuid::new_v4(),
            update_job_id: job_id,
            node_id: *node_id,
            depth: *depth,
            status: UpdateTargetStatus::Waiting,
            started_at: None,
            completed_at: None,
            error_message: None,
        };
        pathweaver_storage::db::insert_update_target(pool, &target).await?;
        targets.push(target);
    }

    let summary = build_summary(&targets);

    Ok(UpdateStatus {
        job,
        targets,
        summary,
    })
}

pub async fn get_update_status(
    State(state): State<Arc<AppState>>,
    Path(job_id_str): Path<String>,
) -> impl IntoResponse {
    let job_id = match Uuid::parse_str(&job_id_str) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({"error": format!("invalid job ID: {e}")}))).into_response(),
    };

    let pool = state.db.pool();

    let job = match pathweaver_storage::db::get_update_job(pool, &job_id).await {
        Ok(Some(j)) => j,
        Ok(None) => return (StatusCode::NOT_FOUND,
            Json(json!({"error": "Update job not found"}))).into_response(),
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    };

    let targets = match pathweaver_storage::db::get_update_targets(pool, &job_id).await {
        Ok(t) => t,
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    };

    let summary = build_summary(&targets);

    (StatusCode::OK, Json(UpdateStatus {
        job,
        targets,
        summary,
    })).into_response()
}

pub async fn list_updates(
    State(state): State<Arc<AppState>>,
) -> impl IntoResponse {
    let pool = state.db.pool();
    match pathweaver_storage::db::list_update_jobs(pool).await {
        Ok(jobs) => (StatusCode::OK, Json(jobs)).into_response(),
        Err(e) => (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    }
}

pub async fn start_update(
    State(state): State<Arc<AppState>>,
    Path(job_id_str): Path<String>,
) -> impl IntoResponse {
    let job_id = match Uuid::parse_str(&job_id_str) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({"error": format!("invalid job ID: {e}")}))).into_response(),
    };

    let pool = state.db.pool();

    if let Err(e) = pathweaver_storage::db::update_job_status(
        pool, &job_id, &UpdateJobStatus::DistributingArtifacts,
    ).await {
        return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response();
    }

    let targets = match pathweaver_storage::db::get_update_targets(pool, &job_id).await {
        Ok(t) => t,
        Err(e) => return (StatusCode::INTERNAL_SERVER_ERROR,
            Json(json!({"error": e.to_string()}))).into_response(),
    };

    for target in &targets {
        let _ = pathweaver_storage::db::update_target_status(
            pool, &target.id,
            &UpdateTargetStatus::Prefetching, None,
        ).await;
    }

    (StatusCode::OK, Json(json!({
        "status": "started",
        "phase": "DistributingArtifacts",
        "target_count": targets.len(),
    }))).into_response()
}

pub async fn rollback_update(
    State(state): State<Arc<AppState>>,
    Path(job_id_str): Path<String>,
) -> impl IntoResponse {
    let job_id = match Uuid::parse_str(&job_id_str) {
        Ok(id) => id,
        Err(e) => return (StatusCode::BAD_REQUEST,
            Json(json!({"error": format!("invalid job ID: {e}")}))).into_response(),
    };

    let pool = state.db.pool();

    let _ = pathweaver_storage::db::update_job_status(
        pool, &job_id, &UpdateJobStatus::RollingBack,
    ).await;

    let targets = pathweaver_storage::db::get_update_targets(pool, &job_id)
        .await.unwrap_or_default();

    for target in &targets {
        if target.status != UpdateTargetStatus::Completed
            && target.status != UpdateTargetStatus::RolledBack
        {
            let _ = pathweaver_storage::db::update_target_status(
                pool, &target.id,
                &UpdateTargetStatus::RollingBack, None,
            ).await;
        }
    }

    (StatusCode::OK, Json(json!({
        "status": "rolling_back",
        "target_count": targets.len(),
    }))).into_response()
}

fn build_summary(targets: &[UpdateTarget]) -> UpdateSummary {
    UpdateSummary {
        total: targets.len(),
        completed: targets.iter().filter(|t| t.status == UpdateTargetStatus::Completed).count(),
        failed: targets.iter().filter(|t| {
            matches!(t.status,
                UpdateTargetStatus::DownloadFailed
                | UpdateTargetStatus::SignatureInvalid
                | UpdateTargetStatus::InstallFailed
                | UpdateTargetStatus::HealthCheckFailed)
        }).count(),
        waiting: targets.iter().filter(|t| t.status == UpdateTargetStatus::Waiting).count(),
        in_progress: targets.iter().filter(|t| {
            matches!(t.status,
                UpdateTargetStatus::Prefetching
                | UpdateTargetStatus::Verifying
                | UpdateTargetStatus::Staged
                | UpdateTargetStatus::Installing
                | UpdateTargetStatus::Restarting
                | UpdateTargetStatus::HealthChecking)
        }).count(),
    }
}
