package topology

import (
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

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

func TestNextHopUnreachable(t *testing.T) {
	links := []core.WireGuardLink{
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: true, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
	}
	if _, ok := NextHop(links, "A", "Z"); ok {
		t.Fatal("expected unreachable")
	}
}
