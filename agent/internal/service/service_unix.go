//go:build !windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func Install(configDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	target := "/usr/local/bin/pirate"
	in, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, in, 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=Pirate Fleet ship
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=%s
ExecStart=%s run --config-dir %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, filepath.Join(configDir, "agent.env"), target, configDir)
	if err := os.WriteFile("/etc/systemd/system/pirate-fleet.service", []byte(unit), 0o644); err != nil {
		return err
	}
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
	_, err = exec.Command("systemctl", "enable", "pirate-fleet").CombinedOutput()
	return err
}

func Uninstall() error {
	_ = Stop()
	_, _ = exec.Command("systemctl", "disable", "pirate-fleet").CombinedOutput()
	return os.Remove("/etc/systemd/system/pirate-fleet.service")
}

func Start() error {
	out, err := exec.Command("systemctl", "start", "pirate-fleet").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func Stop() error {
	out, err := exec.Command("systemctl", "stop", "pirate-fleet").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
