use pathweaver_agent::Agent;

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    tracing_subscriber::fmt()
        .with_env_filter("info")
        .init();

    let config = pathweaver_agent::AgentConfig {
        identity_key_path: "/opt/pathweaver/identity.key".into(),
        wg_private_key_path: "/opt/pathweaver/wg.key".into(),
        parent_address: None,
        parent_service_port: 8444,
        node_service_port: 8444,
        wg_port_range_start: 30000,
        wg_port_range_end: 30999,
        enroll_token: None,
    };

    let agent = Agent::new(config);
    agent.run().await?;

    Ok(())
}
