package protocol

import "time"

const (
	DefaultNestURL   = "wss://fleet.teslant.ru/nest"
	DefaultCaptainUI = "127.0.0.1:7842"
	DefaultAPIAddr   = "127.0.0.1:8080"
	AgentVersion     = "0.2.0"

	ModeWorker    = "worker"    // Deckhand
	ModeCaptain   = "captain"   // Captain
	ModeCrowsNest = "crowsnest" // Crow's Nest
)

// Envelope is the wire format over Nest relay and mesh.
type Envelope struct {
	Type   string `json:"type"`
	From   string `json:"from,omitempty"`
	To     string `json:"to,omitempty"` // empty = broadcast via nest presence
	Nest   string `json:"nest,omitempty"`
	Payload []byte `json:"payload,omitempty"`
}

const (
	TypeRegister   = "register"
	TypeRegistered = "registered"
	TypePeerList   = "peer_list"
	TypeForward    = "forward"
	TypePing       = "ping"
	TypePong       = "pong"
	TypeGossip     = "gossip"
	TypeInferReq   = "infer_req"
	TypeInferResp  = "infer_resp"
	TypeEnsureModel = "ensure_model"
)

type RegisterPayload struct {
	NodeID   string   `json:"node_id"`
	Mode     string   `json:"mode"`
	Name     string   `json:"name,omitempty"`
	Addrs    []string `json:"addrs,omitempty"`
	Version  string   `json:"version"`
	JoinHMAC string   `json:"join_hmac,omitempty"` // optional; Nest may ignore
}

type PeerListPayload struct {
	Peers []PeerPresence `json:"peers"`
}

type PeerPresence struct {
	NodeID  string    `json:"node_id"`
	Mode    string    `json:"mode"`
	Name    string    `json:"name,omitempty"`
	Addrs   []string  `json:"addrs,omitempty"`
	SeenAt  time.Time `json:"seen_at"`
}

// GossipPayload is fleet brain data exchanged between ships (not stored on Nest).
type GossipPayload struct {
	NodeID         string    `json:"node_id"`
	Name           string    `json:"name"`
	Mode           string    `json:"mode"`
	Version        string    `json:"version"`
	ModelsReady    []string  `json:"models_ready"`
	GPUName        string    `json:"gpu_name,omitempty"`
	VRAMMb         int       `json:"vram_mb,omitempty"`
	RuntimeHealthy bool      `json:"runtime_healthy"`
	ExposeAPI      bool      `json:"expose_api"`
	APIAdvertise   string    `json:"api_advertise,omitempty"`
	Load           int       `json:"load"`
	HMAC           string    `json:"hmac"` // HMAC(join_secret, canonical fields)
	TS             time.Time `json:"ts"`
}

type InferRequest struct {
	RequestID string `json:"request_id"`
	Path      string `json:"path"`
	Body      []byte `json:"body"`
}

type InferResponse struct {
	RequestID  string `json:"request_id"`
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
	Error      string `json:"error,omitempty"`
}

type EnsureModelPayload struct {
	ModelID   string `json:"model_id"`
	OllamaTag string `json:"ollama_tag,omitempty"`
	URL       string `json:"url,omitempty"`
	Format    string `json:"format,omitempty"`
}
