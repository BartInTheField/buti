package ui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
	"github.com/bartinthefield/buti/internal/reviewcli"
	"github.com/bartinthefield/buti/internal/testrepo"
)

// End-to-end tests drive the UI against the real `but` CLI in the testrepo
// fixture. They are opt-in, like the client integration test:
//
//	BUTI_INTEGRATION=1 go test ./internal/ui -run E2E
//
// With BUTI_SCREENSHOTS=<dir>, every h.snap writes the screen to <dir>/<test>-<name>.ansi,
// and to .png too when `freeze` (github.com/charmbracelet/freeze) is on PATH.

func newRepoHarness(t *testing.T) (*harness, *testrepo.Repo) {
	t.Helper()
	if os.Getenv("BUTI_INTEGRATION") == "" {
		t.Skip("set BUTI_INTEGRATION=1 to run against the real but CLI")
	}
	if _, err := exec.LookPath("but"); err != nil {
		t.Skip("but not on PATH")
	}
	r, err := testrepo.Create(testrepo.TempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	r.Setenv(t.Setenv) // the client inherits the environment
	store, err := review.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, m: New(but.New(r.Dir), Options{Review: store}), wait: 2 * time.Second}
	h.send(tea.WindowSizeMsg{Width: 160, Height: 40})
	h.run(h.m.fetchStatus())
	return h, r
}

