use pathweaver_core::models::*;
use pathweaver_core::error::Result;

pub fn compile_route_tables(
    _policy: &TrafficPolicy,
    path_hops: &[TrafficPolicyPath],
    links: &[WireGuardLink],
) -> Result<Vec<RouteTable>> {
    let mut tables = Vec::new();
    let sorted_hops = {
        let mut h = path_hops.to_vec();
        h.sort_by_key(|h| h.hop_order);
        h
    };

    for (i, hop) in sorted_hops.iter().enumerate() {
        let table_id = (i + 100) as u32;

        if let Some(next_hop) = sorted_hops.get(i + 1) {
            let link = find_link_between_nodes(hop.node_id, next_hop.node_id, links);
            if let Some(link) = link {
                let route = Route {
                    destination: "0.0.0.0/0".to_string(),
                    dev: Some(determine_outgoing_interface(link, hop.node_id)),
                    via: None,
                    metric: 10,
                };
                tables.push(RouteTable {
                    id: table_id,
                    routes: vec![route],
                });
            }
        }
    }

    Ok(tables)
}

fn find_link_between_nodes(a: NodeId, b: NodeId, links: &[WireGuardLink]) -> Option<&WireGuardLink> {
    links.iter().find(|l| {
        (l.node_a == a && l.node_b == b) || (l.node_a == b && l.node_b == a)
    })
}

fn determine_outgoing_interface(link: &WireGuardLink, from_node: NodeId) -> String {
    if link.node_a == from_node {
        link.interface_name_a.clone()
    } else {
        link.interface_name_b.clone()
    }
}

pub fn compile_policy_routing_rules(
    policy: &TrafficPolicy,
    path_hops: &[TrafficPolicyPath],
    match_conditions: &[TrafficPolicyMatch],
) -> Vec<PolicyRoutingRule> {
    let mut rules = Vec::new();

    for (i, _hop) in path_hops.iter().enumerate() {
        let table_id = (i + 100) as u32;
        let fwmark = (1000 + i) as u32;

        for mc in match_conditions {
            let rule = PolicyRoutingRule {
                id: uuid::Uuid::new_v4(),
                priority: policy.priority + i as u32,
                fwmark,
                table_id,
                match_src: mc.source_cidr.clone(),
                match_dst: mc.destination_cidr.clone(),
                protocol: Some(match_protocol_str(&mc.protocol)),
                dport: mc.destination_port_start,
            };
            rules.push(rule);
        }
    }

    rules
}

pub fn compile_nftables_rules(
    _policy: &TrafficPolicy,
    match_conditions: &[TrafficPolicyMatch],
    _hops_count: usize,
) -> Vec<String> {
    let mut rules = Vec::new();

    for (i, mc) in match_conditions.iter().enumerate() {
        let fwmark = (1000 + i) as u32;

        let mut rule = format!(
            "add rule ip pathweaver output ",
        );

        if let Some(ref src) = mc.source_cidr {
            rule.push_str(&format!("ip saddr {} ", src));
        }
        if let Some(ref dst) = mc.destination_cidr {
            rule.push_str(&format!("ip daddr {} ", dst));
        }
        if mc.protocol != MatchProtocol::Any {
            rule.push_str(&format!("{} ", match_protocol_nft(&mc.protocol)));
            if let Some(dport) = mc.destination_port_start {
                rule.push_str(&format!("dport {} ", dport));
            }
        }

        rule.push_str(&format!("meta mark set {:#x}", fwmark));
        rules.push(rule);
    }

    rules
}

fn match_protocol_str(p: &MatchProtocol) -> String {
    match p {
        MatchProtocol::Any => "any".to_string(),
        MatchProtocol::Tcp => "tcp".to_string(),
        MatchProtocol::Udp => "udp".to_string(),
        MatchProtocol::Icmp => "icmp".to_string(),
    }
}

fn match_protocol_nft(p: &MatchProtocol) -> String {
    match p {
        MatchProtocol::Tcp => "tcp dport".to_string(),
        MatchProtocol::Udp => "udp dport".to_string(),
        _ => "".to_string(),
    }
}

