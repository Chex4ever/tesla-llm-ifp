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
	a := GossipMac("s", "n1", "worker", ts, []string{"m1", "m2"})
	b := GossipMac("s", "n1", "worker", ts, []string{"m1", "m2"})
	if a != b || a == "" {
		t.Fatalf("mac mismatch %q %q", a, b)
	}
}
