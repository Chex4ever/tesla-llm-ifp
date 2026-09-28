package agent

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/captain"
	"github.com/Chex4ever/pirate-fleet/agent/internal/nest"
	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
	"github.com/Chex4ever/pirate-fleet/agent/internal/ship"
	"github.com/google/uuid"
)

type RunOptions struct {
	ConfigDir    string
	Mode         string
	ExposeAPI    bool
	ListenAddr   string
	UIAddr       string
	APIAddr      string
	NestAdvert   string // override host in nest URL
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
		return runStandaloneNest(ctx, st, opt)
	case protocol.ModeWorker, protocol.ModeCaptain:
		if st.JoinSecret == "" {
			return fmt.Errorf("not enrolled: run enroll with --join-secret first")
		}
		return runShip(ctx, st, opt)
	default:
		return fmt.Errorf("unknown mode %q (worker|captain|crowsnest)", mode)
	}
}

func runStandaloneNest(ctx context.Context, st *State, opt RunOptions) error {
	listen, wsURL := NestListenURL(opt.ListenAddr, opt.NestAdvert)
	if st.NestID == "" {
		st.NestID = uuid.NewString()
		_ = SaveState(opt.ConfigDir, st)
	}
	ns := nest.New(st.NestID, wsURL, "public")
	log.Printf("Crow's Nest on %s advert=%s id=%s", listen, wsURL, ns.ID)
	// optional uplinks from state nests
	for _, u := range st.Nests {
		if u != "" && u != wsURL {
			go ns.DialPeerNest(ctx, u)
		}
	}
	srv := &http.Server{Addr: listen, Handler: ns.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
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

	var localNestURL, localNestID string
	var nestServer *nest.Server
	errCh := make(chan error, 4)

	if st.Mode == protocol.ModeCaptain {
		if st.NestID == "" {
			st.NestID = uuid.NewString()
		}
		nestListen := st.NestAddr
		if opt.ListenAddr != "" {
			nestListen = opt.ListenAddr
		}
		var wsURL string
		nestListen, wsURL = NestListenURL(nestListen, firstNonEmpty(opt.NestAdvert, st.NestAdvertHost))
		localNestURL = wsURL
		localNestID = st.NestID
		st.LocalNestURL = wsURL
		// Prefer own nest for local deckhands in invite seeds
		st.Nests = uniqueNests([]string{wsURL}, st.Nests)
		_ = SaveState(opt.ConfigDir, st)

		nestServer = nest.New(st.NestID, wsURL, "captain")
		log.Printf("Captain Nest on %s (%s)", nestListen, wsURL)
		for _, u := range st.Nests {
			if u != wsURL {
				go nestServer.DialPeerNest(ctx, u)
			}
		}
		nestSrv := &http.Server{Addr: nestListen, Handler: nestServer.Handler(), ReadHeaderTimeout: 10 * time.Second}
		go func() {
			<-ctx.Done()
			_ = nestSrv.Shutdown(context.Background())
		}()
		go func() {
			if err := nestSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- err
			}
		}()
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
		LocalNestURL: localNestURL,
		LocalNestID:  localNestID,
		Tags:         append([]string{}, st.Tags...),
		MaxVRAMMb:    st.MaxVRAMMb,
		OllamaURL:    or(st.OllamaURL, "http://127.0.0.1:11434"),
		VLLMURL:      or(st.VLLMURL, "http://127.0.0.1:8000"),
		Runtime:      or(st.Runtime, "ollama"),
		OnNestsChanged: func(urls []string) {
			st.Nests = urls
			_ = SaveState(opt.ConfigDir, st)
		},
	})
	if err != nil {
		return err
	}
	go func() { errCh <- sh.Run(ctx) }()

	if st.Mode == protocol.ModeCaptain {
		keys := map[string]struct{}{}
		for _, k := range st.APIKeys {
			keys[k] = struct{}{}
		}
		api := &captain.API{
			Ship:         sh,
			JoinSecret:   st.JoinSecret,
			Nests:        st.Nests,
			APIKeys:      keys,
			Name:         st.Name,
			LocalNestURL: localNestURL,
			NestID:       localNestID,
		}
		handler := api.Handler(captain.UIHandler())
		listen := uiAddr
		if listen == "" {
			listen = protocol.DefaultCaptainUI
		}
		// Always bind Captain UI on UIAddr (default 127.0.0.1:7842).
		// --expose-api enables /v1 on the same listener; APIAddr is advertise-only.
		if st.ExposeAPI || opt.ExposeAPI {
			log.Printf("Captain UI + API on http://%s", listen)
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

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}
