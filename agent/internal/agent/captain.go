package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
)

// BecomeCaptainOptions configures first-run fleet creation (Host Nest + UI).
type BecomeCaptainOptions struct {
	ConfigDir  string
	Name       string
	JoinSecret string // optional; generated if empty
	Nests      []string
}

// BecomeCaptain generates a fleet secret and enrolls with Host Nest on (Infer stays on by default).
func BecomeCaptain(opt BecomeCaptainOptions) (*State, error) {
	secret := opt.JoinSecret
	if secret == "" {
		var b [32]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, fmt.Errorf("generate join secret: %w", err)
		}
		secret = hex.EncodeToString(b[:])
	}
	name := opt.Name
	if name == "" {
		host, _ := os.Hostname()
		name = host
	}
	if err := Enroll(EnrollOptions{
		ConfigDir:  opt.ConfigDir,
		JoinSecret: secret,
		Name:       name,
		HostNest:   boolPtr(true),
		Infer:      boolPtr(true),
		Nests:      opt.Nests,
	}); err != nil {
		return nil, err
	}
	return LoadOrInitState(opt.ConfigDir)
}

// AddNestURL appends a nest URL to state (e.g. after VPS deploy).
func AddNestURL(configDir, nestURL string) (*State, error) {
	st, err := LoadOrInitState(configDir)
	if err != nil {
		return nil, err
	}
	st.Nests = uniqueNests(st.Nests, []string{nestURL})
	if err := SaveState(configDir, st); err != nil {
		return nil, err
	}
	return st, nil
}

// SetHostNest enables or disables local Nest + UI without touching Infer.
func SetHostNest(configDir string, on bool) (*State, error) {
	st, err := LoadOrInitState(configDir)
	if err != nil {
		return nil, err
	}
	if st.JoinSecret == "" {
		return nil, fmt.Errorf("not enrolled: create or join a fleet first")
	}
	st.HostNest = on
	st.Mode = "ship"
	if err := SaveState(configDir, st); err != nil {
		return nil, err
	}
	return st, nil
}

// SetInfer enables or disables accepting inference jobs without touching HostNest.
func SetInfer(configDir string, on bool) (*State, error) {
	st, err := LoadOrInitState(configDir)
	if err != nil {
		return nil, err
	}
	if st.JoinSecret == "" {
		return nil, fmt.Errorf("not enrolled: create or join a fleet first")
	}
	st.Infer = on
	if err := SaveState(configDir, st); err != nil {
		return nil, err
	}
	return st, nil
}

// JoinFromLink parses a pirate://join link and enrolls into the fleet.
// With invite → peer ship. Without invite → Host Nest on (second site).
func JoinFromLink(configDir, link, name string) (*State, error) {
	j, err := ParseJoinLink(link)
	if err != nil {
		return nil, err
	}
	nests := j.Nests
	if j.Nest != "" {
		nests = uniqueNests([]string{j.Nest}, nests)
	}
	hostNest := j.Invite == ""
	if err := Enroll(EnrollOptions{
		ConfigDir:  configDir,
		JoinSecret: j.Secret,
		Invite:     j.Invite,
		Name:       name,
		HostNest:   boolPtr(hostNest),
		Infer:      boolPtr(true),
		Nests:      nests,
	}); err != nil {
		return nil, err
	}
	return LoadOrInitState(configDir)
}
