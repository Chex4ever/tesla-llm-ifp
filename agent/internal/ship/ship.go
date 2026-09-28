package ship

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/fleetcrypto"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/hwinfo"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/roster"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/runtimeadapter"
	"github.com/gorilla/websocket"
	"github.com/google/uuid"
)

type Config struct {
	NodeID       string
	Name         string
	Mode         string
	JoinSecret   string
	Nests        []string
	Version      string
	ExposeAPI    bool
	APIAdvertise string
	OllamaURL    string
	VLLMURL      string
	Runtime      string
}

type Ship struct {
	cfg    Config
	roster *roster.Store
	rt     runtimeadapter.Runtime

	mu       sync.Mutex
	conns    map[string]*websocket.Conn // nestURL -> conn
	pending  map[string]chan protocol.InferResponse
	load     int
}

func New(cfg Config) (*Ship, error) {
	if cfg.NodeID == "" {
		return nil, fmt.Errorf("node_id required")
	}
	if len(cfg.Nests) == 0 {
		cfg.Nests = []string{protocol.DefaultNestURL}
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
		rt:      rt,
		conns:   map[string]*websocket.Conn{},
		pending: map[string]chan protocol.InferResponse{},
	}, nil
}

func (s *Ship) Roster() *roster.Store { return s.roster }

func (s *Ship) Run(ctx context.Context) error {
	for _, nestURL := range s.cfg.Nests {
		go s.maintainNest(ctx, nestURL)
	}
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	s.broadcastGossip()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			s.broadcastGossip()
		}
	}
}

func (s *Ship) maintainNest(ctx context.Context, nestURL string) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.session(ctx, nestURL); err != nil {
			log.Printf("nest %s: %v (retry in %s)", nestURL, err, backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

func (s *Ship) session(ctx context.Context, nestURL string) error {
	u, err := url.Parse(nestURL)
	if err != nil {
		return err
	}
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second}
	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()

	s.mu.Lock()
	s.conns[nestURL] = conn
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.conns, nestURL)
		s.mu.Unlock()
	}()

	reg := protocol.RegisterPayload{
		NodeID:  s.cfg.NodeID,
		Mode:    s.cfg.Mode,
		Name:    s.cfg.Name,
		Version: s.cfg.Version,
		JoinHMAC: fleetcrypto.HMACHex(s.cfg.JoinSecret, s.cfg.NodeID),
	}
	if err := s.writeEnv(conn, protocol.Envelope{
		Type: protocol.TypeRegister,
		From: s.cfg.NodeID,
		Payload: mustJSON(reg),
	}); err != nil {
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
	case protocol.TypePeerList:
		// presence only — fleet trust comes from signed gossip
	case protocol.TypeGossip:
		var g protocol.GossipPayload
		if err := json.Unmarshal(env.Payload, &g); err != nil {
			return
		}
		if g.NodeID == s.cfg.NodeID {
			return
		}
		want := fleetcrypto.GossipMac(s.cfg.JoinSecret, g.NodeID, g.Mode, g.TS, g.ModelsReady)
		if g.HMAC != want {
			return
		}
		s.roster.Upsert(g)
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
	case protocol.TypeForward:
		// unwrap not needed — Nest sends original typed envelopes
	}
}

func (s *Ship) handleInfer(env protocol.Envelope) {
	var req protocol.InferRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
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
		Type:    protocol.TypeInferResp,
		From:    s.cfg.NodeID,
		To:      env.From,
		Payload: mustJSON(resp),
	})
}

func (s *Ship) broadcastGossip() {
	info := hwinfo.Collect()
	healthy, models, _ := s.rt.Health(context.Background())
	if models == nil {
		models = []string{}
	}
	s.mu.Lock()
	load := s.load
	s.mu.Unlock()
	ts := time.Now().UTC()
	g := protocol.GossipPayload{
		NodeID:         s.cfg.NodeID,
		Name:           s.cfg.Name,
		Mode:           s.cfg.Mode,
		Version:        s.cfg.Version,
		ModelsReady:    models,
		GPUName:        info.GPUName,
		VRAMMb:         info.VRAMMb,
		RuntimeHealthy: healthy,
		ExposeAPI:      s.cfg.ExposeAPI,
		APIAdvertise:   s.cfg.APIAdvertise,
		Load:           load,
		TS:             ts,
	}
	g.HMAC = fleetcrypto.GossipMac(s.cfg.JoinSecret, g.NodeID, g.Mode, g.TS, g.ModelsReady)
	s.roster.Upsert(g) // include self
	env := protocol.Envelope{
		Type:    protocol.TypeGossip,
		From:    s.cfg.NodeID,
		Payload: mustJSON(g),
	}
	s.broadcast(env)
}

func (s *Ship) broadcast(env protocol.Envelope) {
	b, _ := json.Marshal(env)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.conns {
		_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
		_ = c.WriteMessage(websocket.TextMessage, b)
	}
}

func (s *Ship) sendTo(to string, env protocol.Envelope) error {
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
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
	b, err := json.Marshal(env)
	if err != nil {
		return err
	}
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.WriteMessage(websocket.TextMessage, b)
}

// DispatchInfer sends a job to a peer via Nest relay and waits for response.
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
		Type: protocol.TypeInferReq, From: s.cfg.NodeID, To: nodeID, Payload: mustJSON(req),
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
		Type: protocol.TypeEnsureModel, From: s.cfg.NodeID, To: nodeID, Payload: mustJSON(m),
	})
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
