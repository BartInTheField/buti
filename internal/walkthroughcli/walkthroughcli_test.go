package walkthroughcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/walkthrough"
)

const body = "## Summary\n\nAdds token auth.\n\n```mermaid\nsequenceDiagram\n    Client->>API: POST /login\n    API-->>Client: token\n```\n"

// fakeStatus has a stack with auth (cli id au) on api, and an empty branch.
func fakeStatus() *but.Status {
	return &but.Status{Stacks: []but.Stack{
		{Branches: []but.Branch{
			{CliID: "au", Name: "auth", Commits: []but.Commit{{CommitID: "a2", ChangeID: "ca2"}, {CommitID: "a1", ChangeID: "ca1"}}},
			{CliID: "ap", Name: "api", Commits: []but.Commit{{CommitID: "p1", ChangeID: "cp1"}}},
		}},
		{Branches: []but.Branch{{CliID: "em", Name: "empty"}}},
	}}
}

type fakeBut struct {
	st    *but.Status
	calls int
}

func (f *fakeBut) Status(context.Context) (*but.Status, error) {
	f.calls++
	return f.st, nil
}

type harness struct {
	t     *testing.T
	store *walkthrough.Store
	but   *fakeBut
	stdin string
}

func newHarness(t *testing.T) *harness {
	return &harness{t: t, store: walkthrough.New(filepath.Join(t.TempDir(), "walkthroughs.json")), but: &fakeBut{st: fakeStatus()}}
}

func (h *harness) run(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Run(context.Background(), args, Env{
		Stdout: &out, Stderr: &errOut, Stdin: strings.NewReader(h.stdin),
		Store: func() (*walkthrough.Store, error) { return h.store, nil },
		But:   h.but,
	})
	return code, out.String(), errOut.String()
}

func TestSetFromFlagFileAndStdin(t *testing.T) {
	h := newHarness(t)
	if code, out, errOut := h.run("set", "auth", "--body", body, "--title", " Token auth "); code != 0 || out != "set the walkthrough of auth\n" {
		t.Fatalf("set --body: exit %d, %q %q", code, out, errOut)
	}
	w, err := h.store.Get("auth")
	if err != nil {
		t.Fatal(err)
	}
	if w.Body != body || w.Title != "Token auth" || w.Author != walkthrough.AuthorAgent || w.Head != "a2" || w.ChangeID != "ca2" {
		t.Errorf("stored %+v", w)
	}

	file := filepath.Join(t.TempDir(), "w.md")
	if err := os.WriteFile(file, []byte("from a file"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Flags after the branch, a cli id for the branch and an explicit author.
	if code, _, errOut := h.run("set", "ap", "--body-file", file, "--author", "user"); code != 0 {
		t.Fatalf("set --body-file: exit %d, %q", code, errOut)
	}
	if w, _ := h.store.Get("api"); w.Body != "from a file" || w.Author != "user" || w.Head != "p1" {
		t.Errorf("stored %+v", w)
	}

	h.stdin = "from stdin"
	if code, _, errOut := h.run("set", "empty"); code != 0 {
		t.Fatalf("set from stdin: exit %d, %q", code, errOut)
	}
	if w, _ := h.store.Get("empty"); w.Body != "from stdin" || w.Head != "" {
		t.Errorf("stored %+v", w)
	}
	h.stdin = "replaced"
	if code, _, _ := h.run("set", "empty", "--body-file", "-"); code != 0 {
		t.Fatal("set --body-file -")
	}
	if w, _ := h.store.Get("empty"); w.Body != "replaced" {
		t.Errorf("set did not replace: %+v", w)
	}
}

func TestSetErrors(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"set"}, 2, "expected one branch"},
		{[]string{"set", "auth", "--body", "x", "--body-file", "f"}, 2, "not both"},
		{[]string{"set", "auth", "--body", " "}, 1, "empty walkthrough"},
		{[]string{"set", "nope", "--body", "x"}, 1, "no branch nope in the workspace"},
		{[]string{"set", "auth", "--body-file", "/does/not/exist"}, 1, "no such file"},
	} {
		code, _, errOut := h.run(tc.args...)
		if code != tc.code || !strings.Contains(errOut, tc.want) {
			t.Errorf("%v: exit %d, %q; want %d and %q", tc.args, code, errOut, tc.code, tc.want)
		}
	}
	if ws, _ := h.store.List(); len(ws) != 0 {
		t.Errorf("a failed set stored %+v", ws)
	}
}

