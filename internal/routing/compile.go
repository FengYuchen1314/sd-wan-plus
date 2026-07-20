package routing

import (
	"fmt"
	"log"
	"net"
	"sort"
	"strings"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/netutil"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
)

// Compile builds per-node DesiredState from the WireGuard graph.
// Overlay reachability follows the enrollment control tree (parent/child backbone):
// AllowedIPs + host routes via ControlNextHop. Does not create links or use mesh shortcuts.
func Compile(db *storage.DB, generation uint64, box interface {
	Decrypt(string) (string, error)
}) (map[string]*core.NodeDesiredState, error) {
	g, err := topology.Build(db)
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

		linkIdx := map[string]int{} // linkID -> index in st.WireGuardLinks
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
			if eps, err := db.ListLinkEndpoints(l.ID); err == nil {
				for _, ep := range eps {
					if ep.NodeID != n.ID {
						continue
					}
					if ep.InterfaceName != "" {
						cfg.InterfaceName = ep.InterfaceName
					}
					if ep.PeerPublicKey != "" {
						cfg.PeerPublicKey = ep.PeerPublicKey
					}
					cfg.IsInitiator = ep.IsInitiator
					cfg.ListenPort = uint32(ep.ListenPort)
					cfg.PersistentKeepalive = uint32(ep.PersistentKeepalive)
					if ep.PeerEndpoint != nil {
						host := *ep.PeerEndpoint
						if h, _, err := net.SplitHostPort(*ep.PeerEndpoint); err == nil {
							host = h
						}
						if netutil.IsPublicDialable(host) || ep.IsInitiator {
							cfg.PeerEndpoint = *ep.PeerEndpoint
						}
					}
					break
				}
			}
			if cfg.PeerEndpoint == "" && cfg.IsInitiator {
				cfg.PeerEndpoint = fmt.Sprintf("%s:%d", l.ListenerAddress, l.ListenerPort)
				if cfg.PersistentKeepalive == 0 {
					cfg.PersistentKeepalive = 25
				}
			}
			if cfg.ListenPort == 0 && !cfg.IsInitiator {
				cfg.ListenPort = uint32(l.ListenerPort)
			}
			if cfg.PersistentKeepalive == 0 && cfg.PeerEndpoint != "" {
				cfg.PersistentKeepalive = 25
			}
			if peer.OverlayIPv4 != "" {
				cfg.AllowedIPs = append(cfg.AllowedIPs, ensureHostCIDR(peer.OverlayIPv4))
			}
			linkIdx[l.ID] = len(st.WireGuardLinks)
			st.WireGuardLinks = append(st.WireGuardLinks, cfg)
		}

		// Overlay multi-hop along control-tree backbone only.
		for _, m := range g.Nodes {
			if m.ID == n.ID || m.OverlayIPv4 == "" {
				continue
			}
			hop, ok := topology.ControlNextHop(g.Nodes, g.Links, n.ID, m.ID)
			if !ok {
				continue
			}
			idx, ok := linkIdx[hop.LinkID]
			if !ok {
				continue
			}
			cidr := ensureHostCIDR(m.OverlayIPv4)
			st.WireGuardLinks[idx].AllowedIPs = appendUniqueCIDR(st.WireGuardLinks[idx].AllowedIPs, cidr)
			st.RouteTables = appendOverlayRoute(st.RouteTables, cidr, hop.Iface, n.OverlayIPv4)
		}

		for i := range st.WireGuardLinks {
			sort.Strings(st.WireGuardLinks[i].AllowedIPs)
		}
		if err := st.Seal(); err != nil {
			return nil, err
		}
		out[n.ID] = st
	}
	return out, nil
}

func ensureHostCIDR(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	if strings.Contains(ip, "/") {
		return ip
	}
	return ip + "/32"
}

func appendUniqueCIDR(list []string, cidr string) []string {
	if cidr == "" {
		return list
	}
	for _, x := range list {
		if x == cidr {
			return list
		}
	}
	return append(list, cidr)
}

// appendOverlayRoute stores host routes in table 0 (main) for netd.
func appendOverlayRoute(tables []core.RouteTableCfg, dest, iface, src string) []core.RouteTableCfg {
	const mainTable = 0
	rt := core.RouteCfg{Destination: dest, Dev: iface}
	_ = src // src applied by netd from overlay identity
	for i := range tables {
		if tables[i].TableID == mainTable {
			for _, r := range tables[i].Routes {
				if r.Destination == dest {
					return tables
				}
			}
			tables[i].Routes = append(tables[i].Routes, rt)
			return tables
		}
	}
	return append(tables, core.RouteTableCfg{
		TableID: mainTable,
		Routes:  []core.RouteCfg{rt},
	})
}
