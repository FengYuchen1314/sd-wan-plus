use std::sync::Arc;
use tokio::sync::RwLock;
use uuid::Uuid;
use pathweaver_core::models::*;
use pathweaver_core::state::AgentState;
use pathweaver_security::crypto;

pub mod bootstrap;

pub struct AgentConfig {
    pub identity_key_path: String,
    pub wg_private_key_path: String,
    pub parent_address: Option<String>,
    pub parent_service_port: u16,
    pub node_service_port: u16,
    pub wg_port_range_start: u16,
    pub wg_port_range_end: u16,
    pub enroll_token: Option<String>,
}

pub struct Agent {
    state: Arc<RwLock<AgentStateStore>>,
    config: AgentConfig,
}

pub struct AgentStateStore {
    pub node_id: Option<NodeId>,
    pub state: AgentState,
    pub config_generation: Option<u64>,
    pub overlay_ip: Option<std::net::Ipv4Addr>,
    pub identity_public_key: Option<String>,
    pub wg_public_key: Option<String>,
    pub parent_wg_public_key: Option<String>,
    pub parent_wg_endpoint: Option<String>,
    pub recovery_endpoint: Option<String>,
}

impl Agent {
    pub fn new(config: AgentConfig) -> Self {
        Self {
            state: Arc::new(RwLock::new(AgentStateStore {
                node_id: None,
                state: AgentState::Bootstrap,
                config_generation: None,
                overlay_ip: None,
                identity_public_key: None,
                wg_public_key: None,
                parent_wg_public_key: None,
                parent_wg_endpoint: None,
                recovery_endpoint: None,
            })),
            config,
        }
    }

    pub async fn get_state(&self) -> AgentState {
        self.state.read().await.state.clone()
    }

    pub async fn set_state(&self, new_state: AgentState) {
        self.state.write().await.state = new_state;
    }

    pub async fn get_node_id(&self) -> Option<NodeId> {
        self.state.read().await.node_id
    }

    pub async fn run(&self) -> anyhow::Result<()> {
        tracing::info!("PathWeaver agent starting...");

        self.generate_keys_if_needed().await?;
        self.set_state(AgentState::Bootstrap).await;

        match &self.config.enroll_token {
            Some(token) if !token.is_empty() => {
                self.set_state(AgentState::Registering).await;
                self.enroll_with_token(token).await?;
            }
            _ => {
                tracing::info!("No enrollment token, starting in regular mode");
                self.set_state(AgentState::OverlayConnecting).await;
            }
        }

        self.run_heartbeat_loop().await?;
        Ok(())
    }

    async fn generate_keys_if_needed(&self) -> anyhow::Result<()> {
        let mut state = self.state.write().await;

        if state.identity_public_key.is_none() {
            let (private, public) = crypto::generate_identity_keypair();
            state.identity_public_key = Some(public);
            tracing::info!("Generated identity key pair");
        }

        if state.wg_public_key.is_none() {
            let (_private, public) = crypto::generate_wireguard_keypair();
            state.wg_public_key = Some(public);
            tracing::info!("Generated WireGuard key pair");
        }

        Ok(())
    }

    async fn enroll_with_token(&self, token: &str) -> anyhow::Result<()> {
        let parent_addr = self.config.parent_address.as_deref().unwrap_or("");
        let parent_port = self.config.parent_service_port;

        tracing::info!("Enrolling with parent node at {}:{}", parent_addr, parent_port);

        let state = self.state.read().await;
        let wg_public = state.wg_public_key.clone().unwrap_or_default();
        let identity_public = state.identity_public_key.clone().unwrap_or_default();
        drop(state);

        let enrollment_url = format!(
            "https://{}:{}/bootstrap/enroll",
            parent_addr, parent_port
        );

        let request_body = serde_json::json!({
            "token": token,
            "node_name": "new-node",
            "wg_public_key": wg_public,
            "identity_public_key": identity_public,
            "agent_version": "0.1.0",
            "protocol_version": 1,
        });

        let client = reqwest::Client::builder()
            .danger_accept_invalid_certs(true)
            .build()?;

        let response = client
            .post(&enrollment_url)
            .json(&request_body)
            .send()
            .await;

        match response {
            Ok(resp) if resp.status().is_success() => {
                let result: serde_json::Value = resp.json().await?;

                let node_id: Uuid = result["node_id"].as_str()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or_else(|| {
                        tracing::warn!("No valid node_id in response, using generated ID");
                        uuid::Uuid::new_v4()
                    });

                let overlay_ip: std::net::Ipv4Addr = result["overlay_ipv4"]
                    .as_str()
                    .and_then(|s| s.parse().ok())
                    .unwrap_or_else(|| "10.250.0.1".parse().unwrap());

                let parent_wg_key = result["parent_wg_public_key"].as_str().unwrap_or("").to_string();
                let parent_wg_endpoint = result["parent_wg_endpoint"].as_str().unwrap_or("").to_string();
                let recovery = result["recovery_endpoint"].as_str().unwrap_or("").to_string();

                let mut state = self.state.write().await;
                state.node_id = Some(node_id);
                state.overlay_ip = Some(overlay_ip);
                state.parent_wg_public_key = Some(parent_wg_key);
                state.parent_wg_endpoint = Some(parent_wg_endpoint);
                state.recovery_endpoint = Some(recovery);

                tracing::info!("Enrollment successful. Node ID: {}, Overlay IP: {}", node_id, overlay_ip);

                self.set_state(AgentState::OverlayConnecting).await;
                Ok(())
            }
            Ok(resp) => {
                let status = resp.status();
                let body = resp.text().await.unwrap_or_default();
                anyhow::bail!("Enrollment failed ({}): {}", status, body);
            }
            Err(e) => {
                anyhow::bail!("Enrollment request failed: {}", e);
            }
        }
    }

    async fn run_heartbeat_loop(&self) -> anyhow::Result<()> {
        tracing::info!("Heartbeat loop started");
        loop {
            tokio::time::sleep(tokio::time::Duration::from_secs(30)).await;
            let state = self.state.read().await;
            if let Some(node_id) = state.node_id {
                tracing::debug!("Heartbeat from node {}", node_id);
            }
        }
    }
}
