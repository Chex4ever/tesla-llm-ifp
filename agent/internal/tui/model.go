package tui

import (
	"fmt"
	"strings"

	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ModalType int

const (
	ModalNone ModalType = iota
	ModalBecomeCaptain // create fleet
	ModalDeployNest
	ModalDeployProgress
	ModalJoinLink
	ModalHelp
)

type deployStatus int

const (
	deployIdle deployStatus = iota
	deployRunning
	deployOK
	deployFailed
)

type Model struct {
	ConfigDir string
	Version   string
	Width     int
	Height    int

	State *agent.State
	Log   string

	MenuItems []menuItem
	MenuIdx   int

	Modal  ModalType
	Inputs []textinput.Model
	Focus  int

	DeployStatus deployStatus
	DeployHost   string
	DeployLines  []string
	DeployURL    string
	DeployErr    string
	deployLogCh  chan string

	JoinLink string
}

func NewModel(configDir, version string) Model {
	st, _ := agent.LoadOrInitState(configDir)
	m := Model{
		ConfigDir: configDir,
		Version:   version,
		State:     st,
		Log:       "Ahoy. One app — Join or Create, then Run.",
		Width:     80,
		Height:    24,
	}
	m.refreshJoinLink()
	m.resetMenu()
	return m
}

func (m *Model) resetMenu() {
	items, idx := buildMenu(m.State)
	m.MenuItems = items
	m.MenuIdx = idx
}

func (m *Model) refreshJoinLink() {
	if m.State == nil || m.State.JoinSecret == "" {
		m.JoinLink = ""
		return
	}
	m.JoinLink = agent.FormatJoinLink(m.State.JoinSecret, m.State.Nests...)
}

func (m Model) Init() tea.Cmd {
	return nil
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("213"))
	hintStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	boxStyle    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	activeStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("51")).Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Pirate Fleet") + "  " + hintStyle.Render("v"+m.Version) + "\n\n")

	if m.Modal != ModalNone {
		b.WriteString(m.viewModal())
		b.WriteString("\n")
		b.WriteString(m.modalHints())
		return b.String()
	}

	if isEnrolled(m.State) {
		parts := []string{"Ship"}
		if isHostNest(m.State) {
			parts = append(parts, "Host Nest")
		}
		if isInfer(m.State) {
			parts = append(parts, "Infer")
		} else {
			parts = append(parts, "no-infer")
		}
		b.WriteString(okStyle.Render(strings.Join(parts, " · ")+": "+m.State.Name) + "\n")
		if len(m.State.Tags) > 0 {
			b.WriteString("Tags: " + strings.Join(m.State.Tags, ", ") + "\n")
		}
		if len(m.State.Nests) > 0 {
			b.WriteString("Nests:\n")
			for _, n := range m.State.Nests {
				b.WriteString("  • " + n + "\n")
			}
		} else if isHostNest(m.State) {
			b.WriteString(warnStyle.Render("No public nest yet — Deploy Nest optional for multi-site.") + "\n")
		}
		if m.JoinLink != "" && len(m.State.Nests) > 0 {
			b.WriteString("\n" + okStyle.Render("Join link (share with another PC):") + "\n")
			b.WriteString(boxStyle.Render(m.JoinLink) + "\n")
		}
		b.WriteString("\n")
	} else {
		b.WriteString(warnStyle.Render("Not enrolled. Create fleet or Join fleet.") + "\n\n")
	}

	b.WriteString("Actions:\n")
	for i, it := range m.MenuItems {
		line := fmt.Sprintf("%s  %s", it.Label, dimStyle.Render(it.Hint))
		if i == m.MenuIdx {
			b.WriteString(activeStyle.Render("► "+line) + "\n")
		} else {
			b.WriteString(dimStyle.Render("  "+line) + "\n")
		}
	}

	b.WriteString("\n" + hintStyle.Render("Log: ") + m.Log + "\n\n")
	b.WriteString(hintStyle.Render("[↑↓] Move  [Enter] Select  [c/n/j/s] Shortcuts  [q] Quit"))
	return b.String()
}

func (m Model) modalHints() string {
	switch m.Modal {
	case ModalDeployProgress:
		switch m.DeployStatus {
		case deployRunning:
			return hintStyle.Render("Deploy in progress… please wait")
		case deployFailed:
			return hintStyle.Render("[Enter] Back to form  [Esc] Menu")
		case deployOK:
			return hintStyle.Render("[Enter] Continue")
		}
	case ModalHelp:
		return hintStyle.Render("[Enter/Esc] Close")
	}
	return hintStyle.Render("[Enter] OK  [Esc] Cancel  [Tab] Next field")
}

func (m Model) viewModal() string {
	switch m.Modal {
	case ModalHelp:
		return boxStyle.Render(`One app — capabilities, not roles

  Create fleet → Host Nest on (toggle Infer off on a laptop without GPU)
  Join fleet   → paste pirate://join?…
  Run          → start

  Host Nest + UI  and  Accept inference  are independent.
  Console PC: Host Nest on, Accept inference off.
  GPU PC: Infer on; Host Nest optional.

Nest TLS is self-signed. Password is never saved.`)

	case ModalDeployProgress:
		var b strings.Builder
		switch m.DeployStatus {
		case deployRunning:
			b.WriteString(warnStyle.Render("Deploying nest to "+m.DeployHost+"…") + "\n\n")
		case deployOK:
			b.WriteString(okStyle.Render("Nest ready: "+m.DeployURL) + "\n\n")
		case deployFailed:
			b.WriteString(errStyle.Render("Deploy failed") + "\n")
			b.WriteString(errStyle.Render(m.DeployErr) + "\n\n")
		}
		lines := m.DeployLines
		if len(lines) > 20 {
			lines = lines[len(lines)-20:]
		}
		for _, line := range lines {
			b.WriteString(dimStyle.Render(line) + "\n")
		}
		if m.DeployStatus == deployRunning && len(m.DeployLines) == 0 {
			b.WriteString(dimStyle.Render("starting…") + "\n")
		}
		return boxStyle.Render(b.String())
	}

	var title string
	switch m.Modal {
	case ModalBecomeCaptain:
		title = "Create fleet"
	case ModalDeployNest:
		title = "Deploy Nest (self-signed TLS)"
	case ModalJoinLink:
		title = "Join fleet — paste pirate://join?…"
	}
	var b strings.Builder
	b.WriteString(titleStyle.Render(title) + "\n\n")
	for i, in := range m.Inputs {
		label := in.Placeholder
		if i == m.Focus {
			b.WriteString(activeStyle.Render("► "+label) + "\n")
		} else {
			b.WriteString("  " + label + "\n")
		}
		b.WriteString(in.View() + "\n\n")
	}
	if m.Modal == ModalDeployNest {
		b.WriteString(dimStyle.Render("Password is used only for this SSH session and is never saved.") + "\n")
	}
	return boxStyle.Render(b.String())
}
