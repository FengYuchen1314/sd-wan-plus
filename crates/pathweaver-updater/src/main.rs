use pathweaver_updater::Updater;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter("info")
        .init();

    let updater = Updater::new("/opt/pathweaver", "0.1.0");
    tracing::info!("PathWeaver updater started");

    Ok(())
}
