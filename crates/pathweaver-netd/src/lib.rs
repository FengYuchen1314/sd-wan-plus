use serde::{Deserialize, Serialize};
use pathweaver_core::models::{WireGuardLinkConfig, NatRule, RouteTable};

pub struct NetworkManager {
    wireguard_interfaces: Vec<String>,
    route_tables: Vec<u32>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct WireGuardInterface {
    pub name: String,
    pub private_key: String,
    pub listen_port: u16,
    pub address: String,
    pub peer_public_key: String,
    pub peer_endpoint: Option<String>,
    pub peer_allowed_ips: Vec<String>,
    pub persistent_keepalive: u16,
}

impl NetworkManager {
    pub fn new() -> Self {
        Self {
            wireguard_interfaces: Vec::new(),
            route_tables: Vec::new(),
        }
    }

    pub fn create_wireguard_interface(
        &mut self,
        config: &WireGuardLinkConfig,
    ) -> anyhow::Result<()> {
        tracing::info!(
            "Creating WireGuard interface: {} (port {})",
            config.interface_name,
            config.listen_port
        );

        self.wireguard_interfaces.push(config.interface_name.clone());
        Ok(())
    }

    pub fn remove_wireguard_interface(&mut self, name: &str) -> anyhow::Result<()> {
        tracing::info!("Removing WireGuard interface: {}", name);
        self.wireguard_interfaces.retain(|i| i != name);
        Ok(())
    }

    pub fn create_dummy_interface(&self, name: &str, address: &str) -> anyhow::Result<()> {
        tracing::info!("Creating dummy interface: {} ({})", name, address);
        Ok(())
    }

    pub fn add_ip_rule(&self, fwmark: u32, table_id: u32) -> anyhow::Result<()> {
        tracing::info!("Adding ip rule: fwmark {} -> table {}", fwmark, table_id);
        Ok(())
    }

    pub fn add_route(&self, table_id: u32, route: &RouteTable) -> anyhow::Result<()> {
        tracing::info!("Adding routes to table {}", table_id);
        Ok(())
    }

    pub fn enable_forwarding(&self) -> anyhow::Result<()> {
        tracing::info!("Enabling IP forwarding");
        Ok(())
    }

    pub fn add_nat_rule(&self, rule: &NatRule) -> anyhow::Result<()> {
        tracing::info!(
            "Adding NAT rule: {:?} (type: {:?})",
            rule.out_interface,
            rule.rule_type
        );
        Ok(())
    }

    pub fn rollback(&self) -> anyhow::Result<()> {
        tracing::info!("Rolling back network configuration");
        Ok(())
    }
}
