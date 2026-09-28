//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const serviceName = "TeslaAgent"

func Install(configDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, err = filepath.Abs(exe)
	if err != nil {
		return err
	}
	binPath := filepath.Join(configDir, "tesla-agent.exe")
	_ = os.MkdirAll(configDir, 0o755)
	in, err := os.ReadFile(exe)
	if err != nil {
		return err
	}
	if err := os.WriteFile(binPath, in, 0o755); err != nil {
		return err
	}
	bin := fmt.Sprintf(`"%s" run --config "%s"`, binPath, filepath.Join(configDir, "agent.env"))
	cmd := exec.Command("sc", "create", serviceName, "binPath=", bin, "start=", "auto", "DisplayName=", "Tesla LLM Agent")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc create: %v (%s)", err, string(out))
	}
	_, _ = exec.Command("sc", "description", serviceName, "Tesla LLM distributed inference agent").CombinedOutput()
	return nil
}

func Uninstall() error {
	_ = Stop()
	out, err := exec.Command("sc", "delete", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc delete: %v (%s)", err, string(out))
	}
	return nil
}

func Start() error {
	out, err := exec.Command("sc", "start", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc start: %v (%s)", err, string(out))
	}
	return nil
}

func Stop() error {
	out, err := exec.Command("sc", "stop", serviceName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc stop: %v (%s)", err, string(out))
	}
	return nil
}
