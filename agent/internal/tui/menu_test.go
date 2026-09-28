package tui

import (
	"testing"

	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
	tea "github.com/charmbracelet/bubbletea"
)

func TestBuildMenuDefaultCreateFleet(t *testing.T) {
	items, idx := buildMenu(nil)
	if items[idx].ID != menuCreateFleet {
		t.Fatalf("default=%v want CreateFleet", items[idx].ID)
	}
}

func TestBuildMenuDefaultDeployAfterCaptain(t *testing.T) {
	st := &agent.State{JoinSecret: "x", Mode: "captain", Name: "c"}
	items, idx := buildMenu(st)
	if items[idx].ID != menuDeployNest {
		t.Fatalf("default=%v want DeployNest", items[idx].ID)
	}
}

func TestBuildMenuDefaultRunWhenHasNest(t *testing.T) {
	st := &agent.State{
		JoinSecret: "x", Mode: "captain", Name: "c",
		Nests: []string{"wss://1.2.3.4/nest"},
	}
	items, idx := buildMenu(st)
	if items[idx].ID != menuRun {
		t.Fatalf("default=%v want Run", items[idx].ID)
	}
	found := false
	for _, it := range items {
		if it.ID == menuCopyJoin {
			found = true
		}
	}
	if !found {
		t.Fatal("Copy join missing from menu")
	}
}

func TestNextMenuIndexAfterBecomeCaptain(t *testing.T) {
	dir := t.TempDir()
	st, err := agent.BecomeCaptain(agent.BecomeCaptainOptions{ConfigDir: dir, Name: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if nextMenuIndex(st) != indexOfMenu(mustItems(st), menuDeployNest) {
		t.Fatalf("after create fleet next should be Deploy Nest")
	}
}

func mustItems(st *agent.State) []menuItem {
	items, _ := buildMenu(st)
	return items
}

func TestMenuEnterSelectsHighlighted(t *testing.T) {
	dir := t.TempDir()
	m := NewModel(dir, "test")
	if m.MenuItems[m.MenuIdx].ID != menuCreateFleet {
		t.Fatalf("idx=%d id=%v", m.MenuIdx, m.MenuItems[m.MenuIdx].ID)
	}
	nm, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.Modal != ModalBecomeCaptain {
		t.Fatalf("modal=%v want BecomeCaptain/CreateFleet", m.Modal)
	}
}