pub fn compile_desired_state(
    node_id: NodeId,
    generation: u64,
    node: &Node,
    links: &[WireGuardLink],
    policies: &[TrafficPolicy],
    path_hops_map: &std::collections::HashMap<PolicyId, Vec<TrafficPolicyPath>>,
    match_map: &std::collections::HashMap<PolicyId, Vec<TrafficPolicyMatch>>,
) -> Result<NodeDesiredConfig> {
    let my_links: Vec<_> = links
        .iter()
        .filter(|l| l.node_a == node_id || l.node_b == node_id)
        .collect();

    let wg_configs: Vec<_> = my_links
        .iter()
        .map(|link| {
            WireGuardLinkConfig {
                link_id: link.id,
                interface_name: if link.node_a == node_id {
                    link.interface_name_a.clone()
                } else {
                    link.interface_name_b.clone()
                },
                listen_port: if link.listener_node_id == node_id {
                    link.listener_port
                } else {
                    0
                },
                peer_public_key: String::new(),
                peer_endpoint: if link.initiator_node_id == node_id {
                    Some(format!("{}:{}", link.listener_address, link.listener_port))
                } else {
                    None
                },
                persistent_keepalive: if link.initiator_node_id == node_id { 25 } else { 0 },
                is_initiator: link.initiator_node_id == node_id,
                node_private_key: String::new(),
                overlay_ip: node.overlay_ipv4.to_string(),
                peer_overlay_ip: String::new(),
            }
        })
        .collect();

    let mut all_rules = Vec::new();
    let mut all_route_tables = Vec::new();

    for policy in policies {
        if let Some(hops) = path_hops_map.get(&policy.id) {
            if let Some(matches) = match_map.get(&policy.id) {
                all_rules.extend(compile_policy_routing_rules(policy, hops, matches));
                all_route_tables.extend(compile_route_tables(policy, hops, links)?);
            }
        }
    }

    let config_data = format!("{}-{}", node_id, generation);
    let config_hash = pathweaver_security::crypto::sha256_hex(config_data.as_bytes());

    Ok(NodeDesiredConfig {
        id: uuid::Uuid::new_v4(),
        config_revision_id: uuid::Uuid::new_v4(),
        node_id,
        overlay_ipv4: Some(node.overlay_ipv4),
        overlay_ipv6: node.overlay_ipv6.clone(),
        wireguard_links: wg_configs,
        nftables_rules: vec![],
        policy_routing_rules: all_rules,
        route_tables: all_route_tables,
        forwarding_enabled: true,
        nat_rules: vec![],
        control_parent_id: node.control_parent_id,
        recovery_endpoint: None,
        config_hash,
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_compile_policy_rules() {
        let policy = TrafficPolicy {
            id: uuid::Uuid::new_v4(),
            name: "test".into(),
            priority: 10,
            enabled: true,
            description: None,
            created_at: chrono::Utc::now(),
            updated_at: chrono::Utc::now(),
        };

        let matches = vec![TrafficPolicyMatch {
            id: uuid::Uuid::new_v4(),
            policy_id: policy.id,
            source_node_id: None,
            source_node_group: None,
            source_cidr: Some("10.0.0.0/24".into()),
            destination_node_id: None,
            destination_cidr: Some("192.168.1.0/24".into()),
            protocol: MatchProtocol::Tcp,
            destination_port_start: Some(443),
            destination_port_end: Some(443),
        }];

        let hops = vec![
            TrafficPolicyPath { id: uuid::Uuid::new_v4(), policy_id: policy.id, hop_order: 0, node_id: uuid::Uuid::new_v4() },
            TrafficPolicyPath { id: uuid::Uuid::new_v4(), policy_id: policy.id, hop_order: 1, node_id: uuid::Uuid::new_v4() },
        ];

        let rules = compile_policy_routing_rules(&policy, &hops, &matches);
        assert_eq!(rules.len(), 2);
    }
}
