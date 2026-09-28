package tui

import (
	"testing"

	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCannotDoubleStartDeploy(t *testing.T) {
	dir := t.TempDir()
	st, err := agent.BecomeCaptain(agent.BecomeCaptainOptions{ConfigDir: dir, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(dir, "test")
	m.State = st
	m.DeployStatus = deployRunning
	m.Modal = ModalDeployProgress
	if m.canStartDeploy() {
		t.Fatal("should not start while running")
	}

	m.Inputs = deployFormInputs("1.2.3.4", "secret", "22")
	m.Modal = ModalDeployNest
	m.DeployStatus = deployRunning
	nm, cmd := m.beginDeploy()
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("beginDeploy must not return cmd while already running")
	}
	if m.Modal != ModalDeployNest {
		t.Fatalf("modal changed unexpectedly: %v", m.Modal)
	}
}

func TestDeployLogMsgAppends(t *testing.T) {
	m := NewModel(t.TempDir(), "test")
	m.Modal = ModalDeployProgress
	m.DeployStatus = deployRunning
	ch := make(chan string)
	m.deployLogCh = ch
	nm, cmd := m.Update(deployLogMsg("building Linux pirate binary…"))
	m = nm.(Model)
	if len(m.DeployLines) != 1 || m.DeployLines[0] != "building Linux pirate binary…" {
		t.Fatalf("lines=%v", m.DeployLines)
	}
	if cmd == nil {
		t.Fatal("expected continue listen cmd")
	}
}

func TestDeployFailedEnterGoesToFormNotRedeploy(t *testing.T) {
	dir := t.TempDir()
	st, err := agent.BecomeCaptain(agent.BecomeCaptainOptions{ConfigDir: dir, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	m := NewModel(dir, "test")
	m.State = st
	m.Modal = ModalDeployProgress
	m.DeployStatus = deployFailed
	m.DeployErr = "ssh failed"
	m.DeployHost = "1.2.3.4"

	nm, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if cmd != nil {
		t.Fatal("Enter on failed must not start deploy cmd")
	}
	if m.Modal != ModalDeployNest {
		t.Fatalf("modal=%v want DeployNest form", m.Modal)
	}
	if m.DeployStatus == deployRunning {
		t.Fatal("must not be running after Enter on failed")
	}
}

func TestDeployDoneFailedKeepsProgressModal(t *testing.T) {
	m := NewModel(t.TempDir(), "test")
	m.Modal = ModalDeployProgress
	m.DeployStatus = deployRunning
	nm, _ := m.Update(deployDoneMsg{err: errString("boom")})
	m = nm.(Model)
	if m.DeployStatus != deployFailed {
		t.Fatalf("status=%v", m.DeployStatus)
	}
	if m.Modal != ModalDeployProgress {
		t.Fatalf("modal=%v want progress", m.Modal)
	}
	if m.canStartDeploy() == false {
		// failed means not running — can start after returning to form
	}
	if m.DeployStatus == deployRunning {
		t.Fatal("still running")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func deployFormInputs(host, pass, port string) []textinput.Model {
	mk := func(v string) textinput.Model {
		in := textinput.New()
		in.SetValue(v)
		return in
	}
	return []textinput.Model{mk(host), mk(pass), mk(port)}
}
