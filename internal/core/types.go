package core

import "time"

const (
	ProtocolVersion = 1
	DefaultUsername = "admin"
)

// ProductVersion is overridden at link time by release builds (-X ...ProductVersion=).
var ProductVersion = "0.1.0"

// Config rollout statuses
const (
	RolloutPending        = "Pending"
	RolloutDispatched     = "Dispatched"
	RolloutPreparing      = "Preparing"
	RolloutPrepared       = "Prepared"
	RolloutActivating     = "Activating"
	RolloutActive         = "Active"
	RolloutPrepareFailed  = "PrepareFailed"
	RolloutActivateFailed = "ActivateFailed"
	RolloutVerifyFailed   = "VerifyFailed"
	RolloutRollingBack    = "RollingBack"
	RolloutRolledBack     = "RolledBack"
)

// Update target statuses
const (
	UpdateWaiting         = "Waiting"
	UpdatePrefetching     = "Prefetching"
	UpdateVerifying       = "Verifying"
	UpdateStaged          = "Staged"
	UpdateInstalling      = "Installing"
	UpdateRestarting      = "Restarting"
	UpdateHealthChecking  = "HealthChecking"
	UpdateCompleted       = "Completed"
	UpdateDownloadFailed  = "DownloadFailed"
	UpdateSignatureInvalid = "SignatureInvalid"
	UpdateInstallFailed   = "InstallFailed"
	UpdateHealthFailed    = "HealthCheckFailed"
	UpdateRollingBack     = "RollingBack"
	UpdateRolledBack      = "RolledBack"
)

// Link statuses
const (
	LinkPending  = "Pending"
	LinkActive   = "Active"
	LinkDisabled = "Disabled"
	LinkFailed   = "Failed"
)

// Protocol match values
const (
	ProtoAny  = "Any"
	ProtoTCP  = "TCP"
	ProtoUDP  = "UDP"
	ProtoICMP = "ICMP"
)

type Admin struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Session struct {
	ID        string    `json:"id"`
	AdminID   string    `json:"admin_id"`
	Token     string    `json:"-"`
	IPAddress string    `json:"ip_address"`
	UserAgent string    `json:"user_agent"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Revoked   bool      `json:"revoked"`
}

type Network struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	OverlayIPv4CIDR string    `json:"overlay_ipv4_cidr"`
	OverlayIPv6CIDR string    `json:"overlay_ipv6_cidr,omitempty"`
	IPv6Enabled     bool      `json:"ipv6_enabled"`
	CreatedAt       time.Time `json:"created_at"`
}

type Node struct {
	ID                       string     `json:"id"`
	DisplayName              string     `json:"display_name"`
	OverlayIPv4              string     `json:"overlay_ipv4"`
	OverlayIPv6              string     `json:"overlay_ipv6,omitempty"`
	WGPublicKey              string     `json:"wg_public_key"`
	WGPrivateKeyEncrypted    string     `json:"-"`
	IdentityPublicKey        string     `json:"identity_public_key"`
	IdentityPrivateKeyEnc    string     `json:"-"`
	ControlParentID          *string    `json:"control_parent_id,omitempty"`
	NodeServicePort          int        `json:"node_service_port"`
	WGPortRangeStart         int        `json:"wg_port_range_start"`
	WGPortRangeEnd           int        `json:"wg_port_range_end"`
	IsController             bool       `json:"is_controller"`
	AgentVersion             string     `json:"agent_version,omitempty"`
	ProtocolVersion          int        `json:"protocol_version"`
	DesiredGeneration        *int64     `json:"desired_generation,omitempty"`
	ActiveGeneration         *int64     `json:"active_generation,omitempty"`
	CreatedAt                time.Time  `json:"created_at"`
	UpdatedAt                time.Time  `json:"updated_at"`
	LastSeenAt               *time.Time `json:"last_seen_at,omitempty"`
	LastHandshakeAt          *time.Time `json:"last_handshake_at,omitempty"`
	EnrollmentTokenID        *string    `json:"enrollment_token_id,omitempty"`
}

type NodeAddress struct {
	ID          string    `json:"id"`
	NodeID      string    `json:"node_id"`
	Address     string    `json:"address"`
	AddressType string    `json:"address_type"`
	IsPrimary   bool      `json:"is_primary"`
	CreatedAt   time.Time `json:"created_at"`
}

type ControlRelation struct {
	ID                string    `json:"id"`
	ParentID          string    `json:"parent_id"`
	ChildID           string    `json:"child_id"`
	EnrolledAt        time.Time `json:"enrolled_at"`
	EnrollmentTokenID string    `json:"enrollment_token_id"`
}

type WireGuardLink struct {
	ID              string     `json:"id"`
	NodeA           string     `json:"node_a"`
	NodeB           string     `json:"node_b"`
	InitiatorNodeID string     `json:"initiator_node_id"`
	ListenerNodeID  string     `json:"listener_node_id"`
	ListenerAddress string     `json:"listener_address"`
	ListenerPort    int        `json:"listener_port"`
	InterfaceNameA  string     `json:"interface_name_a"`
	InterfaceNameB  string     `json:"interface_name_b"`
	Enabled         bool       `json:"enabled"`
	AdminWeight     int        `json:"admin_weight"`
	LastHandshakeA  *time.Time `json:"last_handshake_a,omitempty"`
	LastHandshakeB  *time.Time `json:"last_handshake_b,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       time.Time  `json:"created_at"`
}

