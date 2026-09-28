package ship

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/fleetcrypto"
	"github.com/Chex4ever/pirate-fleet/agent/internal/hwinfo"
	"github.com/Chex4ever/pirate-fleet/agent/internal/nestcatalog"
	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/Chex4ever/pirate-fleet/agent/internal/roster"
	"github.com/Chex4ever/pirate-fleet/agent/internal/runtimeadapter"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type Config struct {
	NodeID         string
	Name           string
	Mode           string
	JoinSecret     string
	Nests          []string // seed / prefer
	Version        string
	ExposeAPI      bool
	APIAdvertise   string
	LocalNestURL   string // Host Nest URL when HostNest
	LocalNestID    string
	HostNest       bool
	Infer          bool // accept inference jobs
	Tags           []string
	MaxVRAMMb      int // 0 = use detected GPU VRAM as capacity
	OllamaURL      string
	VLLMURL        string
	Runtime        string
	MaxActiveNests int
	OnNestsChanged func([]string) // persist catalog URLs
}

type Ship struct {
	cfg     Config
	roster  *roster.Store
	catalog *nestcatalog.Catalog
	rt      runtimeadapter.Runtime

	mu       sync.Mutex
	conns    map[string]*websocket.Conn
	pending  map[string]chan protocol.InferResponse
	load     int
	active   []string
	cancelMaint context.CancelFunc
}

func New(cfg Config) (*Ship, error) {
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node_id required")
	}
	if cfg.MaxActiveNests <= 0 {
		cfg.MaxActiveNests = protocol.MaxActiveNests
	}
	var rt runtimeadapter.Runtime
	switch cfg.Runtime {
	case "vllm":
		rt = runtimeadapter.NewVLLM(cfg.VLLMURL)
	default:
		rt = runtimeadapter.NewOllama(cfg.OllamaURL)
	}
	return &Ship{
		cfg:     cfg,
		roster:  roster.New(),
		catalog: nestcatalog.New(cfg.Nests),
		rt:      rt,
		conns:   map[string]*websocket.Conn{},
		pending: map[string]chan protocol.InferResponse{},
	}, nil
}

func (s *Ship) Roster() *roster.Store          { return s.roster }
func (s *Ship) Catalog() *nestcatalog.Catalog  { return s.catalog }
func (s *Ship) ActiveNests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.active...)
}
func (s *Ship) LocalNestURL() string { return s.cfg.LocalNestURL }

func (s *Ship) Run(ctx context.Context) error {
	maintCtx, cancel := context.WithCancel(ctx)
	s.cancelMaint = cancel
	go s.maintainActive(maintCtx)

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	probe := time.NewTicker(45 * time.Second)
	defer probe.Stop()
	s.broadcastGossip()
	if s.cfg.LocalNestURL != "" {
		s.broadcastNestAdvert()
	}
	for {
		select {
		case <-ctx.Done():
			cancel()
			return nil
		case <-ticker.C:
			s.broadcastGossip()
			if s.cfg.LocalNestURL != "" {
				s.broadcastNestAdvert()
			}
		case <-probe.C:
			s.probeAll(ctx)
			s.rebalance(maintCtx)
		}
	}
}

func (s *Ship) maintainActive(ctx context.Context) {
	s.rebalance(ctx)
	<-ctx.Done()
}

func (s *Ship) rebalance(ctx context.Context) {
	want := s.catalog.SelectActive(s.cfg.MaxActiveNests)
	s.mu.Lock()
	prev := s.active
	s.active = want
	s.mu.Unlock()
	if s.cfg.OnNestsChanged != nil {
		s.cfg.OnNestsChanged(s.catalog.URLs())
	}
	have := map[string]bool{}
	s.mu.Lock()
	for u := range s.conns {
		have[u] = true
	}
	s.mu.Unlock()
	wantSet := map[string]bool{}
	for _, u := range want {
		wantSet[u] = true
		if !have[u] {
			go s.maintainNest(ctx, u)
		}
	}
	for u := range have {
		if !wantSet[u] {
			s.mu.Lock()
			if c := s.conns[u]; c != nil {
				_ = c.Close()
				delete(s.conns, u)
			}
			s.mu.Unlock()
		}
	}
	_ = prev
}