func TestShowListClear(t *testing.T) {
	h := newHarness(t)
	if code, out, _ := h.run("list"); code != 0 || out != "no walkthroughs\n" {
		t.Errorf("empty list: %d %q", code, out)
	}
	if code, out, _ := h.run("list", "--json"); code != 0 || strings.TrimSpace(out) != "[]" {
		t.Errorf("empty list --json: %d %q", code, out)
	}
	h.run("set", "auth", "--body", body, "--title", "Token auth")
	h.run("set", "api", "--body", "no newline")

	if code, out, _ := h.run("show", "auth"); code != 0 || out != body {
		t.Errorf("show: %d %q", code, out)
	}
	if _, out, _ := h.run("show", "api"); out != "no newline\n" {
		t.Errorf("show adds a final newline: %q", out)
	}
	code, out, _ := h.run("show", "auth", "--json")
	var shown map[string]any
	if err := json.Unmarshal([]byte(out), &shown); code != 0 || err != nil {
		t.Fatalf("show --json: %d %v %q", code, err, out)
	}
	for _, k := range []string{"branch", "title", "author", "updated_at", "head", "body"} {
		if _, ok := shown[k]; !ok {
			t.Errorf("show --json lacks %s: %s", k, out)
		}
	}
	if shown["body"] != body || shown["head"] != "a2" {
		t.Errorf("show --json: %s", out)
	}
	// A Mermaid arrow must come out as written, not as \u003e.
	if !strings.Contains(out, "Client->>API") {
		t.Errorf("show --json escapes the body: %s", out)
	}

	_, out, _ = h.run("list")
	if lines := strings.Split(strings.TrimSpace(out), "\n"); len(lines) != 2 ||
		!strings.HasPrefix(lines[0], "api ") || !strings.Contains(lines[0], " - ") ||
		!strings.HasPrefix(lines[1], "auth ") || !strings.Contains(lines[1], "Token auth") {
		t.Errorf("list:\n%s", out)
	}
	_, out, _ = h.run("list", "--json")
	var listed []map[string]any
	if err := json.Unmarshal([]byte(out), &listed); err != nil || len(listed) != 2 {
		t.Fatalf("list --json: %v %s", err, out)
	}
	if _, ok := listed[0]["body"]; ok {
		t.Errorf("list --json includes bodies: %s", out)
	}

	if code, out, _ := h.run("clear", "auth"); code != 0 || out != "cleared the walkthrough of auth\n" {
		t.Errorf("clear: %d %q", code, out)
	}
	if code, _, errOut := h.run("show", "auth"); code != 1 || !strings.Contains(errOut, "no walkthrough for branch auth") {
		t.Errorf("show after clear: %d %q", code, errOut)
	}
	if code, _, _ := h.run("clear", "auth"); code != 1 {
		t.Errorf("second clear: exit %d", code)
	}
	if h.but.calls != 2 {
		t.Errorf("but status ran %d times, want only for the two sets", h.but.calls)
	}
}

