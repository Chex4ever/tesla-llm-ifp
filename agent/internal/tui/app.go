package tui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
	"github.com/Chex4ever/pirate-fleet/agent/internal/vpsdeploy"
	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type logMsg string
type deployLogMsg string
type deployDoneMsg struct {
	url string
	err error
}
type stateMsg struct {
	st  *agent.State
	err error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case logMsg:
		m.Log = string(msg)
		return m, nil

	case deployLogMsg:
		line := string(msg)
		m.DeployLines = append(m.DeployLines, line)
		m.Log = line
		if m.deployLogCh != nil {
			return m, listenDeployLog(m.deployLogCh)
		}
		return m, nil

	case deployDoneMsg:
		m.deployLogCh = nil
		if msg.err != nil {
			m.DeployStatus = deployFailed
			m.DeployErr = msg.err.Error()
			m.Log = errStyle.Render(msg.err.Error())
			return m, nil
		}
		st, err := agent.AddNestURL(m.ConfigDir, msg.url)
		if err != nil {
			m.DeployStatus = deployFailed
			m.DeployErr = err.Error()
			m.Log = errStyle.Render(err.Error())
			return m, nil
		}
		m.State = st
		m.refreshJoinLink()
		m.DeployStatus = deployOK
		m.DeployURL = msg.url
		m.DeployLines = append(m.DeployLines, "done: "+msg.url)
		m.Log = okStyle.Render("Nest ready: " + msg.url)
		return m, nil

	case stateMsg:
		if msg.err != nil {
			m.Log = errStyle.Render(msg.err.Error())
			return m, nil
		}
		m.State = msg.st
		m.refreshJoinLink()
		m.resetMenu()
		m.Log = "State reloaded"
		return m, nil

	case tea.KeyMsg:
		key := normalizeKey(msg.String())
		if m.Modal != ModalNone {
			return m.handleModalKey(key, msg)
		}
		return m.handleMenuKey(key)
	}
	return m, nil
}

func (m Model) handleMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k", "shift+tab":
		if len(m.MenuItems) > 0 {
			m.MenuIdx = (m.MenuIdx - 1 + len(m.MenuItems)) % len(m.MenuItems)
		}
		return m, nil
	case "down", "tab":
		if len(m.MenuItems) > 0 {
			m.MenuIdx = (m.MenuIdx + 1) % len(m.MenuItems)
		}
		return m, nil
	case "enter":
		if len(m.MenuItems) == 0 {
			return m, nil
		}
		return m.activateMenu(m.MenuItems[m.MenuIdx].ID)
	case "?":
		m.Modal = ModalHelp
		return m, nil
	case "c":
		return m.openBecomeCaptain()
	case "n":
		return m.openDeployNest()
	case "j":
		return m.openJoinLink()
	case "s":
		return m, startShip(m.ConfigDir, m.State)
	case "r":
		return m, reloadState(m.ConfigDir)
	}
	return m, nil
}

