package desktop

import (
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

const commentsStatus = `{
  "uncommittedChanges": [{"cliId": "f1", "filePath": "README.md", "changeType": "modified"}],
  "stacks": [{"cliId": "s1", "branches": [{
    "cliId": "b1", "name": "api",
    "commits": [{"cliId": "c1", "changeId": "chg1", "commitId": "abcdef0123456", "message": "add api",
      "changes": [{"cliId": "c1:a", "filePath": "README.md", "changeType": "modified"}]}]
  }]}]
}`

// startInRepo serves a fake `but` on a fresh git repository, which the comment store needs.
func startInRepo(t *testing.T) (*Server, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	script := "#!/bin/sh\n" +
		"case \"$*\" in\n" +
		"  *diff*) cat <<'DIFF'\n" + sampleDiff + "\nDIFF\n ;;\n" +
		"  *) cat <<'STATUS'\n" + commentsStatus + "\nSTATUS\n ;;\n" +
		"esac\n"
	client := but.New(dir)
	client.Bin = writeBut(t, script)
	srv, err := Start(client, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })
	return srv, dir
}

func listed(t *testing.T, srv *Server) []any {
	t.Helper()
	res := get(t, srv, "/comments", srv.Token, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list %d %v", res.StatusCode, decode(t, res))
	}
	return decode(t, res)["comments"].([]any)
}

func TestCommentsLifecycle(t *testing.T) {
	srv, dir := startInRepo(t)

	if got := listed(t, srv); len(got) != 0 {
		t.Fatalf("want no comments, got %v", got)
	}

	res := post(t, srv, "/comments", srv.Token,
		`{"anchor":{"kind":"unassigned","path":"README.md","side":"new","line":2,"line_text":"b"},"body":"why b?"}`, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("add %d %v", res.StatusCode, decode(t, res))
	}
	id := decode(t, res)["comment"].(map[string]any)["id"].(string)

	// Same file as the TUI and `buti review`: <git dir>/buti/review.json.
	if _, err := os.Stat(filepath.Join(dir, ".git", "buti", "review.json")); err != nil {
		t.Fatalf("store file: %v", err)
	}

	cs := listed(t, srv)
	if len(cs) != 1 {
		t.Fatalf("comments %v", cs)
	}
	c := cs[0].(map[string]any)
	if c["id"] != id || c["state"] != "open" || c["target"] != "zz" || c["fileId"] != "f1" || c["body"] != "why b?" {
		t.Fatalf("located %v", c)
	}
	if at := c["at"].(map[string]any); at["line"] != float64(2) || at["path"] != "README.md" {
		t.Fatalf("at %v", at)
	}

	for _, step := range []struct{ path, body, state string }{
		{"/comments/edit", `{"id":"` + id + `","body":"why not c?"}`, "open"},
		{"/comments/reply", `{"id":"` + id + `","body":"done"}`, "open"},
		{"/comments/resolve", `{"id":"` + id + `"}`, "resolved"},
		{"/comments/reopen", `{"id":"` + id + `"}`, "open"},
	} {
		if res := post(t, srv, step.path, srv.Token, step.body, ""); res.StatusCode != http.StatusOK {
			t.Fatalf("%s %d %v", step.path, res.StatusCode, decode(t, res))
		}
		if got := listed(t, srv)[0].(map[string]any)["state"]; got != step.state {
			t.Fatalf("after %s state %v, want %s", step.path, got, step.state)
		}
	}
	c = listed(t, srv)[0].(map[string]any)
	if c["body"] != "why not c?" || len(c["replies"].([]any)) != 1 {
		t.Fatalf("edited %v", c)
	}

	if res := post(t, srv, "/comments/delete", srv.Token, `{"id":"`+id+`"}`, ""); res.StatusCode != http.StatusOK {
		t.Fatalf("delete %d", res.StatusCode)
	}
	if got := listed(t, srv); len(got) != 0 {
		t.Fatalf("after delete %v", got)
	}
	if res := post(t, srv, "/comments/delete", srv.Token, `{"id":"`+id+`"}`, ""); res.StatusCode != http.StatusNotFound {
		t.Fatalf("delete again %d", res.StatusCode)
	}
}

func TestCommentOnBranchMovesToCommit(t *testing.T) {
	srv, dir := startInRepo(t)
	res := post(t, srv, "/comments", srv.Token,
		`{"anchor":{"branch":"api","path":"README.md","side":"new","line":2,"line_text":"b"},"body":"on the branch"}`, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("add %d %v", res.StatusCode, decode(t, res))
	}
	cs, err := review.New(filepath.Join(dir, ".git", "buti", "review.json")).List()
	if err != nil || len(cs) != 1 {
		t.Fatalf("stored %v %v", cs, err)
	}
	if a := cs[0].Anchor; a.Kind != review.KindCommit || a.ChangeID != "chg1" || a.Branch != "api" {
		t.Fatalf("anchor %+v", a)
	}

	res = post(t, srv, "/comments", srv.Token,
		`{"anchor":{"branch":"api","path":"README.md","side":"new","line":9,"line_text":"nowhere"},"body":"x"}`, "")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("lines in no commit: %d", res.StatusCode)
	}
}

func TestCommentsValidationAndAuth(t *testing.T) {
	srv, _ := startInRepo(t)
	if res := get(t, srv, "/comments", "", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token %d", res.StatusCode)
	}
	if res := post(t, srv, "/comments/edit", "wrong", `{}`, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token %d", res.StatusCode)
	}
	for _, body := range []string{
		`{"anchor":{"kind":"unassigned","path":"README.md","line":1},"body":"  "}`,
		`{"anchor":{"kind":"unassigned","path":"","line":1},"body":"x"}`,
		`{"anchor":{"kind":"commit","path":"a","line":1},"body":"x"}`,
		`not json`,
	} {
		if res := post(t, srv, "/comments", srv.Token, body, ""); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: %d", body, res.StatusCode)
		}
	}
}

func TestHighlight(t *testing.T) {
	srv := start(t, writeBut(t, "#!/bin/sh\nexit 1\n"))
	res := post(t, srv, "/highlight", srv.Token,
		`{"files":[{"path":"main.go","texts":["package main\n\n// hi\nfunc f() {}",""]}]}`, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("highlight %d", res.StatusCode)
	}
	files := decode(t, res)["files"].([]any)
	texts := files[0].(map[string]any)["texts"].([]any)
	lines := texts[0].([]any)
	if len(lines) != 4 {
		t.Fatalf("lines %v", lines)
	}
	first := lines[0].([]any)[0].([]any)
	if first[0] != "k" || first[1] != "package" {
		t.Fatalf("first token %v", first)
	}
	if lines[2].([]any)[0].([]any)[0] != "m" {
		t.Fatalf("comment %v", lines[2])
	}
	if len(texts[1].([]any)) != 1 {
		t.Fatalf("empty text %v", texts[1])
	}
	if res := post(t, srv, "/highlight", "", `{}`, ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token %d", res.StatusCode)
	}
}
