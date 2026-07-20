package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

// NodeDesiredState is the structured config pushed to agents (no shell commands).
type NodeDesiredState struct {
	Generation         uint64               `json:"generation"`
	NodeID             string               `json:"node_id"`
	OverlayIdentity    OverlayIdentity      `json:"overlay_identity"`
	WireGuardLinks     []WireGuardLinkCfg   `json:"wireguard_links"`
	ListenPorts        []uint32             `json:"listen_ports"`
	Peers              []PeerCfg            `json:"peers"`
	NftablesRules      []NftablesRule       `json:"nftables_rules"`
	PolicyRules        []PolicyRule         `json:"policy_rules"`
	RouteTables        []RouteTableCfg      `json:"route_tables"`
	ForwardingSettings ForwardingSettings   `json:"forwarding_settings"`
	NatRules           []NatRuleCfg         `json:"nat_rules"`
	ControlParentID    string               `json:"control_parent_id,omitempty"`
	RecoveryEndpoint   string               `json:"recovery_endpoint,omitempty"`
	ConfigHash         string               `json:"config_hash"`
}

type OverlayIdentity struct {
	IPv4           string `json:"ipv4"`
	IPv6           string `json:"ipv6,omitempty"`
	DummyInterface string `json:"dummy_interface"`
}

type WireGuardLinkCfg struct {
	LinkID              string   `json:"link_id"`
	InterfaceName       string   `json:"interface_name"`
	ListenPort          uint32   `json:"listen_port"`
	PeerPublicKey       string   `json:"peer_public_key"`
	PeerEndpoint        string   `json:"peer_endpoint,omitempty"`
	PersistentKeepalive uint32   `json:"persistent_keepalive"`
	IsInitiator         bool     `json:"is_initiator"`
	NodePrivateKey      string   `json:"node_private_key,omitempty"`
	OverlayIP           string   `json:"overlay_ip"`
	PeerOverlayIP       string   `json:"peer_overlay_ip"`
	AllowedIPs          []string `json:"allowed_ips,omitempty"` // overlay destinations via this peer (/32)
}

type PeerCfg struct {
	PublicKey           string   `json:"public_key"`
	Endpoint            string   `json:"endpoint,omitempty"`
	AllowedIPs          []string `json:"allowed_ips"`
	PersistentKeepalive uint32   `json:"persistent_keepalive"`
}

type NftablesRule struct {
	Table     string `json:"table"`
	Chain     string `json:"chain"`
	MatchSrc  string `json:"match_src,omitempty"`
	MatchDst  string `json:"match_dst,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
	Dport     uint32 `json:"dport,omitempty"`
	Fwmark    uint32 `json:"fwmark"`
	Action    string `json:"action"`
}

type PolicyRule struct {
	Priority uint32 `json:"priority"`
	Fwmark   uint32 `json:"fwmark"`
	TableID  uint32 `json:"table_id"`
}

type RouteTableCfg struct {
	TableID uint32     `json:"table_id"`
	Routes  []RouteCfg `json:"routes"`
}

type RouteCfg struct {
	Destination string `json:"destination"`
	Via         string `json:"via,omitempty"`
	Dev         string `json:"dev,omitempty"`
	Metric      uint32 `json:"metric,omitempty"`
}

type ForwardingSettings struct {
	IPv4Forwarding bool `json:"ipv4_forwarding"`
	IPv6Forwarding bool `json:"ipv6_forwarding"`
}

type NatRuleCfg struct {
	RuleType     string `json:"rule_type"`
	Source       string `json:"source,omitempty"`
	Destination  string `json:"destination,omitempty"`
	OutInterface string `json:"out_interface,omitempty"`
}

func (s *NodeDesiredState) ComputeHash() (string, error) {
	clone := *s
	clone.ConfigHash = ""
	b, err := json.Marshal(clone)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func (s *NodeDesiredState) Seal() error {
	h, err := s.ComputeHash()
	if err != nil {
		return err
	}
	s.ConfigHash = h
	return nil
}
