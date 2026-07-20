use pathweaver_controller;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let db_path = std::env::temp_dir().join("pathweaver.db");
    let db_url = db_path.to_string_lossy().to_string();

    let bind_addr = format!(
        "0.0.0.0:{}",
        std::env::var("PW_WEB_PORT").unwrap_or_else(|_| "8443".into())
    );

    pathweaver_controller::run_controller(&db_url, &bind_addr).await
}
