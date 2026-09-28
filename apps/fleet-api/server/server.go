package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/internal/auth"
	"github.com/Chex4ever/tesla-llm-ifp/internal/bus"
	"github.com/Chex4ever/tesla-llm-ifp/internal/headscale"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
)

var nodesOnline = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "tesla_fleet_nodes_online",
	Help: "Number of online fleet nodes",
})

type Config struct {
	JWTSecret            string
	AdminUser            string
	AdminPassword        string
	PublicFleetURL       string
	PublicAPIURL         string
	PublicHSURL          string
	PublicNATSURL        string
	NATSToken            string
	MinioEndpoint        string
	MinioAccessKey       string
	MinioSecretKey       string
	MinioBucketModels    string
	MinioBucketAgents    string
	MinioUseSSL          bool
	OpenAIBootstrapKey   string
	InternalSharedSecret string
}

type Deps struct {
	DB        *pgxpool.Pool
	Redis     *redis.Client
	NATS      *nats.Conn
	JS        nats.JetStreamContext
	Headscale *headscale.Client
	Cfg       Config
}

type Server struct {
	deps  Deps
	minio *minio.Client
}

func New(d Deps) (*Server, error) {
	mc, err := minio.New(d.Cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(d.Cfg.MinioAccessKey, d.Cfg.MinioSecretKey, ""),
		Secure: d.Cfg.MinioUseSSL,
	})
	if err != nil {
		log.Printf("minio client warning: %v (presigns may fail until MinIO is up)", err)
	}
	s := &Server{deps: d, minio: mc}
	go s.metricsLoop()
	return s, nil
}

func (s *Server) Bootstrap(ctx context.Context) error {
	var n int
	if err := s.deps.DB.QueryRow(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		hash, err := auth.HashPassword(s.deps.Cfg.AdminPassword)
		if err != nil {
			return err
		}
		id := uuid.New()
		_, err = s.deps.DB.Exec(ctx, `INSERT INTO admins (id, username, password_hash) VALUES ($1,$2,$3)`,
			id, s.deps.Cfg.AdminUser, hash)
		if err != nil {
			return err
		}
		log.Printf("bootstrap admin user %q created", s.deps.Cfg.AdminUser)
	}
	if s.deps.Cfg.OpenAIBootstrapKey != "" {
		var exists int
		h := auth.HashToken(s.deps.Cfg.OpenAIBootstrapKey)
		_ = s.deps.DB.QueryRow(ctx, `SELECT COUNT(*) FROM api_keys WHERE key_hash=$1`, h).Scan(&exists)
		if exists == 0 {
			id := uuid.New()
			prefix := s.deps.Cfg.OpenAIBootstrapKey
			if len(prefix) > 16 {
				prefix = prefix[:16]
			}
			_, _ = s.deps.DB.Exec(ctx, `
				INSERT INTO api_keys (id, name, key_prefix, key_hash) VALUES ($1,$2,$3,$4)`,
				id, "open-webui", prefix, h)
			log.Printf("bootstrap openai api key registered")
		}
	}
	return nil
}

