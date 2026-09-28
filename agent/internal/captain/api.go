package captain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/fleetcrypto"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/ship"
)

type API struct {
	Ship       *ship.Ship
	JoinSecret string
	Nests      []string
	APIKeys    map[string]struct{} // raw keys allowed for expose-api
	Name       string
}

func (a *API) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"status": "ok", "role": "captain"})
	})
	mux.HandleFunc("GET /api/v1/peers", a.handlePeers)
	mux.HandleFunc("POST /api/v1/invites", a.handleInvite)
	mux.HandleFunc("POST /api/v1/nests", a.handleAddNestHint)
	mux.HandleFunc("POST /api/v1/ensure-model", a.handleEnsure)
	mux.HandleFunc("GET /v1/models", a.withKey(a.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", a.withKey(a.handleChat))
	mux.HandleFunc("POST /v1/completions", a.withKey(a.handleChat))
	if ui != nil {
		mux.Handle("/", ui)
	}
	return mux
}

func (a *API) withKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if len(a.APIKeys) == 0 {
			next(w, r)
			return
		}
		key := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if key == "" {
			key = r.Header.Get("X-API-Key")
		}
		if _, ok := a.APIKeys[key]; !ok {
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (a *API) handlePeers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{"peers": a.Ship.Roster().List(), "nests": a.Nests})
}

func (a *API) handleInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		TTL  string `json:"ttl"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "deckhand"
	}
	ttl := 24 * time.Hour
	if req.TTL != "" {
		if d, err := time.ParseDuration(req.TTL); err == nil {
			ttl = d
		}
	}
	tok, err := fleetcrypto.IssueInvite(a.JoinSecret, req.Name, a.Nests, ttl)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]any{
		"token": tok,
		"nests": a.Nests,
		"instructions": map[string]string{
			"enroll": "tesla-agent enroll --invite " + tok + " --join-secret <FLEET_JOIN_SECRET>",
			"run":    "tesla-agent run --mode=worker",
		},
	})
}

func (a *API) handleAddNestHint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "bad request", 400)
		return
	}
	for _, n := range a.Nests {
		if n == req.URL {
			writeJSON(w, map[string]any{"nests": a.Nests})
			return
		}
	}
	a.Nests = append(a.Nests, req.URL)
	writeJSON(w, map[string]any{"nests": a.Nests, "note": "restart agent to connect new nest in this MVP"})
}

func (a *API) handleEnsure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID    string `json:"node_id"`
		ModelID   string `json:"model_id"`
		OllamaTag string `json:"ollama_tag"`
		URL       string `json:"url"`
		Format    string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" || req.ModelID == "" {
		http.Error(w, "bad request", 400)
		return
	}
	if err := a.Ship.SendEnsureModel(req.NodeID, protocol.EnsureModelPayload{
		ModelID: req.ModelID, OllamaTag: req.OllamaTag, URL: req.URL, Format: req.Format,
	}); err != nil {
		http.Error(w, err.Error(), 502)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

func (a *API) handleModels(w http.ResponseWriter, _ *http.Request) {
	seen := map[string]bool{}
	data := []map[string]any{}
	for _, p := range a.Ship.Roster().List() {
		for _, m := range p.ModelsReady {
			if seen[m] {
				continue
			}
			seen[m] = true
			data = append(data, map[string]any{"id": m, "object": "model", "owned_by": "pirate-fleet"})
		}
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

func (a *API) handleChat(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", 400)
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		http.Error(w, `{"error":"model required"}`, 400)
		return
	}
	peer, ok := a.Ship.Roster().PickLeastLoad(req.Model)
	if !ok {
		http.Error(w, `{"error":"no ready deckhand for model"}`, http.StatusServiceUnavailable)
		return
	}
	path := "/v1/chat/completions"
	if strings.HasSuffix(r.URL.Path, "/completions") && !strings.Contains(r.URL.Path, "chat") {
		path = "/v1/completions"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	resp, err := a.Ship.DispatchInfer(ctx, peer.NodeID, path, body)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, 502)
		return
	}
	if resp.Error != "" {
		http.Error(w, `{"error":"`+resp.Error+`"}`, 502)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Pirate-Node", peer.Name)
	if resp.StatusCode == 0 {
		resp.StatusCode = 200
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
