export interface Node {
  id: string;
  display_name: string;
  overlay_ipv4: string;
  overlay_ipv6: string | null;
  is_controller: boolean;
  control_parent_id: string | null;
  control_parent: string | null;
  agent_version: string | null;
  protocol_version: number;
  last_seen_at: string | null;
  last_handshake_at: string | null;
  node_service_port: number;
  wg_port_range_start: number;
  wg_port_range_end: number;
  wg_public_key: string;
  desired_generation: number | null;
  active_generation: number | null;
  addresses: NodeAddress[];
  links: WireGuardLink[];
}

export interface NodeAddress {
  id: string;
  node_id: string;
  address: string;
  address_type: string;
  is_primary: boolean;
}

export interface WireGuardLink {
  id: string;
  node_a: string;
  node_b: string;
  node_a_name: string;
  node_b_name: string;
  initiator_node_id: string;
  listener_node_id: string;
  listener_address: string;
  listener_port: number;
  interface_name_a: string;
  interface_name_b: string;
  enabled: boolean;
  admin_weight: number;
  status: string;
  last_handshake_a: string | null;
  last_handshake_b: string | null;
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
  created_at: string;
  updated_at: string;
}

export interface TrafficPolicyMatch {
  id: string;
  policy_id: string;
  source_node_id: string | null;
  source_cidr: string | null;
  destination_node_id: string | null;
  destination_cidr: string | null;
  protocol: string;
  destination_port_start: number | null;
  destination_port_end: number | null;
}

export interface TrafficPolicyPath {
  id: string;
  policy_id: string;
  hop_order: number;
  node_id: string;
  node_name?: string;
}

export interface TrafficPolicyConfig {
  id: string;
  policy_id: string;
  egress_nat: boolean;
  return_path_type: string;
  egress_node_id: string | null;
  failover_enabled: boolean;
}

export interface TokenResponse {
  token_id: string;
  token: string;
  install_command: string;
  expires_at: string;
  parent_node_name: string;
}

export interface ConfigPreview {
  generation: number;
  node_configs: NodeConfigPreview[];
  validation_errors: string[];
  ready: boolean;
}

export interface NodeConfigPreview {
  node_id: string;
  display_name: string;
  overlay_ipv4: string;
  wireguard_links: unknown[];
  policy_routing_rules: unknown[];
  route_tables: unknown[];
  nat_rules: unknown[];
}

export interface UpdateJob {
  id: string;
  target_version: string;
  manifest_sha256: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  targets: UpdateTarget[];
  summary: UpdateSummary;
}

export interface UpdateTarget {
  id: string;
  update_job_id: string;
  node_id: string;
  node_name: string;
  depth: number;
  status: string;
  error_message: string | null;
}

export interface UpdateSummary {
  total: number;
  completed: number;
  failed: number;
  waiting: number;
  in_progress: number;
}

export interface ConfigRevision {
  id: string;
  generation: number;
  reason: string;
  status: string;
  created_at: string;
  completed_at: string | null;
  rollout: ConfigRolloutNode[];
}

export interface ConfigRolloutNode {
  id: string;
  config_revision_id: string;
  node_id: string;
  status: string;
  error_message: string | null;
}

export interface AuditLog {
  id: string;
  admin_id: string | null;
  action: string;
  resource_type: string;
  resource_id: string | null;
  details: string | null;
  ip_address: string;
  created_at: string;
}

export interface SessionInfo {
  id: string;
  ip_address: string;
  user_agent: string;
  created_at: string;
  expires_at: string;
  revoked: boolean;
}

export interface AdminInfo {
  id: string;
  username: string;
  created_at: string;
}

export interface PublishResult {
  revision_id: string;
  generation: number;
  status: string;
  node_count: number;
}
