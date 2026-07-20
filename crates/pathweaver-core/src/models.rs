use serde::{Deserialize, Serialize};
use uuid::Uuid;
use chrono::{DateTime, Utc};
use std::net::Ipv4Addr;

pub type NodeId = Uuid;
pub type LinkId = Uuid;
pub type PolicyId = Uuid;
pub type TokenId = Uuid;
pub type NetworkId = Uuid;
pub type ArtifactId = Uuid;
pub type UpdateJobId = Uuid;
pub type SessionId = Uuid;
pub type ConfigRevisionId = Uuid;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Network {
    pub id: NetworkId,
    pub name: String,
    pub overlay_ipv4_cidr: String,
    pub overlay_ipv6_cidr: Option<String>,
    pub ipv6_enabled: bool,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Node {
    pub id: NodeId,
    pub display_name: String,
    pub overlay_ipv4: Ipv4Addr,
    pub overlay_ipv6: Option<String>,
    pub wg_public_key: String,
    pub wg_private_key_encrypted: String,
    pub identity_public_key: String,
    pub identity_private_key_encrypted: String,
    pub control_parent_id: Option<NodeId>,
    pub node_service_port: u16,
    pub wg_port_range_start: u16,
    pub wg_port_range_end: u16,
    pub is_controller: bool,
    pub agent_version: Option<String>,
    pub protocol_version: u32,
    pub desired_generation: Option<u64>,
    pub active_generation: Option<u64>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
    pub last_seen_at: Option<DateTime<Utc>>,
    pub last_handshake_at: Option<DateTime<Utc>>,
    pub enrollment_token_id: Option<TokenId>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodeAddress {
    pub id: Uuid,
    pub node_id: NodeId,
    pub address: String,
    pub address_type: AddressType,
    pub is_primary: bool,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum AddressType {
    PublicIp,
    PrivateIp,
    Domain,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodePortPool {
    pub id: Uuid,
    pub node_id: NodeId,
    pub protocol: PortProtocol,
    pub port_range_start: u16,
    pub port_range_end: u16,
    pub allocated_ports: Vec<u16>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum PortProtocol {
    Tcp,
    Udp,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ControlRelation {
    pub id: Uuid,
    pub parent_id: NodeId,
    pub child_id: NodeId,
    pub enrolled_at: DateTime<Utc>,
    pub enrollment_token_id: TokenId,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WireGuardLink {
    pub id: LinkId,
    pub node_a: NodeId,
    pub node_b: NodeId,
    pub initiator_node_id: NodeId,
    pub listener_node_id: NodeId,
    pub listener_address: String,
    pub listener_port: u16,
    pub interface_name_a: String,
    pub interface_name_b: String,
    pub enabled: bool,
    pub admin_weight: u32,
    pub last_handshake_a: Option<DateTime<Utc>>,
    pub last_handshake_b: Option<DateTime<Utc>>,
    pub status: LinkStatus,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum LinkStatus {
    Pending,
    Prepared,
    Active,
    Failed,
    Disabled,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WireGuardLinkEndpoint {
    pub id: Uuid,
    pub link_id: LinkId,
    pub node_id: NodeId,
    pub interface_name: String,
    pub listen_port: u16,
    pub peer_endpoint: Option<String>,
    pub peer_public_key: String,
    pub persistent_keepalive: u16,
    pub is_initiator: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficPolicy {
    pub id: PolicyId,
    pub name: String,
    pub priority: u32,
    pub enabled: bool,
    pub description: Option<String>,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficPolicyMatch {
    pub id: Uuid,
    pub policy_id: PolicyId,
    pub source_node_id: Option<NodeId>,
    pub source_node_group: Option<String>,
    pub source_cidr: Option<String>,
    pub destination_node_id: Option<NodeId>,
    pub destination_cidr: Option<String>,
    pub protocol: MatchProtocol,
    pub destination_port_start: Option<u16>,
    pub destination_port_end: Option<u16>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum MatchProtocol {
    Any,
    Tcp,
    Udp,
    Icmp,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficPolicyPath {
    pub id: Uuid,
    pub policy_id: PolicyId,
    pub hop_order: u32,
    pub node_id: NodeId,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TrafficPolicyConfig {
    pub id: Uuid,
    pub policy_id: PolicyId,
    pub egress_node_id: Option<NodeId>,
    pub egress_nat: bool,
    pub return_path_type: ReturnPathType,
    pub failover_enabled: bool,
    pub failover_path_id: Option<PolicyId>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum ReturnPathType {
    Symmetric,
    Independent,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConfigRevision {
    pub id: ConfigRevisionId,
    pub generation: u64,
    pub reason: String,
    pub status: ConfigRevisionStatus,
    pub created_at: DateTime<Utc>,
    pub completed_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum ConfigRevisionStatus {
    Draft,
    Pending,
    Dispatching,
    DispatchingCompleted,
    RolledBack,
    Failed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NodeDesiredConfig {
    pub id: Uuid,
    pub config_revision_id: ConfigRevisionId,
    pub node_id: NodeId,

    pub overlay_ipv4: Option<Ipv4Addr>,
    pub overlay_ipv6: Option<String>,
    pub wireguard_links: Vec<WireGuardLinkConfig>,
    pub nftables_rules: Vec<String>,
    pub policy_routing_rules: Vec<PolicyRoutingRule>,
    pub route_tables: Vec<RouteTable>,
    pub forwarding_enabled: bool,
    pub nat_rules: Vec<NatRule>,

    pub control_parent_id: Option<NodeId>,
    pub recovery_endpoint: Option<String>,
    pub config_hash: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WireGuardLinkConfig {
    pub link_id: LinkId,
    pub interface_name: String,
    pub listen_port: u16,
    pub peer_public_key: String,
    pub peer_endpoint: Option<String>,
    pub persistent_keepalive: u16,
    pub is_initiator: bool,
    pub node_private_key: String,
    pub overlay_ip: String,
    pub peer_overlay_ip: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct PolicyRoutingRule {
    pub id: Uuid,
    pub priority: u32,
    pub fwmark: u32,
    pub table_id: u32,
    pub match_src: Option<String>,
    pub match_dst: Option<String>,
    pub protocol: Option<String>,
    pub dport: Option<u16>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RouteTable {
    pub id: u32,
    pub routes: Vec<Route>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Route {
    pub destination: String,
    pub via: Option<String>,
    pub dev: Option<String>,
    pub metric: u32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NatRule {
    pub rule_type: NatRuleType,
    pub source: Option<String>,
    pub destination: Option<String>,
    pub out_interface: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum NatRuleType {
    Masquerade,
    Snat,
    Dnat,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ConfigRolloutNode {
    pub id: Uuid,
    pub config_revision_id: ConfigRevisionId,
    pub node_id: NodeId,
    pub status: ConfigRolloutStatus,
    pub started_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
    pub error_message: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum ConfigRolloutStatus {
    Pending,
    Dispatched,
    Preparing,
    Prepared,
    Activating,
    Active,
    PrepareFailed,
    ActivateFailed,
    VerifyFailed,
    RollingBack,
    RolledBack,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Heartbeat {
    pub id: Uuid,
    pub node_id: NodeId,
    pub agent_version: String,
    pub protocol_version: u32,
    pub active_generation: Option<u64>,
    pub wireguard_snapshots: Vec<WireGuardSnapshot>,
    pub received_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WireGuardSnapshot {
    pub link_id: LinkId,
    pub interface_name: String,
    pub last_handshake: Option<DateTime<Utc>>,
    pub rx_bytes: u64,
    pub tx_bytes: u64,
    pub peer_public_key: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProbeJob {
    pub id: Uuid,
    pub probe_type: ProbeType,
    pub source_node_id: NodeId,
    pub target_node_id: Option<NodeId>,
    pub policy_id: Option<PolicyId>,
    pub created_at: DateTime<Utc>,
    pub completed_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum ProbeType {
    Link,
    EndToEnd,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ProbeResult {
    pub id: Uuid,
    pub probe_job_id: Uuid,
    pub node_id: NodeId,
    pub link_id: Option<LinkId>,
    pub success_count: u32,
    pub failure_count: u32,
    pub packet_loss_pct: f64,
    pub rtt_min_ms: f64,
    pub rtt_median_ms: f64,
    pub rtt_p95_ms: f64,
    pub jitter_ms: f64,
    pub probed_at: DateTime<Utc>,
    pub is_stale: bool,
    pub error_code: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct EnrollmentToken {
    pub id: TokenId,
    pub token: String,
    pub network_id: NetworkId,
    pub parent_node_id: NodeId,
    pub suggested_node_name: String,
    pub allowed_install_mode: String,
    pub expires_at: DateTime<Utc>,
    pub used_at: Option<DateTime<Utc>>,
    pub revoked: bool,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Artifact {
    pub id: ArtifactId,
    pub name: String,
    pub version: String,
    pub architecture: String,
    pub sha256: String,
    pub size_bytes: u64,
    pub manifest_id: Option<Uuid>,
    pub created_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArtifactCacheRecord {
    pub id: Uuid,
    pub node_id: NodeId,
    pub artifact_id: ArtifactId,
    pub cached_at: DateTime<Utc>,
    pub is_complete: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReleaseManifest {
    pub product_version: String,
    pub protocol_version: u32,
    pub target_architecture: String,
    pub files: Vec<ReleaseManifestFile>,
    pub min_compatible_version: String,
    pub git_commit: String,
    pub build_time: DateTime<Utc>,
    pub allow_downgrade: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ReleaseManifestFile {
    pub name: String,
    pub sha256: String,
    pub size_bytes: u64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateJob {
    pub id: UpdateJobId,
    pub target_version: String,
    pub manifest_sha256: String,
    pub status: UpdateJobStatus,
    pub created_at: DateTime<Utc>,
    pub completed_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum UpdateJobStatus {
    Created,
    Downloading,
    DistributingArtifacts,
    Installing,
    Completed,
    Failed,
    RollingBack,
    RolledBack,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UpdateTarget {
    pub id: Uuid,
    pub update_job_id: UpdateJobId,
    pub node_id: NodeId,
    pub depth: u32,
    pub status: UpdateTargetStatus,
    pub started_at: Option<DateTime<Utc>>,
    pub completed_at: Option<DateTime<Utc>>,
    pub error_message: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
pub enum UpdateTargetStatus {
    Waiting,
    Prefetching,
    Verifying,
    Staged,
    Installing,
    Restarting,
    HealthChecking,
    Completed,
    DownloadFailed,
    SignatureInvalid,
    InstallFailed,
    HealthCheckFailed,
    RollingBack,
    RolledBack,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Admin {
    pub id: Uuid,
    pub username: String,
    pub password_hash: String,
    pub created_at: DateTime<Utc>,
    pub updated_at: DateTime<Utc>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Session {
    pub id: SessionId,
    pub admin_id: Uuid,
    pub token: String,
    pub ip_address: String,
    pub user_agent: String,
    pub created_at: DateTime<Utc>,
    pub expires_at: DateTime<Utc>,
    pub revoked: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AuditLog {
    pub id: Uuid,
    pub admin_id: Option<Uuid>,
    pub action: String,
    pub resource_type: String,
    pub resource_id: Option<String>,
    pub details: Option<String>,
    pub ip_address: String,
    pub created_at: DateTime<Utc>,
}
