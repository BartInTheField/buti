package reviewcli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

const (
	tokenLine  = `func Token() string { return "secret" }`
	tokenHunk  = "@@ -0,0 +1,3 @@\n+package auth\n+\n+" + tokenLine + "\n"
	readmeHunk = "@@ -1,3 +1,7 @@\n # demo\n \n A small service.\n+\n+## Usage\n+\n+    go run ./src\n"
)

// fakeStatus has README.md unassigned (rl), docs/a.md assigned to the stack of "docs" (k0), and a stack m0 whose
// branch auth (au) has a commit adding token.go (knl) and a newer one adding a line to it (tdy).
var fakeStatus = but.Status{
	UncommittedChanges: []but.Change{{CliID: "rl", FilePath: "README.md"}},
	Stacks: []but.Stack{
		{CliID: "m0", Branches: []but.Branch{{CliID: "au", Name: "auth", Commits: []but.Commit{{
			CliID: "tdy", ChangeID: "change-tidy", CommitID: "fedcba9876543210", Message: "Document the token",
			Changes: []but.Change{{CliID: "t:m", FilePath: "src/auth/token.go"}},
		}, {
			CliID: "knl", ChangeID: "change-token", CommitID: "0123456789abcdef", Message: "Add token auth\n\nMore.",
			Changes: []but.Change{{CliID: "k:m", FilePath: "src/auth/token.go"}},
		}}}}},
		{CliID: "k0", AssignedChanges: []but.Change{{CliID: "rd", FilePath: "docs/a.md"}},
			Branches: []but.Branch{{Name: "docs"}}},
	},
}

var fakeDiffs = map[string]string{
	"rl":  diffJSON("README.md", readmeHunk),
	"k:m": diffJSON("src/auth/token.go", tokenHunk),
	"rd":  diffJSON("docs/a.md", "@@ -0,0 +1,1 @@\n+hello\n"),
	"t:m": diffJSON("src/auth/token.go", "@@ -1,3 +1,4 @@\n package auth\n \n "+tokenLine+"\n+// Token is for tests.\n"),
}

func diffJSON(path, hunk string) string {
	fd := but.FileDiff{Path: path}
	fd.Diff.Type = "patch"
	fd.Diff.Hunks = []but.Hunk{{Diff: hunk}}
	b, _ := json.Marshal(but.Diff{Changes: []but.FileDiff{fd}})
	return string(b)
}

// fakeBut installs a `but` stand-in serving fakeStatus and fakeDiffs, logging every call to log.
func fakeBut(t *testing.T) (c *but.Client, log string) {
	t.Helper()
	dir := t.TempDir()
	st, _ := json.Marshal(fakeStatus)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("status.json", string(st))
	for id, d := range fakeDiffs {
		write("diff-"+id+".json", d)
	}
	log = filepath.Join(dir, "log")
	write("but", "#!/bin/sh\n"+
		"echo \"$*\" >> '"+log+"'\n"+
		"case \"$1\" in\n"+
		"  status) cat '"+dir+"/status.json' ;;\n"+
		"  diff) cat \""+dir+"/diff-$3.json\" ;;\n"+
		"  *) exit 1 ;;\n"+
		"esac\n")
	c = but.New(dir)
	c.Bin = filepath.Join(dir, "but")
	return c, log
}

type harness struct {
	t      *testing.T
	store  *review.Store
	but    *but.Client
	log    string
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newHarness(t *testing.T) *harness {
	c, log := fakeBut(t)
	return &harness{t: t, store: review.New(filepath.Join(t.TempDir(), "review.json")), but: c, log: log}
}

// run runs `buti review args` and returns the exit code; the output is in h.stdout and h.stderr.
func (h *harness) run(args ...string) int {
	h.stdout.Reset()
	h.stderr.Reset()
	return Run(context.Background(), args, Env{
		Stdout: &h.stdout,
		Stderr: &h.stderr,
		Store:  func() (*review.Store, error) { return h.store, nil },
		But:    h.but,
	})
}

func (h *harness) ok(args ...string) string {
	h.t.Helper()
	if code := h.run(args...); code != 0 {
		h.t.Fatalf("buti review %s: exit %d\n%s", strings.Join(args, " "), code, h.stderr.String())
	}
	return h.stdout.String()
}

func (h *harness) add(a review.Anchor, body string) review.Comment {
	h.t.Helper()
	c, err := h.store.Add(a, "", body)
	if err != nil {
		h.t.Fatal(err)
	}
	return c
}

func (h *harness) butCalls() []string {
	b, _ := os.ReadFile(h.log)
	return strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(b)), " ", "_"))
}

