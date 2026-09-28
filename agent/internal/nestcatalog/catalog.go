package nestcatalog

import (
	"sort"
	"sync"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

type Entry struct {
	Advert    protocol.NestAdvert `json:"advert"`
	RTTMs     float64             `json:"rtt_ms"`
	LastRTT   time.Time           `json:"last_rtt,omitempty"`
	Active    bool                `json:"active"`
	Reachable bool                `json:"reachable"`
}

type Catalog struct {
	mu      sync.RWMutex
	byURL   map[string]*Entry
	prefer  []string // invite-seeded nests first
}

func New(prefer []string) *Catalog {
	c := &Catalog{byURL: map[string]*Entry{}, prefer: append([]string{}, prefer...)}
	for _, u := range prefer {
		if u == "" {
			continue
		}
		c.byURL[u] = &Entry{
			Advert: protocol.NestAdvert{URL: u, Kind: "public", TS: time.Now()},
			Reachable: true,
		}
	}
	return c
}

func (c *Catalog) UpsertAdvert(a protocol.NestAdvert) {
	if a.URL == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byURL[a.URL]
	if !ok {
		e = &Entry{}
		c.byURL[a.URL] = e
	}
	e.Advert = a
	e.Reachable = true
	c.evictLocked()
}

func (c *Catalog) SetRTT(url string, rttMs float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.byURL[url]
	if !ok {
		e = &Entry{Advert: protocol.NestAdvert{URL: url, TS: time.Now()}, Reachable: true}
		c.byURL[url] = e
	}
	e.RTTMs = rttMs
	e.LastRTT = time.Now()
	e.Reachable = true
}

func (c *Catalog) MarkUnreachable(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.byURL[url]; ok {
		e.Reachable = false
		e.Active = false
	}
}

func (c *Catalog) List() []Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]Entry, 0, len(c.byURL))
	now := time.Now()
	for _, e := range c.byURL {
		if now.Sub(e.Advert.TS) > protocol.NestAdvertTTL && e.Advert.NestID != "" {
			continue
		}
		out = append(out, *e)
	}
	return out
}

func (c *Catalog) URLs() []string {
	list := c.List()
	out := make([]string, 0, len(list))
	for _, e := range list {
		out = append(out, e.Advert.URL)
	}
	return out
}

// SelectActive picks up to max nests: prefer seeds, then lowest RTT among reachable.
func (c *Catalog) SelectActive(max int) []string {
	if max <= 0 {
		max = protocol.MaxActiveNests
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	type scored struct {
		url   string
		score float64
	}
	var all []scored
	preferSet := map[string]bool{}
	for i, u := range c.prefer {
		preferSet[u] = true
		if e, ok := c.byURL[u]; ok && e.Reachable {
			all = append(all, scored{url: u, score: float64(i) - 1000}) // strong prefer
		}
	}
	for u, e := range c.byURL {
		if preferSet[u] || !e.Reachable {
			continue
		}
		rtt := e.RTTMs
		if rtt <= 0 {
			rtt = 500
		}
		all = append(all, scored{url: u, score: rtt + float64(e.Advert.Peers)*5})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].score < all[j].score })
	// reset active flags
	for _, e := range c.byURL {
		e.Active = false
	}
	var out []string
	for _, s := range all {
		if len(out) >= max {
			break
		}
		out = append(out, s.url)
		if e := c.byURL[s.url]; e != nil {
			e.Active = true
		}
	}
	return out
}

func (c *Catalog) evictLocked() {
	if len(c.byURL) <= protocol.MaxNestCatalog {
		return
	}
	type item struct {
		url string
		ts  time.Time
	}
	var list []item
	for u, e := range c.byURL {
		list = append(list, item{u, e.Advert.TS})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ts.Before(list[j].ts) })
	for len(c.byURL) > protocol.MaxNestCatalog && len(list) > 0 {
		u := list[0].url
		list = list[1:]
		// keep prefer
		keep := false
		for _, p := range c.prefer {
			if p == u {
				keep = true
				break
			}
		}
		if keep {
			continue
		}
		delete(c.byURL, u)
	}
}

func (c *Catalog) AddPrefer(url string) {
	if url == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.prefer {
		if p == url {
			return
		}
	}
	c.prefer = append([]string{url}, c.prefer...)
	if _, ok := c.byURL[url]; !ok {
		c.byURL[url] = &Entry{
			Advert:    protocol.NestAdvert{URL: url, Kind: "captain", TS: time.Now()},
			Reachable: true,
		}
	}
}
