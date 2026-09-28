package tui

import (
	"github.com/Chex4ever/pirate-fleet/agent/internal/agent"
)

type menuID int

const (
	menuCreateFleet menuID = iota
	menuDeployNest
	menuJoinFleet
	menuCopyJoin
	menuHostNest
	menuInfer
	menuRun
	menuHelp
	menuQuit
)

type menuItem struct {
	ID    menuID
	Label string
	Hint  string
}

// buildMenu returns items and the recommended cursor index for the next step.
func buildMenu(st *agent.State) (items []menuItem, defaultIdx int) {
	enrolled := isEnrolled(st)
	hostNest := isHostNest(st)
	hasNest := st != nil && len(st.Nests) > 0

	items = []menuItem{
		{ID: menuCreateFleet, Label: "Create fleet", Hint: "generate secret + Host Nest"},
		{ID: menuJoinFleet, Label: "Join fleet", Hint: "paste pirate://join link"},
		{ID: menuDeployNest, Label: "Deploy Nest", Hint: "VPS host + root password"},
	}
	if enrolled && hasNest {
		items = append(items, menuItem{ID: menuCopyJoin, Label: "Copy join link", Hint: "share with another PC"})
	}
	if enrolled {
		if hostNest {
			items = append(items, menuItem{
				ID: menuHostNest, Label: "Host Nest + UI: ON",
				Hint: "Nest + panel on this PC — Enter to turn off",
			})
		} else {
			items = append(items, menuItem{
				ID: menuHostNest, Label: "Host Nest + UI: OFF",
				Hint: "Enter to enable Nest + panel",
			})
		}

		if isInfer(st) {
			items = append(items, menuItem{
				ID: menuInfer, Label: "Accept inference: ON",
				Hint: "this PC takes LLM jobs — Enter to stop",
			})
		} else {
			items = append(items, menuItem{
				ID: menuInfer, Label: "Accept inference: OFF",
				Hint: "console only, no jobs — Enter to start taking jobs",
			})
		}
	}
	items = append(items,
		menuItem{ID: menuRun, Label: "Run", Hint: "start ship"},
		menuItem{ID: menuHelp, Label: "Help", Hint: ""},
		menuItem{ID: menuQuit, Label: "Quit", Hint: ""},
	)

	switch {
	case !enrolled:
		defaultIdx = indexOfMenu(items, menuCreateFleet)
	case hostNest && !hasNest:
		defaultIdx = indexOfMenu(items, menuDeployNest)
	default:
		defaultIdx = indexOfMenu(items, menuRun)
	}
	if defaultIdx < 0 {
		defaultIdx = 0
	}
	return items, defaultIdx
}

func indexOfMenu(items []menuItem, id menuID) int {
	for i, it := range items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func isEnrolled(st *agent.State) bool {
	return st != nil && st.JoinSecret != ""
}

func isHostNest(st *agent.State) bool {
	return st != nil && st.JoinSecret != "" && st.HostNest
}

func isInfer(st *agent.State) bool {
	return st != nil && st.Infer
}

func isCaptain(st *agent.State) bool {
	return isHostNest(st)
}

func nextMenuIndex(st *agent.State) int {
	_, idx := buildMenu(st)
	return idx
}
