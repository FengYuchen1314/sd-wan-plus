use pathweaver_controller;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    let db_path = std::env::temp_dir().join("pathweaver.db");
    let db_url = db_path.to_string_lossy().to_string();
    eprintln!("Using database: {}", db_url);
    pathweaver_controller::run_controller(&db_url, "127.0.0.1:8443").await
}
