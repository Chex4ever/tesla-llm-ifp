package nest

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/gorilla/websocket"
)

func TestNestRegisterAndPeerList(t *testing.T) {
	s := New("test-nest", "ws://localhost/nest", "public")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/nest"
	c1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	reg := func(c *websocket.Conn, id string) {
		payload, _ := json.Marshal(protocol.RegisterPayload{NodeID: id, Mode: "worker", Version: "t"})
		env, _ := json.Marshal(protocol.Envelope{Type: protocol.TypeRegister, From: id, Payload: payload})
		if err := c.WriteMessage(websocket.TextMessage, env); err != nil {
			t.Fatal(err)
		}
	}
	reg(c1, "n1")
	reg(c2, "n2")

	deadline := time.Now().Add(2 * time.Second)
	seen := false
	for time.Now().Before(deadline) {
		_ = c1.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		_, data, err := c1.ReadMessage()
		if err != nil {
			continue
		}
		var env protocol.Envelope
		_ = json.Unmarshal(data, &env)
		if env.Type == protocol.TypePeerList {
			var pl protocol.PeerListPayload
			_ = json.Unmarshal(env.Payload, &pl)
			if len(pl.Peers) >= 2 {
				seen = true
				break
			}
		}
	}
	if !seen {
		t.Fatal("expected peer_list with 2 peers")
	}
}

func TestNestHelloRepliesOnce(t *testing.T) {
	s := New("nest-b", "ws://b/nest", "public")
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/nest"
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	reg, _ := json.Marshal(protocol.RegisterPayload{
		NodeID: "nest-a", Mode: protocol.ModeCrowsNest, Version: "t", IsNest: true,
	})
	hello, _ := json.Marshal(protocol.NestHello{NestID: "nest-a", URL: "ws://a/nest", Kind: "captain"})
	for _, env := range []protocol.Envelope{
		{Type: protocol.TypeRegister, From: "nest-a", Payload: reg},
		{Type: protocol.TypeNestHello, From: "nest-a", Nest: "nest-a", Payload: hello},
	} {
		raw, _ := json.Marshal(env)
		if err := c.WriteMessage(websocket.TextMessage, raw); err != nil {
			t.Fatal(err)
		}
	}

	// Echo the first nest_hello back (old buggy peers did this unconditionally).
	hellos := 0
	echoed := false
	_ = c.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			break
		}
		var env protocol.Envelope
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.Type != protocol.TypeNestHello {
			continue
		}
		hellos++
		if !echoed {
			echoed = true
			reply, _ := json.Marshal(protocol.Envelope{
				Type: protocol.TypeNestHello, From: "nest-a", Nest: "nest-a", Payload: hello,
			})
			if err := c.WriteMessage(websocket.TextMessage, reply); err != nil {
				t.Fatalf("echo nest_hello: %v", err)
			}
		}
	}
	if hellos != 1 {
		t.Fatalf("expected exactly 1 nest_hello reply after echo, got %d (ping-pong?)", hellos)
	}

	s.mu.RLock()
	_, ok := s.peers["nest-a"]
	s.mu.RUnlock()
	if !ok {
		t.Fatal("expected nest-a registered on acceptor")
	}
}

func TestNestToNestDialHandshake(t *testing.T) {
	b := New("nest-b", "ws://b/nest", "public")
	srvB := httptest.NewServer(b.Handler())
	defer srvB.Close()
	wsURL := "ws" + strings.TrimPrefix(srvB.URL, "http") + "/nest"

	a := New("nest-a", "ws://a/nest", "captain")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.DialPeerNest(ctx, wsURL)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		a.mu.RLock()
		_, gotB := a.peers["nest-b"]
		a.mu.RUnlock()
		b.mu.RLock()
		_, gotA := b.peers["nest-a"]
		b.mu.RUnlock()
		if gotA && gotB {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expected mutual nest registration via DialPeerNest")
}
