package desktop

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveLaunch(t *testing.T) {
	t.Setenv("BUTI_DESKTOP_APP", "")
	t.Setenv("BUTI_DESKTOP_DIR", t.TempDir())

	if _, _, err := resolveLaunch(Config{ServeOnly: true, Dev: true}); err == nil || !strings.Contains(err.Error(), "--serve") {
		t.Fatalf("both flags: %v", err)
	}
	mode, target, err := resolveLaunch(Config{ServeOnly: true})
	if err != nil || mode != "serve" || target != "" {
		t.Fatalf("serve: %s %s %v", mode, target, err)
	}
	if _, _, err := resolveLaunch(Config{Dev: true}); err == nil || !strings.Contains(err.Error(), "frontend") {
		t.Fatalf("dev without frontend: %v", err)
	}
	if _, _, err := resolveLaunch(Config{}); err == nil || !strings.Contains(err.Error(), "not built") {
		t.Fatalf("missing shell: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if _, _, err := resolveLaunch(Config{AppPath: missing}); err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("explicit app: %v", err)
	}
}

func TestFindBuiltApp(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "src-tauri", "target", "debug", appBinaryName())
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findBuiltApp(dir); got != bin {
		t.Fatalf("found %q, want %q", got, bin)
	}
	if got := findBuiltApp(filepath.Join(dir, "empty")); got != "" {
		t.Fatalf("unexpected %q", got)
	}
}

func TestRunServeStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{
			Dir:       t.TempDir(),
			ServeOnly: true,
			Stderr:    ioDiscard(t),
			OnReady: func(url, token string) {
				res, err := http.Get(url + "/health")
				if err != nil {
					t.Errorf("health: %v", err)
					return
				}
				res.Body.Close()
				if res.StatusCode != http.StatusOK {
					t.Errorf("health %d", res.StatusCode)
				}
				if token == "" {
					t.Error("empty token")
				}
				ready <- url
			},
		})
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("exited before ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not become ready")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop")
	}
}

func TestRunPassesAPIToChild(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "buti-desktop")
	script := `#!/bin/sh
python3 - <<'PY'
import os, urllib.request
url = os.environ["BUTI_API_URL"] + "/health"
token = os.environ["BUTI_API_TOKEN"]
assert token
urllib.request.urlopen(url).read()
PY
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	err := Run(context.Background(), Config{
		Dir:     t.TempDir(),
		AppPath: bin,
		Stdout:  ioDiscard(t),
		Stderr:  ioDiscard(t),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func ioDiscard(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}
