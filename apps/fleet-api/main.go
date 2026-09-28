package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/apps/fleet-api/server"
	"github.com/Chex4ever/tesla-llm-ifp/internal/bus"
	"github.com/Chex4ever/tesla-llm-ifp/internal/config"
	"github.com/Chex4ever/tesla-llm-ifp/internal/db"
	"github.com/Chex4ever/tesla-llm-ifp/internal/headscale"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.Connect(ctx, config.Get("DATABASE_URL", "postgres://tesla:tesla@localhost:5432/tesla?sslmode=disable"))
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Get("REDIS_ADDR", "localhost:6379"),
		Password: config.Get("REDIS_PASSWORD", ""),
	})
	defer rdb.Close()

	nc, js, err := bus.Connect(config.Get("NATS_URL", "nats://localhost:4222"), config.Get("NATS_TOKEN", ""))
	if err != nil {
		log.Fatalf("nats: %v", err)
	}
	defer nc.Close()

	hs := headscale.New(config.Get("HEADSCALE_URL", "http://localhost:8080"), config.Get("HEADSCALE_API_KEY", ""))
	_ = hs.EnsureUser(ctx, "workers")

	srv, err := server.New(server.Deps{
		DB:       pool,
		Redis:    rdb,
		NATS:     nc,
		JS:       js,
		Headscale: hs,
		Cfg: server.Config{
			JWTSecret:            config.Get("INTERNAL_SHARED_SECRET", "dev-secret"),
			AdminUser:            config.Get("ADMIN_USER", "admin"),
			AdminPassword:        config.Get("ADMIN_PASSWORD", "admin"),
			PublicFleetURL:       config.Get("PUBLIC_FLEET_URL", "http://localhost:8081"),
			PublicAPIURL:         config.Get("PUBLIC_API_URL", "http://localhost:8080"),
			PublicHSURL:          config.Get("PUBLIC_HS_URL", "http://localhost:8080"),
			PublicNATSURL:        config.Get("PUBLIC_NATS_URL", "nats://localhost:4222"),
			NATSToken:            config.Get("NATS_TOKEN", ""),
			MinioEndpoint:        config.Get("MINIO_ENDPOINT", "localhost:9000"),
			MinioAccessKey:       config.Get("MINIO_ACCESS_KEY", "minioadmin"),
			MinioSecretKey:       config.Get("MINIO_SECRET_KEY", "minioadmin"),
			MinioBucketModels:    config.Get("MINIO_BUCKET_MODELS", "models"),
			MinioBucketAgents:    config.Get("MINIO_BUCKET_AGENTS", "agents"),
			MinioUseSSL:          config.Get("MINIO_USE_SSL", "false") == "true",
			OpenAIBootstrapKey:   config.Get("OPENAI_BOOTSTRAP_KEY", ""),
			InternalSharedSecret: config.Get("INTERNAL_SHARED_SECRET", "dev-secret"),
		},
	})
	if err != nil {
		log.Fatalf("server: %v", err)
	}
	if err := srv.Bootstrap(ctx); err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	mux := http.NewServeMux()
	srv.Routes(mux)
	mux.Handle("/metrics", promhttp.Handler())

	addr := config.Get("HTTP_ADDR", ":8081")
	httpSrv := &http.Server{Addr: addr, Handler: withCORS(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("fleet-api listening on %s", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, c := context.WithTimeout(context.Background(), 10*time.Second)
	defer c()
	_ = httpSrv.Shutdown(shutdownCtx)
	os.Exit(0)
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Internal-Secret")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