// status reads the workspace straight from but, bypassing the UI.
func (h *harness) status() *but.Status {
	h.t.Helper()
	s, err := h.m.client.Status(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	return s
}

// commits returns the subjects of a branch's commits, newest first.
func (h *harness) commits(branch string) []string {
	h.t.Helper()
	for _, st := range h.status().Stacks {
		for _, b := range st.Branches {
			if b.Name == branch {
				var out []string
				for _, c := range b.Commits {
					out = append(out, c.Subject())
				}
				return out
			}
		}
	}
	h.t.Fatalf("no branch %q", branch)
	return nil
}

func (h *harness) wantOnScreen(texts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, text := range texts {
		if !strings.Contains(s, text) {
			h.t.Errorf("%q not on screen:\n%s", text, s)
		}
	}
}

// snap saves the current screen when BUTI_SCREENSHOTS is set.
func (h *harness) snap(name string) {
	h.t.Helper()
	dir := os.Getenv("BUTI_SCREENSHOTS")
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	base := filepath.Join(dir, strings.TrimPrefix(h.t.Name(), "TestE2E")+"-"+name)
	content := h.m.View().Content
	if err := os.WriteFile(base+".ansi", []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
	if _, err := exec.LookPath("freeze"); err != nil {
		return
	}
	// freeze now and then crashes in the Go runtime on CI. The PNG is only for people to look at and the .ansi is
	// saved, so a render that keeps failing is logged rather than failing a test whose assertions passed.
	var out []byte
	var err error
	for range 3 {
		cmd := exec.Command("freeze", "--language", "ansi", "--window=false", "--padding", "20",
			"--font.size", "14", "--output", base+".png")
		cmd.Stdin = strings.NewReader(content)
		if out, err = cmd.CombinedOutput(); err == nil {
			return
		}
	}
	h.t.Logf("freeze %s: %v\n%.2000s", name, err, out)
}

func TestE2EWorkspace(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.wantOnScreen("Unstaged", "README.md", "strings.go", "old.md",
		"auth", "api", "Add token auth", "Add API routes",
		"fix-typo", "Bump Go version", "empty")
	h.snap("workspace")

	h.selectText("README.md")
	h.keys("d")
	h.wantOnScreen("## Usage")
	h.snap("file-diff")

	h.selectText("Add users endpoint")
	h.wantOnScreen("2 files changed", "routes.go")
	h.snap("commit-diff")

	// The README screenshot (mise run readme-screenshot): a diff, and a commit target being picked.
	h.selectText("strings.go")
	h.keys("c")
	h.hover("branch:auth")
	h.snap("hero")
	h.keys("esc")

	h.keys("?")
	h.snap("help")
	h.typeText("u")
	h.snap("help-search")
	h.keys("esc", "ctrl+p")
	h.snap("palette")
	// A command picked from the palette acts on the live model: it opens a picker that selects.
	h.typeText("go to branch")
	h.keys("enter")
	h.typeText("fix-typo")
	h.keys("enter")
	if got := h.selectedKey(); got != "branch:fix-typo" {
		h.t.Fatalf("selected %q, want branch:fix-typo", got)
	}
	h.snap("palette-goto-branch")
}

func TestE2ECommitFileToBranch(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("README.md")
	h.keys("c")
	h.hover("branch:empty")
	h.wantOnScreen("commit here")
	h.snap("target")
	h.keys("enter")
	h.typeText("Document usage")
	h.snap("composer")
	h.keys("enter")
	if got := h.commits("empty"); len(got) != 1 || got[0] != "Document usage" {
		t.Fatalf("empty has %q", got)
	}
	h.wantOnScreen("Document usage")
	h.snap("done")
}

func TestE2ESquashCommits(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("Test token auth")
	h.keys("r", "u")
	h.selectText("Add token auth")
	h.snap("target")
	h.keys("enter")
	if got := h.commits("auth"); len(got) != 1 || got[0] != "Add token auth" {
		t.Fatalf("auth has %q", got)
	}
	h.snap("done")
}

func TestE2EUncommitByDragging(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.dragTo("Bump Go version", "Unstaged")
	if got := h.commits("fix-typo"); len(got) != 1 {
		t.Fatalf("fix-typo has %q", got)
	}
	h.wantOnScreen("go.mod")
	h.snap("done")
}

func TestE2EResolveInEditMode(t *testing.T) {
	h, r := newRepoHarness(t)
	if err := r.Conflict(); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.fetchStatus())
	h.selectText("Start a changelog")
	h.keys("d")
	h.wantOnScreen("✗ Conflicted", "e resolves it in edit mode")
	h.snap("conflicted")

	h.keys("e")
	h.wantOnScreen("You are editing commit", "Start a changelog", "CHANGELOG.md", "Conflicted", "Save and exit")
	h.snap("edit-mode")

	// Fix the file as an editor would; the view picks it up on the next refresh.
	if err := os.WriteFile(filepath.Join(r.Dir, "CHANGELOG.md"), []byte("# Changelog\n\n- Health endpoint\n- Token auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.fetchStatus())
	h.wantOnScreen("Resolved")
	h.snap("resolved")

	h.keys("e")
	for _, st := range h.status().Stacks {
		for _, b := range st.Branches {
			if b.Name == "changelog" && b.Commits[0].Conflicted != nil && *b.Commits[0].Conflicted {
				t.Fatal("saving left the commit conflicted")
			}
		}
	}
	h.wantOnScreen("Unstaged", "changelog", "Start a changelog")
	h.snap("done")
}

func TestE2EDiffLineCursor(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("Add users endpoint")
	h.keys("d", "tab")
	for range 4 {
		h.keys("j")
	}
	s, ok := h.m.det.lineSelection()
	if !ok || s.path == "" || s.line == 0 {
		t.Fatalf("no line under the cursor: %+v", s)
	}
	h.wantOnScreen("j/k line")
	h.snap("cursor")

	h.keys("v", "j", "j")
	if s, _ := h.m.det.lineSelection(); s.endLine <= s.line {
		t.Fatalf("range not extended: %+v", s)
	}
	h.wantOnScreen("cancel range")
	h.snap("range")
}

func TestE2EReviewComments(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("README.md")
	h.keys("d")
	h.click("## Usage")
	h.keys("C")
	h.typeText("Document the flags here too")
	h.snap("composer")
	h.keys("ctrl+s")
	h.wantOnScreen("you · line 5", "Document the flags here too", "✎1")
	h.snap("uncommitted")

	h.selectText("Add users endpoint")
	h.click(`"/health", "/users"`)
	h.keys("C")
	h.typeText("Keep the routes sorted, and say which ones need auth")
	h.keys("ctrl+s")
	h.wantOnScreen("you · line 3", "Keep the routes sorted", "✎1")
	h.snap("commit")

	// The agent resolves the README comment; the next refresh shows it.
	cs, err := h.m.review.List()
	if err != nil || len(cs) != 2 {
		t.Fatalf("comments %+v, %v", cs, err)
	}
	if _, err := h.m.review.Resolve(cs[0].ID, "Listed -C, --diff and --remember-selection"); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.fetchStatus())
	h.selectText("README.md")
	h.wantOnScreen("✓ resolved · Document the flags here too — Listed -C")
	h.snap("resolved")

	h.keys("ctrl+p")
	h.typeText("review comments")
	h.keys("enter")
	h.wantOnScreen("src/api/routes.go line 3")
	h.snap("palette")
	h.keys("enter")
	if l, ok := h.m.det.cursorNote(); !ok || l.Comment.ID != cs[1].ID {
		t.Fatalf("the palette did not jump to the comment:\n%s", h.screen())
	}
	h.wantOnScreen("e edit", "x resolve")
	h.snap("jump")
}

// An agent reviews with `buti review comment` (as /buti-review does), and buti shows its comments.
func TestE2EAgentReview(t *testing.T) {
	h, r := newRepoHarness(t)
	agent := func(args ...string) string {
		t.Helper()
		var out, errOut strings.Builder
		code := reviewcli.Run(context.Background(), append([]string{"comment"}, args...), reviewcli.Env{
			Stdout: &out, Stderr: &errOut, But: but.New(r.Dir),
			Store: func() (*review.Store, error) { return review.Open(r.Dir) },
		})
		if code != 0 {
			t.Fatalf("buti review comment %v: exit %d\n%s", args, code, errOut.String())
		}
		return out.String()
	}
	agent("--file", "README.md", "--line", "7",
		"--body", "[suggestion] Say which flags `go run` takes, or link to the usage docs")
	agent("--shortcode", "auth", "--file", "src/auth/token.go", "--line", "3",
		"--body", "[must-fix] The token is hard-coded; read it from the environment")
	agent("--shortcode", "auth", "--file", "src/auth/token_test.go", "--line", "5",
		"--body", "[question] Should this test check the token at all?")

	h.run(h.m.loadComments(true))
	h.selectText("README.md")
	h.keys("d")
	h.wantOnScreen("agent · line 7", "[suggestion] Say which flags", "✎1")
	h.snap("uncommitted")

	h.selectText("Add token auth")
	h.wantOnScreen("agent · line 3", "[must-fix] The token is hard-coded", "✎1")
	h.snap("commit")

	cs, err := h.m.review.List()
	if err != nil || len(cs) != 3 {
		t.Fatalf("comments %+v, %v", cs, err)
	}
	for _, c := range cs {
		if c.Author != "agent" {
			t.Errorf("comment %s by %q", c.ID, c.Author)
		}
	}
	if a := cs[1].Anchor; a.Kind != review.KindCommit || a.Branch != "auth" || a.LineText != `func Token() string { return "secret" }` {
		t.Errorf("commit anchor %+v", a)
	}
	if a := cs[2].Anchor; a.Kind != review.KindCommit || a.LineText != "func TestToken(t *testing.T) {}" {
		t.Errorf("test anchor %+v", a)
	}

	h.keys("ctrl+p")
	h.typeText("review comments")
	h.keys("enter")
	h.wantOnScreen("README.md line 7  [suggestion]", "agent · zz", "src/auth/token.go line 3  [must-fix]")
	h.snap("palette")
}

// A comment left in a branch's diff goes on the branch's commit that changes the line, and `buti review list` gives
// that commit's shortcode.
func TestE2EBranchComment(t *testing.T) {
	h, r := newRepoHarness(t)
	h.hover("branch:api")
	h.keys("d")
	h.click(`"/health", "/users"`)
	h.keys("C")
	h.typeText("Keep the routes sorted")
	h.wantOnScreen("on src/api/routes.go line 3 in")
	h.snap("composer")
	h.keys("ctrl+s")
	h.wantOnScreen("you · line 3", "Keep the routes sorted")
	h.snap("branch")

	// Context in the newest commit, added by the one before it.
	h.click("package api")
	h.keys("C")
	h.typeText("Name the package routes?")
	h.keys("ctrl+s")
	h.wantOnScreen("you · line 1", "Name the package routes?")

	cs, err := h.m.review.List()
	if err != nil || len(cs) != 2 {
		t.Fatalf("comments %+v, %v", cs, err)
	}
	commits := map[string]string{} // subject -> cli id
	for _, s := range h.status().Stacks {
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				commits[c.Subject()] = c.CliID
			}
		}
	}
	var out, errOut strings.Builder
	if code := reviewcli.Run(context.Background(), []string{"list", "--json"}, reviewcli.Env{
		Stdout: &out, Stderr: &errOut, But: but.New(r.Dir),
		Store: func() (*review.Store, error) { return review.Open(r.Dir) },
	}); code != 0 {
		t.Fatalf("buti review list: exit %d\n%s", code, errOut.String())
	}
	var listed []struct {
		Shortcode string `json:"shortcode"`
		Kind      string `json:"kind"`
		Branch    string `json:"branch"`
		Body      string `json:"body"`
	}
	if err := json.Unmarshal([]byte(out.String()), &listed); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	want := map[string]string{"Keep the routes sorted": commits["Add users endpoint"], "Name the package routes?": commits["Add API routes"]}
	for _, l := range listed {
		if l.Kind != "commit" || l.Branch != "api" || l.Shortcode != want[l.Body] {
			t.Errorf("listed %+v, want shortcode %s", l, want[l.Body])
		}
	}

	h.selectText("Add API routes")
	h.wantOnScreen("you · line 1", "Name the package routes?", "✎1")
}

