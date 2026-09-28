package fleetcrypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func HMACHex(secret, msg string) string {
	m := hmac.New(sha256.New, []byte(secret))
	_, _ = m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}

func VerifyHMAC(secret, msg, got string) bool {
	want := HMACHex(secret, msg)
	return hmac.Equal([]byte(want), []byte(got))
}

func GossipMac(secret, nodeID, mode string, ts time.Time, models, tags []string) string {
	canon := fmt.Sprintf("%s|%s|%d|%s|%s", nodeID, mode, ts.Unix(), strings.Join(models, ","), strings.Join(tags, ","))
	return HMACHex(secret, canon)
}

type Invite struct {
	Name      string   `json:"name"`
	Nests     []string `json:"nests"`
	ExpiresAt int64    `json:"exp"`
	ModeHint  string   `json:"mode_hint,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	MaxVRAMMb int      `json:"max_vram_mb,omitempty"`
	Sig       string   `json:"sig"`
}

type inviteBody struct {
	Name      string   `json:"name"`
	Nests     []string `json:"nests"`
	ExpiresAt int64    `json:"exp"`
	ModeHint  string   `json:"mode_hint,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	MaxVRAMMb int      `json:"max_vram_mb,omitempty"`
}

func IssueInvite(secret, name string, nests []string, ttl time.Duration) (string, error) {
	return IssueInviteFull(secret, Invite{
		Name: name, Nests: nests, ExpiresAt: time.Now().Add(ttl).Unix(), ModeHint: "worker",
	})
}

func IssueInviteFull(secret string, inv Invite) (string, error) {
	if inv.ExpiresAt == 0 {
		inv.ExpiresAt = time.Now().Add(24 * time.Hour).Unix()
	}
	if inv.ModeHint == "" {
		inv.ModeHint = "worker"
	}
	body := inviteBody{
		Name: inv.Name, Nests: inv.Nests, ExpiresAt: inv.ExpiresAt,
		ModeHint: inv.ModeHint, Tags: inv.Tags, MaxVRAMMb: inv.MaxVRAMMb,
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	inv.Sig = HMACHex(secret, string(raw))
	inv.Name = body.Name
	inv.Nests = body.Nests
	inv.ExpiresAt = body.ExpiresAt
	inv.ModeHint = body.ModeHint
	inv.Tags = body.Tags
	inv.MaxVRAMMb = body.MaxVRAMMb
	full, err := json.Marshal(inv)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(full), nil
}

func ParseInvite(secret, token string) (*Invite, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("invite decode: %w", err)
	}
	var inv Invite
	if err := json.Unmarshal(raw, &inv); err != nil {
		return nil, err
	}
	body, _ := json.Marshal(inviteBody{
		Name: inv.Name, Nests: inv.Nests, ExpiresAt: inv.ExpiresAt,
		ModeHint: inv.ModeHint, Tags: inv.Tags, MaxVRAMMb: inv.MaxVRAMMb,
	})
	if !VerifyHMAC(secret, string(body), inv.Sig) {
		return nil, fmt.Errorf("invalid invite signature")
	}
	if time.Now().Unix() > inv.ExpiresAt {
		return nil, fmt.Errorf("invite expired")
	}
	return &inv, nil
}