// decode is the JSON of a `list --json` as generic maps, to check the exact field names and nulls.
func decode(t *testing.T, s string) []map[string]any {
	t.Helper()
	var out []map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		t.Fatalf("%v\n%s", err, s)
	}
	return out
}

func TestListJSON(t *testing.T) {
	h := newHarness(t)
	usage := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}, "Say more")
	token := h.add(review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", CommitID: "0123456789abcdef",
		Branch: "auth", Path: "src/auth/token.go", Line: 3, LineText: tokenLine}, "Don't hard-code the token")
	docs := h.add(review.Anchor{Kind: review.KindAssigned, Branch: "docs", Path: "docs/a.md", Line: 1, LineText: "hello"}, "Hi")
	gone := h.add(review.Anchor{Kind: review.KindCommit, ChangeID: "gone", CommitID: "fedcba", Branch: "auth",
		Path: "src/x.go", Line: 1, LineText: "x"}, "Orphaned")

	got := decode(t, h.ok("list", "--json"))
	want := []map[string]any{
		{
			"id": usage.ID, "status": "open", "author": "user", "body": "Say more", "file": "README.md", "line": 5.0, "end_line": 5.0,
			"side": "new", "shortcode": "zz", "file_shortcode": "rl", "kind": "unassigned", "branch": nil,
			"commit": nil, "outdated": false,
			"context": "  3 | A small service.\n  4 | \n> 5 | ## Usage\n  6 | \n  7 |     go run ./src",
		},
		{
			"id": token.ID, "status": "open", "author": "user", "body": "Don't hard-code the token", "file": "src/auth/token.go",
			"line": 3.0, "end_line": 3.0, "side": "new", "shortcode": "knl", "file_shortcode": "k:m", "kind": "commit",
			"branch": "auth", "commit": map[string]any{"title": "Add token auth", "sha": "0123456789abcdef"},
			"outdated": false, "context": "  1 | package auth\n  2 | \n> 3 | " + tokenLine,
		},
		{
			"id": docs.ID, "status": "open", "author": "user", "body": "Hi", "file": "docs/a.md", "line": 1.0, "end_line": 1.0,
			"side": "new", "shortcode": "k0", "file_shortcode": "rd", "kind": "assigned", "branch": "docs",
			"commit": nil, "outdated": false, "context": "> 1 | hello",
		},
		{
			"id": gone.ID, "status": "open", "author": "user", "body": "Orphaned", "file": "src/x.go", "line": 1.0, "end_line": 1.0,
			"side": "new", "shortcode": nil, "file_shortcode": nil, "kind": "commit", "branch": "auth",
			"commit": map[string]any{"title": "", "sha": "fedcba"}, "outdated": true, "context": "> 1 | x",
		},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d comments, want %d:\n%s", len(got), len(want), h.stdout.String())
	}
	for i := range want {
		g, _ := json.Marshal(got[i])
		w, _ := json.Marshal(want[i])
		if string(g) != string(w) {
			t.Errorf("comment %d:\n got  %s\n want %s", i, g, w)
		}
	}

	// One status and one diff per distinct file, the context reusing the diffs re-anchoring read.
	if calls := h.butCalls(); len(calls) != 4 || calls[0] != "status_--json_-f" {
		t.Errorf("but calls %q", calls)
	}
}

