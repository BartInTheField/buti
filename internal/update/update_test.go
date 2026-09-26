package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2026.09.26.2", "2026.09.26.1", true},
		{"2026.09.26.10", "2026.09.26.9", true},
		{"2026.10.01.1", "2026.09.30.5", true},
		{"2026.09.26.1", "2026.09.26.1", false},
		{"2026.09.26.1", "2026.09.26.2", false},
		{"2026.09.26.1", "dev", false},
		{"garbage", "2026.09.26.1", false},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// fakeRelease serves a GitHub-like release of version whose binary contains body.
func fakeRelease(t *testing.T, version string, body []byte, corrupt bool) {
	t.Helper()
	name := fmt.Sprintf("buti_%s_%s_%s", version, runtime.GOOS, runtime.GOARCH)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: name + "/buti", Mode: 0o755, Size: int64(len(body))})
	_, _ = tw.Write(body)
	tw.Close()
	gz.Close()
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	if corrupt {
		archive = append(archive, 0)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+version, http.StatusFound)
	})
	mux.HandleFunc("/releases/download/"+version+"/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "%s  %s.tar.gz\n", hex.EncodeToString(sum[:]), name)
	})
	mux.HandleFunc("/releases/download/"+version+"/"+name+".tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := BaseURL
	BaseURL = srv.URL
	t.Cleanup(func() { BaseURL = old })
}

func TestLatestCaches(t *testing.T) {
	fakeRelease(t, "2026.09.26.2", nil, false)
	cache := filepath.Join(t.TempDir(), "latest")
	v, err := Latest(context.Background(), cache)
	if err != nil || v != "2026.09.26.2" {
		t.Fatalf("Latest = %q, %v", v, err)
	}
	// A fresh cache answers without asking the server.
	BaseURL = "http://127.0.0.1:1"
	if v, err := Latest(context.Background(), cache); err != nil || v != "2026.09.26.2" {
		t.Fatalf("cached Latest = %q, %v", v, err)
	}
}

func TestInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on Windows")
	}
	fakeRelease(t, "2026.09.26.2", []byte("new binary"), false)
	exe := filepath.Join(t.TempDir(), "buti")
	if err := os.WriteFile(exe, []byte("old binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(context.Background(), "2026.09.26.2", exe); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new binary" {
		t.Fatalf("binary = %q", got)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %v", fi.Mode())
	}
}

func TestInstallRejectsBadChecksum(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("self-update is not supported on Windows")
	}
	fakeRelease(t, "2026.09.26.2", []byte("new binary"), true)
	exe := filepath.Join(t.TempDir(), "buti")
	_ = os.WriteFile(exe, []byte("old binary"), 0o755)
	if err := Install(context.Background(), "2026.09.26.2", exe); err == nil {
		t.Fatal("expected a checksum error")
	}
	if got, _ := os.ReadFile(exe); string(got) != "old binary" {
		t.Fatalf("binary was replaced: %q", got)
	}
}