type WireGuardLinkEndpoint struct {
	ID                   string  `json:"id"`
	LinkID               string  `json:"link_id"`
	NodeID               string  `json:"node_id"`
	InterfaceName        string  `json:"interface_name"`
	ListenPort           int     `json:"listen_port"`
	PeerEndpoint         *string `json:"peer_endpoint,omitempty"`
	PeerPublicKey        string  `json:"peer_public_key"`
	PersistentKeepalive  int     `json:"persistent_keepalive"`
	IsInitiator          bool    `json:"is_initiator"`
}

type TrafficPolicy struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Priority    int       `json:"priority"`
	Enabled     bool      `json:"enabled"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type TrafficPolicyMatch struct {
	ID                   string  `json:"id"`
	PolicyID             string  `json:"policy_id"`
	SourceNodeID         *string `json:"source_node_id,omitempty"`
	SourceNodeGroup      *string `json:"source_node_group,omitempty"`
	SourceCIDR           *string `json:"source_cidr,omitempty"`
	DestinationNodeID    *string `json:"destination_node_id,omitempty"`
	DestinationCIDR      *string `json:"destination_cidr,omitempty"`
	Protocol             string  `json:"protocol"`
	DestinationPortStart *int    `json:"destination_port_start,omitempty"`
	DestinationPortEnd   *int    `json:"destination_port_end,omitempty"`
}

type TrafficPolicyPathHop struct {
	ID       string `json:"id"`
	PolicyID string `json:"policy_id"`
	HopOrder int    `json:"hop_order"`
	NodeID   string `json:"node_id"`
}

type TrafficPolicyConfig struct {
	ID               string  `json:"id"`
	PolicyID         string  `json:"policy_id"`
	EgressNodeID     *string `json:"egress_node_id,omitempty"`
	EgressNAT        bool    `json:"egress_nat"`
	ReturnPathType   string  `json:"return_path_type"`
	FailoverEnabled  bool    `json:"failover_enabled"`
	FailoverPathID   *string `json:"failover_path_id,omitempty"`
}

type ConfigRevision struct {
	ID          string     `json:"id"`
	Generation  int64      `json:"generation"`
	Reason      string     `json:"reason"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

type EnrollmentToken struct {
	ID                 string     `json:"id"`
	Token              string     `json:"token"`
	NetworkID          string     `json:"network_id"`
	ParentNodeID       string     `json:"parent_node_id"`
	SuggestedNodeName  string     `json:"suggested_node_name"`
	AllowedInstallMode string     `json:"allowed_install_mode"`
	ExpiresAt          time.Time  `json:"expires_at"`
	UsedAt             *time.Time `json:"used_at,omitempty"`
	Revoked            bool       `json:"revoked"`
	CreatedAt          time.Time  `json:"created_at"`
}

type UpdateJob struct {
	ID            string     `json:"id"`
	TargetVersion string     `json:"target_version"`
	ManifestSHA256 string    `json:"manifest_sha256"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type UpdateTarget struct {
	ID           string     `json:"id"`
	UpdateJobID  string     `json:"update_job_id"`
	NodeID       string     `json:"node_id"`
	Depth        int        `json:"depth"`
	Status       string     `json:"status"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
}

type AuditLog struct {
	ID           string    `json:"id"`
	AdminID      *string   `json:"admin_id,omitempty"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   *string   `json:"resource_id,omitempty"`
	Details      string    `json:"details,omitempty"`
	IPAddress    string    `json:"ip_address"`
	CreatedAt    time.Time `json:"created_at"`
}

// NodeStatus aggregates multi-dimensional health for UI.
type NodeStatus struct {
	NodeID            string     `json:"node_id"`
	DisplayName       string     `json:"display_name"`
	AgentOnline       bool       `json:"agent_online"`
	ConfigConsistent  bool       `json:"config_consistent"`
	WGInterfacesOK    bool       `json:"wg_interfaces_ok"`
	LastHandshakeAt   *time.Time `json:"last_handshake_at,omitempty"`
	OverlayReachable  bool       `json:"overlay_reachable"`
	PathAvailable     bool       `json:"path_available"`
	UpdateConsistent  bool       `json:"update_consistent"`
	AgentVersion      string     `json:"agent_version,omitempty"`
	DesiredGeneration *int64     `json:"desired_generation,omitempty"`
	ActiveGeneration  *int64     `json:"active_generation,omitempty"`
}
