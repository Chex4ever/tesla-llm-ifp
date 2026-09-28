package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/agent"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/service"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}
	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Printf("tesla-agent %s %s/%s (Pirate Fleet)\n", version, runtime.GOOS, runtime.GOARCH)
	case "enroll":
		fs := flag.NewFlagSet("enroll", flag.ExitOnError)
		cfgDir := fs.String("config-dir", agent.DefaultConfigDir(), "state directory")
		join := fs.String("join-secret", "", "fleet join secret")
		invite := fs.String("invite", "", "invite token from a Captain")
		name := fs.String("name", "", "ship name")
		mode := fs.String("mode", "", "worker|captain")
		nests := fs.String("nests", "", "comma-separated Crow's Nest URLs")
		_ = fs.Parse(os.Args[2:])
		loadDotEnv(agent.DefaultConfigPath())
		js := *join
		if js == "" {
			js = env("JOIN_SECRET", "")
		}
		inv := *invite
		if inv == "" {
			inv = env("INVITE_TOKEN", "")
		}
		var nestList []string
		if *nests != "" {
			nestList = strings.Split(*nests, ",")
		}
		if err := agent.Enroll(agent.EnrollOptions{
			ConfigDir:  *cfgDir,
			JoinSecret: js,
			Invite:     inv,
			Name:       *name,
			Mode:       *mode,
			Nests:      nestList,
		}); err != nil {
			log.Fatalf("enroll: %v", err)
		}
	case "run":
		fs := flag.NewFlagSet("run", flag.ExitOnError)
		cfgDir := fs.String("config-dir", agent.DefaultConfigDir(), "state directory")
		mode := fs.String("mode", "", "worker|captain|crowsnest")
		expose := fs.Bool("expose-api", false, "Captain: serve OpenAI /v1")
		listen := fs.String("listen", "", "listen addr (crowsnest default :7843)")
		uiAddr := fs.String("ui", "", "captain UI addr")
		apiAddr := fs.String("api", "", "captain API addr when exposing")
		_ = fs.Parse(os.Args[2:])
		loadDotEnv(agent.DefaultConfigPath())
		m := *mode
		if m == "" {
			m = env("MODE", "")
		}
		ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()
		if err := agent.Run(ctx, agent.RunOptions{
			ConfigDir:    *cfgDir,
			Mode:         m,
			ExposeAPI:    *expose || env("EXPOSE_API", "") == "1",
			ListenAddr:   *listen,
			UIAddr:       *uiAddr,
			APIAddr:      *apiAddr,
			AgentVersion: version,
		}); err != nil {
			log.Fatalf("run: %v", err)
		}
	case "service":
		if len(os.Args) < 3 {
			log.Fatal("usage: tesla-agent service [install|uninstall|start|stop]")
		}
		switch os.Args[2] {
		case "install":
			if err := service.Install(agent.DefaultConfigDir()); err != nil {
				log.Fatal(err)
			}
			fmt.Println("service installed")
		case "uninstall":
			if err := service.Uninstall(); err != nil {
				log.Fatal(err)
			}
			fmt.Println("service uninstalled")
		case "start":
			if err := service.Start(); err != nil {
				log.Fatal(err)
			}
			fmt.Println("service started")
		case "stop":
			if err := service.Stop(); err != nil {
				log.Fatal(err)
			}
			fmt.Println("service stopped")
		default:
			log.Fatal("unknown service command")
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Printf(`tesla-agent %s — Pirate Fleet

Roles:
  worker     Deckhand — inference + gossip
  captain    Captain  — GUI + invites (+ optional --expose-api)
  crowsnest  Crow's Nest — zero NAT router (rendezvous/relay)

Usage:
  tesla-agent enroll --join-secret SECRET [--invite TOKEN] [--name NAME] [--nests wss://...]
  tesla-agent run --mode=worker
  tesla-agent run --mode=captain [--expose-api] [--ui 127.0.0.1:7842]
  tesla-agent run --mode=crowsnest [--listen :7843]
  tesla-agent service install|start|stop|uninstall
  tesla-agent version

Default Nest: %s
`, version, protocol.DefaultNestURL)
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func loadDotEnv(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
}
