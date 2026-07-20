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

    // 创建网络
    let net = pathweaver_core::models::Network {
        id: network_id,
        name: "default".into(),
        overlay_ipv4_cidr: "10.250.0.0/16".into(),
        overlay_ipv6_cidr: None,
        ipv6_enabled: false,
        created_at: chrono::Utc::now(),
    };
    if pathweaver_storage::db::get_network(db.pool(), &network_id).await.unwrap_or(None).is_none() {
        pathweaver_storage::db::create_network(db.pool(), &net).await?;
    }

    // 控制机自注册为第一个节点 (仅首次)
    let existing_nodes = pathweaver_storage::db::list_all_nodes(db.pool()).await?;
    let has_controller = existing_nodes.iter().any(|n| n.is_controller);

    let ctrl_id = if has_controller {
        let c = existing_nodes.iter().find(|n| n.is_controller).unwrap();
        tracing::info!("已有控制机节点: {} ({})", c.display_name, c.id);
        c.id
    } else {
        let cid = uuid::Uuid::new_v4();
        let (wg_priv, wg_pub) = pathweaver_security::crypto::generate_wireguard_keypair();
        let (id_priv, id_pub) = pathweaver_security::crypto::generate_identity_keypair();
        let ctrl = pathweaver_core::models::Node {
            id: cid,
            display_name: "controller".into(),
            overlay_ipv4: "10.250.0.1".parse().unwrap(),
            overlay_ipv6: None,
            wg_public_key: wg_pub,
            wg_private_key_encrypted: wg_priv,
            identity_public_key: id_pub,
            identity_private_key_encrypted: id_priv,
            control_parent_id: None,
            node_service_port: std::env::var("PW_NODE_PORT").ok().and_then(|s| s.parse().ok()).unwrap_or(8444u16),
            wg_port_range_start: std::env::var("PW_WG_PORT_START").ok().and_then(|s| s.parse().ok()).unwrap_or(30000u16),
            wg_port_range_end: std::env::var("PW_WG_PORT_END").ok().and_then(|s| s.parse().ok()).unwrap_or(30999u16),
            is_controller: true,
            agent_version: Some("0.1.0".into()),
            protocol_version: 1,
            desired_generation: None,
            active_generation: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
            last_seen_at: Some(chrono::Utc::now()),
            last_handshake_at: None,
            enrollment_token_id: None,
        };
        pathweaver_storage::db::insert_node(db.pool(), &ctrl).await?;
        tracing::info!("控制机自注册: {} (10.250.0.1)", cid);
        cid
    };

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
