package auth

import "testing"

func TestPasswordAndAPIKey(t *testing.T) {
	hash, err := HashPassword("secret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "secret") {
		t.Fatal("expected password match")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("expected password mismatch")
	}
	raw, prefix, h, err := APIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 20 || prefix == "" || h == "" {
		t.Fatalf("bad api key parts: %q %q %q", raw, prefix, h)
	}
	if HashToken(raw) != h {
		t.Fatal("hash mismatch")
	}
}
