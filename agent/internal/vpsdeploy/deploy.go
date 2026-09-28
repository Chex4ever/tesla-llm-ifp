package vpsdeploy

import (
	"bytes"
	"context"
	"crypto/tls"
	"embed"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

//go:embed templates/*
var templatesFS embed.FS

const remoteDir = "/opt/pirate-crowsnest"

// Options for remote Crow's Nest provisioning.
type Options struct {
	Host     string
	Port     int
	User     string
	Password string
	// NestHost is the public IP/hostname used in Caddy + wss URL (defaults to Host).
	NestHost string
	// OnLog receives progress lines (optional).
	OnLog func(string)
	// BinaryPath optional pre-built Linux pirate binary; if empty, tries to build.
	BinaryPath string
	// ModuleRoot for `go build` (auto-detected if empty).
	ModuleRoot string
}

// Result of a successful deploy.
type Result struct {
	NestURL string // wss://host/nest
	Host    string
}

func (o Options) logf(format string, args ...any) {
	if o.OnLog != nil {
		o.OnLog(fmt.Sprintf(format, args...))
	}
}

// Deploy SSHs to the VPS, installs Docker if needed, uploads nest stack, brings it up.
func Deploy(ctx context.Context, opt Options) (*Result, error) {
	if err := ValidateOptions(opt); err != nil {
		return nil, err
	}
	if opt.User == "" {
		opt.User = "root"
	}
	if opt.Port == 0 {
		opt.Port = 22
	}
	if opt.NestHost == "" {
		opt.NestHost = opt.Host
	}
	opt.NestHost = strings.TrimSpace(opt.NestHost)

	opt.logf("building Linux pirate binary…")
	binPath, cleanup, err := ensureLinuxBinary(ctx, opt)
	if err != nil {
		return nil, err
	}
	if cleanup != nil {
		defer cleanup()
	}

	opt.logf("connecting %s@%s:%d…", opt.User, opt.Host, opt.Port)
	client, err := dialSSH(opt)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	opt.logf("ensuring Docker…")
	boot := string(mustRead("templates/bootstrap.sh"))
	if err := runRemote(client, "bash -s", boot); err != nil {
		return nil, fmt.Errorf("bootstrap docker: %w", err)
	}

	opt.logf("uploading nest stack to %s…", remoteDir)
	if err := runRemote(client, fmt.Sprintf("mkdir -p %s", remoteDir), ""); err != nil {
		return nil, err
	}
	files := map[string][]byte{
		"Caddyfile":    mustRead("templates/Caddyfile"),
		"compose.yml":  mustRead("templates/compose.yml"),
		"Dockerfile":   mustRead("templates/Dockerfile"),
		"gen-certs.sh": mustRead("templates/gen-certs.sh"),
		".env":         []byte(fmt.Sprintf("NEST_HOST=%s\n", opt.NestHost)),
	}
	binData, err := os.ReadFile(binPath)
	if err != nil {
		return nil, err
	}
	files["pirate"] = binData

	for name, data := range files {
		remote := remoteDir + "/" + name
		mode := os.FileMode(0o644)
		if name == "pirate" || name == "gen-certs.sh" {
			mode = 0o755
		}
		if err := uploadBytes(client, remote, data, mode); err != nil {
			return nil, fmt.Errorf("upload %s: %w", name, err)
		}
	}

	opt.logf("generating self-signed TLS cert for %s…", opt.NestHost)
	gen := fmt.Sprintf("cd %s && bash ./gen-certs.sh %s ./certs", remoteDir, shellQuote(opt.NestHost))
	if err := runRemote(client, gen, ""); err != nil {
		return nil, fmt.Errorf("gen certs: %w", err)
	}

	opt.logf("docker compose up…")
	up := fmt.Sprintf("cd %s && docker compose up -d --build --force-recreate", remoteDir)
	if err := runRemote(client, up, ""); err != nil {
		return nil, fmt.Errorf("compose up: %w", err)
	}

	nestURL := fmt.Sprintf("wss://%s/nest", opt.NestHost)
	opt.logf("waiting for https://%s/healthz…", opt.NestHost)
	if err := waitHealth(ctx, opt.NestHost); err != nil {
		logs, _ := runRemoteOutput(client, fmt.Sprintf("cd %s && docker compose logs --tail=30 caddy 2>&1", remoteDir), "")
		if logs != "" {
			opt.logf("caddy logs:\n%s", logs)
		}
		return nil, fmt.Errorf("health check: %w (nest may still be starting; URL would be %s)", err, nestURL)
	}
	opt.logf("nest ready: %s", nestURL)
	return &Result{NestURL: nestURL, Host: opt.NestHost}, nil
}

func mustRead(name string) []byte {
	b, err := templatesFS.ReadFile(name)
	if err != nil {
		panic(err)
	}
	return stripCR(b)
}

// stripCR removes Windows CR so remote bash never sees pipefail\r etc.
func stripCR(b []byte) []byte {
	if !bytes.Contains(b, []byte{'\r'}) {
		return b
	}
	return bytes.ReplaceAll(b, []byte{'\r'}, nil)
}

func dialSSH(opt Options) (*ssh.Client, error) {
	cfg := &ssh.ClientConfig{
		User:            opt.User,
		Auth:            []ssh.AuthMethod{ssh.Password(opt.Password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // first-run VPS bootstrap
		Timeout:         20 * time.Second,
	}
	addr := net.JoinHostPort(opt.Host, fmt.Sprint(opt.Port))
	return ssh.Dial("tcp", addr, cfg)
}

func runRemote(client *ssh.Client, cmdline string, stdin string) error {
	_, err := runRemoteOutput(client, cmdline, stdin)
	return err
}

func runRemoteOutput(client *ssh.Client, cmdline string, stdin string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr
	if stdin != "" {
		sess.Stdin = strings.NewReader(stdin)
	}
	if err := sess.Run(cmdline); err != nil {
		msg := strings.TrimSpace(stderr.String() + "\n" + stdout.String())
		if msg == "" {
			msg = err.Error()
		}
		return stdout.String() + stderr.String(), fmt.Errorf("%s: %s", cmdline, msg)
	}
	return stdout.String() + stderr.String(), nil
}

func uploadBytes(client *ssh.Client, remote string, data []byte, mode os.FileMode) error {
	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	var stderr bytes.Buffer
	sess.Stderr = &stderr
	stdin, err := sess.StdinPipe()
	if err != nil {
		return err
	}
	if err := sess.Start(fmt.Sprintf("cat > %s && chmod %o %s", shellQuote(remote), mode.Perm(), shellQuote(remote))); err != nil {
		return err
	}
	if _, err := io.Copy(stdin, bytes.NewReader(data)); err != nil {
		_ = stdin.Close()
		return err
	}
	_ = stdin.Close()
	if err := sess.Wait(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return fmt.Errorf("%s", msg)
		}
		return err
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ShellQuote quotes s for a remote POSIX shell (exported for tests).
func ShellQuote(s string) string { return shellQuote(s) }

// ValidateOptions checks required deploy fields.
func ValidateOptions(opt Options) error {
	if strings.TrimSpace(opt.Host) == "" {
		return fmt.Errorf("host required")
	}
	if opt.Password == "" {
		return fmt.Errorf("password required")
	}
	return nil
}

func waitHealth(ctx context.Context, host string) error {
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // self-signed nest
		},
	}
	url := "https://" + host + "/healthz"
	deadline := time.Now().Add(3 * time.Minute)
	var last error
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 500 {
				return nil
			}
			last = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			last = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if last == nil {
		last = fmt.Errorf("timeout")
	}
	return last
}

func ensureLinuxBinary(ctx context.Context, opt Options) (path string, cleanup func(), err error) {
	if opt.BinaryPath != "" {
		if _, err := os.Stat(opt.BinaryPath); err != nil {
			return "", nil, fmt.Errorf("binary: %w", err)
		}
		return opt.BinaryPath, nil, nil
	}
	candidates := []string{
		"bin/pirate-linux-amd64",
		"bin/pirate-linux-arm64",
	}
	root := opt.ModuleRoot
	if root == "" {
		root = findModuleRoot()
	}
	for _, c := range candidates {
		p := c
		if root != "" {
			p = filepath.Join(root, c)
		}
		if st, err := os.Stat(p); err == nil && st.Size() > 0 {
			return p, nil, nil
		}
	}

	if root == "" {
		return "", nil, fmt.Errorf("cannot find module root to build Linux binary; place bin/pirate-linux-amd64 or pass BinaryPath")
	}
	tmp, err := os.CreateTemp("", "pirate-linux-*")
	if err != nil {
		return "", nil, err
	}
	outPath := tmp.Name()
	_ = tmp.Close()
	if runtime.GOOS == "windows" {
		_ = os.Remove(outPath)
		outPath += ".bin"
	}

	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", outPath, "./agent/cmd/pirate")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		_ = os.Remove(outPath)
		return "", nil, fmt.Errorf("go build linux: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}
	return outPath, func() { _ = os.Remove(outPath) }, nil
}

func findModuleRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir = filepath.Dir(exe)
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	return ""
}
