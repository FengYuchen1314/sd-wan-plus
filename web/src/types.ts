export interface Node {
  id: string;
  display_name: string;
  overlay_ipv4: string;
  is_controller: boolean;
  control_parent_id: string | null;
  agent_version: string | null;
  protocol_version: number;
  last_seen_at: string | null;
  last_handshake_at: string | null;
  node_service_port: number;
}

export interface WireGuardLink {
  id: string;
  node_a: string;
  node_b: string;
  initiator_node_id: string;
  listener_node_id: string;
  listener_address: string;
  listener_port: number;
  interface_name_a: string;
  interface_name_b: string;
  enabled: boolean;
  status: string;
}

export interface PolicyDetail {
  policy: TrafficPolicy;
  matches: TrafficPolicyMatch[];
  path: TrafficPolicyPath[];
  config: TrafficPolicyConfig | null;
}

export interface TrafficPolicy {
  id: string;
  name: string;
  priority: number;
  enabled: boolean;
  description: string | null;
}

export interface TrafficPolicyMatch {
  id: string;
  policy_id: string;
  source_cidr: string | null;
  destination_cidr: string | null;
  protocol: string;
  destination_port_start: number | null;
}

export interface TrafficPolicyPath {
  id: string;
  policy_id: string;
  hop_order: number;
  node_id: string;
}

export interface TrafficPolicyConfig {
  egress_nat: boolean;
  return_path_type: string;
  egress_node_id: string | null;
}

export interface TokenResponse {
  token_id: string;
  token: string;
  install_command: string;
  expires_at: string;
}

export interface ConfigPreview {
  generation: number;
  node_configs: NodeConfigPreview[];
  validation_errors: string[];
}

export interface NodeConfigPreview {
  node_id: string;
  display_name: string;
  overlay_ip: string;
  wireguard_links: unknown[];
  policy_routing_rules: unknown[];
  route_tables: unknown[];
  nat_rules: unknown[];
}

export interface UpdateJob {
  id: string;
  target_version: string;
  status: string;
  created_at: string;
}

export interface UpdateTarget {
  node_id: string;
  depth: number;
  status: string;
  error_message: string | null;
}
