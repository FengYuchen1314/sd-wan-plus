use pathweaver_core::models::*;
use pathweaver_core::error::{PathWeaverError, Result};
use std::collections::{HashMap, HashSet};

pub fn validate_control_tree(
    relations: &[ControlRelation],
    nodes: &[Node],
) -> Result<()> {
    let node_ids: HashSet<NodeId> = nodes.iter().map(|n| n.id).collect();
    let mut parent_count: HashMap<NodeId, usize> = HashMap::new();
    let mut visited = HashSet::new();

    for rel in relations {
        if !node_ids.contains(&rel.parent_id) {
            return Err(PathWeaverError::ParentNotFound(rel.parent_id.to_string()));
        }
        if !node_ids.contains(&rel.child_id) {
            return Err(PathWeaverError::NodeNotFound(rel.child_id.to_string()));
        }
        if rel.parent_id == rel.child_id {
            return Err(PathWeaverError::CircularControlTree);
        }
        *parent_count.entry(rel.child_id).or_insert(0) += 1;
        if parent_count[&rel.child_id] > 1 {
            return Err(PathWeaverError::InternalError(
                format!("node {} has multiple control parents", rel.child_id)
            ));
        }
    }

    for rel in relations {
        if !visited.contains(&rel.child_id) {
            check_circular(rel.child_id, relations, &mut visited, &mut HashSet::new())?;
        }
    }

    Ok(())
}

fn check_circular(
    node_id: NodeId,
    relations: &[ControlRelation],
    visited: &mut HashSet<NodeId>,
    stack: &mut HashSet<NodeId>,
) -> Result<()> {
    if stack.contains(&node_id) {
        return Err(PathWeaverError::CircularControlTree);
    }
    if visited.contains(&node_id) {
        return Ok(());
    }

    visited.insert(node_id);
    stack.insert(node_id);

    if let Some(rel) = relations.iter().find(|r| r.child_id == node_id) {
        check_circular(rel.parent_id, relations, visited, stack)?;
    }

    stack.remove(&node_id);
    Ok(())
}

pub fn build_control_tree_depths(
    relations: &[ControlRelation],
) -> HashMap<NodeId, u32> {
    let mut depths = HashMap::new();
    let mut children: HashMap<NodeId, Vec<NodeId>> = HashMap::new();

    for rel in relations {
        children.entry(rel.parent_id).or_default().push(rel.child_id);
        children.entry(rel.child_id).or_default();
    }

    let roots: Vec<NodeId> = nodes_without_parent(relations);

    for root in roots {
        assign_depths(root, 0, &children, &mut depths);
    }

    depths
}

fn nodes_without_parent(relations: &[ControlRelation]) -> Vec<NodeId> {
    let children: HashSet<NodeId> = relations.iter().map(|r| r.child_id).collect();
    relations
        .iter()
        .map(|r| r.parent_id)
        .filter(|id| !children.contains(id))
        .collect()
}

fn assign_depths(
    node_id: NodeId,
    depth: u32,
    children: &HashMap<NodeId, Vec<NodeId>>,
    depths: &mut HashMap<NodeId, u32>,
) {
    depths.insert(node_id, depth);
    if let Some(kids) = children.get(&node_id) {
        for kid in kids {
            assign_depths(*kid, depth + 1, children, depths);
        }
    }
}

pub fn build_update_order(
    relations: &[ControlRelation],
) -> Vec<NodeId> {
    let depths = build_control_tree_depths(relations);
    let mut nodes: Vec<(NodeId, u32)> = depths.into_iter().collect();
    nodes.sort_by_key(|(_, depth)| std::cmp::Reverse(*depth));
    nodes.into_iter().map(|(id, _)| id).collect()
}

pub fn validate_wireguard_path(
    path: &[NodeId],
    links: &[WireGuardLink],
) -> Result<()> {
    if path.len() < 2 {
        return Err(PathWeaverError::ValidationError("path must have at least 2 nodes".into()));
    }

    let mut seen = HashSet::new();
    for node_id in path {
        if !seen.insert(*node_id) {
            return Err(PathWeaverError::CircularPath);
        }
    }

    for window in path.windows(2) {
        let a = window[0];
        let b = window[1];
        if !link_exists(a, b, links) {
            return Err(PathWeaverError::PathMissingLink(
                a.to_string(),
                b.to_string(),
            ));
        }
    }

    Ok(())
}

