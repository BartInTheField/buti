package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestOpenRepo(t *testing.T) {
	// The fake `but` fails in a folder holding a "not-gitbutler" file, like `but status` outside a workspace.
	bin := writeBut(t, "#!/bin/sh\nif [ -e not-gitbutler ]; then echo 'Error: Setup required' >&2; exit 1; fi\ncat <<'EOF'\n"+sampleStatus+"\nEOF\n")
	srv := start(t, bin)
	first := srv.but().Dir

	if res := get(t, srv, "/repo", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("repo without token: %d", res.StatusCode)
	}
	if body := decode(t, get(t, srv, "/repo", srv.Token, "")); body["dir"] != first || body["chosen"] != false {
		t.Fatalf("repo %v, want %s", body, first)
	}
	if res := post(t, srv, "/repo", "", `{"dir":"/"}`, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("open repo without token: %d", res.StatusCode)
	}

	for _, body := range []string{`{"dir":""}`, `{"dir":"relative/path"}`, `{"dir":"/does/not/exist"}`} {
		if res := post(t, srv, "/repo", srv.Token, body, ""); res.StatusCode != http.StatusBadRequest {
			t.Errorf("open repo %s: %d", body, res.StatusCode)
		}
	}

	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "not-gitbutler"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	res := post(t, srv, "/repo", srv.Token, `{"dir":"`+bad+`"}`, "")
	if body := decode(t, res); res.StatusCode != http.StatusBadGateway || body["error"].(map[string]any)["message"] != "Setup required" {
		t.Fatalf("open non-GitButler folder: %d %v", res.StatusCode, body)
	}
	if srv.but().Dir != first {
		t.Fatalf("a failed open switched to %s", srv.but().Dir)
	}

	next := t.TempDir()
	if res := post(t, srv, "/repo", srv.Token, `{"dir":"`+next+`"}`, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("open repo: %d %v", res.StatusCode, decode(t, res))
	}
	if srv.but().Dir != next || srv.but().Bin != bin || !srv.chosen.Load() {
		t.Fatalf("client %+v, want dir %s bin %s", srv.but(), next, bin)
	}
	if ws := decode(t, get(t, srv, "/workspace", srv.Token, ""))["workspace"].(map[string]any); ws["repo"] != next {
		t.Fatalf("workspace repo %v, want %s", ws["repo"], next)
	}
}
