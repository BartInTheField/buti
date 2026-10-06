package desktop

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
)

func post(t *testing.T, srv *Server, path, token, body, origin string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
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

const sampleDiff = `{"changes":[{"id":"f1","path":"README.md","status":"modified","diff":{"type":"patch","hunks":[{"oldStart":1,"oldLines":1,"newStart":1,"newLines":2,"diff":"@@ -1 +1,2 @@\n-a\n+a\n+b\n"}]}}]}`

func TestWorkspaceAndDiff(t *testing.T) {
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *diff*)\n" +
		"    cat <<'DIFF'\n" + sampleDiff + "\nDIFF\n" +
		"    ;;\n" +
		"  *)\n" +
		"    cat <<'STATUS'\n" + sampleStatus + "\nSTATUS\n" +
		"    ;;\n" +
		"esac\n"
	srv := start(t, writeBut(t, script))

	res := get(t, srv, "/workspace", srv.Token, "http://localhost:1420")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("workspace %d", res.StatusCode)
	}
	body := decode(t, res)
	ws := body["workspace"].(map[string]any)
	changes := ws["uncommittedChanges"].([]any)
	if len(changes) != 2 {
		t.Fatalf("changes %#v", changes)
	}
	first := changes[0].(map[string]any)
	if first["cliId"] != "f1" || first["filePath"] != "README.md" {
		t.Fatalf("first %#v", first)
	}
	stacks := ws["stacks"].([]any)
	branches := stacks[0].(map[string]any)["branches"].([]any)
	auth := branches[1].(map[string]any)
	commits := auth["commits"].([]any)
	if auth["name"] != "auth" || len(commits) != 2 {
		t.Fatalf("auth %#v", auth)
	}
	if commits[0].(map[string]any)["cliId"] != "c2" {
		t.Fatalf("commit %#v", commits[0])
	}

	res = get(t, srv, "/diff?id=f1", srv.Token, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("diff %d", res.StatusCode)
	}
	diffBody := decode(t, res)
	diff := diffBody["diff"].(map[string]any)
	files := diff["changes"].([]any)
	if len(files) != 1 || files[0].(map[string]any)["path"] != "README.md" {
		t.Fatalf("diff %#v", diff)
	}
}

func TestOpsCommitAmendMoveUncommit(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "but.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >>" + logPath + "\n" +
		"case \"$*\" in\n" +
		"  status*)\n" +
		"    cat <<'STATUS'\n" + sampleStatus + "\nSTATUS\n" +
		"    ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	srv := start(t, writeBut(t, script))

	cases := []struct {
		path string
		body string
		want string
	}{
		{"/ops/commit", `{"changes":["f1"],"message":"wip","placement":{"branch":"auth"}}`, "commit --message wip f1 --branch auth"},
		{"/ops/amend", `{"target":"c2","changes":["f1"]}`, "amend --target c2 f1"},
		{"/ops/move", `{"sources":["c3"],"placement":{"branch":"api"}}`, "move c3 --branch api"},
		{"/ops/uncommit", `{"sources":["c2"]}`, "uncommit c2"},
		{"/ops/move", `{"sources":["c2"],"placement":{"newBranch":true}}`, "move c2 --unstack"},
	}
	for _, tc := range cases {
		res := post(t, srv, tc.path, srv.Token, tc.body, "http://localhost:1420")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d body %#v", tc.path, res.StatusCode, decode(t, res))
		}
		if decode(t, res)["ok"] != true {
			t.Fatalf("%s: not ok", tc.path)
		}
	}

	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(logged)
	for _, tc := range cases {
		if !strings.Contains(text, tc.want) {
			t.Fatalf("log missing %q in:\n%s", tc.want, text)
		}
	}
}

func TestOpsValidation(t *testing.T) {
	bin := writeBut(t, "#!/bin/sh\necho '{}'\n")
	srv := start(t, bin)
	res := post(t, srv, "/ops/amend", srv.Token, `{"changes":["f1"]}`, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("amend without target: %d", res.StatusCode)
	}
	res = post(t, srv, "/ops/move", srv.Token, `{"placement":{"branch":"api"}}`, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("move without sources: %d", res.StatusCode)
	}
	res = post(t, srv, "/ops/commit", "", `{"message":"x"}`, "")
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth commit: %d", res.StatusCode)
	}
}

func TestWorkspaceFromNilSlices(t *testing.T) {
	got := workspaceFrom("/repo", &but.Status{})
	if got.UncommittedChanges == nil || got.Stacks == nil {
		t.Fatal("nil slices encode as null")
	}
	if got.Repo != "/repo" {
		t.Fatalf("%+v", got)
	}
}
