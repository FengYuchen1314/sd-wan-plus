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
// Overlay reachability: shortest-path next-hop over enabled links (AllowedIPs + host routes),
// then preferred overlay paths override next-hops for specific pairs.
// Does not create new WireGuard links.
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

		// Overlay multi-hop: every reachable remote overlay IP via shortest-path next hop.
		for _, m := range g.Nodes {
			if m.ID == n.ID || m.OverlayIPv4 == "" {
				continue
			}
			hop, ok := topology.NextHop(g.Links, n.ID, m.ID)
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

		out[n.ID] = st
	}

	if err := applyPreferredOverlayPaths(db, g.Links, nodeByID, out); err != nil {
		return nil, err
	}

	for _, st := range out {
		for i := range st.WireGuardLinks {
			sort.Strings(st.WireGuardLinks[i].AllowedIPs)
		}
		if err := st.Seal(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

const overlayPathDesc = "overlay-path"

func applyPreferredOverlayPaths(db *storage.DB, links []core.WireGuardLink, nodeByID map[string]core.Node, out map[string]*core.NodeDesiredState) error {
	policies, err := db.ListPolicies()
	if err != nil {
		return err
	}
	for _, p := range policies {
		if !p.Enabled || p.Description != overlayPathDesc {
			continue
		}
		hopsRows, err := db.ListPolicyHops(p.ID)
		if err != nil || len(hopsRows) < 2 {
			continue
		}
		hops := make([]string, len(hopsRows))
		for i, h := range hopsRows {
			hops[i] = h.NodeID
		}
		if err := topology.ValidatePath(links, hops); err != nil {
			return fmt.Errorf("overlay path %s: %w", p.ID, err)
		}
		src, dst := hops[0], hops[len(hops)-1]
		srcNode, okS := nodeByID[src]
		dstNode, okD := nodeByID[dst]
		if !okS || !okD || srcNode.OverlayIPv4 == "" || dstNode.OverlayIPv4 == "" {
			continue
		}
		srcCIDR := ensureHostCIDR(srcNode.OverlayIPv4)
		dstCIDR := ensureHostCIDR(dstNode.OverlayIPv4)
		// Forward: each hop routes dstCIDR via next hop on path.
		for i := 0; i < len(hops)-1; i++ {
			if err := overrideOverlayNextHop(out[hops[i]], links, hops[i], hops[i+1], dstCIDR); err != nil {
				return fmt.Errorf("overlay path %s forward: %w", p.ID, err)
			}
		}
		// Reverse: each hop routes srcCIDR via previous hop on path.
		for i := len(hops) - 1; i > 0; i-- {
			if err := overrideOverlayNextHop(out[hops[i]], links, hops[i], hops[i-1], srcCIDR); err != nil {
				return fmt.Errorf("overlay path %s reverse: %w", p.ID, err)
			}
		}
	}
	return nil
}

func overrideOverlayNextHop(st *core.NodeDesiredState, links []core.WireGuardLink, from, next, destCIDR string) error {
	if st == nil || destCIDR == "" {
		return nil
	}
	iface, linkID, ok := topology.LinkToward(links, from, next)
	if !ok {
		return fmt.Errorf("no enabled link %s→%s", from, next)
	}
	removeCIDRFromState(st, destCIDR)
	idx := -1
	for i := range st.WireGuardLinks {
		if st.WireGuardLinks[i].LinkID == linkID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("link %s missing on node %s desired state", linkID, from)
	}
	st.WireGuardLinks[idx].AllowedIPs = appendUniqueCIDR(st.WireGuardLinks[idx].AllowedIPs, destCIDR)
	st.RouteTables = setOverlayRoute(st.RouteTables, destCIDR, iface)
	return nil
}

func removeCIDRFromState(st *core.NodeDesiredState, cidr string) {
	for i := range st.WireGuardLinks {
		st.WireGuardLinks[i].AllowedIPs = filterCIDR(st.WireGuardLinks[i].AllowedIPs, cidr)
	}
	for i := range st.RouteTables {
		routes := st.RouteTables[i].Routes[:0]
		for _, r := range st.RouteTables[i].Routes {
			if r.Destination != cidr {
				routes = append(routes, r)
			}
		}
		st.RouteTables[i].Routes = routes
	}
}

func filterCIDR(list []string, cidr string) []string {
	out := list[:0]
	for _, x := range list {
		if x != cidr {
			out = append(out, x)
		}
	}
	return out
}

// setOverlayRoute replaces any existing route to dest with one via iface (main table).
func setOverlayRoute(tables []core.RouteTableCfg, dest, iface string) []core.RouteTableCfg {
	const mainTable = 0
	rt := core.RouteCfg{Destination: dest, Dev: iface}
	for i := range tables {
		if tables[i].TableID != mainTable {
			continue
		}
		found := false
		for j := range tables[i].Routes {
			if tables[i].Routes[j].Destination == dest {
				tables[i].Routes[j] = rt
				found = true
				break
			}
		}
		if !found {
			tables[i].Routes = append(tables[i].Routes, rt)
		}
		return tables
	}
	return append(tables, core.RouteTableCfg{
		TableID: mainTable,
		Routes:  []core.RouteCfg{rt},
	})
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

// appendOverlayRoute stores host routes in table 0 (main) as a single RouteTableCfg with TableID 254
// is awkward; use TableID 0 to mean main table for netd.
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
