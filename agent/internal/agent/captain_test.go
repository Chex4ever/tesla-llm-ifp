package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Chex4ever/pirate-fleet/agent/internal/fleetcrypto"
)

func TestBecomeCaptainGeneratesSecret(t *testing.T) {
	dir := t.TempDir()
	st, err := BecomeCaptain(BecomeCaptainOptions{
		ConfigDir: dir,
		Name:      "cap-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "captain" {
		t.Fatalf("mode=%q", st.Mode)
	}
	if st.Name != "cap-test" {
		t.Fatalf("name=%q", st.Name)
	}
	if len(st.JoinSecret) != 64 {
		t.Fatalf("secret len=%d want 64 hex chars", len(st.JoinSecret))
	}
	if len(st.Nests) != 0 {
		t.Fatalf("nests should be empty, got %v", st.Nests)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatal(err)
	}
}

func TestJoinFromLinkAndAddNest(t *testing.T) {
	dir := t.TempDir()
	secret := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	nest := "wss://203.0.113.10/nest"
	link := FormatJoinLink(secret, nest)

	st, err := JoinFromLink(dir, link, "peer-cap")
	if err != nil {
		t.Fatal(err)
	}
	if st.JoinSecret != secret {
		t.Fatalf("secret mismatch")
	}
	if st.Mode != "captain" {
		t.Fatalf("mode=%q", st.Mode)
	}
	if len(st.Nests) == 0 || st.Nests[0] != nest {
		t.Fatalf("nests=%v", st.Nests)
	}

	st2, err := AddNestURL(dir, "wss://203.0.113.11/nest")
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.Nests) != 2 {
		t.Fatalf("nests=%v", st2.Nests)
	}
}

func TestJoinFromLinkWithInvite(t *testing.T) {
	dir := t.TempDir()
	secret := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	nest := "wss://203.0.113.10/nest"
	tok, err := fleetcrypto.IssueInviteFull(secret, fleetcrypto.Invite{
		Name: "office-pc", Nests: []string{nest},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		ModeHint: "worker", Tags: []string{"office"}, MaxVRAMMb: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	link := FormatJoinLinkFull(secret, tok, nest)
	st, err := JoinFromLink(dir, link, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "worker" {
		t.Fatalf("mode=%q want worker", st.Mode)
	}
	if st.Name != "office-pc" {
		t.Fatalf("name=%q", st.Name)
	}
	if len(st.Tags) != 1 || st.Tags[0] != "office" || st.MaxVRAMMb != 8192 {
		t.Fatalf("tags/vram: %+v max=%d", st.Tags, st.MaxVRAMMb)
	}
}

func TestSetHostNest(t *testing.T) {
	dir := t.TempDir()
	st, err := BecomeCaptain(BecomeCaptainOptions{ConfigDir: dir, Name: "c"})
	if err != nil {
		t.Fatal(err)
	}
	st, err = SetHostNest(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "worker" {
		t.Fatalf("mode=%q", st.Mode)
	}
	st, err = SetHostNest(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode != "captain" {
		t.Fatalf("mode=%q", st.Mode)
	}
}