func (m Model) activateMenu(id menuID) (tea.Model, tea.Cmd) {
	switch id {
	case menuCreateFleet:
		return m.openBecomeCaptain()
	case menuDeployNest:
		return m.openDeployNest()
	case menuJoinFleet:
		return m.openJoinLink()
	case menuCopyJoin:
		if m.JoinLink == "" {
			m.Log = warnStyle.Render("No join link yet")
			return m, nil
		}
		if err := clipboard.WriteAll(m.JoinLink); err != nil {
			m.Log = warnStyle.Render("Clipboard unavailable — select the link above")
			return m, nil
		}
		m.Log = okStyle.Render("Join link copied to clipboard")
		return m, nil
	case menuHostNest:
		on := !isHostNest(m.State)
		st, err := agent.SetHostNest(m.ConfigDir, on)
		if err != nil {
			m.Log = errStyle.Render(err.Error())
			return m, nil
		}
		m.State = st
		m.resetMenu()
		if on {
			m.Log = okStyle.Render("Host Nest + UI enabled")
		} else {
			m.Log = okStyle.Render("Host Nest off — peer ship only")
		}
		return m, nil
	case menuRun:
		return m, startShip(m.ConfigDir, m.State)
	case menuHelp:
		m.Modal = ModalHelp
		return m, nil
	case menuQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleModalKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Modal == ModalHelp {
		if key == "esc" || key == "enter" || key == "q" || key == "?" {
			m.Modal = ModalNone
		}
		return m, nil
	}

	if m.Modal == ModalDeployProgress {
		switch m.DeployStatus {
		case deployRunning:
			return m, nil // ignore keys while running
		case deployFailed:
			if key == "enter" {
				return m.openDeployNest()
			}
			if key == "esc" {
				m.Modal = ModalNone
				m.DeployStatus = deployIdle
				m.resetMenu()
			}
			return m, nil
		case deployOK:
			if key == "enter" || key == "esc" {
				m.Modal = ModalNone
				m.DeployStatus = deployIdle
				m.resetMenu()
			}
			return m, nil
		}
		return m, nil
	}

	switch key {
	case "esc":
		m.Modal = ModalNone
		return m, nil
	case "tab", "down":
		if len(m.Inputs) > 0 {
			m.Inputs[m.Focus].Blur()
			m.Focus = (m.Focus + 1) % len(m.Inputs)
			m.Inputs[m.Focus].Focus()
		}
		return m, nil
	case "shift+tab", "up":
		if len(m.Inputs) > 0 {
			m.Inputs[m.Focus].Blur()
			m.Focus = (m.Focus - 1 + len(m.Inputs)) % len(m.Inputs)
			m.Inputs[m.Focus].Focus()
		}
		return m, nil
	case "enter":
		return m.submitModal()
	}
	if len(m.Inputs) > 0 && m.Focus < len(m.Inputs) {
		var cmd tea.Cmd
		m.Inputs[m.Focus], cmd = m.Inputs[m.Focus].Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) openBecomeCaptain() (tea.Model, tea.Cmd) {
	host, _ := os.Hostname()
	in := textinput.New()
	in.Placeholder = "Ship name"
	in.SetValue(host)
	in.Focus()
	in.CharLimit = 64
	in.Width = 40
	m.Modal = ModalBecomeCaptain
	m.Inputs = []textinput.Model{in}
	m.Focus = 0
	return m, nil
}

func (m Model) openDeployNest() (tea.Model, tea.Cmd) {
	if !isEnrolled(m.State) {
		m.Log = warnStyle.Render("Create or Join a fleet first")
		m.Modal = ModalNone
		m.resetMenu()
		return m, nil
	}
	if !isHostNest(m.State) {
		m.Log = warnStyle.Render("Turn on Host Nest + UI first (or Create fleet)")
		m.Modal = ModalNone
		m.resetMenu()
		return m, nil
	}
	mk := func(ph string, echo textinput.EchoMode, val string) textinput.Model {
		in := textinput.New()
		in.Placeholder = ph
		in.EchoMode = echo
		in.SetValue(val)
		in.Width = 40
		return in
	}
	host := mk("Host / IP", textinput.EchoNormal, m.DeployHost)
	host.Focus()
	pass := mk("Root password *", textinput.EchoPassword, "")
	port := mk("SSH port", textinput.EchoNormal, "22")
	m.Modal = ModalDeployNest
	m.DeployStatus = deployIdle
	m.Inputs = []textinput.Model{host, pass, port}
	m.Focus = 0
	return m, nil
}

func (m Model) openJoinLink() (tea.Model, tea.Cmd) {
	in := textinput.New()
	in.Placeholder = "pirate://join?… or pf1.…"
	in.Focus()
	in.Width = 60
	m.Modal = ModalJoinLink
	m.Inputs = []textinput.Model{in}
	m.Focus = 0
	return m, nil
}

func (m Model) submitModal() (tea.Model, tea.Cmd) {
	switch m.Modal {
	case ModalBecomeCaptain:
		name := strings.TrimSpace(m.Inputs[0].Value())
		st, err := agent.BecomeCaptain(agent.BecomeCaptainOptions{
			ConfigDir: m.ConfigDir,
			Name:      name,
		})
		if err != nil {
			m.Log = errStyle.Render(err.Error())
			return m, nil
		}
		m.State = st
		m.refreshJoinLink()
		m.Modal = ModalNone
		m.resetMenu()
		m.Log = okStyle.Render("Fleet created as " + st.Name + " — next: Deploy Nest or Run")
		return m, nil

	case ModalJoinLink:
		link := strings.TrimSpace(m.Inputs[0].Value())
		name, _ := os.Hostname()
		st, err := agent.JoinFromLink(m.ConfigDir, link, name)
		if err != nil {
			m.Log = errStyle.Render(err.Error())
			return m, nil
		}
		m.State = st
		m.refreshJoinLink()
		m.Modal = ModalNone
		m.resetMenu()
		role := "ship"
		if isHostNest(st) {
			role = "ship + Host Nest"
		}
		m.Log = okStyle.Render("Joined fleet as " + role + " " + st.Name + " — Run to start")
		return m, nil

	case ModalDeployNest:
		return m.beginDeploy()
	}
	return m, nil
}

func (m Model) beginDeploy() (tea.Model, tea.Cmd) {
	if m.DeployStatus == deployRunning {
		return m, nil
	}
	host := strings.TrimSpace(m.Inputs[0].Value())
	pass := m.Inputs[1].Value()
	portStr := strings.TrimSpace(m.Inputs[2].Value())
	port, _ := strconv.Atoi(portStr)
	if port == 0 {
		port = 22
	}
	if host == "" || pass == "" {
		m.Log = warnStyle.Render("Host and password required")
		return m, nil
	}

	logCh := make(chan string, 64)
	m.deployLogCh = logCh
	m.DeployHost = host
	m.DeployLines = nil
	m.DeployURL = ""
	m.DeployErr = ""
	m.DeployStatus = deployRunning
	m.Modal = ModalDeployProgress
	m.Log = "Deploying nest to " + host + "…"

	return m, tea.Batch(
		runDeployCmd(host, pass, port, logCh),
		listenDeployLog(logCh),
	)
}

func runDeployCmd(host, pass string, port int, logCh chan string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		res, err := vpsdeploy.Deploy(ctx, vpsdeploy.Options{
			Host:     host,
			Port:     port,
			User:     "root",
			Password: pass,
			NestHost: host,
			OnLog: func(s string) {
				logCh <- s
			},
		})
		close(logCh)
		if err != nil {
			return deployDoneMsg{err: err}
		}
		return deployDoneMsg{url: res.NestURL}
	}
}

func listenDeployLog(logCh <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-logCh
		if !ok {
			return nil
		}
		return deployLogMsg(line)
	}
}

