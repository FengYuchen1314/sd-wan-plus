use pathweaver_netd::NetworkManager;

fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter("info")
        .init();

    let mut nm = NetworkManager::new();
    tracing::info!("PathWeaver netd started");

    Ok(())
}