func (s *Server) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("GET /api/v1/me", s.admin(s.handleMe))

	mux.HandleFunc("GET /api/v1/nodes", s.admin(s.handleListNodes))
	mux.HandleFunc("POST /api/v1/nodes/invites", s.admin(s.handleCreateInvite))
	mux.HandleFunc("GET /api/v1/nodes/invites/{id}", s.admin(s.handleGetInvite))
	mux.HandleFunc("POST /api/v1/nodes/{id}/drain", s.admin(s.handleDrainNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/undrain", s.admin(s.handleUndrainNode))
	mux.HandleFunc("DELETE /api/v1/nodes/{id}", s.admin(s.handleDeleteNode))
	mux.HandleFunc("POST /api/v1/nodes/{id}/models/{modelId}", s.admin(s.handleAssignModel))

	mux.HandleFunc("POST /api/v1/nodes/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/nodes/heartbeat", s.handleHeartbeat)

	mux.HandleFunc("GET /api/v1/models", s.admin(s.handleListModels))
	mux.HandleFunc("POST /api/v1/models", s.admin(s.handleCreateModel))
	mux.HandleFunc("POST /api/v1/models/upload-url", s.admin(s.handleModelUploadURL))

	mux.HandleFunc("GET /api/v1/api-keys", s.admin(s.handleListAPIKeys))
	mux.HandleFunc("POST /api/v1/api-keys", s.admin(s.handleCreateAPIKey))
	mux.HandleFunc("DELETE /api/v1/api-keys/{id}", s.admin(s.handleRevokeAPIKey))

	mux.HandleFunc("GET /api/v1/agent/windows/latest", s.handleAgentWindowsLatest)
	mux.HandleFunc("GET /api/v1/agent/linux/install.sh", s.handleLinuxInstallScript)
	mux.HandleFunc("GET /api/v1/agent/linux/latest", s.handleAgentLinuxLatest)

	mux.HandleFunc("GET /api/v1/internal/nodes/ready", s.internal(s.handleReadyNodes))
	mux.HandleFunc("POST /api/v1/internal/validate-key", s.internal(s.handleValidateKey))

	mux.HandleFunc("GET /api/v1/stats", s.admin(s.handleStats))
}

func (s *Server) metricsLoop() {
	t := time.NewTicker(15 * time.Second)
	defer t.Stop()
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		var n int
		_ = s.deps.DB.QueryRow(ctx, `
			SELECT COUNT(*) FROM nodes
			WHERE status='online' AND last_heartbeat > now() - interval '90 seconds'`).Scan(&n)
		nodesOnline.Set(float64(n))
		cancel()
	}
}

type ctxKey string

const claimsKey ctxKey = "claims"

