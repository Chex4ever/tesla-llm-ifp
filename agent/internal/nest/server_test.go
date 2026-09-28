package nest

import (
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
