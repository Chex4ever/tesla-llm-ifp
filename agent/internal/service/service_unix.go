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
	target := "/usr/local/bin/tesla-agent"
	in, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, in, 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(`[Unit]
Description=Tesla LLM Agent
After=network-online.target
Wants=network-online.target

[Service]
EnvironmentFile=%s
ExecStart=%s run --config %s
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
`, filepath.Join(configDir, "agent.env"), target, filepath.Join(configDir, "agent.env"))
	if err := os.WriteFile("/etc/systemd/system/tesla-agent.service", []byte(unit), 0o644); err != nil {
		return err
	}
	_, _ = exec.Command("systemctl", "daemon-reload").CombinedOutput()
	_, err = exec.Command("systemctl", "enable", "tesla-agent").CombinedOutput()
	return err
}

func Uninstall() error {
	_ = Stop()
	_, _ = exec.Command("systemctl", "disable", "tesla-agent").CombinedOutput()
	return os.Remove("/etc/systemd/system/tesla-agent.service")
}

func Start() error {
	out, err := exec.Command("systemctl", "start", "tesla-agent").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}

func Stop() error {
	out, err := exec.Command("systemctl", "stop", "tesla-agent").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, out)
	}
	return nil
}
