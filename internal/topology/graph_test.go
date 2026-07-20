package topology_test

import (
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
	"github.com/FengYuchen1314/sd-wan-plus/internal/topology"
)

func TestValidatePath(t *testing.T) {
	links := []core.WireGuardLink{
		{NodeA: "a", NodeB: "b", Enabled: true},
		{NodeA: "b", NodeB: "c", Enabled: true},
	}
	if err := topology.ValidatePath(links, []string{"a", "b", "c"}); err != nil {
		t.Fatal(err)
	}
	if err := topology.ValidatePath(links, []string{"a", "c"}); err == nil {
		t.Fatal("expected missing link error")
	}
	if err := topology.ValidatePath(links, []string{"a", "b", "a"}); err == nil {
		t.Fatal("expected loop error")
	}
}