func TestStatus(t *testing.T) {
	h := newHarness(t)
	h.run("set", "auth", "--body", body)
	h.run("set", "api", "--body", body)
	h.run("set", "empty", "--body", body)

	status := func(branch string) (string, jsonStatus) {
		t.Helper()
		_, text, _ := h.run("status", branch)
		code, out, errOut := h.run("status", branch, "--json")
		var s jsonStatus
		if err := json.Unmarshal([]byte(out), &s); code != 0 || err != nil {
			t.Fatalf("status --json %s: %d %v %q %q", branch, code, err, out, errOut)
		}
		return text, s
	}

	if text, s := status("auth"); !s.Exists || !s.InWorkspace || s.Stale || s.RenamedTo != nil || !strings.Contains(text, "up to date") {
		t.Errorf("current: %q %+v", text, s)
	}
	if text, s := status("missing"); s.Exists || s.Title != nil || !strings.Contains(text, "missing has no walkthrough") {
		t.Errorf("missing: %q %+v", text, s)
	}

	// auth gets a new commit, api is renamed to http, and empty is unapplied.
	st := fakeStatus()
	b := &st.Stacks[0].Branches
	(*b)[0].Commits = append([]but.Commit{{CommitID: "a3", ChangeID: "ca3"}}, (*b)[0].Commits...)
	(*b)[1].Name = "http"
	st.Stacks = st.Stacks[:1]
	h.but.st = st

	// The first status after the rename moves the walkthrough to the new name and says so; from then on it is http's.
	_, out, _ := h.run("status", "api", "--json")
	var renamed jsonStatus
	if err := json.Unmarshal([]byte(out), &renamed); err != nil || renamed.RenamedTo == nil || *renamed.RenamedTo != "http" ||
		!renamed.Exists || renamed.Stale {
		t.Errorf("renamed: %s", out)
	}
	if text, s := status("http"); !s.Exists || s.RenamedTo != nil || s.Stale || !strings.Contains(text, "up to date") {
		t.Errorf("after the rename: %q %+v", text, s)
	}
	if _, s := status("api"); s.Exists {
		t.Errorf("the old name still has one: %+v", s)
	}
	if text, s := status("auth"); !s.Stale || !strings.Contains(text, "outdated") {
		t.Errorf("moved on: %q %+v", text, s)
	}
	if text, s := status("empty"); s.InWorkspace || !s.Exists || !s.Stale || !strings.Contains(text, "not in the workspace") {
		t.Errorf("unapplied: %q %+v", text, s)
	}
}

// TestSetAfterRename replaces the walkthrough of a renamed branch under its new name, rather than adding a second.
func TestSetAfterRename(t *testing.T) {
	h := newHarness(t)
	h.run("set", "auth", "--body", "old")
	st := fakeStatus()
	st.Stacks[0].Branches[0].Name = "login"
	h.but.st = st
	if code, _, errOut := h.run("set", "login", "--body", "new"); code != 0 {
		t.Fatalf("set: %d %q", code, errOut)
	}
	ws, _ := h.store.List()
	if len(ws) != 1 || ws[0].Branch != "login" || ws[0].Body != "new" {
		t.Errorf("stored %+v", ws)
	}
}

func TestUsage(t *testing.T) {
	h := newHarness(t)
	if code, _, errOut := h.run(); code != 2 || !strings.Contains(errOut, "buti walkthrough set <branch>") {
		t.Errorf("no args: %d %q", code, errOut)
	}
	if code, out, _ := h.run("help"); code != 0 || !strings.Contains(out, "buti walkthrough status") {
		t.Errorf("help: %d %q", code, out)
	}
	if code, _, errOut := h.run("frob"); code != 2 || !strings.Contains(errOut, `unknown command "frob"`) {
		t.Errorf("unknown: %d %q", code, errOut)
	}
	if code, _, _ := h.run("show", "a", "b"); code != 2 {
		t.Errorf("two branches: exit %d", code)
	}
	if !NeedsBut([]string{"set"}) || !NeedsBut([]string{"status"}) || NeedsBut([]string{"show"}) || NeedsBut(nil) {
		t.Error("NeedsBut")
	}
}

func TestStoreError(t *testing.T) {
	var errOut bytes.Buffer
	code := Run(context.Background(), []string{"list"}, Env{
		Stdout: &bytes.Buffer{}, Stderr: &errOut,
		Store: func() (*walkthrough.Store, error) { return nil, errors.New("not a git repository") },
	})
	if code != 1 || !strings.Contains(errOut.String(), "buti walkthrough: not a git repository") {
		t.Errorf("exit %d, %q", code, errOut.String())
	}
}
