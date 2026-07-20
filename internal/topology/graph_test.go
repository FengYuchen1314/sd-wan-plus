package topology

import (
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

func ptr(s string) *string { return &s }

func TestNextHopChain(t *testing.T) {
	// A—B—C
	links := []core.WireGuardLink{
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: true, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
		{ID: "bc", NodeA: "B", NodeB: "C", Enabled: true, InterfaceNameA: "pwl-bc", InterfaceNameB: "pwl-cb"},
	}
	hop, ok := NextHop(links, "C", "A")
	if !ok {
		t.Fatal("expected path C→A")
	}
	if hop.PeerID != "B" || hop.Iface != "pwl-cb" || hop.LinkID != "bc" {
		t.Fatalf("got %+v", hop)
	}
	hop, ok = NextHop(links, "A", "C")
	if !ok || hop.PeerID != "B" || hop.Iface != "pwl-ab" {
		t.Fatalf("A→C got %+v ok=%v", hop, ok)
	}
	hop, ok = NextHop(links, "A", "B")
	if !ok || hop.PeerID != "B" {
		t.Fatalf("A→B got %+v", hop)
	}
}

func TestNextHopDisabledIgnored(t *testing.T) {
	links := []core.WireGuardLink{
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: false, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
	}
	if _, ok := NextHop(links, "A", "B"); ok {
		t.Fatal("disabled link should not be used")
	}
}

func TestControlNextHopTree(t *testing.T) {
	// Controller R — A — B
	//              \— C
	nodes := []core.Node{
		{ID: "R", DisplayName: "root"},
		{ID: "A", ControlParentID: ptr("R")},
		{ID: "B", ControlParentID: ptr("A")},
		{ID: "C", ControlParentID: ptr("A")},
	}
	links := []core.WireGuardLink{
		{ID: "ra", NodeA: "R", NodeB: "A", Enabled: true, InterfaceNameA: "pwl-ra", InterfaceNameB: "pwl-ar"},
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: true, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
		{ID: "ac", NodeA: "A", NodeB: "C", Enabled: true, InterfaceNameA: "pwl-ac", InterfaceNameB: "pwl-ca"},
		// mesh shortcut B—C must NOT be used for overlay next hop
		{ID: "bc", NodeA: "B", NodeB: "C", Enabled: true, InterfaceNameA: "pwl-bc", InterfaceNameB: "pwl-cb"},
	}

	hop, ok := ControlNextHop(nodes, links, "B", "R")
	if !ok || hop.PeerID != "A" || hop.LinkID != "ab" {
		t.Fatalf("B→R want via A, got %+v ok=%v", hop, ok)
	}
	hop, ok = ControlNextHop(nodes, links, "B", "C")
	if !ok || hop.PeerID != "A" || hop.LinkID != "ab" {
		t.Fatalf("B→C want via A (not shortcut), got %+v ok=%v", hop, ok)
	}
	hop, ok = ControlNextHop(nodes, links, "C", "B")
	if !ok || hop.PeerID != "A" || hop.LinkID != "ac" {
		t.Fatalf("C→B want via A, got %+v ok=%v", hop, ok)
	}
	hop, ok = ControlNextHop(nodes, links, "B", "A")
	if !ok || hop.PeerID != "A" {
		t.Fatalf("B→A got %+v ok=%v", hop, ok)
	}
}

func TestControlNextHopMissingLink(t *testing.T) {
	nodes := []core.Node{
		{ID: "R"},
		{ID: "A", ControlParentID: ptr("R")},
	}
	// no WG link between R and A
	if _, ok := ControlNextHop(nodes, nil, "A", "R"); ok {
		t.Fatal("expected unreachable without WG on tree edge")
	}
}

func TestLinkToward(t *testing.T) {
	links := []core.WireGuardLink{
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: true, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
	}
	iface, id, ok := LinkToward(links, "B", "A")
	if !ok || iface != "pwl-ba" || id != "ab" {
		t.Fatalf("got %s %s %v", iface, id, ok)
	}
}
