pub mod api;
pub mod enrollment;
pub mod relay;
pub mod bootstrap;
pub mod wireguard;
pub mod links;
pub mod policies;
pub mod updates;
pub mod middleware;

use axum::{Router, routing::get};
use tower_http::cors::CorsLayer;
use tower_http::trace::TraceLayer;
use tower_http::services::ServeDir;
use pathweaver_storage::Database;
use std::sync::Arc;

pub struct AppState {
    pub db: Database,
    pub network_id: uuid::Uuid,
    pub controller_public_key: String,
    pub static_dir: Option<String>,
}

pub async fn run_controller(db_path: &str, bind_addr: &str) -> anyhow::Result<()> {
    run_controller_with_static(db_path, bind_addr, None).await
}

pub async fn run_controller_with_static(
    db_path: &str,
    bind_addr: &str,
    static_dir: Option<&str>,
) -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter("pathweaver_controller=debug,pathweaver_core=debug,info")
        .init();

    let db = Database::open(db_path).await?;
    db.run_migrations().await?;

    let default_password = "admin123";
    match pathweaver_storage::db::get_admin_by_username(db.pool(), "admin").await {
        Ok(Some(_)) => tracing::info!("Admin account exists"),
        _ => {
            let hash = pathweaver_security::auth::hash_password(default_password)
                .expect("failed to hash password");
            pathweaver_storage::db::create_admin(db.pool(), "admin", &hash).await?;
            tracing::info!("Created default admin (password: {default_password})");
        }
    }

    let network_id = uuid::Uuid::new_v4();
    let (_controller_private, controller_public) =
        pathweaver_security::crypto::generate_identity_keypair();

    let state = Arc::new(AppState {
        db,
        network_id,
        controller_public_key: controller_public,
        static_dir: static_dir.map(|s| s.to_string()),
    });

    let app = Router::new()
        .route("/api/health", get(api::health_check))
        .route("/api/auth/login", axum::routing::post(api::login))
        .route("/api/nodes", get(api::list_nodes))
        .route("/api/nodes/{id}/rename", axum::routing::put(api::rename_node))
        .route("/api/enrollment/tokens", get(api::list_tokens))
        .route("/api/enrollment/tokens", axum::routing::post(api::create_token))
        .route("/api/enrollment/tokens/{id}/revoke", axum::routing::post(api::revoke_token))
        .route("/api/enrollment/command/{token}", get(api::get_install_command))
        .route("/bootstrap/enroll", axum::routing::post(bootstrap::handle_bootstrap_enroll))
        .route("/bootstrap/install.sh", get(bootstrap::handle_install_script))
        .route("/bootstrap/artifacts/{name}", get(bootstrap::handle_artifact))
        .route("/bootstrap/manifest", get(bootstrap::handle_manifest))
        .route("/api/links", get(links::list_links))
        .route("/api/links", axum::routing::post(links::create_link))
        .route("/api/links/{id}", get(links::get_link))
        .route("/api/links/{id}", axum::routing::delete(links::delete_link))
        .route("/api/policies", axum::routing::get(policies::list_policies))
        .route("/api/policies", axum::routing::post(policies::create_policy))
        .route("/api/config/preview", axum::routing::get(policies::preview_config))
        .route("/api/updates", axum::routing::get(updates::list_updates))
        .route("/api/updates", axum::routing::post(updates::create_update_job))
        .route("/api/updates/{id}", axum::routing::get(updates::get_update_status))
        .route("/api/updates/{id}/start", axum::routing::post(updates::start_update))
        .route("/api/updates/{id}/rollback", axum::routing::post(updates::rollback_update))
        .fallback_service(serve_static_files())
        .layer(CorsLayer::permissive())
        .layer(TraceLayer::new_for_http())
        .with_state(state);

    tracing::info!("PathWeaver controller starting on {}", bind_addr);

    let listener = tokio::net::TcpListener::bind(bind_addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}

fn serve_static_files() -> Router {
    let static_dir = std::env::var("PW_STATIC_DIR")
        .unwrap_or_else(|_| "web/dist".to_string());

    if std::path::Path::new(&static_dir).exists() {
        tracing::info!("Serving static files from {}", static_dir);
        use tower_http::services::ServeFile;
        Router::new()
            .nest_service("/", ServeDir::new(&static_dir))
            .fallback_service(ServeFile::new(format!("{}/index.html", static_dir)))
    } else {
        tracing::warn!("Static directory '{}' not found, UI will not be available", static_dir);
        Router::new()
            .fallback(|| async { axum::http::StatusCode::NOT_FOUND })
    }
}
