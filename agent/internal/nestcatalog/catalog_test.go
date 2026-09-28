package nestcatalog

import (
	"testing"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

func TestSelectActivePrefersSeed(t *testing.T) {
	c := New([]string{"ws://captain/nest"})
	c.UpsertAdvert(protocol.NestAdvert{URL: "wss://far/nest", Kind: "public", TS: time.Now()})
	c.SetRTT("wss://far/nest", 10)
	c.SetRTT("ws://captain/nest", 200)
	active := c.SelectActive(1)
	if len(active) != 1 || active[0] != "ws://captain/nest" {
		t.Fatalf("expected captain seed first, got %v", active)
	}
}
