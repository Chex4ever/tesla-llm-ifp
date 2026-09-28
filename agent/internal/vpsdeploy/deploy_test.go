package vpsdeploy

import (
	"bytes"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	if got := ShellQuote("abc"); got != "'abc'" {
		t.Fatalf("got %q", got)
	}
	if got := ShellQuote("a'b"); !strings.Contains(got, `'\''`) {
		t.Fatalf("got %q", got)
	}
}

func TestValidateOptions(t *testing.T) {
	if err := ValidateOptions(Options{}); err == nil {
		t.Fatal("expected host error")
	}
	if err := ValidateOptions(Options{Host: "1.2.3.4"}); err == nil {
		t.Fatal("expected password error")
	}
	if err := ValidateOptions(Options{Host: "1.2.3.4", Password: "x"}); err != nil {
		t.Fatal(err)
	}
}

func TestEmbedTemplatesPresent(t *testing.T) {
	for _, name := range []string{
		"templates/Caddyfile",
		"templates/compose.yml",
		"templates/Dockerfile",
		"templates/bootstrap.sh",
		"templates/gen-certs.sh",
	} {
		b := mustRead(name)
		if len(b) == 0 {
			t.Fatalf("%s empty", name)
		}
		if bytes.Contains(b, []byte{'\r'}) {
			t.Fatalf("%s contains CR (Windows line endings break remote bash)", name)
		}
	}
	caddy := string(mustRead("templates/Caddyfile"))
	if !strings.Contains(caddy, "auto_https off") {
		t.Fatal("Caddyfile must disable auto_https (no ACME)")
	}
	if !strings.Contains(caddy, "cert.pem") {
		t.Fatal("Caddyfile must use explicit cert.pem")
	}
	if strings.Contains(caddy, "tls internal") {
		t.Fatal("tls internal is unreliable for raw IPs; use openssl certs")
	}
	if strings.Contains(caddy, "ACME") || strings.Contains(strings.ToLower(caddy), "letsencrypt") {
		t.Fatal("Caddyfile must not reference ACME/Let's Encrypt")
	}
	boot := string(mustRead("templates/bootstrap.sh"))
	if !strings.Contains(boot, "systemctl start docker") {
		t.Fatal("bootstrap must start docker daemon")
	}
	if !strings.Contains(boot, "docker info") {
		t.Fatal("bootstrap must wait for docker info")
	}
	gen := string(mustRead("templates/gen-certs.sh"))
	if !strings.Contains(gen, "subjectAltName") {
		t.Fatal("gen-certs must set SAN")
	}
}

func TestStripCR(t *testing.T) {
	in := []byte("set -euo pipefail\r\necho hi\r\n")
	out := stripCR(in)
	if bytes.Contains(out, []byte{'\r'}) {
		t.Fatalf("still has CR: %q", out)
	}
	if string(out) != "set -euo pipefail\necho hi\n" {
		t.Fatalf("got %q", out)
	}
}