func (s *Ship) maintainNest(ctx context.Context, nestURL string) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		// still wanted?
		s.mu.Lock()
		wanted := false
		for _, u := range s.active {
			if u == nestURL {
				wanted = true
				break
			}
		}
		s.mu.Unlock()
		if !wanted {
			return
		}
		if err := s.session(ctx, nestURL); err != nil {
			log.Printf("nest %s: %v", nestURL, err)
			s.catalog.MarkUnreachable(nestURL)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 20*time.Second {
			backoff *= 2
		}
	}
}

func (s *Ship) session(ctx context.Context, nestURL string) error {
	u, err := url.Parse(nestURL)
	if err != nil {
		return err
	}
	dialer := nestDialer(15 * time.Second)
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	s.mu.Lock()
	if old := s.conns[nestURL]; old != nil {
		_ = old.Close()
	}
	s.conns[nestURL] = conn
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.conns[nestURL] == conn {
			delete(s.conns, nestURL)
		}
		s.mu.Unlock()
	}()

	reg := protocol.RegisterPayload{
		NodeID: s.cfg.NodeID, Mode: s.cfg.Mode, Name: s.cfg.Name, Version: s.cfg.Version,
		JoinHMAC: fleetcrypto.HMACHex(s.cfg.JoinSecret, s.cfg.NodeID),
	}
	if err := s.writeEnv(conn, protocol.Envelope{Type: protocol.TypeRegister, From: s.cfg.NodeID, Payload: mustJSON(reg)}); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		s.handleEnvelope(data)
	}
}

func (s *Ship) handleEnvelope(data []byte) {
	var env protocol.Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return
	}
	switch env.Type {
	case protocol.TypeGossip:
		var g protocol.GossipPayload
		if err := json.Unmarshal(env.Payload, &g); err != nil || g.NodeID == s.cfg.NodeID {
			return
		}
		want := fleetcrypto.GossipMac(s.cfg.JoinSecret, g.NodeID, g.Mode, g.TS, g.ModelsReady, g.Tags)
		if g.HMAC != want {
			return
		}
		s.roster.Upsert(g)
		if g.NestURL != "" {
			s.catalog.UpsertAdvert(protocol.NestAdvert{
				NestID: g.NodeID, URL: g.NestURL, Kind: "captain", Name: g.Name, TS: g.TS,
				HMAC: fleetcrypto.NestAdvertMac(s.cfg.JoinSecret, g.NodeID, g.NestURL, "captain", g.TS),
			})
		}
		for _, nu := range g.NestsConnected {
			s.catalog.UpsertAdvert(protocol.NestAdvert{URL: nu, Kind: "public", TS: time.Now()})
		}
	case protocol.TypeNestAdvert:
		var a protocol.NestAdvert
		if err := json.Unmarshal(env.Payload, &a); err != nil || a.URL == "" {
			return
		}
		want := fleetcrypto.NestAdvertMac(s.cfg.JoinSecret, a.NestID, a.URL, a.Kind, a.TS)
		if a.HMAC != "" && a.HMAC != want {
			return
		}
		s.catalog.UpsertAdvert(a)
	case protocol.TypeNestPong:
		var p protocol.NestPong
		if err := json.Unmarshal(env.Payload, &p); err != nil {
			return
		}
		rtt := float64(time.Now().UnixMilli()-p.TS) // approx if nest echoes ts
		if p.TS > 0 {
			rtt = float64(time.Now().UnixMilli() - p.TS)
		}
		if env.Nest != "" {
			// map via active conns — use From nest url from pending probes stored in ID
		}
		_ = rtt
	case protocol.TypeInferReq:
		if env.To != "" && env.To != s.cfg.NodeID {
			return
		}
		go s.handleInfer(env)
	case protocol.TypeInferResp:
		var resp protocol.InferResponse
		if err := json.Unmarshal(env.Payload, &resp); err != nil {
			return
		}
		s.mu.Lock()
		ch := s.pending[resp.RequestID]
		s.mu.Unlock()
		if ch != nil {
			select {
			case ch <- resp:
			default:
			}
		}
	case protocol.TypeEnsureModel:
		if env.To != "" && env.To != s.cfg.NodeID {
			return
		}
		var m protocol.EnsureModelPayload
		if err := json.Unmarshal(env.Payload, &m); err != nil {
			return
		}
		go func() {
			_ = s.rt.EnsureModel(context.Background(), runtimeadapter.DesiredModel{
				ModelID: m.ModelID, OllamaTag: m.OllamaTag, DownloadURL: m.URL, Format: m.Format,
			})
		}()
	}
}

