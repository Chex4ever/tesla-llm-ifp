package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/internal/bus"
	"github.com/Chex4ever/tesla-llm-ifp/internal/config"
	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	reqTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tesla_gateway_requests_total",
		Help: "Total gateway requests",
	}, []string{"path", "code"})
	reqDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tesla_gateway_request_duration_seconds",
		Help:    "Gateway request duration",
		Buckets: prometheus.DefBuckets,
	}, []string{"path"})
)

type gateway struct {
	fleetURL       string
	internalSecret string
	httpClient     *http.Client
	nc             *nats.Conn
}

type inferRequest struct {
	RequestID string          `json:"request_id"`
	NodeID    string          `json:"node_id"`
	Path      string          `json:"path"`
	Body      json.RawMessage `json:"body"`
	Stream    bool            `json:"stream"`
}

type inferResponse struct {
	RequestID  string `json:"request_id"`
	StatusCode int    `json:"status_code"`
	Body       []byte `json:"body"`
	Error      string `json:"error,omitempty"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	nc, _, err := bus.Connect(config.Get("NATS_URL", "nats://localhost:4222"), config.Get("NATS_TOKEN", ""))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	g := &gateway{
		fleetURL:       strings.TrimRight(config.Get("FLEET_API_URL", "http://fleet-api:8081"), "/"),
		internalSecret: config.Get("INTERNAL_SHARED_SECRET", "dev-secret"),
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		nc:             nc,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("GET /v1/models", g.withAuth(g.handleModels))
	mux.HandleFunc("POST /v1/chat/completions", g.withAuth(g.handleChatCompletions))
	mux.HandleFunc("POST /v1/completions", g.withAuth(g.handleChatCompletions))
	mux.HandleFunc("POST /v1/embeddings", g.withAuth(g.handleEmbeddings))

	addr := config.Get("HTTP_ADDR", ":8080")
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("gateway listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()
	<-ctx.Done()
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = srv.Shutdown(shutdownCtx)
	os.Exit(0)
}

func (g *gateway) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		key := bearer(r)
		if key == "" {
			reqTotal.WithLabelValues(r.URL.Path, "401").Inc()
			http.Error(w, `{"error":"missing api key"}`, http.StatusUnauthorized)
			return
		}
		ok, err := g.validateKey(r.Context(), key)
		if err != nil || !ok {
			reqTotal.WithLabelValues(r.URL.Path, "401").Inc()
			http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
			return
		}
		ww := &statusWriter{ResponseWriter: w, code: 200}
		next(ww, r)
		reqTotal.WithLabelValues(r.URL.Path, strconv.Itoa(ww.code)).Inc()
		reqDuration.WithLabelValues(r.URL.Path).Observe(time.Since(start).Seconds())
	}
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (w *statusWriter) WriteHeader(code int) {
	w.code = code
	w.ResponseWriter.WriteHeader(code)
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return r.Header.Get("X-API-Key")
}

func (g *gateway) validateKey(ctx context.Context, key string) (bool, error) {
	body, _ := json.Marshal(map[string]string{"key": key})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.fleetURL+"/api/v1/internal/validate-key", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Secret", g.internalSecret)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	var out struct {
		Valid bool `json:"valid"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return out.Valid, nil
}

func (g *gateway) handleModels(w http.ResponseWriter, r *http.Request) {
	nodes, err := g.readyNodes(r.Context(), "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	seen := map[string]bool{}
	data := []map[string]any{}
	for _, n := range nodes {
		for _, m := range n.ModelsReady {
			if seen[m] {
				continue
			}
			seen[m] = true
			data = append(data, map[string]any{"id": m, "object": "model", "owned_by": "tesla-llm"})
		}
	}
	writeJSON(w, map[string]any{"object": "list", "data": data})
}

type readyNode struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	OverlayIP   string   `json:"overlay_ip"`
	ModelsReady []string `json:"models_ready"`
}

func (g *gateway) readyNodes(ctx context.Context, model string) ([]readyNode, error) {
	url := g.fleetURL + "/api/v1/internal/nodes/ready"
	if model != "" {
		url += "?model=" + model
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Internal-Secret", g.internalSecret)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Nodes []readyNode `json:"nodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Nodes, nil
}

func (g *gateway) dispatch(ctx context.Context, node readyNode, path string, body []byte, stream bool) (*inferResponse, error) {
	reqID := uuid.NewString()
	payload, _ := json.Marshal(inferRequest{
		RequestID: reqID,
		NodeID:    node.ID,
		Path:      path,
		Body:      body,
		Stream:    stream,
	})
	msg, err := g.nc.RequestWithContext(ctx, bus.SubjectInferReq+"."+node.ID, payload)
	if err != nil {
		return nil, err
	}
	var resp inferResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

func (g *gateway) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.Model == "" {
		http.Error(w, `{"error":"model required"}`, http.StatusBadRequest)
		return
	}
	nodes, err := g.readyNodes(r.Context(), req.Model)
	if err != nil || len(nodes) == 0 {
		http.Error(w, `{"error":"no ready workers for model"}`, http.StatusServiceUnavailable)
		return
	}
	node := nodes[0]
	path := "/v1/chat/completions"
	if strings.HasSuffix(r.URL.Path, "/completions") && !strings.Contains(r.URL.Path, "chat") {
		path = "/v1/completions"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	resp, err := g.dispatch(ctx, node, path, body, req.Stream)
	if err != nil {
		http.Error(w, `{"error":"worker dispatch failed: `+err.Error()+`"}`, http.StatusBadGateway)
		return
	}
	if resp.Error != "" {
		http.Error(w, `{"error":"`+resp.Error+`"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tesla-Node", node.Name)
	if resp.StatusCode == 0 {
		resp.StatusCode = 200
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func (g *gateway) handleEmbeddings(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var req struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &req)
	nodes, err := g.readyNodes(r.Context(), req.Model)
	if err != nil || len(nodes) == 0 {
		http.Error(w, `{"error":"no ready workers"}`, http.StatusServiceUnavailable)
		return
	}
	node := nodes[0]
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	resp, err := g.dispatch(ctx, node, "/v1/embeddings", body, false)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Tesla-Node", node.Name)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(resp.Body)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