func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		claims, err := auth.ParseAdminJWT(s.deps.Cfg.JWTSecret, strings.TrimPrefix(h, "Bearer "))
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), claimsKey, claims)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) internal(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Secret") != s.deps.Cfg.InternalSharedSecret {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var id uuid.UUID
	var hash string
	err := s.deps.DB.QueryRow(r.Context(), `SELECT id, password_hash FROM admins WHERE username=$1`, req.Username).Scan(&id, &hash)
	if err != nil || !auth.CheckPassword(hash, req.Password) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	token, err := auth.IssueAdminJWT(s.deps.Cfg.JWTSecret, id, req.Username, 24*time.Hour)
	if err != nil {
		http.Error(w, "token error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "username": req.Username})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	c := r.Context().Value(claimsKey).(*auth.Claims)
	writeJSON(w, http.StatusOK, map[string]any{"id": c.AdminID, "username": c.Username})
}

func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT id, name, tags, status, agent_version, os, arch, hostname, gpu_name, vram_mb, ram_mb,
		       disk_free_gb, overlay_ip, runtime, runtime_healthy, drained, models_ready, last_heartbeat, created_at
		FROM nodes ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var (
			id, name, status, runtime                        string
			agentVersion, osName, arch, hostname, gpu, overlay *string
			tags, models                                     []string
			vram, ram, disk                                  *int
			runtimeHealthy, drained                          bool
			created                                          time.Time
			lastHBp                                          *time.Time
		)
		var nid uuid.UUID
		if err := rows.Scan(&nid, &name, &tags, &status, &agentVersion, &osName, &arch, &hostname, &gpu, &vram, &ram,
			&disk, &overlay, &runtime, &runtimeHealthy, &drained, &models, &lastHBp, &created); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id = nid.String()
		item := map[string]any{
			"id": id, "name": name, "tags": tags, "status": status, "runtime": runtime,
			"runtime_healthy": runtimeHealthy, "drained": drained, "models_ready": models, "created_at": created,
		}
		if agentVersion != nil {
			item["agent_version"] = *agentVersion
		}
		if osName != nil {
			item["os"] = *osName
		}
		if arch != nil {
			item["arch"] = *arch
		}
		if hostname != nil {
			item["hostname"] = *hostname
		}
		if gpu != nil {
			item["gpu_name"] = *gpu
		}
		if vram != nil {
			item["vram_mb"] = *vram
		}
		if ram != nil {
			item["ram_mb"] = *ram
		}
		if disk != nil {
			item["disk_free_gb"] = *disk
		}
		if overlay != nil {
			item["overlay_ip"] = *overlay
		}
		if lastHBp != nil {
			item["last_heartbeat"] = *lastHBp
			if time.Since(*lastHBp) > 90*time.Second && status == "online" {
				item["status"] = "stale"
			}
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": list})
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name       string   `json:"name"`
		Tags       []string `json:"tags"`
		TTLMinutes int      `json:"ttl_minutes"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.TTLMinutes <= 0 {
		req.TTLMinutes = 1440
	}
	if req.Tags == nil {
		req.Tags = []string{}
	}
	token, err := auth.RandomToken(24)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	ttl := time.Duration(req.TTLMinutes) * time.Minute
	preauth, err := s.deps.Headscale.CreatePreAuthKey(r.Context(), "workers", ttl)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	nodeID := uuid.New()
	inviteID := uuid.New()
	expires := time.Now().Add(ttl)
	tx, err := s.deps.DB.Begin(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), `
		INSERT INTO nodes (id, name, tags, status) VALUES ($1,$2,$3,'pending')`,
		nodeID, req.Name, req.Tags)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = tx.Exec(r.Context(), `
		INSERT INTO invites (id, token_hash, node_id, name, tags, ttl_minutes, headscale_preauth_key, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		inviteID, auth.HashToken(token), nodeID, req.Name, req.Tags, req.TTLMinutes, preauth, expires)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invite_id":        inviteID,
		"node_id":          nodeID,
		"token":            token,
		"expires_at":       expires,
		"headscale_authkey": preauth,
		"download_windows": s.deps.Cfg.PublicFleetURL + "/api/v1/agent/windows/latest",
		"download_linux":   s.deps.Cfg.PublicFleetURL + "/api/v1/agent/linux/latest",
		"instructions": map[string]string{
			"windows": fmt.Sprintf("Download the installer, run it, paste token:\n%s\n\nOr PowerShell:\n$env:FLEET_URL='%s'; $env:INVITE_TOKEN='%s'; .\\tesla-agent.exe enroll",
				token, s.deps.Cfg.PublicFleetURL, token),
			"linux": fmt.Sprintf("curl -fsSL %s/api/v1/agent/linux/install.sh | sudo bash -s -- --token %s",
				s.deps.Cfg.PublicFleetURL, token),
		},
		"headscale_login_server": s.deps.Cfg.PublicHSURL,
	})
}

