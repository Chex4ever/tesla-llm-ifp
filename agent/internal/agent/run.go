package agent

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/captain"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/nest"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/ship"
)

type RunOptions struct {
	ConfigDir    string
	Mode         string
	ExposeAPI    bool
	ListenAddr   string // nest listen or combined
	UIAddr       string
	APIAddr      string
	AgentVersion string
}

func Run(ctx context.Context, opt RunOptions) error {
	st, err := LoadOrInitState(opt.ConfigDir)
	if err != nil {
		return err
	}
	mode := opt.Mode
	if mode == "" {
		mode = st.Mode
	}
	if mode == "" {
		mode = protocol.ModeWorker
	}
	st.Mode = mode
	if opt.ExposeAPI {
		st.ExposeAPI = true
	}
	_ = SaveState(opt.ConfigDir, st)

	switch mode {
	case protocol.ModeCrowsNest:
		addr := opt.ListenAddr
		if addr == "" {
			addr = ":7843"
		}
		log.Printf("starting Crow's Nest on %s", addr)
		srv := &http.Server{Addr: addr, Handler: nest.New().Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			_ = srv.Shutdown(context.Background())
		}()
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	case protocol.ModeWorker, protocol.ModeCaptain:
		if st.JoinSecret == "" {
			return fmt.Errorf("not enrolled: run enroll with --join-secret first")
		}
		return runShip(ctx, st, opt)
	default:
		return fmt.Errorf("unknown mode %q (worker|captain|crowsnest)", mode)
	}
}

func runShip(ctx context.Context, st *State, opt RunOptions) error {
	apiAddr := st.APIAddr
	if opt.APIAddr != "" {
		apiAddr = opt.APIAddr
	}
	uiAddr := st.UIAddr
	if opt.UIAddr != "" {
		uiAddr = opt.UIAddr
	}
	if opt.ExposeAPI {
		st.ExposeAPI = true
	}
	sh, err := ship.New(ship.Config{
		NodeID:       st.NodeID,
		Name:         st.Name,
		Mode:         st.Mode,
		JoinSecret:   st.JoinSecret,
		Nests:        st.Nests,
		Version:      opt.AgentVersion,
		ExposeAPI:    st.ExposeAPI,
		APIAdvertise: apiAddr,
		OllamaURL:    or(st.OllamaURL, "http://127.0.0.1:11434"),
		VLLMURL:      or(st.VLLMURL, "http://127.0.0.1:8000"),
		Runtime:      or(st.Runtime, "ollama"),
	})
	if err != nil {
		return err
	}

	errCh := make(chan error, 2)
	go func() {
		errCh <- sh.Run(ctx)
	}()

	if st.Mode == protocol.ModeCaptain {
		keys := map[string]struct{}{}
		for _, k := range st.APIKeys {
			keys[k] = struct{}{}
		}
		api := &captain.API{
			Ship:       sh,
			JoinSecret: st.JoinSecret,
			Nests:      st.Nests,
			APIKeys:    keys,
			Name:       st.Name,
		}
		// Captain UI always; OpenAI on same mux when expose-api (or always on captain for local use)
		handler := api.Handler(captain.UIHandler())
		listen := uiAddr
		if st.ExposeAPI || opt.ExposeAPI {
			// Prefer API addr if exposing publicly; UI shares it
			if apiAddr != "" {
				listen = apiAddr
			}
			log.Printf("Captain UI + API on http://%s (UI /  API /v1)", listen)
		} else {
			log.Printf("Captain UI on http://%s", listen)
		}
		srv := &http.Server{Addr: listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			_ = srv.Shutdown(context.Background())
		}()
		go func() {
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-errCh:
		return err
	}
}

func or(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
