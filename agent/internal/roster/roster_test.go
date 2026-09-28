package roster

import (
	"testing"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

func TestReadyForModelTagsAndVRAM(t *testing.T) {
	s := New()
	now := time.Now()
	s.Upsert(protocol.GossipPayload{
		NodeID: "a", Name: "a", Mode: protocol.ModeWorker, TS: now,
		ModelsReady: []string{"m1"}, Tags: []string{"office"}, VRAMMb: 8192, MaxVRAMMb: 8192,
		RuntimeHealthy: true,
	})
	s.Upsert(protocol.GossipPayload{
		NodeID: "b", Name: "b", Mode: protocol.ModeCaptain, TS: now,
		ModelsReady: []string{"m1"}, Tags: []string{"lab"}, VRAMMb: 24576, MaxVRAMMb: 24576,
		RuntimeHealthy: true,
	})
	s.Upsert(protocol.GossipPayload{
		NodeID: "c", Name: "c", Mode: protocol.ModeCrowsNest, TS: now,
		ModelsReady: []string{"m1"}, RuntimeHealthy: true,
	})

	got := s.ReadyForModel("m1", PickOpts{RequireTags: []string{"office"}})
	if len(got) != 1 || got[0].NodeID != "a" {
		t.Fatalf("tag filter: %+v", got)
	}

	got = s.ReadyForModel("m1", PickOpts{MinVRAMMb: 16000})
	if len(got) != 1 || got[0].NodeID != "b" {
		t.Fatalf("vram filter: %+v", got)
	}

	// captain is eligible for inference
	got = s.ReadyForModel("m1")
	if len(got) != 2 {
		t.Fatalf("want 2 ships (worker+captain), got %d", len(got))
	}
}
