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

func TestOpsParity(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "but.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >>" + logPath + "\n" +
		"case \"$*\" in\n" +
		"  status*) cat <<'STATUS'\n" + sampleStatus + "\nSTATUS\n ;;\n" +
		"  undo*) echo 'Undid commit' ;;\n" +
		"  'branch list'*) echo '{\"appliedStacks\":[],\"branches\":[{\"name\":\"old\",\"hasLocal\":true,\"lastCommitAt\":1}]}' ;;\n" +
		"  'branch show'*) echo '{\"reviews\":[{\"url\":\"https://example.com/pr/3\"}]}' ;;\n" +
		"  'oplog list'*) echo '[{\"id\":\"abc1234def\",\"createdAt\":1,\"details\":{\"operation\":\"CreateCommit\",\"title\":\"CreateCommit\",\"body\":\"\"}}]' ;;\n" +
		"  *) exit 0 ;;\n" +
		"esac\n"
	srv := start(t, writeBut(t, script))

	cases := []struct {
		path, body, want, output string
	}{
		{"/ops/empty-commit", `{"placement":{"above":"c2"}}`, "commit --empty --no-message --above c2", ""},
		{"/ops/absorb", `{}`, "absorb", ""},
		{"/ops/absorb", `{"sources":["f1","f2"]}`, "absorb f2", ""},
		{"/ops/squash", `{"sources":["c3"],"target":"c2","mode":"target"}`, "squash --target c2 --use-target-message c3", ""},
		{"/ops/reword", `{"target":"c2","message":"better"}`, "reword c2 --message better", ""},
		{"/ops/discard", `{"targets":["f1"]}`, "discard f1", ""},
		{"/ops/branch-new", `{"name":"my feature","placement":{"below":"b2"}}`, "branch new --below b2 my-feature", ""},
		{"/ops/branch-delete", `{"branches":["auth"]}`, "branch delete auth", ""},
		{"/ops/pick", `{"sources":["c9"],"placement":{"branch":"api"}}`, "pick c9 --branch api", ""},
		{"/ops/apply", `{"branch":"old"}`, "apply old", ""},
		{"/ops/unapply", `{"branch":"auth"}`, "unapply auth", ""},
		{"/ops/push", `{"branch":"auth","force":true}`, "push auth --with-force", ""},
		{"/ops/pull", ``, "pull", ""},
		{"/ops/undo", `{}`, "undo", "Undid commit"},
		{"/ops/redo", `{}`, "redo", ""},
		{"/ops/pr-new", `{"branch":"auth","draft":true}`, "pr new auth --default --draft", ""},
		{"/ops/oplog-restore", `{"snapshot":"abc"}`, "oplog restore abc", ""},
		{"/ops/land", `{"branch":"api"}`, "land api --yes", ""},
		{"/ops/clean", `{}`, "clean", ""},
		{"/ops/resolve-start", `{"commit":"c2"}`, "resolve c2", ""},
		{"/ops/resolve-finish", `{}`, "resolve finish", ""},
		{"/ops/resolve-cancel", `{"force":true}`, "resolve cancel --force", ""},
		{"/exec", `{"line":"but undo 'with space'"}`, "undo with space", "Undid commit"},
	}
	for _, tc := range cases {
		res := post(t, srv, tc.path, srv.Token, tc.body, "")
		body := decode(t, res)
		if res.StatusCode != http.StatusOK || body["ok"] != true {
			t.Fatalf("%s: status %d body %#v", tc.path, res.StatusCode, body)
		}
		if tc.output != "" && body["output"] != tc.output {
			t.Fatalf("%s: output %#v", tc.path, body["output"])
		}
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(logged)
	for _, tc := range cases {
		if !strings.Contains(text, tc.want+"\n") {
			t.Fatalf("log missing %q in:\n%s", tc.want, text)
		}
	}
	if !strings.Contains(text, "status --json -f --refresh-prs") {
		t.Fatalf("push and pr new should sync PRs:\n%s", text)
	}

	res := get(t, srv, "/workspace?sync=1", srv.Token, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("synced workspace %d", res.StatusCode)
	}
	res = get(t, srv, "/branches", srv.Token, "")
	branches := decode(t, res)["branches"].(map[string]any)["branches"].([]any)
	if len(branches) != 1 || branches[0].(map[string]any)["name"] != "old" {
		t.Fatalf("branches %#v", branches)
	}
	res = get(t, srv, "/oplog", srv.Token, "")
	entries := decode(t, res)["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["id"] != "abc1234def" {
		t.Fatalf("oplog %#v", entries)
	}
	res = get(t, srv, "/review-url?branch=auth", srv.Token, "")
	if url := decode(t, res)["url"]; url != "https://example.com/pr/3" {
		t.Fatalf("review url %#v", url)
	}
}

func TestOpsParityValidation(t *testing.T) {
	srv := start(t, writeBut(t, "#!/bin/sh\necho '{}'\n"))
	cases := []struct{ path, body string }{
		{"/ops/squash", `{"target":"c2"}`},
		{"/ops/squash", `{"sources":["c3"],"mode":"weird"}`},
		{"/ops/reword", `{"target":"c2","message":"  "}`},
		{"/ops/apply", `{}`},
		{"/ops/push", `{}`},
		{"/ops/land", `{}`},
		{"/ops/resolve-start", `{}`},
		{"/exec", `{"line":"  "}`},
		{"/exec", `{"line":"commit -m 'oops"}`},
		{"/ops/undo", `not json`},
	}
	for _, tc := range cases {
		if res := post(t, srv, tc.path, srv.Token, tc.body, ""); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s %s: %d", tc.path, tc.body, res.StatusCode)
		}
	}
	if res := get(t, srv, "/review-url", srv.Token, ""); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("review-url without branch: %d", res.StatusCode)
	}
	for _, path := range []string{"/oplog", "/branches", "/review-url?branch=x"} {
		if res := get(t, srv, path, "", ""); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s unauth: %d", path, res.StatusCode)
		}
	}
	if res := post(t, srv, "/exec", "", `{"line":"status"}`, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("exec unauth: %d", res.StatusCode)
	}
}
