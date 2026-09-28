package agent

import "testing"

func TestJoinLinkRoundTrip(t *testing.T) {
	secret := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	nest := "wss://203.0.113.10/nest"
	link := FormatJoinLink(secret, nest)
	j, err := ParseJoinLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if j.Secret != secret {
		t.Fatalf("secret: got %q", j.Secret)
	}
	if j.Nest != nest && (len(j.Nests) == 0 || j.Nests[0] != nest) {
		t.Fatalf("nest: %+v", j)
	}

	blob := FormatJoinBlob(secret, nest)
	j2, err := ParseJoinLink(blob)
	if err != nil {
		t.Fatal(err)
	}
	if j2.Secret != secret {
		t.Fatalf("blob secret: got %q", j2.Secret)
	}

	raw := nest + "?secret=" + secret
	j3, err := ParseJoinLink(raw)
	if err != nil {
		t.Fatal(err)
	}
	if j3.Secret != secret || j3.Nest != nest {
		t.Fatalf("wss parse: %+v", j3)
	}
}

func TestJoinLinkWithInvite(t *testing.T) {
	secret := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	nest := "wss://203.0.113.10/nest"
	invite := "invitetoken"
	link := FormatJoinLinkFull(secret, invite, nest)
	j, err := ParseJoinLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if j.Invite != invite || j.Secret != secret {
		t.Fatalf("got %+v", j)
	}
}
