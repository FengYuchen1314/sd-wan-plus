package topology

import (
	"fmt"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/storage"
)

// Graph holds control-tree and WireGuard data-graph separately.
type Graph struct {
	Nodes      []core.Node              `json:"nodes"`
	Control    []core.ControlRelation   `json:"control_relations"`
	Links      []core.WireGuardLink     `json:"links"`
	Addresses  map[string][]core.NodeAddress `json:"addresses"`
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
