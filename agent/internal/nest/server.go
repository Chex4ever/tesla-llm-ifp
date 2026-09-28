package nest

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/gorilla/websocket"
)

// Server is a Crow's Nest: zero-state rendezvous + byte relay. No fleet brain.
type Server struct {
	mu    sync.RWMutex
	peers map[string]*peerConn
	up    websocket.Upgrader
}

type peerConn struct {
	nodeID string
	mode   string
	name   string
	addrs  []string
	conn   *websocket.Conn
	send   chan []byte
	seen   time.Time
}

func New() *Server {
	return &Server{
		peers: map[string]*peerConn{},
		up: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok","role":"crowsnest"}`))
	})
	mux.HandleFunc("GET /nest", s.handleWS)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("Pirate Fleet Crow's Nest — zero router. Connect via /nest (WebSocket).\n"))
	})
	return mux
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	pc := &peerConn{
		conn: c,
		send: make(chan []byte, 64),
		seen: time.Now(),
	}
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
		switch env.Type {
		case protocol.TypeRegister:
			var reg protocol.RegisterPayload
			if err := json.Unmarshal(env.Payload, &reg); err != nil || reg.NodeID == "" {
				continue
			}
			pc.nodeID = reg.NodeID
			pc.mode = reg.Mode
			pc.name = reg.Name
			pc.addrs = reg.Addrs
			pc.seen = time.Now()
			s.register(pc)
			s.sendJSON(pc, protocol.Envelope{Type: protocol.TypeRegistered, To: pc.nodeID})
			s.broadcastPeerList()
		case protocol.TypeForward:
			if env.To == "" {
				continue
			}
			s.forward(env.To, data)
		case protocol.TypePing:
			s.sendJSON(pc, protocol.Envelope{Type: protocol.TypePong, To: pc.nodeID})
		case protocol.TypeGossip, protocol.TypeInferReq, protocol.TypeInferResp, protocol.TypeEnsureModel:
			// Ships may send typed envelopes as forward shorthand
			if env.To != "" {
				s.forward(env.To, data)
			} else {
				// broadcast to all other peers (signaling aid)
				s.broadcastExcept(pc.nodeID, data)
			}
		default:
			if env.To != "" {
				s.forward(env.To, data)
			}
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
	if old, ok := s.peers[pc.nodeID]; ok && old != pc {
		_ = old.conn.Close()
	}
	s.peers[pc.nodeID] = pc
	log.Printf("crowsnest: registered %s (%s)", pc.nodeID, pc.mode)
}

func (s *Server) unregister(pc *peerConn) {
	s.mu.Lock()
	if pc.nodeID != "" {
		if cur, ok := s.peers[pc.nodeID]; ok && cur == pc {
			delete(s.peers, pc.nodeID)
			log.Printf("crowsnest: left %s", pc.nodeID)
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

func (s *Server) forward(to string, data []byte) {
	s.mu.RLock()
	dst := s.peers[to]
	s.mu.RUnlock()
	s.trySend(dst, data)
}

func (s *Server) broadcastExcept(from string, data []byte) {
	s.mu.RLock()
	targets := make([]*peerConn, 0, len(s.peers))
	for id, p := range s.peers {
		if id == from {
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
		if p.nodeID == "" {
			continue
		}
		list = append(list, protocol.PeerPresence{
			NodeID: p.nodeID,
			Mode:   p.mode,
			Name:   p.name,
			Addrs:  p.addrs,
			SeenAt: p.seen,
		})
		targets = append(targets, p)
	}
	s.mu.RUnlock()

	payload, _ := json.Marshal(protocol.PeerListPayload{Peers: list})
	env, _ := json.Marshal(protocol.Envelope{Type: protocol.TypePeerList, Payload: payload})
	for _, p := range targets {
		s.trySend(p, env)
	}
}

func (s *Server) sendJSON(pc *peerConn, env protocol.Envelope) {
	b, _ := json.Marshal(env)
	s.trySend(pc, b)
}

// ListenAndServe runs the Nest HTTP server.
func ListenAndServe(addr string) error {
	s := New()
	log.Printf("Crow's Nest listening on %s (WS /nest)", addr)
	return http.ListenAndServe(addr, s.Handler())
}
