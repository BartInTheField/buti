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
	r, err := testrepo.Create(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r.Setenv(t.Setenv) // the client inherits the environment
	h := &harness{t: t, m: New(but.New(r.Dir), Options{}), wait: 2 * time.Second}
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
	cmd := exec.Command("freeze", "--language", "ansi", "--window=false", "--padding", "20",
		"--font.size", "14", "--output", base+".png")
	cmd.Stdin = strings.NewReader(content)
	if out, err := cmd.CombinedOutput(); err != nil {
		h.t.Errorf("freeze %s: %v\n%s", name, err, out)
	}
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
