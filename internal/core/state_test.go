package core_test

import (
	"testing"

	"github.com/FengYuchen1314/sd-wan-plus/internal/core"
)

func TestRolloutStateMachine(t *testing.T) {
	s := core.RolloutPending
	s = core.NextRolloutStatus(s, true, core.PhasePrepare)
	if s != core.RolloutDispatched {
		t.Fatalf("got %s", s)
	}
	s = core.NextRolloutStatus(s, true, core.PhasePrepare)
	if s != core.RolloutPreparing {
		t.Fatalf("got %s", s)
	}
	s = core.NextRolloutStatus(core.RolloutPreparing, false, core.PhasePrepare)
	if s != core.RolloutPrepareFailed {
		t.Fatalf("got %s", s)
	}
}

func TestUpdateStateMachine(t *testing.T) {
	s := core.UpdateWaiting
	s = core.NextUpdateStatus(s, true, core.UpdatePhaseFetch)
	if s != core.UpdatePrefetching {
		t.Fatalf("got %s", s)
	}
	s = core.NextUpdateStatus(core.UpdatePrefetching, false, core.UpdatePhaseFetch)
	if s != core.UpdateDownloadFailed {
		t.Fatalf("got %s", s)
	}
}

func TestDesiredStateHash(t *testing.T) {
	st := &core.NodeDesiredState{
		Generation: 1,
		NodeID:     "n1",
		OverlayIdentity: core.OverlayIdentity{IPv4: "10.250.0.1", DummyInterface: "pw-lo"},
	}
	if err := st.Seal(); err != nil {
		t.Fatal(err)
	}
	if st.ConfigHash == "" {
		t.Fatal("empty hash")
	}
	h2, _ := st.ComputeHash()
	if h2 != st.ConfigHash {
		t.Fatal("hash unstable")
	}
}