func (s *Ship) handleInfer(env protocol.Envelope) {
	var req protocol.InferRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		return
	}
	if !s.cfg.Infer {
		_ = s.sendTo(env.From, protocol.Envelope{
			Type: protocol.TypeInferResp, From: s.cfg.NodeID, To: env.From, TTL: protocol.MaxNestHops,
			Payload: mustJSON(protocol.InferResponse{RequestID: req.RequestID, StatusCode: 503, Error: "inference disabled on this ship"}),
		})
		return
	}
	s.mu.Lock()
	s.load++
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.load--
		s.mu.Unlock()
	}()
	status, body, err := s.rt.Proxy(context.Background(), req.Path, req.Body)
	resp := protocol.InferResponse{RequestID: req.RequestID, StatusCode: status, Body: body}
	if err != nil {
		resp.Error = err.Error()
		resp.StatusCode = 502
	}
	_ = s.sendTo(env.From, protocol.Envelope{
		Type: protocol.TypeInferResp, From: s.cfg.NodeID, To: env.From, TTL: protocol.MaxNestHops, Payload: mustJSON(resp),
	})
}

func (s *Ship) broadcastGossip() {
	info := hwinfo.Collect()
	healthy, models, _ := s.rt.Health(context.Background())
	if models == nil {
		models = []string{}
	}
	tags := append([]string{}, s.cfg.Tags...)
	maxVRAM := s.cfg.MaxVRAMMb
	if maxVRAM <= 0 {
		maxVRAM = info.VRAMMb
	}
	s.mu.Lock()
	load := s.load
	active := append([]string{}, s.active...)
	s.mu.Unlock()
	ts := time.Now().UTC()
	infer := s.cfg.Infer
	g := protocol.GossipPayload{
		NodeID: s.cfg.NodeID, Name: s.cfg.Name, Mode: s.cfg.Mode, Version: s.cfg.Version,
		ModelsReady: models, Tags: tags, GPUName: info.GPUName, VRAMMb: info.VRAMMb, MaxVRAMMb: maxVRAM,
		HostNest: s.cfg.HostNest, Infer: &infer,
		RuntimeHealthy: healthy,
		ExposeAPI: s.cfg.ExposeAPI, APIAdvertise: s.cfg.APIAdvertise,
		NestURL: s.cfg.LocalNestURL, NestsConnected: active, Load: load, TS: ts,
	}
	g.HMAC = fleetcrypto.GossipMac(s.cfg.JoinSecret, g.NodeID, g.Mode, g.TS, g.ModelsReady, g.Tags)
	s.roster.Upsert(g)
	s.broadcast(protocol.Envelope{Type: protocol.TypeGossip, From: s.cfg.NodeID, TTL: protocol.MaxNestHops, Payload: mustJSON(g)})
}

