package agent

import (
	"os"
	"path/filepath"
	"runtime"
)

func DefaultConfigDir() string {
	if runtime.GOOS == "windows" {
		base := os.Getenv("PROGRAMDATA")
		if base == "" {
			base = `C:\ProgramData`
		}
		return filepath.Join(base, "PirateFleet")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "pirate-fleet")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "pirate-fleet")
	}
	return "/etc/pirate-fleet"
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "agent.env")
}
