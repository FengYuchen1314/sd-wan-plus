package topology

import (
	"strings"
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

func TestSimplePathsDiamond(t *testing.T) {
	// A—B—C and A—D—C
	links := []core.WireGuardLink{
		{ID: "ab", NodeA: "A", NodeB: "B", Enabled: true, InterfaceNameA: "pwl-ab", InterfaceNameB: "pwl-ba"},
		{ID: "bc", NodeA: "B", NodeB: "C", Enabled: true, InterfaceNameA: "pwl-bc", InterfaceNameB: "pwl-cb"},
		{ID: "ad", NodeA: "A", NodeB: "D", Enabled: true, InterfaceNameA: "pwl-ad", InterfaceNameB: "pwl-da"},
		{ID: "dc", NodeA: "D", NodeB: "C", Enabled: true, InterfaceNameA: "pwl-dc", InterfaceNameB: "pwl-cd"},
	}
	paths := SimplePaths(links, "A", "C", 8, 64)
	if len(paths) < 2 {
		t.Fatalf("want >=2 paths, got %v", paths)
	}
	seen := map[string]bool{}
	for _, p := range paths {
		key := strings.Join(p, ",")
		seen[key] = true
		for i := 0; i < len(p); i++ {
			for j := i + 1; j < len(p); j++ {
				if p[i] == p[j] {
					t.Fatalf("cycle in path %v", p)
				}
			}
		}
	}
	if !seen["A,B,C"] || !seen["A,D,C"] {
		t.Fatalf("missing expected paths: %v", paths)
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