func (s *Server) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var name string
	var expires time.Time
	var used, revoked *time.Time
	var nodeID uuid.UUID
	err := s.deps.DB.QueryRow(r.Context(), `
		SELECT name, node_id, expires_at, used_at, revoked_at FROM invites WHERE id=$1`, id).
		Scan(&name, &nodeID, &expires, &used, &revoked)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id": id, "name": name, "node_id": nodeID, "expires_at": expires,
		"used_at": used, "revoked_at": revoked,
		"download_windows": s.deps.Cfg.PublicFleetURL + "/api/v1/agent/windows/latest",
	})
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token        string `json:"token"`
		Hostname     string `json:"hostname"`
		OS           string `json:"os"`
		Arch         string `json:"arch"`
		AgentVersion string `json:"agent_version"`
	}
	if err := readJSON(r, &req); err != nil || req.Token == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var inviteID, nodeID uuid.UUID
	var preauth string
	var expires time.Time
	var used, revoked *time.Time
	err := s.deps.DB.QueryRow(r.Context(), `
		SELECT id, node_id, headscale_preauth_key, expires_at, used_at, revoked_at
		FROM invites WHERE token_hash=$1`, auth.HashToken(req.Token)).
		Scan(&inviteID, &nodeID, &preauth, &expires, &used, &revoked)
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if revoked != nil || time.Now().After(expires) {
		http.Error(w, "invite expired or revoked", http.StatusUnauthorized)
		return
	}
	agentToken, err := auth.RandomToken(32)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, err = s.deps.DB.Exec(r.Context(), `
		UPDATE nodes SET status='enrolled', hostname=$2, os=$3, arch=$4, agent_version=$5, updated_at=now()
		WHERE id=$1`, nodeID, req.Hostname, req.OS, req.Arch, req.AgentVersion)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if used == nil {
		_, _ = s.deps.DB.Exec(r.Context(), `UPDATE invites SET used_at=now() WHERE id=$1`, inviteID)
	}
	_ = s.deps.Redis.Set(r.Context(), "agent_token:"+auth.HashToken(agentToken), nodeID.String(), 0).Err()
	writeJSON(w, http.StatusOK, map[string]any{
		"node_id":                nodeID,
		"agent_token":            agentToken,
		"headscale_login_server": s.deps.Cfg.PublicHSURL,
		"headscale_authkey":      preauth,
		"nats_url":               s.deps.Cfg.PublicNATSURL,
		"nats_token":             s.deps.Cfg.NATSToken,
		"fleet_url":              s.deps.Cfg.PublicFleetURL,
		"models_endpoint":        strings.Replace(s.deps.Cfg.PublicFleetURL, "fleet.", "models.", 1),
	})
}