func TestListFollowsMovedLines(t *testing.T) {
	h := newHarness(t)
	// Stored at line 2, the text is now at line 3.
	c := h.add(review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", Path: "src/auth/token.go", Line: 2,
		LineText: tokenLine}, "Moved")
	got := decode(t, h.ok("list", "--json"))
	if len(got) != 1 || got[0]["line"] != 3.0 || got[0]["shortcode"] != "knl" || got[0]["outdated"] != false {
		t.Fatalf("got %v", got)
	}
	if stored, _ := h.store.Get(c.ID); stored.Anchor.Line != 3 || stored.Anchor.CommitID != "0123456789abcdef" {
		t.Errorf("the move was not saved: %+v", stored.Anchor)
	}
}

func TestListStatusFilter(t *testing.T) {
	h := newHarness(t)
	open := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}, "Open")
	outdated := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 1, LineText: "gone"}, "Outdated")
	done := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 1, LineText: "# demo"}, "Done")
	dismissed := h.add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 1, LineText: "# demo"}, "No")
	h.ok("resolve", done.ID, "--summary", "Did it")
	h.ok("dismiss", dismissed.ID)

	ids := func(args ...string) string {
		t.Helper()
		var out []string
		for _, c := range decode(t, h.ok(append(args, "--json")...)) {
			out = append(out, c["id"].(string))
		}
		return strings.Join(out, " ")
	}
	for _, tc := range []struct {
		args []string
		want []string
	}{
		{[]string{"list"}, []string{open.ID, outdated.ID}},
		{[]string{"list", "--status", "open"}, []string{open.ID, outdated.ID}},
		{[]string{"list", "--status", "outdated"}, []string{outdated.ID}},
		{[]string{"list", "--status", "orphaned"}, nil},
		{[]string{"list", "--status", "resolved"}, []string{done.ID}},
		{[]string{"list", "--status", "dismissed"}, []string{dismissed.ID}},
		{[]string{"list", "--status", "all"}, []string{open.ID, outdated.ID, done.ID, dismissed.ID}},
	} {
		if got := ids(tc.args...); got != strings.Join(tc.want, " ") {
			t.Errorf("%v: got %q, want %q", tc.args, got, tc.want)
		}
	}
	if got := h.ok("list", "--status", "orphaned", "--json"); strings.TrimSpace(got) != "[]" {
		t.Errorf("an empty list is %q, want []", got)
	}
	if code := h.run("list", "--status", "bogus"); code != 2 {
		t.Errorf("bad --status: exit %d", code)
	}
}

