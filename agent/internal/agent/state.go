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
	Mode           string   `json:"mode"` // ship | crowsnest (legacy: captain|worker migrated on load)
	HostNest       bool     `json:"host_nest"`
	Infer          bool     `json:"infer"` // accept inference jobs
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

// stateFile supports pointer flags so missing host_nest/infer can be migrated from legacy mode.
type stateFile struct {
	NodeID         string   `json:"node_id"`
	Name           string   `json:"name"`
	JoinSecret     string   `json:"join_secret"`
	Nests          []string `json:"nests"`
	Mode           string   `json:"mode"`
	HostNest       *bool    `json:"host_nest"`
	Infer          *bool    `json:"infer"`
	ExposeAPI      bool     `json:"expose_api"`
	Tags           []string `json:"tags,omitempty"`
	MaxVRAMMb      int      `json:"max_vram_mb,omitempty"`
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
		var raw stateFile
		if err := json.Unmarshal(b, &raw); err != nil {
			return nil, err
		}
		st := migrateState(raw)
		return st, nil
	}
	st := &State{
		NodeID:    uuid.NewString(),
		Nests:     []string{},
		Mode:      protocol.ModeShip,
		HostNest:  false,
		Infer:     true,
		APIAddr:   protocol.DefaultAPIAddr,
		UIAddr:    protocol.DefaultCaptainUI,
		NestAddr:  ":7843",
		OllamaURL: "http://127.0.0.1:11434",
		Runtime:   "ollama",
	}
	return st, SaveState(dir, st)
}

func migrateState(raw stateFile) *State {
	st := &State{
		NodeID: raw.NodeID, Name: raw.Name, JoinSecret: raw.JoinSecret,
		Nests: raw.Nests, Mode: raw.Mode, ExposeAPI: raw.ExposeAPI,
		Tags: raw.Tags, MaxVRAMMb: raw.MaxVRAMMb, APIKeys: raw.APIKeys,
		APIAddr: raw.APIAddr, UIAddr: raw.UIAddr, NestAddr: raw.NestAddr,
		NestID: raw.NestID, NestAdvertHost: raw.NestAdvertHost, LocalNestURL: raw.LocalNestURL,
		OllamaURL: raw.OllamaURL, VLLMURL: raw.VLLMURL, Runtime: raw.Runtime,
	}
	switch st.Mode {
	case protocol.ModeCaptain:
		if raw.HostNest == nil {
			st.HostNest = true
		} else {
			st.HostNest = *raw.HostNest
		}
		st.Mode = protocol.ModeShip
	case protocol.ModeWorker, "":
		if raw.HostNest == nil {
			st.HostNest = false
		} else {
			st.HostNest = *raw.HostNest
		}
		if st.Mode == "" || st.Mode == protocol.ModeWorker {
			st.Mode = protocol.ModeShip
		}
	case protocol.ModeShip:
		if raw.HostNest != nil {
			st.HostNest = *raw.HostNest
		}
	case protocol.ModeCrowsNest:
		// leave mode as-is
		if raw.HostNest != nil {
			st.HostNest = *raw.HostNest
		}
	default:
		if raw.HostNest != nil {
			st.HostNest = *raw.HostNest
		}
	}
	if raw.Infer != nil {
		st.Infer = *raw.Infer
	} else {
		st.Infer = true
	}
	if st.Mode == "" {
		st.Mode = protocol.ModeShip
	}
	return st
}

func SaveState(dir string, st *State) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	normalizeShipMode(st)
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(StatePath(dir), raw, 0o600)
}

func normalizeShipMode(st *State) {
	if st == nil {
		return
	}
	if st.Mode == protocol.ModeCaptain || st.Mode == protocol.ModeWorker {
		st.Mode = protocol.ModeShip
	}
	if st.Mode == "" {
		st.Mode = protocol.ModeShip
	}
}

func StatePath(dir string) string {
	return filepath.Join(dir, "state.json")
}
