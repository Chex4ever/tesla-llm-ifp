package fleetcrypto

import (
	"testing"
	"time"
)

func TestInviteRoundTrip(t *testing.T) {
	secret := "test-secret"
	tok, err := IssueInvite(secret, "deck-1", []string{"wss://fleet.teslant.ru/nest"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseInvite(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if inv.Name != "deck-1" {
		t.Fatalf("name %q", inv.Name)
	}
	if _, err := ParseInvite("wrong", tok); err == nil {
		t.Fatal("expected bad secret to fail")
	}
}

func TestGossipMacStable(t *testing.T) {
	ts := time.Unix(1700000000, 0).UTC()
	a := GossipMac("s", "n1", "worker", ts, []string{"m1", "m2"}, []string{"office"})
	b := GossipMac("s", "n1", "worker", ts, []string{"m1", "m2"}, []string{"office"})
	if a != b || a == "" {
		t.Fatalf("mac mismatch %q %q", a, b)
	}
	c := GossipMac("s", "n1", "worker", ts, []string{"m1", "m2"}, []string{"other"})
	if a == c {
		t.Fatal("tags should change mac")
	}
}

func TestInviteWithTags(t *testing.T) {
	secret := "test-secret"
	tok, err := IssueInviteFull(secret, Invite{
		Name: "ship-1", Nests: []string{"wss://n/nest"},
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
		ModeHint: "worker", Tags: []string{"office", "gpu"}, MaxVRAMMb: 8192,
	})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseInvite(secret, tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Tags) != 2 || inv.Tags[0] != "office" || inv.MaxVRAMMb != 8192 {
		t.Fatalf("invite: %+v", inv)
	}
}
