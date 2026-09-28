package roster

import (
	"sync"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
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

func (s *Store) ReadyForModel(model string) []protocol.GossipPayload {
	var out []protocol.GossipPayload
	for _, p := range s.List() {
		if p.Mode == protocol.ModeCrowsNest || !p.RuntimeHealthy {
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

func (s *Store) PickLeastLoad(model string) (protocol.GossipPayload, bool) {
	list := s.ReadyForModel(model)
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
