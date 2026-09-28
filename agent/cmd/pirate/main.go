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

	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
	"github.com/Chex4ever/pirate-fleet/agent/internal/service"
	"github.com/Chex4ever/pirate-fleet/agent/internal/tui"
)

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		if err := tui.Run(agent.DefaultConfigDir(), version); err != nil {
			log.Fatalf("tui: %v", err)
		}
		return
	}
	cmd := os.Args[1]
	switch cmd {
	case "version":
		fmt.Printf("pirate %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
	case "tui":
		cfgDir := agent.DefaultConfigDir()
		if len(os.Args) > 2 {
			fs := flag.NewFlagSet("tui", flag.ExitOnError)
			d := fs.String("config-dir", agent.DefaultConfigDir(), "state directory")
			_ = fs.Parse(os.Args[2:])
			cfgDir = *d
		}
		if err := tui.Run(cfgDir, version); err != nil {
			log.Fatalf("tui: %v", err)
		}
	case "enroll":
		fs := flag.NewFlagSet("enroll", flag.ExitOnError)
		cfgDir := fs.String("config-dir", agent.DefaultConfigDir(), "state directory")
		join := fs.String("join-secret", "", "fleet join secret")
		invite := fs.String("invite", "", "invite token from a Host Nest UI")
		name := fs.String("name", "", "ship name")
		mode := fs.String("mode", "", "worker|captain")
		nests := fs.String("nests", "", "comma-separated Crow's Nest URLs")
		tags := fs.String("tags", "", "comma-separated tags")
		maxVRAM := fs.Int("max-vram-mb", 0, "max model VRAM this ship will run (0=auto)")
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
		var tagList []string
		if *tags != "" {
			for _, t := range strings.Split(*tags, ",") {
				t = strings.TrimSpace(t)
				if t != "" {
					tagList = append(tagList, t)
				}
			}
		}
		if err := agent.Enroll(agent.EnrollOptions{
			ConfigDir:  *cfgDir,
			JoinSecret: js,
			Invite:     inv,
			Name:       *name,
			Mode:       *mode,
			Nests:      nestList,
			Tags:       tagList,
			MaxVRAMMb:  *maxVRAM,
		}); err != nil {
			log.Fatalf("enroll: %v", err)
		}
	case "join":
		fs := flag.NewFlagSet("join", flag.ExitOnError)
		cfgDir := fs.String("config-dir", agent.DefaultConfigDir(), "state directory")
		name := fs.String("name", "", "captain name")
		_ = fs.Parse(os.Args[2:])
		if fs.NArg() < 1 {
			log.Fatal("usage: pirate join <pirate://join?…|pf1.…>")
		}
		st, err := agent.JoinFromLink(*cfgDir, fs.Arg(0), *name)
		if err != nil {
			log.Fatalf("join: %v", err)
		}
		fmt.Printf("joined as captain %s nests=%v\n", st.Name, st.Nests)
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
			log.Fatal("usage: pirate service [install|uninstall|start|stop]")
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
	fmt.Printf(`pirate %s — Pirate Fleet

TUI (default):
  pirate
  pirate tui

One app: Create/Join fleet → Run. Host Nest + UI is optional.

CLI:
  pirate join <pirate://join?…|pf1.…>
  pirate enroll --join-secret SECRET [--invite TOKEN] [--name NAME] [--tags a,b] [--max-vram-mb N]
  pirate run [--mode=worker|captain|crowsnest] [--expose-api]
  pirate service install|start|stop|uninstall
  pirate version
`, version)
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
