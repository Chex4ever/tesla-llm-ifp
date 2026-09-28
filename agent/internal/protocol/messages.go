package protocol

import "time"

const (
	// DefaultNestURL is empty: captains deploy or paste their own nest (no central host required).
	DefaultNestURL = ""
	DefaultCaptainUI = "127.0.0.1:7842"
	DefaultAPIAddr   = "127.0.0.1:8080"
	DefaultNestAddr  = "0.0.0.0:7843"
	AgentVersion     = "0.3.0"

	ModeWorker    = "worker"
	ModeCaptain   = "captain"
	ModeCrowsNest = "crowsnest"

	MaxNestCatalog  = 32
	MaxActiveNests  = 3
	MaxNestHops     = 3
	NestAdvertTTL   = 10 * time.Minute
)

// Envelope is the wire format over Nest relay and mesh.
type Envelope struct {
	Type    string   `json:"type"`
	From    string   `json:"from,omitempty"`
	To      string   `json:"to,omitempty"`
	Nest    string   `json:"nest,omitempty"`
	TTL     int      `json:"ttl,omitempty"`
	Via     []string `json:"via,omitempty"`
	Payload []byte   `json:"payload,omitempty"`
}

const (
	TypeRegister    = "register"
	TypeRegistered  = "registered"
	TypePeerList    = "peer_list"
	TypeForward     = "forward"
	TypePing        = "ping"
	TypePong        = "pong"
	TypeGossip      = "gossip"
	TypeInferReq    = "infer_req"
	TypeInferResp   = "infer_resp"
	TypeEnsureModel = "ensure_model"
	TypeNestAdvert  = "nest_advert"
	TypeNestPing    = "nest_ping"
	TypeNestPong    = "nest_pong"
	TypeLocate      = "locate"
	TypeLocateResult = "locate_result"
	TypeNestHello   = "nest_hello" // nest-to-nest link
)

type RegisterPayload struct {
	NodeID   string   `json:"node_id"`
	Mode     string   `json:"mode"`
	Name     string   `json:"name,omitempty"`
	Addrs    []string `json:"addrs,omitempty"`
	Version  string   `json:"version"`
	JoinHMAC string   `json:"join_hmac,omitempty"`
	IsNest   bool     `json:"is_nest,omitempty"` // connecting nest speaks nest_hello after
}

type PeerListPayload struct {
	Peers []PeerPresence `json:"peers"`
}

type PeerPresence struct {
	NodeID string    `json:"node_id"`
	Mode   string    `json:"mode"`
	Name   string    `json:"name,omitempty"`
	Addrs  []string  `json:"addrs,omitempty"`
	SeenAt time.Time `json:"seen_at"`
}

type GossipPayload struct {
	NodeID         string    `json:"node_id"`
	Name           string    `json:"name"`
	Mode           string    `json:"mode"`
	Version        string    `json:"version"`
	ModelsReady    []string  `json:"models_ready"`
	Tags           []string  `json:"tags,omitempty"`
	GPUName        string    `json:"gpu_name,omitempty"`
	VRAMMb         int       `json:"vram_mb,omitempty"`
	MaxVRAMMb      int       `json:"max_vram_mb,omitempty"` // capacity: will run models up to this
	RuntimeHealthy bool      `json:"runtime_healthy"`
	ExposeAPI      bool      `json:"expose_api"`
	APIAdvertise   string    `json:"api_advertise,omitempty"`
	NestURL        string    `json:"nest_url,omitempty"` // captain's local nest
	NestsConnected []string  `json:"nests_connected,omitempty"`
	Load           int       `json:"load"`
	HMAC           string    `json:"hmac"`
	TS             time.Time `json:"ts"`
}

type NestAdvert struct {
	NestID string    `json:"nest_id"`
	URL    string    `json:"url"`
	Kind   string    `json:"kind"` // captain | public
	Name   string    `json:"name,omitempty"`
	Peers  int       `json:"peers,omitempty"`
	TS     time.Time `json:"ts"`
	HMAC   string    `json:"hmac"`
}

type NestPing struct {
	ID int64 `json:"id"`
	TS int64 `json:"ts"` // unix milli
}

type NestPong struct {
	ID int64 `json:"id"`
	TS int64 `json:"ts"`
}

type LocateQuery struct {
	NodeID    string `json:"node_id"`
	RequestID string `json:"request_id"`
}

type LocateResult struct {
	NodeID    string `json:"node_id"`
	RequestID string `json:"request_id"`
	Found     bool   `json:"found"`
	NestID    string `json:"nest_id,omitempty"`
	NestURL   string `json:"nest_url,omitempty"`
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

type NestHello struct {
	NestID string `json:"nest_id"`
	URL    string `json:"url"`
	Kind   string `json:"kind"`
}
