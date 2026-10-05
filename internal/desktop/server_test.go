package desktop

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
)

const sampleStatus = `{
  "uncommittedChanges": [
    {"cliId": "f1", "filePath": "README.md", "changeType": "modified"},
    {"cliId": "f2", "filePath": "main.go", "changeType": "added"}
  ],
  "stacks": [
    {
      "cliId": "s1",
      "branches": [
        {
          "cliId": "b1",
          "name": "api",
          "commits": [{"cliId": "c1", "message": "add api"}],
          "upstreamCommits": [],
          "branchStatus": "integrated",
          "reviewId": ""
        },
        {
          "cliId": "b2",
          "name": "auth",
          "commits": [
            {"cliId": "c2", "message": "add auth"},
            {"cliId": "c3", "message": "fix auth"}
          ],
          "upstreamCommits": [{"cliId": "c4", "message": "remote"}],
          "branchStatus": "active",
          "reviewId": "(#3)"
        }
      ]
    }
  ],
  "upstreamState": {"behind": 2}
}`

func writeBut(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "but")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func start(t *testing.T, bin string) *Server {
	t.Helper()
	client := but.New(t.TempDir())
	client.Bin = bin
	srv, err := Start(client, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := srv.Shutdown(context.Background()); err != nil {
			t.Error(err)
		}
	})
	return srv
}

func get(t *testing.T, srv *Server, path, token, origin string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func decode(t *testing.T, res *http.Response) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestServerLoopbackAndStatus(t *testing.T) {
	bin := writeBut(t, "#!/bin/sh\ncat <<'EOF'\n"+sampleStatus+"\nEOF\n")
	srv := start(t, bin)

	host, port, err := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() || port == "" || port == "0" {
		t.Fatalf("url %s is not a loopback endpoint", srv.URL)
	}

	res := get(t, srv, "/status", srv.Token, "http://localhost:1420")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "http://localhost:1420" {
		t.Fatalf("cors origin %q", got)
	}
	body := decode(t, res)
	if body["ok"] != true {
		t.Fatalf("body %#v", body)
	}
	summary := body["summary"].(map[string]any)
	if summary["uncommitted"].(float64) != 2 || summary["upstreamBehind"].(float64) != 2 {
		t.Fatalf("summary %#v", summary)
	}
	stacks := summary["stacks"].([]any)
	branches := stacks[0].(map[string]any)["branches"].([]any)
	auth := branches[1].(map[string]any)
	if auth["name"] != "auth" || auth["commits"].(float64) != 2 || auth["pr"] != "#3" || auth["upstream"].(float64) != 1 {
		t.Fatalf("auth branch %#v", auth)
	}
}

func TestServerMissingBut(t *testing.T) {
	cases := []struct{ name, bin string }{
		{"missing path", filepath.Join(t.TempDir(), "no-such-but")},
		{"not on PATH", "but-not-on-path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := start(t, tc.bin)
			res := get(t, srv, "/status", srv.Token, "")
			if res.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status %d", res.StatusCode)
			}
			body := decode(t, res)
			errObj := body["error"].(map[string]any)
			if errObj["code"] != "but_missing" || !strings.Contains(errObj["message"].(string), "`but`") {
				t.Fatalf("error %#v", errObj)
			}
			if errObj["docsUrl"] != gitButlerCLIDocs {
				t.Fatalf("docs %v", errObj["docsUrl"])
			}
		})
	}
}

func TestServerButFailed(t *testing.T) {
	bin := writeBut(t, "#!/bin/sh\necho 'not a gitbutler repository' >&2\nexit 1\n")
	srv := start(t, bin)
	res := get(t, srv, "/status", srv.Token, "")
	if res.StatusCode != http.StatusBadGateway {
		t.Fatalf("status %d", res.StatusCode)
	}
	body := decode(t, res)
	errObj := body["error"].(map[string]any)
	if errObj["code"] != "but_failed" || errObj["message"] != "not a gitbutler repository" {
		t.Fatalf("error %#v", errObj)
	}
}

func TestServerAuthAndHost(t *testing.T) {
	bin := writeBut(t, "#!/bin/sh\necho '{}'\n")
	srv := start(t, bin)

	res := get(t, srv, "/status", "", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token: %d", res.StatusCode)
	}
	res = get(t, srv, "/status", "nope", "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", res.StatusCode)
	}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "evil.example"
	req.Header.Set("Authorization", "Bearer "+srv.Token)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("rebinding host: %d", res.StatusCode)
	}

	rec := httptest.NewRecorder()
	remote := httptest.NewRequest(http.MethodGet, "/health", nil)
	remote.RemoteAddr = "203.0.113.4:40000"
	remote.Host = srv.host
	srv.wrap(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("non-loopback request reached the handler")
	})).ServeHTTP(rec, remote)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("remote: %d", rec.Code)
	}

	// Health is unauthenticated and still loopback-bound. It must not echo the token.
	res = get(t, srv, "/health", "", "https://evil.example")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("disallowed origin was reflected")
	}
	raw := decode(t, res)
	if _, ok := raw["token"]; ok {
		t.Fatal("health leaked a token")
	}
}

func TestCORSPreflight(t *testing.T) {
	bin := writeBut(t, "#!/bin/sh\necho '{}'\n")
	srv := start(t, bin)
	req, err := http.NewRequest(http.MethodOptions, srv.URL+"/status", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Origin", "https://tauri.localhost")
	req.Header.Set("Access-Control-Request-Method", "GET")
	req.Header.Set("Access-Control-Request-Headers", "authorization")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight %d", res.StatusCode)
	}
	if res.Header.Get("Access-Control-Allow-Origin") != "https://tauri.localhost" {
		t.Fatalf("origin %q", res.Header.Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(strings.ToLower(res.Header.Get("Access-Control-Allow-Headers")), "authorization") {
		t.Fatalf("headers %q", res.Header.Get("Access-Control-Allow-Headers"))
	}
}

func TestSummarizeResolving(t *testing.T) {
	st := &but.Status{
		Resolving: &but.Resolution{Conflicted: []string{"a.txt", "b.txt"}, Resolved: []string{"c.txt"}},
	}
	got := summarize("/repo", st)
	if got.Repo != "/repo" || got.Resolving == nil || got.Resolving.Conflicted != 2 || got.Resolving.Resolved != 1 || got.Resolving.AllResolved {
		t.Fatalf("%+v", got)
	}
	if got.UncommittedPaths == nil || got.Stacks == nil {
		t.Fatal("nil slices encode as null")
	}
}
