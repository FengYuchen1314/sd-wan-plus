package routing

import (
	"fmt"
	"log"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
)

// Compile builds per-node DesiredState from topology + policies.
func Compile(db *storage.DB, generation uint64, box interface {
	Decrypt(string) (string, error)
}) (map[string]*core.NodeDesiredState, error) {
	g, err := topology.Build(db)
	if err != nil {
		return nil, err
	}
	policies, err := db.ListPolicies()
	if err != nil {
		return nil, err
	}

	nodeByID := map[string]core.Node{}
	for _, n := range g.Nodes {
		nodeByID[n.ID] = n
	}

	out := map[string]*core.NodeDesiredState{}
	for _, n := range g.Nodes {
		priv, err := box.Decrypt(n.WGPrivateKeyEncrypted)
		if err != nil {
			// 旧节点可能存了空/损坏密钥；跳过解密错误，agent 可用本地 wg_private.key 补齐
			log.Printf("compile: decrypt wg key for %s: %v (using empty; agent may inject local key)", n.ID, err)
			priv = ""
		}
		parent := ""
		if n.ControlParentID != nil {
			parent = *n.ControlParentID
		}
		st := &core.NodeDesiredState{
			Generation: generation,
			NodeID:     n.ID,
			OverlayIdentity: core.OverlayIdentity{
				IPv4: n.OverlayIPv4, IPv6: n.OverlayIPv6, DummyInterface: "pw-lo",
			},
			ForwardingSettings: core.ForwardingSettings{IPv4Forwarding: true},
			ControlParentID:    parent,
		}

		for _, l := range g.Links {
			if !l.Enabled && l.Status != core.LinkPending {
				continue
			}
			if l.NodeA != n.ID && l.NodeB != n.ID {
				continue
			}
			peerID := l.NodeB
			iface := l.InterfaceNameA
			if n.ID == l.NodeB {
				peerID = l.NodeA
				iface = l.InterfaceNameB
			}
			peer := nodeByID[peerID]
			isInit := n.ID == l.InitiatorNodeID
			cfg := core.WireGuardLinkCfg{
				LinkID:         l.ID,
				InterfaceName:  iface,
				PeerPublicKey:  peer.WGPublicKey,
				IsInitiator:    isInit,
				NodePrivateKey: priv,
				OverlayIP:      n.OverlayIPv4,
				PeerOverlayIP:  peer.OverlayIPv4,
			}
			if isInit {
				// 主动端：固定 Endpoint，keepalive 单向发起握手
				cfg.PeerEndpoint = fmt.Sprintf("%s:%d", l.ListenerAddress, l.ListenerPort)
				cfg.PersistentKeepalive = 25
				cfg.ListenPort = 0
			} else {
				// 被动端：仅监听；Endpoint 由内核在收到握手后动态学习/维护
				cfg.ListenPort = uint32(l.ListenerPort)
				cfg.PeerEndpoint = ""
				cfg.PersistentKeepalive = 0
			}
			st.WireGuardLinks = append(st.WireGuardLinks, cfg)
		}

		// Policy compilation: fwmark + route tables per hop
		fwBase := uint32(100)
		tableBase := uint32(1000)
		for pi, p := range policies {
			if !p.Enabled {
				continue
			}
			hops, err := db.ListPolicyHops(p.ID)
			if err != nil {
				return nil, err
			}
			matches, err := db.ListPolicyMatches(p.ID)
			if err != nil {
				return nil, err
			}
			hopIDs := make([]string, len(hops))
			for i, h := range hops {
				hopIDs[i] = h.NodeID
			}
			if err := topology.ValidatePath(g.Links, hopIDs); err != nil {
				return nil, err
			}
			// find this node's position
			idx := -1
			for i, id := range hopIDs {
				if id == n.ID {
					idx = i
					break
				}
			}
			if idx < 0 || idx == len(hopIDs)-1 {
				// egress NAT on last hop
				if idx == len(hopIDs)-1 {
					cfg, _ := db.GetPolicyConfig(p.ID)
					if cfg != nil && cfg.EgressNAT {
						st.NatRules = append(st.NatRules, core.NatRuleCfg{
							RuleType: "masquerade", Source: "0.0.0.0/0", OutInterface: "eth0",
						})
					}
				}
				continue
			}
			next := hopIDs[idx+1]
			// find link iface to next
			var nextIface string
			for _, l := range g.Links {
				if (l.NodeA == n.ID && l.NodeB == next) || (l.NodeB == n.ID && l.NodeA == next) {
					if n.ID == l.NodeA {
						nextIface = l.InterfaceNameA
					} else {
						nextIface = l.InterfaceNameB
					}
					break
				}
			}
			if nextIface == "" {
				continue
			}
			mark := fwBase + uint32(pi)
			table := tableBase + uint32(pi)
			for _, m := range matches {
				src := ""
				dst := ""
				if m.SourceCIDR != nil {
					src = *m.SourceCIDR
				}
				if m.DestinationCIDR != nil {
					dst = *m.DestinationCIDR
				}
				proto := m.Protocol
				var dport uint32
				if m.DestinationPortStart != nil {
					dport = uint32(*m.DestinationPortStart)
				}
				st.NftablesRules = append(st.NftablesRules, core.NftablesRule{
					Table: "pathweaver", Chain: "mangle", MatchSrc: src, MatchDst: dst,
					Protocol: proto, Dport: dport, Fwmark: mark, Action: "mark",
				})
			}
			st.PolicyRules = append(st.PolicyRules, core.PolicyRule{Priority: uint32(p.Priority), Fwmark: mark, TableID: table})
			st.RouteTables = append(st.RouteTables, core.RouteTableCfg{
				TableID: table,
				Routes:  []core.RouteCfg{{Destination: "0.0.0.0/0", Dev: nextIface}},
			})
		}

		if err := st.Seal(); err != nil {
			return nil, err
		}
		out[n.ID] = st
	}
	return out, nil
}
