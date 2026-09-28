package nest

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Server is a Crow's Nest: rendezvous + local relay + Nest↔Nest multi-hop. RAM only.
type Server struct {
	ID   string
	URL  string // advertised URL of this nest
	Kind string // captain | public

	mu          sync.RWMutex
	peers       map[string]*peerConn // local ships + remote nests keyed by node/nest id
	locateCache map[string]locateHit // node_id -> nest
	peerNests   map[string]string    // nest_id -> url

	up websocket.Upgrader
}

type locateHit struct {
	NestID  string
	NestURL string
	At      time.Time
}

type peerConn struct {
	id        string
	mode      string
	name      string
	addrs     []string
	isNest    bool
	nestURL   string
	helloSent bool // nest↔nest: only one NestHello reply per connection
	conn      *websocket.Conn
	send      chan []byte
	seen      time.Time
}

func New(id, advertURL, kind string) *Server {
	if id == "" {
		id = uuid.NewString()
	}
	if kind == "" {
		kind = "public"
	}
	return &Server{
		ID:          id,
		URL:         advertURL,
		Kind:        kind,
		peers:       map[string]*peerConn{},
		locateCache: map[string]locateHit{},
		peerNests:   map[string]string{},
		up: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		s.mu.RLock()
		n := len(s.peers)
		s.mu.RUnlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "ok", "role": "crowsnest", "nest_id": s.ID, "peers": n, "kind": s.Kind,
		})
	})
	mux.HandleFunc("GET /nest", s.handleWS)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Pirate Fleet Crow's Nest — zero router. Connect via /nest (WebSocket).\n"))
	})
	return mux
}

func (s *Server) LocalPeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, p := range s.peers {
		if !p.isNest {
			n++
		}
	}
	return n
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	pc := &peerConn{conn: c, send: make(chan []byte, 128), seen: time.Now()}
	go s.writePump(pc)
	s.readPump(pc)
}

func (s *Server) readPump(pc *peerConn) {
	defer func() {
		s.unregister(pc)
		_ = pc.conn.Close()
	}()
	_ = pc.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
	pc.conn.SetPongHandler(func(string) error {
		_ = pc.conn.SetReadDeadline(time.Now().Add(120 * time.Second))
		return nil
	})
	for {
		_, data, err := pc.conn.ReadMessage()
		if err != nil {
			return
		}
		var env protocol.Envelope
		if err := json.Unmarshal(data, &env); err != nil {
			continue
		}
		s.handleEnvelope(pc, &env, data)
	}
}

func (s *Server) handleEnvelope(pc *peerConn, env *protocol.Envelope, raw []byte) {
	switch env.Type {
	case protocol.TypeRegister:
		var reg protocol.RegisterPayload
		if err := json.Unmarshal(env.Payload, &reg); err != nil || reg.NodeID == "" {
			return
		}
		pc.id = reg.NodeID
		pc.mode = reg.Mode
		pc.name = reg.Name
		pc.addrs = reg.Addrs
		pc.isNest = reg.IsNest || reg.Mode == protocol.ModeCrowsNest
		pc.seen = time.Now()
		s.register(pc)
		s.sendJSON(pc, protocol.Envelope{Type: protocol.TypeRegistered, To: pc.id, Nest: s.ID})
		s.broadcastPeerList()
		if pc.isNest {
			// expect nest_hello soon
		}
	case protocol.TypeNestHello:
		var h protocol.NestHello
		if err := json.Unmarshal(env.Payload, &h); err != nil || h.NestID == "" {
			return
		}
		pc.id = h.NestID
		pc.isNest = true
		pc.nestURL = h.URL
		pc.mode = protocol.ModeCrowsNest
		s.register(pc)
		s.mu.Lock()
		s.peerNests[h.NestID] = h.URL
		s.mu.Unlock()
		// Reply once so dialer↔acceptor don't echo NestHello forever.
		if !pc.helloSent {
			pc.helloSent = true
			s.sendJSON(pc, protocol.Envelope{
				Type: protocol.TypeNestHello, From: s.ID, Nest: s.ID,
				Payload: mustJSON(protocol.NestHello{NestID: s.ID, URL: s.URL, Kind: s.Kind}),
			})
		}
	case protocol.TypeForward, protocol.TypeGossip, protocol.TypeInferReq, protocol.TypeInferResp,
		protocol.TypeEnsureModel, protocol.TypeNestAdvert:
		s.route(env, raw, pc.id)
	case protocol.TypePing:
		s.sendJSON(pc, protocol.Envelope{Type: protocol.TypePong, To: pc.id, Nest: s.ID})
	case protocol.TypeNestPing:
		s.sendJSON(pc, protocol.Envelope{Type: protocol.TypeNestPong, To: pc.id, Nest: s.ID, Payload: env.Payload})
	case protocol.TypeLocate:
		s.handleLocate(pc, env)
	case protocol.TypeLocateResult:
		var lr protocol.LocateResult
		if err := json.Unmarshal(env.Payload, &lr); err == nil && lr.Found {
			s.mu.Lock()
			s.locateCache[lr.NodeID] = locateHit{NestID: lr.NestID, NestURL: lr.NestURL, At: time.Now()}
			s.mu.Unlock()
		}
		if env.To != "" {
			s.deliverLocalOrHop(env, raw)
		}
	default:
		if env.To != "" {
			s.route(env, raw, pc.id)
		}
	}
}

