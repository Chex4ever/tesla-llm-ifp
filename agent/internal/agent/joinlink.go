package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// JoinLink is a shareable fleet bootstrap (optional invite for tags/name).
type JoinLink struct {
	Nest   string   `json:"nest,omitempty"`
	Nests  []string `json:"nests,omitempty"`
	Secret string   `json:"secret"`
	Invite string   `json:"invite,omitempty"`
}

// FormatJoinLink builds pirate://join?secret=&nest=…
func FormatJoinLink(secret string, nests ...string) string {
	return FormatJoinLinkFull(secret, "", nests...)
}

// FormatJoinLinkFull builds pirate://join with optional invite token.
func FormatJoinLinkFull(secret, invite string, nests ...string) string {
	nests = uniqueNests(nests)
	primary := ""
	if len(nests) > 0 {
		primary = nests[0]
	}
	q := url.Values{}
	q.Set("secret", secret)
	if primary != "" {
		q.Set("nest", primary)
	}
	for _, n := range nests {
		if n != primary {
			q.Add("nests", n)
		}
	}
	if invite != "" {
		q.Set("invite", invite)
	}
	return "pirate://join?" + q.Encode()
}

// FormatJoinBlob returns a compact base64 join token (pf1.…).
func FormatJoinBlob(secret string, nests ...string) string {
	nests = uniqueNests(nests)
	primary := ""
	if len(nests) > 0 {
		primary = nests[0]
	}
	raw, _ := json.Marshal(JoinLink{Nest: primary, Nests: nests, Secret: secret})
	return "pf1." + base64.RawURLEncoding.EncodeToString(raw)
}

// ParseJoinLink accepts pirate://join?…, pf1.…, or raw wss://… with ?secret= / #secret=.
func ParseJoinLink(s string) (*JoinLink, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("empty join link")
	}
	if strings.HasPrefix(s, "pf1.") {
		raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, "pf1."))
		if err != nil {
			return nil, fmt.Errorf("join blob: %w", err)
		}
		var j JoinLink
		if err := json.Unmarshal(raw, &j); err != nil {
			return nil, fmt.Errorf("join blob json: %w", err)
		}
		if j.Secret == "" {
			return nil, fmt.Errorf("join blob missing secret")
		}
		return &j, nil
	}
	if strings.HasPrefix(s, "pirate://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		secret := q.Get("secret")
		if secret == "" {
			return nil, fmt.Errorf("join link missing secret")
		}
		j := &JoinLink{
			Secret: secret,
			Nest:   q.Get("nest"),
			Nests:  q["nests"],
			Invite: q.Get("invite"),
		}
		if j.Nest != "" {
			j.Nests = uniqueNests([]string{j.Nest}, j.Nests)
		}
		return j, nil
	}
	// wss://host/nest?secret=HEX or …#secret=HEX
	if strings.HasPrefix(s, "wss://") || strings.HasPrefix(s, "ws://") {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}
		secret := u.Query().Get("secret")
		if secret == "" && u.Fragment != "" {
			if strings.HasPrefix(u.Fragment, "secret=") {
				secret = strings.TrimPrefix(u.Fragment, "secret=")
			} else {
				fv, _ := url.ParseQuery(u.Fragment)
				secret = fv.Get("secret")
			}
		}
		if secret == "" {
			return nil, fmt.Errorf("nest URL missing secret (use pirate://join or append ?secret=)")
		}
		invite := u.Query().Get("invite")
		u.RawQuery = ""
		u.Fragment = ""
		nest := u.String()
		return &JoinLink{Secret: secret, Nest: nest, Nests: []string{nest}, Invite: invite}, nil
	}
	return nil, fmt.Errorf("unrecognized join link (want pirate://join?… or pf1.…)")
}
