use pathweaver_core::models::*;
use pathweaver_core::error::{PathWeaverError, Result};
use uuid::Uuid;
use std::collections::HashMap;

pub struct WireGuardCompiler;

impl WireGuardCompiler {
    pub fn compile_parent_child_link(
        parent: &Node,
        child: &Node,
    ) -> Result<WireGuardLink> {
        let link_id = Uuid::new_v4();
        let empty: Vec<WireGuardLink> = Vec::new();

        let child_port = allocate_port(child, &empty)?;
        let parent_port = allocate_port(parent, &[])?;

        let interface_name_child = format!("pwl-{}", &child.display_name);
        let interface_name_parent = format!("pwl-{}", &parent.display_name);

        Ok(WireGuardLink {
            id: link_id,
            node_a: parent.id,
            node_b: child.id,
            initiator_node_id: child.id,
            listener_node_id: parent.id,
            listener_address: parent.overlay_ipv4.to_string(),
            listener_port: parent_port,
            interface_name_a: interface_name_parent,
            interface_name_b: interface_name_child,
            enabled: true,
            admin_weight: 1,
            last_handshake_a: None,
            last_handshake_b: None,
            status: LinkStatus::Pending,
            created_at: chrono::Utc::now(),
        })
    }

    pub fn compile_cross_link(
        initiator: &Node,
        listener: &Node,
        listener_address: &str,
        listener_port: u16,
        link_name: Option<&str>,
    ) -> Result<WireGuardLink> {
        let link_id = Uuid::new_v4();
        let default_suffix = format!("{}-{}",
            &initiator.display_name[..4.min(initiator.display_name.len())],
            &listener.display_name[..4.min(listener.display_name.len())]
        );
        let name_suffix = link_name.unwrap_or(&default_suffix);

        let interface_initiator = format!("pwl-i-{}", name_suffix);
        let interface_listener = format!("pwl-l-{}", name_suffix);

        Ok(WireGuardLink {
            id: link_id,
            node_a: initiator.id,
            node_b: listener.id,
            initiator_node_id: initiator.id,
            listener_node_id: listener.id,
            listener_address: listener_address.to_string(),
            listener_port,
            interface_name_a: interface_initiator,
            interface_name_b: interface_listener,
            enabled: true,
            admin_weight: 1,
            last_handshake_a: None,
            last_handshake_b: None,
            status: LinkStatus::Pending,
            created_at: chrono::Utc::now(),
        })
    }

    pub fn generate_child_wg_config(
        link: &WireGuardLink,
        child: &Node,
        parent: &Node,
    ) -> WireGuardLinkConfig {
        WireGuardLinkConfig {
            link_id: link.id,
            interface_name: if child.id == link.initiator_node_id {
                link.interface_name_b.clone()
            } else {
                link.interface_name_a.clone()
            },
            listen_port: if child.id == link.listener_node_id {
                link.listener_port
            } else {
                0
            },
            peer_public_key: parent.wg_public_key.clone(),
            peer_endpoint: Some(format!("{}:{}", link.listener_address, link.listener_port)),
            persistent_keepalive: 25,
            is_initiator: true,
            node_private_key: child.wg_private_key_encrypted.clone(),
            overlay_ip: format!("{}/32", child.overlay_ipv4),
            peer_overlay_ip: format!("{}/32", parent.overlay_ipv4),
        }
    }

    pub fn generate_parent_wg_config(
        link: &WireGuardLink,
        parent: &Node,
        child: &Node,
    ) -> WireGuardLinkConfig {
        WireGuardLinkConfig {
            link_id: link.id,
            interface_name: if parent.id == link.node_a {
                link.interface_name_a.clone()
            } else {
                link.interface_name_b.clone()
            },
            listen_port: link.listener_port,
            peer_public_key: child.wg_public_key.clone(),
            peer_endpoint: None,
            persistent_keepalive: 0,
            is_initiator: false,
            node_private_key: parent.wg_private_key_encrypted.clone(),
            overlay_ip: format!("{}/32", parent.overlay_ipv4),
            peer_overlay_ip: format!("{}/32", child.overlay_ipv4),
        }
    }
}

pub fn allocate_port(node: &Node, existing_links: &[WireGuardLink]) -> Result<u16> {
    let start = node.wg_port_range_start;
    let end = node.wg_port_range_end;

    let used_ports: std::collections::HashSet<u16> = existing_links
        .iter()
        .filter(|l| l.node_a == node.id || l.node_b == node.id)
        .flat_map(|l| {
            let mut ports = Vec::new();
            if l.node_a == node.id || l.listener_node_id == node.id {
                ports.push(l.listener_port);
            }
            ports
        })
        .collect();

    for port in start..=end {
        if !used_ports.contains(&port) {
            return Ok(port);
        }
    }

    Err(PathWeaverError::NoAvailablePort)
}

pub fn link_to_desired_configs(
    link: &WireGuardLink,
    node_a: &Node,
    node_b: &Node,
) -> (WireGuardLinkConfig, WireGuardLinkConfig) {
    let config_a = if link.initiator_node_id == node_a.id {
        WireGuardCompiler::generate_child_wg_config(link, node_a, node_b)
    } else {
        WireGuardCompiler::generate_parent_wg_config(link, node_a, node_b)
    };

    let config_b = if link.initiator_node_id == node_b.id {
        WireGuardCompiler::generate_child_wg_config(link, node_b, node_a)
    } else {
        WireGuardCompiler::generate_parent_wg_config(link, node_b, node_a)
    };

    (config_a, config_b)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_port_allocation() {
        let node = Node {
            id: Uuid::new_v4(),
            display_name: "test".into(),
            overlay_ipv4: "10.250.0.1".parse().unwrap(),
            overlay_ipv6: None,
            wg_public_key: String::new(),
            wg_private_key_encrypted: String::new(),
            identity_public_key: String::new(),
            identity_private_key_encrypted: String::new(),
            control_parent_id: None,
            node_service_port: 8444,
            wg_port_range_start: 30000,
            wg_port_range_end: 30010,
            is_controller: false,
            agent_version: None,
            protocol_version: 1,
            desired_generation: None,
            active_generation: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
            last_seen_at: None,
            last_handshake_at: None,
            enrollment_token_id: None,
        };

        let port1 = allocate_port(&node, &[]).unwrap();
        assert!(port1 >= 30000 && port1 <= 30010);
    }
}
