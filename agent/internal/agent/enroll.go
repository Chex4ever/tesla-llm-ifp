package agent

import (
	"fmt"
	"os"
	"strings"

	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/fleetcrypto"
	"github.com/Chex4ever/tesla-llm-ifp/agent/internal/protocol"
	"github.com/google/uuid"
)

type EnrollOptions struct {
	ConfigDir  string
	JoinSecret string
	Invite     string
	Name       string
	Nests      []string
	Mode       string
}

func Enroll(opt EnrollOptions) error {
	if opt.JoinSecret == "" {
		return fmt.Errorf("JOIN_SECRET / --join-secret required (fleet membership key)")
	}
	st, err := LoadOrInitState(opt.ConfigDir)
	if err != nil {
		return err
	}
	if st.NodeID == "" {
		st.NodeID = uuid.NewString()
	}
	st.JoinSecret = opt.JoinSecret
	if opt.Mode != "" {
		st.Mode = opt.Mode
	}
	if st.Mode == "" {
		st.Mode = protocol.ModeWorker
	}
	nests := append([]string{}, opt.Nests...)
	name := opt.Name
	if opt.Invite != "" {
		inv, err := fleetcrypto.ParseInvite(opt.JoinSecret, opt.Invite)
		if err != nil {
			return fmt.Errorf("invite: %w", err)
		}
		if name == "" {
			name = inv.Name
		}
		nests = append(nests, inv.Nests...)
		if inv.ModeHint != "" && opt.Mode == "" {
			st.Mode = inv.ModeHint
		}
	}
	if name != "" {
		st.Name = name
	}
	if st.Name == "" {
		host, _ := os.Hostname()
		st.Name = host
	}
	st.Nests = uniqueNests(nests, st.Nests)
	if len(st.Nests) == 0 {
		st.Nests = []string{protocol.DefaultNestURL}
	}
	if err := SaveState(opt.ConfigDir, st); err != nil {
		return err
	}
	env := fmt.Sprintf("JOIN_SECRET=%s\nMODE=%s\n", st.JoinSecret, st.Mode)
	_ = os.WriteFile(DefaultConfigPath(), []byte(env), 0o600)
	fmt.Printf("enrolled node %s (%s) nests=%s\n", st.NodeID, st.Name, strings.Join(st.Nests, ","))
	return nil
}

func uniqueNests(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, n := range list {
			n = strings.TrimSpace(n)
			if n == "" || seen[n] {
				continue
			}
			seen[n] = true
			out = append(out, n)
		}
	}
	return out
}