func TestE2EFullDetailsTree(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("Unstaged")
	h.keys("D")
	h.wantOnScreen("Files", "docs/", "old.md", "src/", "util/", "strings.go", "server.go", "README.md")
	h.snap("uncommitted")

	h.keys("tab", "G") // README.md, the last row
	h.wantOnScreen("# demo")
	if got := h.m.det.activeFile(); got != "README.md" {
		t.Fatalf("active file %q, want README.md", got)
	}
	h.keys("g", "enter") // fold docs/
	if rows := h.m.det.tree.rows; len(rows) == 0 || !rows[0].collapsed {
		t.Fatalf("docs/ should be folded:\n%s", h.screen())
	}
	h.keys("j", "j", "j") // src/, util/, strings.go
	if got := h.m.det.activeFile(); got != "src/util/strings.go" {
		t.Fatalf("active file %q, want src/util/strings.go", got)
	}
	h.wantOnScreen("func Reverse")
	h.snap("folded")
	h.keys("-", "-", "-")
	h.snap("narrow-tree")

	h.keys("esc")
	h.selectText("Add users endpoint")
	h.keys("D")
	h.wantOnScreen("src/api/", "routes.go", "users.go", "2 files changed")
	h.snap("commit")

	// On a short screen the tree is cut off: it says so, and follows the diff.
	h.keys("esc")
	h.selectText("Unstaged")
	h.send(tea.WindowSizeMsg{Width: 160, Height: 9})
	h.keys("D") // the diff starts at README.md, the last row: the tree shows it
	h.wantOnScreen("↑ 2 more", "README.md")
	h.snap("short")
	h.keys("tab", "g") // the cursor to the top row
	h.wantOnScreen("↓ 2 more", "docs/")
	h.snap("short-top")
}

