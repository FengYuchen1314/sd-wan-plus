package topology

import (
	"fmt"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
)

// Graph holds control-tree and WireGuard data-graph separately.
type Graph struct {
	Nodes     []core.Node                   `json:"nodes"`
	Control   []core.ControlRelation        `json:"control_relations"`
	Links     []core.WireGuardLink          `json:"links"`
	Addresses map[string][]core.NodeAddress `json:"addresses"`
}

func Build(db *storage.DB) (*Graph, error) {
	nodes, err := db.ListNodes()
	if err != nil {
		return nil, err
	}
	rels, err := db.ListControlRelations()
	if err != nil {
		return nil, err
	}
	links, err := db.ListLinks()
	if err != nil {
		return nil, err
	}
	addrs := map[string][]core.NodeAddress{}
	for _, n := range nodes {
		list, err := db.ListNodeAddresses(n.ID)
		if err != nil {
			return nil, err
		}
		addrs[n.ID] = list
	}
	return &Graph{Nodes: nodes, Control: rels, Links: links, Addresses: addrs}, nil
}

func HasDirectLink(links []core.WireGuardLink, a, b string) bool {
	for _, l := range links {
		if !l.Enabled {
			continue
		}
		if (l.NodeA == a && l.NodeB == b) || (l.NodeA == b && l.NodeB == a) {
			return true
		}
	}
	return false
}

func ValidatePath(links []core.WireGuardLink, hops []string) error {
	if len(hops) < 2 {
		return core.NewError(core.ErrValidation, "path needs at least 2 nodes")
	}
	seen := map[string]bool{}
	for i, h := range hops {
		if seen[h] {
			return core.NewError(core.ErrForwardingLoop, fmt.Sprintf("duplicate node in path: %s", h))
		}
		seen[h] = true
		if i == 0 {
			continue
		}
		if !HasDirectLink(links, hops[i-1], hops[i]) {
			return core.NewError(core.ErrNoLink, fmt.Sprintf("no WireGuard link between %s and %s", hops[i-1], hops[i]))
		}
	}
	return nil
}

// Neighbor is an adjacent node on an enabled WireGuard link.
type Neighbor struct {
	NodeID string
	LinkID string
	Iface  string // local interface name toward neighbor
}

// EnabledAdj builds adjacency from enabled WireGuard links.
func EnabledAdj(links []core.WireGuardLink) map[string][]Neighbor {
	adj := map[string][]Neighbor{}
	for _, l := range links {
		if !l.Enabled {
			continue
		}
		adj[l.NodeA] = append(adj[l.NodeA], Neighbor{
			NodeID: l.NodeB, LinkID: l.ID, Iface: l.InterfaceNameA,
		})
		adj[l.NodeB] = append(adj[l.NodeB], Neighbor{
			NodeID: l.NodeA, LinkID: l.ID, Iface: l.InterfaceNameB,
		})
	}
	return adj
}

// NextHopResult is the first hop from src toward dst.
type NextHopResult struct {
	PeerID string
	Iface  string
	LinkID string
}

// NextHop returns the BFS shortest-path next hop from src to dst over enabled links.
// Prefer ControlNextHop for overlay routing along the enrollment backbone.
func NextHop(links []core.WireGuardLink, src, dst string) (NextHopResult, bool) {
	if src == "" || dst == "" || src == dst {
		return NextHopResult{}, false
	}
	adj := EnabledAdj(links)
	type item struct {
		node string
		hop  NextHopResult // first hop from src
	}
	seen := map[string]bool{src: true}
	q := make([]item, 0)
	for _, n := range adj[src] {
		first := NextHopResult{PeerID: n.NodeID, Iface: n.Iface, LinkID: n.LinkID}
		if n.NodeID == dst {
			return first, true
		}
		seen[n.NodeID] = true
		q = append(q, item{node: n.NodeID, hop: first})
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, n := range adj[cur.node] {
			if seen[n.NodeID] {
				continue
			}
			seen[n.NodeID] = true
			if n.NodeID == dst {
				return cur.hop, true
			}
			q = append(q, item{node: n.NodeID, hop: cur.hop})
		}
	}
	return NextHopResult{}, false
}

// ControlNextHop returns the first hop from src toward dst along the control tree
// (ControlParentID). Adjacent tree hops must have an enabled WireGuard link.
func ControlNextHop(nodes []core.Node, links []core.WireGuardLink, src, dst string) (NextHopResult, bool) {
	if src == "" || dst == "" || src == dst {
		return NextHopResult{}, false
	}
	parentOf := map[string]string{}
	for _, n := range nodes {
		if n.ControlParentID != nil && *n.ControlParentID != "" {
			parentOf[n.ID] = *n.ControlParentID
		}
	}
	path, ok := controlTreePath(parentOf, src, dst)
	if !ok || len(path) < 2 {
		return NextHopResult{}, false
	}
	// Validate every tree edge has an enabled WG link.
	for i := 0; i < len(path)-1; i++ {
		if !HasDirectLink(links, path[i], path[i+1]) {
			return NextHopResult{}, false
		}
	}
	iface, linkID, ok := LinkToward(links, src, path[1])
	if !ok {
		return NextHopResult{}, false
	}
	return NextHopResult{PeerID: path[1], Iface: iface, LinkID: linkID}, true
}

// controlTreePath returns node IDs from src to dst via their LCA on the parent tree.
func controlTreePath(parentOf map[string]string, src, dst string) ([]string, bool) {
	ancestors := map[string]int{} // node -> depth from src (0 = src)
	up := []string{src}
	cur := src
	for i := 0; ; i++ {
		ancestors[cur] = i
		p, ok := parentOf[cur]
		if !ok {
			break
		}
		up = append(up, p)
		cur = p
		if i > 1024 {
			return nil, false
		}
	}
	down := []string{}
	cur = dst
	for i := 0; ; i++ {
		if _, hit := ancestors[cur]; hit {
			// path: src..LCA + reverse(down without LCA)
			lcaIdx := ancestors[cur]
			path := make([]string, 0, lcaIdx+1+len(down))
			path = append(path, up[:lcaIdx+1]...)
			for j := len(down) - 1; j >= 0; j-- {
				path = append(path, down[j])
			}
			return path, true
		}
		down = append(down, cur)
		p, ok := parentOf[cur]
		if !ok {
			return nil, false
		}
		cur = p
		if i > 1024 {
			return nil, false
		}
	}
}

// LinkToward returns local iface and link id from node `from` toward neighbor `to`.
func LinkToward(links []core.WireGuardLink, from, to string) (iface, linkID string, ok bool) {
	for _, l := range links {
		if !l.Enabled {
			continue
		}
		if l.NodeA == from && l.NodeB == to {
			return l.InterfaceNameA, l.ID, true
		}
		if l.NodeB == from && l.NodeA == to {
			return l.InterfaceNameB, l.ID, true
		}
	}
	return "", "", false
}