fn link_exists(a: NodeId, b: NodeId, links: &[WireGuardLink]) -> bool {
    links.iter().any(|l| {
        (l.node_a == a && l.node_b == b) || (l.node_a == b && l.node_b == a)
    })
}

pub fn check_path_conflicts(
    policies: &[TrafficPolicy],
    matches: &[TrafficPolicyMatch],
) -> Vec<(TrafficPolicy, TrafficPolicy)> {
    let mut conflicts = Vec::new();

    for i in 0..policies.len() {
        for j in (i + 1)..policies.len() {
            let p1 = &policies[i];
            let p2 = &policies[j];
            if p1.priority == p2.priority {
                let m1: Vec<_> = matches.iter().filter(|m| m.policy_id == p1.id).collect();
                let m2: Vec<_> = matches.iter().filter(|m| m.policy_id == p2.id).collect();

                for a in &m1 {
                    for b in &m2 {
                        if match_overlaps(a, b) {
                            conflicts.push((p1.clone(), p2.clone()));
                        }
                    }
                }
            }
        }
    }

    conflicts
}

fn match_overlaps(a: &TrafficPolicyMatch, b: &TrafficPolicyMatch) -> bool {
    let src_eq = a.source_node_id == b.source_node_id
        && a.source_cidr == b.source_cidr
        && a.source_node_group == b.source_node_group;

    let dst_eq = a.destination_node_id == b.destination_node_id
        && a.destination_cidr == b.destination_cidr;

    let proto_eq = a.protocol == b.protocol || a.protocol == MatchProtocol::Any || b.protocol == MatchProtocol::Any;

    let port_overlap = a.destination_port_start.is_none()
        || b.destination_port_start.is_none()
        || port_ranges_overlap(
            a.destination_port_start.unwrap_or(0),
            a.destination_port_end.unwrap_or(65535),
            b.destination_port_start.unwrap_or(0),
            b.destination_port_end.unwrap_or(65535),
        );

    src_eq && dst_eq && proto_eq && port_overlap
}

fn port_ranges_overlap(a_start: u16, a_end: u16, b_start: u16, b_end: u16) -> bool {
    a_start <= b_end && b_start <= a_end
}

#[cfg(test)]
mod tests {
    use super::*;
    use uuid::Uuid;

    fn make_node() -> NodeId {
        Uuid::new_v4()
    }

    fn make_test_node(id: NodeId) -> Node {
        Node {
            id,
            display_name: format!("node-{}", &id.to_string()[..8]),
            overlay_ipv4: "10.250.0.1".parse().unwrap(),
            overlay_ipv6: None,
            wg_public_key: String::new(),
            wg_private_key_encrypted: String::new(),
            identity_public_key: String::new(),
            identity_private_key_encrypted: String::new(),
            control_parent_id: None,
            node_service_port: 8444,
            wg_port_range_start: 30000,
            wg_port_range_end: 30999,
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
        }
    }

    #[test]
    fn test_circular_detection() {
        let a = make_node();
        let b = make_node();
        let c = make_node();

        let relations = vec![
            ControlRelation {
                id: Uuid::new_v4(), parent_id: a, child_id: b,
                enrolled_at: chrono::Utc::now(),
                enrollment_token_id: Uuid::new_v4(),
            },
            ControlRelation {
                id: Uuid::new_v4(), parent_id: b, child_id: c,
                enrolled_at: chrono::Utc::now(),
                enrollment_token_id: Uuid::new_v4(),
            },
        ];

        let nodes: Vec<Node> = [a, b, c].iter().map(|id| make_test_node(*id)).collect();
        assert!(validate_control_tree(&relations, &nodes).is_ok());
    }

    #[test]
    fn test_update_order_leaves_first() {
        let root = make_node();
        let a = make_node();
        let b = make_node();
        let c = make_node();

        let relations = vec![
            ControlRelation { id: Uuid::new_v4(), parent_id: root, child_id: a,
                enrolled_at: chrono::Utc::now(), enrollment_token_id: Uuid::new_v4() },
            ControlRelation { id: Uuid::new_v4(), parent_id: a, child_id: b,
                enrolled_at: chrono::Utc::now(), enrollment_token_id: Uuid::new_v4() },
            ControlRelation { id: Uuid::new_v4(), parent_id: a, child_id: c,
                enrolled_at: chrono::Utc::now(), enrollment_token_id: Uuid::new_v4() },
        ];

        let order = build_update_order(&relations);
        assert!(order[0] != root, "root should be last");
        assert_eq!(*order.last().unwrap(), root, "root should be last");
    }
}
