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

// NextHopResult is the first hop from src toward dst on the WG graph.
type NextHopResult struct {
	PeerID string
	Iface  string
	LinkID string
}

// NextHop returns the BFS shortest-path next hop from src to dst over enabled links.
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

// SimplePaths enumerates simple (acyclic) paths from src to dst over enabled WG links.
func SimplePaths(links []core.WireGuardLink, src, dst string, maxHops, maxPaths int) [][]string {
	if src == "" || dst == "" || src == dst {
		return nil
	}
	if maxHops <= 0 {
		maxHops = 8
	}
	if maxPaths <= 0 {
		maxPaths = 64
	}
	adj := EnabledAdj(links)
	var out [][]string
	var walk func(cur string, path []string, seen map[string]bool)
	walk = func(cur string, path []string, seen map[string]bool) {
		if len(out) >= maxPaths {
			return
		}
		if cur == dst {
			cp := make([]string, len(path))
			copy(cp, path)
			out = append(out, cp)
			return
		}
		if len(path)-1 >= maxHops {
			return
		}
		for _, n := range adj[cur] {
			if seen[n.NodeID] {
				continue
			}
			seen[n.NodeID] = true
			walk(n.NodeID, append(path, n.NodeID), seen)
			delete(seen, n.NodeID)
			if len(out) >= maxPaths {
				return
			}
		}
	}
	seen := map[string]bool{src: true}
	walk(src, []string{src}, seen)
	return out
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

