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
		return filepath.Join(base, "TeslaAgent")
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "tesla-agent")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".config", "tesla-agent")
	}
	return "/etc/tesla-agent"
}

func DefaultConfigPath() string {
	return filepath.Join(DefaultConfigDir(), "agent.env")
}