func (s *Server) route(env *protocol.Envelope, raw []byte, fromID string) {
	if env.TTL == 0 && env.To != "" {
		env.TTL = protocol.MaxNestHops
		raw = remarshal(env)
	}
	if env.To == "" {
		// broadcast to local ships; also flood to peer nests with ttl
		s.broadcastExcept(fromID, raw)
		if env.Type == protocol.TypeGossip || env.Type == protocol.TypeNestAdvert {
			s.floodNests(env, raw, fromID)
		}
		return
	}
	s.deliverLocalOrHop(env, raw)
}

func (s *Server) deliverLocalOrHop(env *protocol.Envelope, raw []byte) {
	s.mu.RLock()
	dst := s.peers[env.To]
	s.mu.RUnlock()
	if dst != nil {
		s.trySend(dst, raw)
		return
	}
	// multi-hop
	if env.TTL <= 0 {
		return
	}
	for _, v := range env.Via {
		if v == s.ID {
			return // loop
		}
	}
	env.Via = append(append([]string{}, env.Via...), s.ID)
	env.TTL--
	raw = remarshal(env)

	// try locate cache
	s.mu.RLock()
	hit, ok := s.locateCache[env.To]
	var nestPeer *peerConn
	if ok {
		nestPeer = s.peers[hit.NestID]
	}
	// else any peer nest
	var anyNest []*peerConn
	for _, p := range s.peers {
		if p.isNest && p.id != "" {
			anyNest = append(anyNest, p)
		}
	}
	s.mu.RUnlock()

	if nestPeer != nil {
		s.trySend(nestPeer, raw)
		return
	}
	for _, p := range anyNest {
		s.trySend(p, raw)
	}
}

func (s *Server) floodNests(env *protocol.Envelope, raw []byte, fromID string) {
	if env.TTL == 0 {
		env.TTL = protocol.MaxNestHops
	}
	for _, v := range env.Via {
		if v == s.ID {
			return
		}
	}
	env2 := *env
	env2.Via = append(append([]string{}, env.Via...), s.ID)
	env2.TTL = env.TTL - 1
	if env2.TTL < 0 {
		return
	}
	raw2 := remarshal(&env2)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.peers {
		if p.isNest && p.id != fromID {
			s.trySend(p, raw2)
		}
	}
}

func (s *Server) handleLocate(pc *peerConn, env *protocol.Envelope) {
	var q protocol.LocateQuery
	if err := json.Unmarshal(env.Payload, &q); err != nil {
		return
	}
	s.mu.RLock()
	_, local := s.peers[q.NodeID]
	s.mu.RUnlock()
	if local {
		res := protocol.LocateResult{NodeID: q.NodeID, RequestID: q.RequestID, Found: true, NestID: s.ID, NestURL: s.URL}
		s.sendJSON(pc, protocol.Envelope{
			Type: protocol.TypeLocateResult, From: s.ID, To: env.From, Nest: s.ID, Payload: mustJSON(res),
		})
		return
	}
	// ask peer nests
	if env.TTL == 0 {
		env.TTL = protocol.MaxNestHops
	}
	if env.TTL <= 0 {
		return
	}
	env.TTL--
	env.Via = append(env.Via, s.ID)
	raw := remarshal(env)
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.peers {
		if p.isNest {
			s.trySend(p, raw)
		}
	}
}

func (s *Server) writePump(pc *peerConn) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case msg := <-pc.send:
			_ = pc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := pc.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = pc.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := pc.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

