package roster

import (
	"testing"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

func boolPtr(v bool) *bool { return &v }

func TestReadyForModelTagsAndVRAM(t *testing.T) {
	s := New()
	now := time.Now()
	s.Upsert(protocol.GossipPayload{
		NodeID: "a", Name: "a", Mode: protocol.ModeShip, TS: now,
		ModelsReady: []string{"m1"}, Tags: []string{"office"}, VRAMMb: 8192, MaxVRAMMb: 8192,
		RuntimeHealthy: true, Infer: boolPtr(true),
	})
	s.Upsert(protocol.GossipPayload{
		NodeID: "b", Name: "b", Mode: protocol.ModeShip, TS: now, HostNest: true,
		ModelsReady: []string{"m1"}, Tags: []string{"lab"}, VRAMMb: 24576, MaxVRAMMb: 24576,
		RuntimeHealthy: true, Infer: boolPtr(true),
	})
	s.Upsert(protocol.GossipPayload{
		NodeID: "c", Name: "c", Mode: protocol.ModeCrowsNest, TS: now,
		ModelsReady: []string{"m1"}, RuntimeHealthy: true,
	})
	s.Upsert(protocol.GossipPayload{
		NodeID: "d", Name: "console", Mode: protocol.ModeShip, TS: now, HostNest: true,
		ModelsReady: []string{"m1"}, RuntimeHealthy: true, Infer: boolPtr(false),
	})

	got := s.ReadyForModel("m1", PickOpts{RequireTags: []string{"office"}})
	if len(got) != 1 || got[0].NodeID != "a" {
		t.Fatalf("tag filter: %+v", got)
	}

	got = s.ReadyForModel("m1", PickOpts{MinVRAMMb: 16000})
	if len(got) != 1 || got[0].NodeID != "b" {
		t.Fatalf("vram filter: %+v", got)
	}

	got = s.ReadyForModel("m1")
	if len(got) != 2 {
		t.Fatalf("want 2 ships (skip crowsnest + no-infer), got %d", len(got))
	}
}
