CREATE TABLE IF NOT EXISTS admin (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    admin_id TEXT NOT NULL REFERENCES admin(id),
    token TEXT NOT NULL UNIQUE,
    ip_address TEXT NOT NULL,
    user_agent TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS networks (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    overlay_ipv4_cidr TEXT NOT NULL,
    overlay_ipv6_cidr TEXT,
    ipv6_enabled INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS nodes (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    overlay_ipv4 TEXT NOT NULL,
    overlay_ipv6 TEXT,
    wg_public_key TEXT NOT NULL,
    wg_private_key_encrypted TEXT NOT NULL,
    identity_public_key TEXT NOT NULL,
    identity_private_key_encrypted TEXT NOT NULL,
    control_parent_id TEXT REFERENCES nodes(id),
    node_service_port INTEGER NOT NULL,
    wg_port_range_start INTEGER NOT NULL,
    wg_port_range_end INTEGER NOT NULL,
    is_controller INTEGER NOT NULL DEFAULT 0,
    agent_version TEXT,
    protocol_version INTEGER NOT NULL DEFAULT 1,
    desired_generation INTEGER,
    active_generation INTEGER,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    last_seen_at TEXT,
    last_handshake_at TEXT,
    enrollment_token_id TEXT
);

CREATE TABLE IF NOT EXISTS node_addresses (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    address TEXT NOT NULL,
    address_type TEXT NOT NULL,
    is_primary INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS node_port_pools (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    protocol TEXT NOT NULL,
    port_range_start INTEGER NOT NULL,
    port_range_end INTEGER NOT NULL,
    allocated_ports TEXT NOT NULL DEFAULT '[]'
);

CREATE TABLE IF NOT EXISTS control_relations (
    id TEXT PRIMARY KEY,
    parent_id TEXT NOT NULL REFERENCES nodes(id),
    child_id TEXT NOT NULL REFERENCES nodes(id),
    enrolled_at TEXT NOT NULL,
    enrollment_token_id TEXT NOT NULL,
    UNIQUE(child_id)
);

CREATE TABLE IF NOT EXISTS wireguard_links (
    id TEXT PRIMARY KEY,
    node_a TEXT NOT NULL REFERENCES nodes(id),
    node_b TEXT NOT NULL REFERENCES nodes(id),
    initiator_node_id TEXT NOT NULL REFERENCES nodes(id),
    listener_node_id TEXT NOT NULL REFERENCES nodes(id),
    listener_address TEXT NOT NULL,
    listener_port INTEGER NOT NULL,
    interface_name_a TEXT NOT NULL,
    interface_name_b TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 0,
    admin_weight INTEGER NOT NULL DEFAULT 1,
    last_handshake_a TEXT,
    last_handshake_b TEXT,
    status TEXT NOT NULL DEFAULT 'Pending',
    created_at TEXT NOT NULL,
    UNIQUE(node_a, node_b)
);

CREATE TABLE IF NOT EXISTS wireguard_link_endpoints (
    id TEXT PRIMARY KEY,
    link_id TEXT NOT NULL REFERENCES wireguard_links(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    interface_name TEXT NOT NULL,
    listen_port INTEGER NOT NULL,
    peer_endpoint TEXT,
    peer_public_key TEXT NOT NULL,
    persistent_keepalive INTEGER NOT NULL DEFAULT 0,
    is_initiator INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS traffic_policies (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    priority INTEGER NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS traffic_policy_matches (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES traffic_policies(id),
    source_node_id TEXT REFERENCES nodes(id),
    source_node_group TEXT,
    source_cidr TEXT,
    destination_node_id TEXT REFERENCES nodes(id),
    destination_cidr TEXT,
    protocol TEXT NOT NULL DEFAULT 'Any',
    destination_port_start INTEGER,
    destination_port_end INTEGER
);

CREATE TABLE IF NOT EXISTS traffic_policy_paths (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES traffic_policies(id),
    hop_order INTEGER NOT NULL,
    node_id TEXT NOT NULL REFERENCES nodes(id)
);

CREATE TABLE IF NOT EXISTS traffic_policy_configs (
    id TEXT PRIMARY KEY,
    policy_id TEXT NOT NULL REFERENCES traffic_policies(id),
    egress_node_id TEXT REFERENCES nodes(id),
    egress_nat INTEGER NOT NULL DEFAULT 0,
    return_path_type TEXT NOT NULL DEFAULT 'Symmetric',
    failover_enabled INTEGER NOT NULL DEFAULT 0,
    failover_path_id TEXT
);

CREATE TABLE IF NOT EXISTS config_revisions (
    id TEXT PRIMARY KEY,
    generation INTEGER NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'Draft',
    created_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS node_desired_configs (
    id TEXT PRIMARY KEY,
    config_revision_id TEXT NOT NULL REFERENCES config_revisions(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    config_data TEXT NOT NULL,
    config_hash TEXT NOT NULL,
    UNIQUE(config_revision_id, node_id)
);

CREATE TABLE IF NOT EXISTS config_rollout_nodes (
    id TEXT PRIMARY KEY,
    config_revision_id TEXT NOT NULL REFERENCES config_revisions(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    status TEXT NOT NULL DEFAULT 'Pending',
    started_at TEXT,
    completed_at TEXT,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS heartbeats (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    agent_version TEXT NOT NULL,
    protocol_version INTEGER NOT NULL,
    active_generation INTEGER,
    wireguard_snapshots TEXT NOT NULL DEFAULT '[]',
    received_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS wireguard_snapshots (
    id TEXT PRIMARY KEY,
    heartbeat_id TEXT NOT NULL REFERENCES heartbeats(id),
    link_id TEXT NOT NULL,
    interface_name TEXT NOT NULL,
    last_handshake TEXT,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    peer_public_key TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS probe_jobs (
    id TEXT PRIMARY KEY,
    probe_type TEXT NOT NULL,
    source_node_id TEXT NOT NULL REFERENCES nodes(id),
    target_node_id TEXT REFERENCES nodes(id),
    policy_id TEXT REFERENCES traffic_policies(id),
    created_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS probe_results (
    id TEXT PRIMARY KEY,
    probe_job_id TEXT NOT NULL REFERENCES probe_jobs(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    link_id TEXT REFERENCES wireguard_links(id),
    success_count INTEGER NOT NULL DEFAULT 0,
    failure_count INTEGER NOT NULL DEFAULT 0,
    packet_loss_pct REAL NOT NULL DEFAULT 0.0,
    rtt_min_ms REAL NOT NULL DEFAULT 0.0,
    rtt_median_ms REAL NOT NULL DEFAULT 0.0,
    rtt_p95_ms REAL NOT NULL DEFAULT 0.0,
    jitter_ms REAL NOT NULL DEFAULT 0.0,
    probed_at TEXT NOT NULL,
    is_stale INTEGER NOT NULL DEFAULT 0,
    error_code TEXT
);

CREATE TABLE IF NOT EXISTS enrollment_tokens (
    id TEXT PRIMARY KEY,
    token TEXT NOT NULL UNIQUE,
    network_id TEXT NOT NULL,
    parent_node_id TEXT NOT NULL REFERENCES nodes(id),
    suggested_node_name TEXT NOT NULL,
    allowed_install_mode TEXT NOT NULL DEFAULT 'node',
    expires_at TEXT NOT NULL,
    used_at TEXT,
    revoked INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifacts (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    version TEXT NOT NULL,
    architecture TEXT NOT NULL,
    sha256 TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    manifest_id TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifact_cache_records (
    id TEXT PRIMARY KEY,
    node_id TEXT NOT NULL REFERENCES nodes(id),
    artifact_id TEXT NOT NULL REFERENCES artifacts(id),
    cached_at TEXT NOT NULL,
    is_complete INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS update_jobs (
    id TEXT PRIMARY KEY,
    target_version TEXT NOT NULL,
    manifest_sha256 TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'Created',
    created_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE IF NOT EXISTS update_targets (
    id TEXT PRIMARY KEY,
    update_job_id TEXT NOT NULL REFERENCES update_jobs(id),
    node_id TEXT NOT NULL REFERENCES nodes(id),
    depth INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'Waiting',
    started_at TEXT,
    completed_at TEXT,
    error_message TEXT
);

CREATE TABLE IF NOT EXISTS audit_logs (
    id TEXT PRIMARY KEY,
    admin_id TEXT,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT,
    details TEXT,
    ip_address TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
CREATE INDEX IF NOT EXISTS idx_sessions_admin ON sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_nodes_parent ON nodes(control_parent_id);
CREATE INDEX IF NOT EXISTS idx_nodes_display ON nodes(display_name);
CREATE INDEX IF NOT EXISTS idx_links_nodes ON wireguard_links(node_a, node_b);
CREATE INDEX IF NOT EXISTS idx_control_relations_parent ON control_relations(parent_id);
CREATE INDEX IF NOT EXISTS idx_control_relations_child ON control_relations(child_id);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_token ON enrollment_tokens(token);
CREATE INDEX IF NOT EXISTS idx_enrollment_tokens_parent ON enrollment_tokens(parent_node_id);
CREATE INDEX IF NOT EXISTS idx_update_targets_job ON update_targets(update_job_id);
CREATE INDEX IF NOT EXISTS idx_update_targets_node ON update_targets(node_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_admin ON audit_logs(admin_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created ON audit_logs(created_at);