func (s *Server) nodeFromAgent(r *http.Request) (uuid.UUID, error) {
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		return uuid.Nil, fmt.Errorf("missing token")
	}
	tok := strings.TrimPrefix(h, "Bearer ")
	val, err := s.deps.Redis.Get(r.Context(), "agent_token:"+auth.HashToken(tok)).Result()
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid agent token")
	}
	return uuid.Parse(val)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	nodeID, err := s.nodeFromAgent(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	var req struct {
		AgentVersion   string   `json:"agent_version"`
		GPUName        string   `json:"gpu_name"`
		VRAMMb         int      `json:"vram_mb"`
		RAMMb          int      `json:"ram_mb"`
		DiskFreeGB     int      `json:"disk_free_gb"`
		OverlayIP      string   `json:"overlay_ip"`
		Runtime        string   `json:"runtime"`
		RuntimeHealthy bool     `json:"runtime_healthy"`
		ModelsReady    []string `json:"models_ready"`
		ModelProgress  []struct {
			ModelID  string  `json:"model_id"`
			State    string  `json:"state"`
			Progress float64 `json:"progress"`
			Error    string  `json:"error"`
		} `json:"model_progress"`
	}
	if err := readJSON(r, &req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.ModelsReady == nil {
		req.ModelsReady = []string{}
	}
	if req.Runtime == "" {
		req.Runtime = "ollama"
	}
	_, err = s.deps.DB.Exec(r.Context(), `
		UPDATE nodes SET status='online', agent_version=$2, gpu_name=$3, vram_mb=$4, ram_mb=$5, disk_free_gb=$6,
		overlay_ip=$7, runtime=$8, runtime_healthy=$9, models_ready=$10, last_heartbeat=now(), updated_at=now()
		WHERE id=$1`,
		nodeID, req.AgentVersion, nullStr(req.GPUName), nullInt(req.VRAMMb), nullInt(req.RAMMb), nullInt(req.DiskFreeGB),
		nullStr(req.OverlayIP), req.Runtime, req.RuntimeHealthy, req.ModelsReady)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, p := range req.ModelProgress {
		_, _ = s.deps.DB.Exec(r.Context(), `
			UPDATE node_models SET state=$3, progress=$4, error=$5, updated_at=now()
			WHERE node_id=$1 AND model_id=(SELECT id FROM models WHERE model_id=$2)`,
			nodeID, p.ModelID, p.State, p.Progress, nullStr(p.Error))
	}
	// Desired models for agent
	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT m.model_id, m.format, m.ollama_tag, m.minio_object, m.checksum_sha256, m.min_vram_mb
		FROM node_models nm JOIN models m ON m.id = nm.model_id
		WHERE nm.node_id=$1 AND nm.desired=true`, nodeID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	desired := []map[string]any{}
	for rows.Next() {
		var mid, format string
		var tag, obj, sum *string
		var minVRAM int
		if err := rows.Scan(&mid, &format, &tag, &obj, &sum, &minVRAM); err != nil {
			continue
		}
		item := map[string]any{"model_id": mid, "format": format, "min_vram_mb": minVRAM}
		if tag != nil {
			item["ollama_tag"] = *tag
		}
		if obj != nil {
			item["minio_object"] = *obj
			if s.minio != nil {
				u, err := s.minio.PresignedGetObject(r.Context(), s.deps.Cfg.MinioBucketModels, *obj, time.Hour, nil)
				if err == nil {
					item["download_url"] = u.String()
				}
			}
		}
		if sum != nil {
			item["checksum_sha256"] = *sum
		}
		desired = append(desired, item)
	}
	payload, _ := json.Marshal(map[string]any{"node_id": nodeID.String(), "ts": time.Now()})
	_, _ = s.deps.JS.Publish(bus.SubjectHeartbeat, payload)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "desired_models": desired})
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func (s *Server) handleDrainNode(w http.ResponseWriter, r *http.Request) {
	_, _ = s.deps.DB.Exec(r.Context(), `UPDATE nodes SET drained=true, updated_at=now() WHERE id=$1`, r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUndrainNode(w http.ResponseWriter, r *http.Request) {
	_, _ = s.deps.DB.Exec(r.Context(), `UPDATE nodes SET drained=false, updated_at=now() WHERE id=$1`, r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDeleteNode(w http.ResponseWriter, r *http.Request) {
	_, _ = s.deps.DB.Exec(r.Context(), `DELETE FROM nodes WHERE id=$1`, r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleAssignModel(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	modelID := r.PathValue("modelId")
	var mid uuid.UUID
	err := s.deps.DB.QueryRow(r.Context(), `SELECT id FROM models WHERE id=$1 OR model_id=$1`, modelID).Scan(&mid)
	if err != nil {
		http.Error(w, "model not found", http.StatusNotFound)
		return
	}
	_, err = s.deps.DB.Exec(r.Context(), `
		INSERT INTO node_models (node_id, model_id, desired, state)
		VALUES ($1,$2,true,'pending')
		ON CONFLICT (node_id, model_id) DO UPDATE SET desired=true, state='pending', updated_at=now()`,
		nodeID, mid)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleListModels(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT id, model_id, display_name, format, ollama_tag, minio_object, checksum_sha256, size_bytes, min_vram_mb, created_at
		FROM models ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var mid, display, format string
		var tag, obj, sum *string
		var size int64
		var minVRAM int
		var created time.Time
		if err := rows.Scan(&id, &mid, &display, &format, &tag, &obj, &sum, &size, &minVRAM, &created); err != nil {
			continue
		}
		item := map[string]any{
			"id": id, "model_id": mid, "display_name": display, "format": format,
			"size_bytes": size, "min_vram_mb": minVRAM, "created_at": created,
		}
		if tag != nil {
			item["ollama_tag"] = *tag
		}
		if obj != nil {
			item["minio_object"] = *obj
		}
		if sum != nil {
			item["checksum_sha256"] = *sum
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": list})
}

func (s *Server) handleCreateModel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ModelID        string `json:"model_id"`
		DisplayName    string `json:"display_name"`
		Format         string `json:"format"`
		OllamaTag      string `json:"ollama_tag"`
		MinioObject    string `json:"minio_object"`
		ChecksumSHA256 string `json:"checksum_sha256"`
		SizeBytes      int64  `json:"size_bytes"`
		MinVRAMMb      int    `json:"min_vram_mb"`
	}
	if err := readJSON(r, &req); err != nil || req.ModelID == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.ModelID
	}
	if req.Format == "" {
		req.Format = "ollama"
	}
	id := uuid.New()
	_, err := s.deps.DB.Exec(r.Context(), `
		INSERT INTO models (id, model_id, display_name, format, ollama_tag, minio_object, checksum_sha256, size_bytes, min_vram_mb)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, req.ModelID, req.DisplayName, req.Format, nullStr(req.OllamaTag), nullStr(req.MinioObject),
		nullStr(req.ChecksumSHA256), req.SizeBytes, req.MinVRAMMb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "model_id": req.ModelID})
}

