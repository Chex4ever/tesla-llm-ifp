package agent

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/Chex4ever/pirate-fleet/agent/internal/protocol"
)

// BecomeCaptainOptions configures first-run fleet creation (Host Nest + UI).
type BecomeCaptainOptions struct {
	ConfigDir  string
	Name       string
	JoinSecret string // optional; generated if empty
	Nests      []string
}

// BecomeCaptain generates a fleet secret if needed and enrolls with Host Nest mode.
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
		Mode:       protocol.ModeCaptain,
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

// SetHostNest enables or disables local Nest + UI (captain extras).
func SetHostNest(configDir string, on bool) (*State, error) {
	st, err := LoadOrInitState(configDir)
	if err != nil {
		return nil, err
	}
	if st.JoinSecret == "" {
		return nil, fmt.Errorf("not enrolled: create or join a fleet first")
	}
	if on {
		st.Mode = protocol.ModeCaptain
	} else {
		st.Mode = protocol.ModeWorker
	}
	if err := SaveState(configDir, st); err != nil {
		return nil, err
	}
	return st, nil
}

// JoinFromLink parses a pirate://join link and enrolls into the fleet.
// With invite → peer ship (worker). Without invite → Host Nest (captain) for second-site.
func JoinFromLink(configDir, link, name string) (*State, error) {
	j, err := ParseJoinLink(link)
	if err != nil {
		return nil, err
	}
	nests := j.Nests
	if j.Nest != "" {
		nests = uniqueNests([]string{j.Nest}, nests)
	}
	mode := protocol.ModeCaptain
	if j.Invite != "" {
		mode = protocol.ModeWorker
	}
	if err := Enroll(EnrollOptions{
		ConfigDir:  configDir,
		JoinSecret: j.Secret,
		Invite:     j.Invite,
		Name:       name,
		Mode:       mode,
		Nests:      nests,
	}); err != nil {
		return nil, err
	}
	return LoadOrInitState(configDir)
}
