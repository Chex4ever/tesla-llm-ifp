package agent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/google/uuid"
)

type State struct {
	NodeID         string   `json:"node_id"`
	Name           string   `json:"name"`
	JoinSecret     string   `json:"join_secret"`
	Nests          []string `json:"nests"`
	Mode           string   `json:"mode"` // worker | captain | crowsnest — captain = Host Nest + UI
	ExposeAPI      bool     `json:"expose_api"`
	Tags           []string `json:"tags,omitempty"`
	MaxVRAMMb      int      `json:"max_vram_mb,omitempty"` // 0 = use detected GPU VRAM
	APIKeys        []string `json:"api_keys,omitempty"`
	APIAddr        string   `json:"api_addr,omitempty"`
	UIAddr         string   `json:"ui_addr,omitempty"`
	NestAddr       string   `json:"nest_addr,omitempty"`
	NestID         string   `json:"nest_id,omitempty"`
	NestAdvertHost string   `json:"nest_advert_host,omitempty"`
	LocalNestURL   string   `json:"local_nest_url,omitempty"`
	OllamaURL      string   `json:"ollama_url,omitempty"`
	VLLMURL        string   `json:"vllm_url,omitempty"`
	Runtime        string   `json:"runtime,omitempty"`
}

func LoadOrInitState(dir string) (*State, error) {
	path := StatePath(dir)
	b, err := os.ReadFile(path)
	if err == nil {
		var st State
		if err := json.Unmarshal(b, &st); err != nil {
			return nil, err
		}
		return &st, nil
	}
	st := &State{
		NodeID:    uuid.NewString(),
		Nests:     []string{},
		Mode:      protocol.ModeWorker,
		APIAddr:   protocol.DefaultAPIAddr,
		UIAddr:    protocol.DefaultCaptainUI,
		NestAddr:  ":7843",
		OllamaURL: "http://127.0.0.1:11434",
		Runtime:   "ollama",
	}
	return st, SaveState(dir, st)
}

func SaveState(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(dir), raw, 0o600)
}

func StatePath(dir string) string {
	return filepath.Join(dir, "state.json")
}
