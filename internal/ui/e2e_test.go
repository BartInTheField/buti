package ui

import (
	"context"
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
