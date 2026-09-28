package roster

import (
	"sync"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

type Store struct {
	mu    sync.RWMutex
	peers map[string]protocol.GossipPayload
}

func New() *Store {
	return &Store{peers: map[string]protocol.GossipPayload{}}
}

func (s *Store) Upsert(g protocol.GossipPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peers[g.NodeID] = g
}

func (s *Store) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.peers, id)
}

func (s *Store) List() []protocol.GossipPayload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]protocol.GossipPayload, 0, len(s.peers))
	now := time.Now()
	for _, p := range s.peers {
		if now.Sub(p.TS) > 2*time.Minute {
			continue
		}
		out = append(out, p)
	}
	return out
}

// PickOpts filters ready ships for routing / UI.
type PickOpts struct {
	RequireTags []string // all must be present on peer
	MinVRAMMb   int      // peer capacity (MaxVRAMMb or VRAMMb) must be >= this
}

func (s *Store) ReadyForModel(model string, opts ...PickOpts) []protocol.GossipPayload {
	var opt PickOpts
	if len(opts) > 0 {
		opt = opts[0]
	}
	var out []protocol.GossipPayload
	for _, p := range s.List() {
		if p.Mode == protocol.ModeCrowsNest || !p.RuntimeHealthy {
			continue
		}
		if !hasAllTags(p.Tags, opt.RequireTags) {
			continue
		}
		capVRAM := p.MaxVRAMMb
		if capVRAM <= 0 {
			capVRAM = p.VRAMMb
		}
		if opt.MinVRAMMb > 0 && capVRAM > 0 && capVRAM < opt.MinVRAMMb {
			continue
		}
		if model == "" {
			out = append(out, p)
			continue
		}
		for _, m := range p.ModelsReady {
			if m == model {
				out = append(out, p)
				break
			}
		}
	}
	return out
}

func (s *Store) PickLeastLoad(model string, opts ...PickOpts) (protocol.GossipPayload, bool) {
	list := s.ReadyForModel(model, opts...)
	if len(list) == 0 {
		return protocol.GossipPayload{}, false
	}
	best := list[0]
	for _, p := range list[1:] {
		if p.Load < best.Load {
			best = p
		}
	}
	return best, true
}

func hasAllTags(have, need []string) bool {
	if len(need) == 0 {
		return true
	}
	set := map[string]bool{}
	for _, t := range have {
		set[t] = true
	}
	for _, t := range need {
		if t == "" {
			continue
		}
		if !set[t] {
			return false
		}
	}
	return true
}