func (s *Server) register(pc *peerConn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if pc.id == "" {
		return
	}
	if old, ok := s.peers[pc.id]; ok && old == pc {
		return // same connection already mapped; avoid log spam
	}
	if old, ok := s.peers[pc.id]; ok && old != pc {
		_ = old.conn.Close()
	}
	s.peers[pc.id] = pc
	log.Printf("crowsnest %s: registered %s (nest=%v)", shortID(s.ID), pc.id, pc.isNest)
}

func shortID(id string) string {
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func (s *Server) unregister(pc *peerConn) {
	s.mu.Lock()
	if pc.id != "" {
		if cur, ok := s.peers[pc.id]; ok && cur == pc {
			delete(s.peers, pc.id)
			log.Printf("crowsnest: left %s", pc.id)
		}
	}
	s.mu.Unlock()
	s.broadcastPeerList()
}

func (s *Server) trySend(pc *peerConn, data []byte) {
	if pc == nil {
		return
	}
	defer func() { _ = recover() }()
	select {
	case pc.send <- data:
	default:
	}
}

func (s *Server) broadcastExcept(from string, data []byte) {
	s.mu.RLock()
	targets := make([]*peerConn, 0, len(s.peers))
	for id, p := range s.peers {
		if id == from || p.isNest {
			continue
		}
		targets = append(targets, p)
	}
	s.mu.RUnlock()
	for _, p := range targets {
		s.trySend(p, data)
	}
}

func (s *Server) broadcastPeerList() {
	s.mu.RLock()
	list := make([]protocol.PeerPresence, 0, len(s.peers))
	targets := make([]*peerConn, 0, len(s.peers))
	for _, p := range s.peers {
		if p.id == "" || p.isNest {
			continue
		}
		list = append(list, protocol.PeerPresence{
			NodeID: p.id, Mode: p.mode, Name: p.name, Addrs: p.addrs, SeenAt: p.seen,
		})
		targets = append(targets, p)
	}
	// also notify nests
	for _, p := range s.peers {
		if p.isNest {
			targets = append(targets, p)
		}
	}
	s.mu.RUnlock()
	payload, _ := json.Marshal(protocol.PeerListPayload{Peers: list})
	env, _ := json.Marshal(protocol.Envelope{Type: protocol.TypePeerList, Nest: s.ID, Payload: payload})
	for _, p := range targets {
		s.trySend(p, env)
	}
}

func (s *Server) sendJSON(pc *peerConn, env protocol.Envelope) {
	s.trySend(pc, mustJSON(env))
}

// DialPeerNest maintains an uplink/peer nest connection.
func (s *Server) DialPeerNest(ctx context.Context, nestURL string) {
	backoff := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := s.sessionPeerNest(ctx, nestURL); err != nil {
			log.Printf("crowsnest peer %s: %v", nestURL, err)
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

func (s *Server) sessionPeerNest(ctx context.Context, nestURL string) error {
	u, err := url.Parse(nestURL)
	if err != nil {
		return err
	}
	d := websocket.Dialer{
		HandshakeTimeout: 15 * time.Second,
		TLSClientConfig:  &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed nests
	}
	conn, _, err := d.DialContext(ctx, u.String(), nil)
	if err != nil {
		return err
	}
	defer conn.Close()
	pc := &peerConn{
		id: "pending-" + nestURL, isNest: true, nestURL: nestURL, helloSent: true,
		conn: conn, send: make(chan []byte, 128), seen: time.Now(),
	}
	go s.writePump(pc)
	// register as nest
	_ = writeEnv(conn, protocol.Envelope{
		Type: protocol.TypeRegister, From: s.ID,
		Payload: mustJSON(protocol.RegisterPayload{
			NodeID: s.ID, Mode: protocol.ModeCrowsNest, Version: protocol.AgentVersion, IsNest: true,
		}),
	})
	_ = writeEnv(conn, protocol.Envelope{
		Type: protocol.TypeNestHello, From: s.ID, Nest: s.ID,
		Payload: mustJSON(protocol.NestHello{NestID: s.ID, URL: s.URL, Kind: s.Kind}),
	})
	s.readPump(pc)
	return nil
}

func writeEnv(c *websocket.Conn, env protocol.Envelope) error {
	_ = c.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return c.WriteMessage(websocket.TextMessage, mustJSON(env))
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func remarshal(env *protocol.Envelope) []byte {
	b, _ := json.Marshal(env)
	return b
}

// ListenAndServe convenience for standalone crowsnest mode.
func ListenAndServe(addr, advertURL string) error {
	s := New("", advertURL, "public")
	log.Printf("Crow's Nest listening on %s (WS /nest) id=%s", addr, s.ID)
	return http.ListenAndServe(addr, s.Handler())
}