// canStartDeploy reports whether a new deploy may begin (for tests).
func (m Model) canStartDeploy() bool {
	return m.DeployStatus != deployRunning
}

func reloadState(cfgDir string) tea.Cmd {
	return func() tea.Msg {
		st, err := agent.LoadOrInitState(cfgDir)
		return stateMsg{st: st, err: err}
	}
}

func startShip(cfgDir string, st *agent.State) tea.Cmd {
	return func() tea.Msg {
		if st == nil || st.JoinSecret == "" {
			return logMsg(warnStyle.Render("Create or Join a fleet first"))
		}
		exe, err := os.Executable()
		if err != nil {
			return logMsg(err.Error())
		}
		mode := st.Mode
		if mode == "" {
			mode = "worker"
		}
		args := []string{"run", "--mode=" + mode, "--config-dir", cfgDir}
		if mode == "captain" {
			args = append(args, "--expose-api")
		}
		cmd := exec.Command(exe, args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			return logMsg(fmt.Sprintf("start: %v", err))
		}
		msg := fmt.Sprintf("Ship started (pid %d, mode=%s)", cmd.Process.Pid, mode)
		if mode == "captain" {
			msg += ". UI http://127.0.0.1:7842"
		}
		return logMsg(msg)
	}
}

func normalizeKey(key string) string {
	cyrToLat := map[rune]rune{
		'й': 'q', 'ц': 'w', 'у': 'e', 'к': 'r', 'е': 't', 'н': 'y', 'г': 'u', 'ш': 'i', 'щ': 'o', 'з': 'p',
		'ф': 'a', 'ы': 's', 'в': 'd', 'а': 'f', 'п': 'g', 'р': 'h', 'о': 'j', 'л': 'k', 'д': 'l',
		'я': 'z', 'ч': 'x', 'с': 'c', 'м': 'v', 'и': 'b', 'т': 'n', 'ь': 'm',
	}
	runes := []rune(key)
	if len(runes) == 1 {
		if lat, ok := cyrToLat[runes[0]]; ok {
			return string(lat)
		}
	}
	return key
}

// Run launches the Bubble Tea program.
func Run(configDir, version string) error {
	p := tea.NewProgram(NewModel(configDir, version), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
