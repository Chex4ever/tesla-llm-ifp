package captain

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/fleetcrypto"
	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/Chex4ever/pirate-fleet/agent/internal/roster"
	"github.com/Chex4ever/pirate-fleet/agent/internal/ship"
)

type API struct {
	Ship         *ship.Ship
	JoinSecret   string
	Nests        []string
	APIKeys      map[string]struct{}
	Name         string
	LocalNestURL string
	NestID       string
}

func (a *API) Handler(ui http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"status": "ok", "role": "captain", "nest_url": a.LocalNestURL})
	})
	mux.HandleFunc("GET /api/v1/peers", a.handlePeers)
	mux.HandleFunc("GET /api/v1/nests", a.handleNests)
	mux.HandleFunc("POST /api/v1/invites", a.handleInvite)
	mux.HandleFunc("POST /api/v1/nests", a.handleAddNest)
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

func (a *API) inviteNests() []string {
	seeds := []string{}
	if a.LocalNestURL != "" {
		seeds = append(seeds, a.LocalNestURL)
	}
	seeds = append(seeds, a.Ship.ActiveNests()...)
	seeds = append(seeds, a.Nests...)
	seen := map[string]bool{}
	var out []string
	for _, n := range seeds {
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}

func (a *API) handlePeers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"peers":        a.Ship.Roster().List(),
		"nests":        a.inviteNests(),
		"my_nest":      a.LocalNestURL,
		"active_nests": a.Ship.ActiveNests(),
	})
}

func (a *API) handleNests(w http.ResponseWriter, _ *http.Request) {
	entries := a.Ship.Catalog().List()
	writeJSON(w, map[string]any{
		"my_nest": a.LocalNestURL,
		"nest_id": a.NestID,
		"active":  a.Ship.ActiveNests(),
		"catalog": entries,
	})
}

func (a *API) handleInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name      string   `json:"name"`
		TTL       string   `json:"ttl"`
		Tags      []string `json:"tags"`
		MaxVRAMMb int      `json:"max_vram_mb"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Name == "" {
		req.Name = "ship"
	}
	ttl := 24 * time.Hour
	if req.TTL != "" {
		if d, err := time.ParseDuration(req.TTL); err == nil {
			ttl = d
		}
	}
	nests := a.inviteNests()
	tok, err := fleetcrypto.IssueInviteFull(a.JoinSecret, fleetcrypto.Invite{
		Name: req.Name, Nests: nests, ExpiresAt: time.Now().Add(ttl).Unix(),
		ModeHint: "worker", Tags: req.Tags, MaxVRAMMb: req.MaxVRAMMb,
	})
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	joinLink := formatJoinLink(a.JoinSecret, tok, nests)
	writeJSON(w, map[string]any{
		"token":     tok,
		"join_link": joinLink,
		"nests":     nests,
		"tags":      req.Tags,
		"instructions": map[string]string{
			"tui":    "On the other PC: pirate → Join fleet → paste join_link → Run",
			"enroll": "pirate enroll --join-secret <SECRET> --invite <token>",
			"run":    "pirate run",
			"note":   "Same app for every ship. Host Nest + UI is optional.",
		},
	})
}

func (a *API) handleAddNest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, "bad request", 400)
		return
	}
	a.Ship.AddNestURL(req.URL)
	a.Nests = append(a.Nests, req.URL)
	writeJSON(w, map[string]any{
		"ok": true, "nests": a.Ship.Catalog().URLs(),
		"note": "Nest advertised to fleet via gossip; no restart required",
	})
}

func (a *API) handleEnsure(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID    string `json:"node_id"`
		ModelID   string `json:"model_id"`
		OllamaTag string `json:"ollama_tag"`
		URL       string `json:"url"`
		Format    string `json:"format"`
		MinVRAMMb int    `json:"min_vram_mb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" || req.ModelID == "" {
		http.Error(w, "bad request", 400)
		return
	}
	if req.MinVRAMMb > 0 {
		for _, p := range a.Ship.Roster().List() {
			if p.NodeID != req.NodeID {
				continue
			}
			cap := p.MaxVRAMMb
			if cap <= 0 {
				cap = p.VRAMMb
			}
			if cap > 0 && cap < req.MinVRAMMb {
				http.Error(w, `{"error":"ship max_vram below model min_vram"}`, http.StatusConflict)
				return
			}
		}
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
		Model      string   `json:"model"`
		PirateTags []string `json:"pirate_tags"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		http.Error(w, `{"error":"model required"}`, 400)
		return
	}
	tags := req.PirateTags
	if hdr := r.Header.Get("X-Pirate-Tags"); hdr != "" {
		for _, t := range strings.Split(hdr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tags = append(tags, t)
			}
		}
	}
	peer, ok := a.Ship.Roster().PickLeastLoad(req.Model, roster.PickOpts{RequireTags: tags})
	if !ok {
		http.Error(w, `{"error":"no ready ship for model"}`, http.StatusServiceUnavailable)
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

func formatJoinLink(secret, invite string, nests []string) string {
	primary := ""
	if len(nests) > 0 {
		primary = nests[0]
	}
	q := url.Values{}
	q.Set("secret", secret)
	if primary != "" {
		q.Set("nest", primary)
	}
	for _, n := range nests {
		if n != "" && n != primary {
			q.Add("nests", n)
		}
	}
	if invite != "" {
		q.Set("invite", invite)
	}
	return "pirate://join?" + q.Encode()
}