func (s *Server) handleModelUploadURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ObjectName string `json:"object_name"`
	}
	if err := readJSON(r, &req); err != nil || req.ObjectName == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.minio == nil {
		http.Error(w, "minio unavailable", http.StatusServiceUnavailable)
		return
	}
	u, err := s.minio.PresignedPutObject(r.Context(), s.deps.Cfg.MinioBucketModels, req.ObjectName, time.Hour)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"upload_url": u.String(), "bucket": s.deps.Cfg.MinioBucketModels, "object": req.ObjectName})
}

func (s *Server) handleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT id, name, key_prefix, created_at, revoked_at, last_used_at FROM api_keys ORDER BY created_at DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name, prefix string
		var created time.Time
		var revoked, last *time.Time
		if err := rows.Scan(&id, &name, &prefix, &created, &revoked, &last); err != nil {
			continue
		}
		list = append(list, map[string]any{
			"id": id, "name": name, "key_prefix": prefix, "created_at": created,
			"revoked_at": revoked, "last_used_at": last,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": list})
}

func (s *Server) handleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil || req.Name == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	raw, prefix, hash, err := auth.APIKey()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id := uuid.New()
	_, err = s.deps.DB.Exec(r.Context(), `
		INSERT INTO api_keys (id, name, key_prefix, key_hash) VALUES ($1,$2,$3,$4)`, id, req.Name, prefix, hash)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "name": req.Name, "key": raw, "key_prefix": prefix})
}

func (s *Server) handleRevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	_, _ = s.deps.DB.Exec(r.Context(), `UPDATE api_keys SET revoked_at=now() WHERE id=$1`, r.PathValue("id"))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleValidateKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Key string `json:"key"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &req)
	if req.Key == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var id uuid.UUID
	var name string
	err := s.deps.DB.QueryRow(r.Context(), `
		SELECT id, name FROM api_keys WHERE key_hash=$1 AND revoked_at IS NULL`, auth.HashToken(req.Key)).
		Scan(&id, &name)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"valid": false})
		return
	}
	_, _ = s.deps.DB.Exec(r.Context(), `UPDATE api_keys SET last_used_at=now() WHERE id=$1`, id)
	writeJSON(w, http.StatusOK, map[string]any{"valid": true, "id": id, "name": name})
}

func (s *Server) handleReadyNodes(w http.ResponseWriter, r *http.Request) {
	model := r.URL.Query().Get("model")
	rows, err := s.deps.DB.Query(r.Context(), `
		SELECT id, name, overlay_ip, models_ready, vram_mb, drained
		FROM nodes
		WHERE status='online' AND runtime_healthy=true AND drained=false
		  AND last_heartbeat > now() - interval '90 seconds'
		ORDER BY last_heartbeat DESC`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	list := []map[string]any{}
	for rows.Next() {
		var id uuid.UUID
		var name string
		var overlay *string
		var models []string
		var vram *int
		var drained bool
		if err := rows.Scan(&id, &name, &overlay, &models, &vram, &drained); err != nil {
			continue
		}
		if model != "" {
			ok := false
			for _, m := range models {
				if m == model {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		item := map[string]any{"id": id, "name": name, "models_ready": models, "drained": drained}
		if overlay != nil {
			item["overlay_ip"] = *overlay
		}
		if vram != nil {
			item["vram_mb"] = *vram
		}
		list = append(list, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": list})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	var total, online, pending int
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM nodes`).Scan(&total)
	_ = s.deps.DB.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM nodes WHERE status='online' AND last_heartbeat > now() - interval '90 seconds'`).Scan(&online)
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM nodes WHERE status='pending'`).Scan(&pending)
	var models int
	_ = s.deps.DB.QueryRow(r.Context(), `SELECT COUNT(*) FROM models`).Scan(&models)
	writeJSON(w, http.StatusOK, map[string]any{
		"nodes_total": total, "nodes_online": online, "nodes_pending": pending, "models": models,
	})
}