func (s *Ship) broadcastNestAdvert() {
	if s.cfg.LocalNestURL == "" {
		return
	}
	ts := time.Now().UTC()
	id := s.cfg.LocalNestID
	if id == "" {
		id = s.cfg.NodeID
	}
	a := protocol.NestAdvert{
		NestID: id, URL: s.cfg.LocalNestURL, Kind: "captain", Name: s.cfg.Name, TS: ts,
		Peers: 0,
	}
	a.HMAC = fleetcrypto.NestAdvertMac(s.cfg.JoinSecret, a.NestID, a.URL, a.Kind, a.TS)
	s.catalog.UpsertAdvert(a)
	s.broadcast(protocol.Envelope{Type: protocol.TypeNestAdvert, From: s.cfg.NodeID, TTL: protocol.MaxNestHops, Payload: mustJSON(a)})
}

func (s *Ship) AdvertiseNest(url string) {
	s.cfg.LocalNestURL = url
	s.catalog.AddPrefer(url)
	s.broadcastNestAdvert()
}

func (s *Ship) AddNestURL(url string) {
	s.catalog.AddPrefer(url)
	s.catalog.UpsertAdvert(protocol.NestAdvert{URL: url, Kind: "public", TS: time.Now()})
	// trigger rebalance soon
	go s.probeAll(context.Background())
}

func (s *Ship) probeAll(ctx context.Context) {
	for _, e := range s.catalog.List() {
		u := e.Advert.URL
		start := time.Now()
		ok := s.probeOne(ctx, u)
		if ok {
			s.catalog.SetRTT(u, float64(time.Since(start).Milliseconds()))
		} else {
			s.catalog.MarkUnreachable(u)
		}
	}
}

func (s *Ship) probeOne(ctx context.Context, nestURL string) bool {
	u, err := url.Parse(nestURL)
	if err != nil {
		return false
	}
	d := nestDialer(5 * time.Second)
	cctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	conn, _, err := d.DialContext(cctx, u.String(), nil)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func (s *Ship) broadcast(env protocol.Envelope) {
	b := mustJSON(env)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = c.WriteMessage(websocket.TextMessage, b)
	}
}

func (s *Ship) sendTo(to string, env protocol.Envelope) error {
	if env.TTL == 0 {
		env.TTL = protocol.MaxNestHops
	}
	b := mustJSON(env)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.conns) == 0 {
		return fmt.Errorf("no nest connections")
	}
	for _, c := range s.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := c.WriteMessage(websocket.TextMessage, b); err == nil {
			return nil
		}
	}
	return fmt.Errorf("send failed")
}

func (s *Ship) writeEnv(c *websocket.Conn, env protocol.Envelope) error {
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.WriteMessage(websocket.TextMessage, mustJSON(env))
}

func (s *Ship) DispatchInfer(ctx context.Context, nodeID, path string, body []byte) (protocol.InferResponse, error) {
	reqID := uuid.NewString()
	ch := make(chan protocol.InferResponse, 1)
	s.mu.Lock()
	s.pending[reqID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, reqID)
		s.mu.Unlock()
	}()
	req := protocol.InferRequest{RequestID: reqID, Path: path, Body: body}
	if err := s.sendTo(nodeID, protocol.Envelope{
		Type: protocol.TypeInferReq, From: s.cfg.NodeID, To: nodeID, TTL: protocol.MaxNestHops, Payload: mustJSON(req),
	}); err != nil {
		return protocol.InferResponse{}, err
	}
	select {
	case <-ctx.Done():
		return protocol.InferResponse{}, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

func (s *Ship) SendEnsureModel(nodeID string, m protocol.EnsureModelPayload) error {
	return s.sendTo(nodeID, protocol.Envelope{
		Type: protocol.TypeEnsureModel, From: s.cfg.NodeID, To: nodeID, TTL: protocol.MaxNestHops, Payload: mustJSON(m),
	})
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

// nestDialer accepts self-signed Crow's Nest certificates (Caddy tls internal).
func nestDialer(handshake time.Duration) websocket.Dialer {
	return websocket.Dialer{
		HandshakeTimeout: handshake,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // intentional for self-signed nests
	}
}