func TestListText(t *testing.T) {
	h := newHarness(t)
	if got := h.ok("list"); got != "No open comments.\n" {
		t.Errorf("empty: %q", got)
	}
	if calls := h.butCalls(); len(calls) != 0 {
		t.Errorf("an empty store ran but: %q", calls)
	}
	c := h.add(review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", Path: "src/auth/token.go", Line: 1,
		EndLine: 3, LineText: "package auth\n\n" + tokenLine}, "Rename this\nand that")
	got := h.ok("list")
	if want := c.ID + "  open  user  knl  src/auth/token.go:1-3  Rename this\n"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestShow(t *testing.T) {
	h := newHarness(t)
	c := h.add(review.Anchor{Kind: review.KindCommit, ChangeID: "change-token", Path: "src/auth/token.go", Line: 3,
		LineText: tokenLine}, "Don't hard-code the token")
	h.ok("reply", c.ID[:3], "--body", "Which variable?")
	h.ok("reply", c.ID, "--body", "TOKEN", "--author", "user")

	out := h.ok("show", c.ID[:2])
	for _, want := range []string{
		c.ID + "  open  by user",
		"src/auth/token.go:3 (new side)",
		`on commit knl "Add token auth" (0123456) on auth, file k:m`,
		"Don't hard-code the token",
		"> 3 | " + tokenLine,
		"agent, ", "Which variable?",
		"user, ", "TOKEN",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("show lacks %q:\n%s", want, out)
		}
	}

	var d map[string]any
	if err := json.Unmarshal([]byte(h.ok("show", c.ID, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	replies, _ := d["replies"].([]any)
	if d["id"] != c.ID || d["shortcode"] != "knl" || d["author"] != "user" || d["resolution"] != nil || len(replies) != 2 {
		t.Errorf("show --json: %v", d)
	}
	if r := replies[0].(map[string]any); r["author"] != "agent" || r["body"] != "Which variable?" {
		t.Errorf("reply %v", r)
	}

	h.ok("resolve", c.ID, "--summary", "Reads $TOKEN now")
	if err := json.Unmarshal([]byte(h.ok("show", c.ID, "--json")), &d); err != nil {
		t.Fatal(err)
	}
	if res, _ := d["resolution"].(map[string]any); d["status"] != "resolved" || res["summary"] != "Reads $TOKEN now" {
		t.Errorf("resolved: %v", d)
	}
	if out := h.ok("show", c.ID); !strings.Contains(out, "resolved") || !strings.Contains(out, ": Reads $TOKEN now") {
		t.Errorf("show of a resolved comment:\n%s", out)
	}
}

func TestResolveDismissClear(t *testing.T) {
	h := newHarness(t)
	a := review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}
	one, two, three := h.add(a, "One"), h.add(a, "Two"), h.add(a, "Three")

	if got := h.ok("resolve", one.ID, "--summary", "Done"); got != "resolved "+one.ID+"\n" {
		t.Errorf("resolve printed %q", got)
	}
	if got := h.ok("dismiss", "--reason", "Intended", two.ID); got != "dismissed "+two.ID+"\n" {
		t.Errorf("dismiss printed %q", got)
	}
	if c, _ := h.store.Get(two.ID); c.Status != review.StatusDismissed || c.Resolution.Summary != "Intended" {
		t.Errorf("dismissed: %+v", c)
	}
	if got := h.ok("clear", "--resolved"); got != "cleared 1 comment\n" {
		t.Errorf("clear printed %q", got)
	}
	cs, _ := h.store.List()
	if len(cs) != 2 || cs[0].ID != two.ID || cs[1].ID != three.ID {
		t.Errorf("after clear --resolved: %+v", cs)
	}
	if got := h.ok("clear", "--dismissed", "--resolved"); got != "cleared 1 comment\n" {
		t.Errorf("clear printed %q", got)
	}
	if calls := h.butCalls(); len(calls) != 0 {
		t.Errorf("writes ran but: %q", calls)
	}
}

func TestErrors(t *testing.T) {
	h := newHarness(t)
	a := review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}
	h.add(a, "One")
	for _, tc := range []struct {
		args []string
		code int
		msg  string
	}{
		{nil, 2, "usage:"},
		{[]string{"frobnicate"}, 2, `unknown command "frobnicate"`},
		{[]string{"resolve"}, 2, "expected one comment id"},
		{[]string{"resolve", "a", "b"}, 2, "expected one comment id"},
		{[]string{"resolve", "zzzz"}, 1, "no such comment: zzzz"},
		{[]string{"show", "zzzz"}, 1, "no such comment: zzzz"},
		{[]string{"reply", "zzzz"}, 2, "--body is required"},
		{[]string{"clear"}, 2, "--resolved"},
		{[]string{"list", "extra"}, 2, `unexpected argument "extra"`},
		{[]string{"list", "--nope"}, 2, "flag provided but not defined"},
	} {
		code := h.run(tc.args...)
		if code != tc.code || !strings.Contains(h.stderr.String(), tc.msg) {
			t.Errorf("%v: exit %d, stderr %q; want exit %d with %q", tc.args, code, h.stderr.String(), tc.code, tc.msg)
		}
	}
	if code := h.run("list", "--help"); code != 0 {
		t.Errorf("--help: exit %d", code)
	}
}

func TestExcerptOnOldSide(t *testing.T) {
	fd := &but.FileDiff{Path: "a.go"}
	fd.Diff.Hunks = []but.Hunk{{Diff: "@@ -8,3 +8,2 @@\n ctx\n-gone\n+new\n"}}
	a := review.Anchor{Side: review.SideOld, Line: 9, EndLine: 9, LineText: "gone"}
	if got := review.Excerpt(fd, a, 2); got != "  8 | ctx\n> 9 | gone" {
		t.Errorf("got %q", got)
	}
	a = review.Anchor{Side: review.SideNew, Line: 9, EndLine: 9, LineText: "new"}
	if got := review.Excerpt(fd, a, 2); got != "  8 | ctx\n> 9 | new" {
		t.Errorf("got %q", got)
	}
}