func (s *Server) handleAgentWindowsLatest(w http.ResponseWriter, r *http.Request) {
	s.serveAgentArtifact(w, r, "windows/tesla-agent.exe", "tesla-agent.exe")
}

func (s *Server) handleAgentLinuxLatest(w http.ResponseWriter, r *http.Request) {
	s.serveAgentArtifact(w, r, "linux/tesla-agent", "tesla-agent")
}

func (s *Server) serveAgentArtifact(w http.ResponseWriter, r *http.Request, object, filename string) {
	if s.minio != nil {
		u, err := s.minio.PresignedGetObject(r.Context(), s.deps.Cfg.MinioBucketAgents, object, time.Hour, nil)
		if err == nil {
			http.Redirect(w, r, u.String(), http.StatusFound)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"message":  "Agent artifact not uploaded yet. Build with: go build -o tesla-agent ./agent/cmd/tesla-agent and upload to MinIO bucket agents/" + object,
		"filename": filename,
		"object":   object,
		"bucket":   s.deps.Cfg.MinioBucketAgents,
	})
}

func (s *Server) handleLinuxInstallScript(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	script := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
FLEET_URL="${FLEET_URL:-%s}"
TOKEN="${1:-}"
if [[ "${1:-}" == "--token" ]]; then TOKEN="${2:-}"; fi
TOKEN="${TOKEN:-%s}"
if [[ -z "$TOKEN" ]]; then echo "usage: install.sh --token <invite>"; exit 1; fi
TMP="$(mktemp -d)"
curl -fsSL "$FLEET_URL/api/v1/agent/linux/latest" -o "$TMP/tesla-agent" || true
if [[ ! -s "$TMP/tesla-agent" ]] || file "$TMP/tesla-agent" | grep -qi json; then
  echo "Agent binary not available in registry yet. Build and upload linux/tesla-agent to MinIO agents bucket."
  exit 1
fi
install -m 0755 "$TMP/tesla-agent" /usr/local/bin/tesla-agent
mkdir -p /etc/tesla-agent
cat >/etc/tesla-agent/agent.env <<EOF
FLEET_URL=$FLEET_URL
INVITE_TOKEN=$TOKEN
EOF
/usr/local/bin/tesla-agent enroll --config /etc/tesla-agent/agent.env
cat >/etc/systemd/system/tesla-agent.service <<'UNIT'
[Unit]
Description=Tesla LLM Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=/etc/tesla-agent/agent.env
ExecStart=/usr/local/bin/tesla-agent run
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now tesla-agent
echo "Tesla agent installed and started."
`, s.deps.Cfg.PublicFleetURL, token)
	w.Header().Set("Content-Type", "text/x-shellscript")
	_, _ = w.Write([]byte(script))
}