func TestE2ESidebarResize(t *testing.T) {
	h, _ := newRepoHarness(t)
	h.selectText("README.md")
	h.keys("d")
	x := h.m.sidebarWidth()
	h.send(tea.MouseClickMsg{X: x, Y: 5, Button: tea.MouseLeft})
	h.send(tea.MouseMotionMsg{X: 24, Y: 5, Button: tea.MouseLeft})
	h.snap("dragging")
	h.send(tea.MouseReleaseMsg{X: 24, Y: 5, Button: tea.MouseLeft})
	if h.m.sidebarWidth() != 24 {
		t.Fatalf("sidebar width %d, want 24", h.m.sidebarWidth())
	}
	h.wantOnScreen("Unstaged", "README.md", "## Usage")
	h.snap("narrow")
}

// zedPair runs `Z` and returns the old and new paths handed to `zed --diff`.
func zedPair(t *testing.T, h *harness, started *[][]string, before int) (oldP, newP string) {
	t.Helper()
	h.keys("Z")
	if len(*started) != before+1 {
		t.Fatalf("started %q, want %d start(s)", *started, before+1)
	}
	args := (*started)[before]
	if len(args) != 4 || args[0] != "zed" || args[1] != "--diff" {
		t.Fatalf("args %q, want zed --diff OLD NEW", args)
	}
	return args[2], args[3]
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func TestE2EZedDiff(t *testing.T) {
	started := detached(t)
	t.Setenv("EDITOR", "zed")
	t.Setenv("VISUAL", "")
	h, r := newRepoHarness(t)
	var tmps []string
	t.Cleanup(func() {
		for _, d := range tmps {
			_ = os.RemoveAll(d)
		}
	})

	// A commit: a directory pair, the added file empty on the old side.
	h.selectText("Add token auth")
	oldP, newP := zedPair(t, h, started, 0)
	tmps = append(tmps, filepath.Dir(oldP))
	if !isDir(oldP) || !isDir(newP) {
		t.Fatalf("want directories, got %q %q", oldP, newP)
	}
	if b, err := os.ReadFile(filepath.Join(newP, "src/auth/token.go")); err != nil ||
		string(b) != "package auth\n\nfunc Token() string { return \"secret\" }\n" {
		t.Errorf("new token.go %q, %v", b, err)
	}
	if b, err := os.ReadFile(filepath.Join(oldP, "src/auth/token.go")); err != nil || len(b) != 0 {
		t.Errorf("old token.go %q, %v; want an empty file", b, err)
	}
	h.wantOnScreen("Opened the diff of", "in Zed")
	h.snap("zed-diff")
	h.keys("?")
	h.typeText("zed")
	h.wantOnScreen("Open diff in Zed")
	h.snap("help-zed")
	h.keys("esc")

	// A branch: one directory pair holding each changed file as of the tip.
	h.selectText("fix-typo")
	oldP, newP = zedPair(t, h, started, 1)
	tmps = append(tmps, filepath.Dir(oldP))
	if !isDir(oldP) || !isDir(newP) {
		t.Fatalf("want directories, got %q %q", oldP, newP)
	}
	var tip string
	for _, st := range h.status().Stacks {
		for _, b := range st.Branches {
			if b.Name == "fix-typo" {
				tip = b.Commits[0].CommitID
			}
		}
	}
	nonEmpty := 0
	err := filepath.WalkDir(newP, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(newP, p)
		want, err := exec.Command("git", "-C", r.Dir, "show", tip+":"+filepath.ToSlash(rel)).Output()
		if err != nil {
			t.Errorf("git show %s: %v", rel, err)
			return nil
		}
		got, _ := os.ReadFile(p)
		if string(got) != string(want) {
			t.Errorf("%s: %q, want %q", rel, got, want)
		}
		if len(got) > 0 {
			nonEmpty++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if nonEmpty == 0 {
		t.Errorf("no non-empty file under %s", newP)
	}

	// An uncommitted file: the worktree file on the right, the committed version on the left.
	h.selectText("README.md")
	oldP, newP = zedPair(t, h, started, 2)
	tmps = append(tmps, filepath.Dir(filepath.Dir(oldP)))
	work := filepath.Join(r.Dir, "README.md")
	if newP != work {
		t.Errorf("right side %q, want %q", newP, work)
	}
	oldB, err1 := os.ReadFile(oldP)
	newB, err2 := os.ReadFile(work)
	if err1 != nil || err2 != nil {
		t.Fatalf("read: %v %v", err1, err2)
	}
	if string(oldB) == string(newB) {
		t.Errorf("old side equals the worktree file %q", newB)
	}
}
