package ui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/update"
)

func TestClickSelects(t *testing.T) {
	h := newHarness(t)
	for _, tc := range []struct{ text, key string }{
		{"branch-2", "branch:branch-2"},
		{"branch-1", "branch:branch-1"},
		{"first", "commit:chg1"},
		{"go.mod", "file:go.mod"},
		{"Unstaged", "all changes"},
	} {
		h.selectText(tc.text)
		if got := h.selectedKey(); got != tc.key {
			t.Errorf("click %q selected %q, want %q", tc.text, got, tc.key)
		}
	}
}

func TestClickFoldsDirectory(t *testing.T) {
	h := newHarness(t)
	h.selectText("cmd/buti/")
	if !h.m.files[0].collapsed || len(h.m.files) != 2 {
		t.Fatalf("directory not folded: %+v", h.m.files)
	}
}

func TestDoubleClickOpensDetails(t *testing.T) {
	h := newHarness(t)
	h.click("go.mod")
	h.click("go.mod")
	if !h.m.det.full {
		t.Fatal("double click should open full-screen details")
	}
	h.click("esc ✕")
	if h.m.det.full {
		t.Fatal("close button should leave full screen")
	}
}

func TestCommitFileToBranch(t *testing.T) {
	h := newHarness(t)
	h.selectText("go.mod")
	h.keys("c")
	if h.m.target == nil || h.m.focus != focusLanes {
		t.Fatal("c should enter commit target mode in the lanes")
	}
	h.hover("branch:branch-2")
	if !strings.Contains(h.screen(), "commit here") {
		t.Fatalf("hovered target should be labelled:\n%s", h.screen())
	}
	h.keys("enter")
	if _, ok := h.m.modal.(*composerModal); !ok {
		t.Fatalf("expected the composer, got %T", h.m.modal)
	}
	h.typeText("Bump deps")
	h.keys("enter")
	h.but.expect("commit --message Bump deps f2 --branch branch-2")
}

func TestCommitAboveCommitWithEmptyMessage(t *testing.T) {
	h := newHarness(t)
	h.selectText("go.mod")
	h.keys("c", "e", "a") // empty message, above
	h.selectText("second")
	h.keys("enter")
	h.but.expect("commit --no-message f2 --above c2")
}

func TestAmendByDraggingFileOntoCommit(t *testing.T) {
	h := newHarness(t)
	h.dragTo("go.mod", "second")
	h.but.expect("amend --target c2 f2")
}

func TestUncommitByDraggingCommitToUnstaged(t *testing.T) {
	h := newHarness(t)
	h.dragTo("first", "Unstaged")
	h.but.expect("uncommit c1")
}

func TestDragCommitToNewBranchLane(t *testing.T) {
	h := newHarness(t)
	h.dragTo("first", "Drop here")
	h.but.expect("move c1 --unstack")
}

func TestSquashCombinesMessages(t *testing.T) {
	h := newHarness(t)
	h.selectText("second")
	h.keys("r")
	h.hover("commit:chg1")
	h.keys("enter")
	cm, ok := h.m.modal.(*composerModal)
	if !ok {
		t.Fatalf("expected the composer for two messages, got %T", h.m.modal)
	}
	if cm.message() != "first\n\nsecond" {
		t.Fatalf("combined message %q", cm.message())
	}
	h.keys("ctrl+s")
	h.but.expect("squash --target c1 --message first\n\nsecond c2")
}

func TestSquashKeepTargetMessage(t *testing.T) {
	h := newHarness(t)
	h.selectText("second")
	h.keys("r", "u")
	h.selectText("first")
	h.keys("enter")
	h.but.expect("squash --target c1 --use-target-message c2")
}

func TestMoveCommitToBranch(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("m")
	h.selectText("branch-2")
	h.keys("enter")
	h.but.expect("move c1 --branch branch-2")
}

func TestInvalidTargetIsRefused(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("m")
	h.m.focus, h.m.fileCursor = focusFiles, 0 // a directory
	h.keys("enter")
	h.but.expect()
	if h.m.target == nil {
		t.Fatal("an invalid target should keep target mode open")
	}
}

func TestMarkedCommitsSquashTogether(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("space")
	h.selectText("second")
	h.keys("space", "r", "u")
	h.selectText("branch-1")
	h.keys("esc") // cancel target mode; marks stay
	if len(h.m.marks) != 2 {
		t.Fatalf("marks: %v", h.m.marks)
	}
	h.keys("esc")
	if len(h.m.marks) != 0 {
		t.Fatal("second esc should clear marks")
	}
}

func TestDiscardAsksFirst(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("x")
	if _, ok := h.m.modal.(*confirmModal); !ok {
		t.Fatalf("expected a confirmation, got %T", h.m.modal)
	}
	h.but.expect()
	h.keys("y")
	h.but.expect("discard c1")
}

func TestNewStackedBranch(t *testing.T) {
	h := newHarness(t)
	h.selectText("branch-2")
	h.keys("b")
	h.typeText("my feature")
	h.keys("enter")
	h.but.expect("branch new --above b2 my-feature")
}

func TestRenameBranch(t *testing.T) {
	h := newHarness(t)
	h.selectText("branch-2")
	h.keys("enter")
	h.typeText("-x")
	h.keys("enter")
	h.but.expect("reword b2 --message branch-2-x")
}

func TestPushAndUndo(t *testing.T) {
	h := newHarness(t)
	h.selectText("branch-4")
	h.keys("P", "u")
	h.but.expect("push branch-4", "undo")
}

func TestPushButton(t *testing.T) {
	h := newHarness(t)
	h.click("Push ⇡")
	h.but.expect("push branch-4")
}

func TestStartCommitButton(t *testing.T) {
	h := newHarness(t)
	// The second lane's button.
	_, y := h.find("Start a commit")
	line := strings.Split(h.screen(), "\n")[y]
	x := strings.LastIndex(line, "Start a commit")
	h.send(tea.MouseClickMsg{X: ansi.StringWidth(line[:x]), Y: y, Button: tea.MouseLeft})
	h.typeText("wip")
	h.keys("enter")
	h.but.expect("commit --message wip --branch branch-2")
}

func TestShowCommitFiles(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("f")
	if !strings.Contains(h.screen(), "x.go") {
		t.Fatal("f should list the commit's files")
	}
	h.keys("j")
	if got := h.selectedKey(); got != "cfile:chg1:x.go" {
		t.Fatalf("selected %q", got)
	}
	h.keys("r")
	h.selectText("Unstaged")
	h.keys("enter")
	h.but.expect("uncommit c1:x")
}

func TestPaletteRunsAction(t *testing.T) {
	h := newHarness(t)
	h.selectText("branch-4")
	h.keys("ctrl+p")
	h.typeText("push")
	h.keys("enter")
	h.but.expect("push branch-4")
}

func TestRenderFitsScreen(t *testing.T) {
	h := newHarness(t)
	check := func(stage string) {
		t.Helper()
		lines := strings.Split(h.m.View().Content, "\n")
		if len(lines) > h.m.height {
			t.Errorf("%s: %d lines on a %d-line screen", stage, len(lines), h.m.height)
		}
		for i, l := range lines {
			if w := ansi.StringWidth(l); w > h.m.width {
				t.Errorf("%s: line %d is %d wide: %q", stage, i, w, ansi.Strip(l))
			}
		}
	}
	check("normal")
	h.keys("d")
	check("details")
	h.selectText("first")
	h.keys("r")
	check("target")
	h.keys("esc", "?")
	check("help")
	h.keys("esc", "D")
	check("full details")
}

func TestUpdateOffered(t *testing.T) {
	h := newHarness(t)
	h.m.opts.Version = "2026.09.26.1"
	h.send(updateAvailableMsg{latest: "2026.09.26.2"})
	if _, ok := h.m.modal.(*confirmModal); !ok {
		t.Fatalf("expected an update prompt, got %T", h.m.modal)
	}
	if !strings.Contains(h.screen(), "2026.09.26.2 is available") {
		t.Fatal("prompt should name the new version")
	}
	h.keys("esc")
	if h.m.modal != nil {
		t.Fatal("esc should dismiss the prompt")
	}
}

// fakeLatest points the update check at a server whose latest release is version.
func fakeLatest(t *testing.T, version string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/releases/tag/"+version)
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	old := update.BaseURL
	update.BaseURL = srv.URL
	t.Cleanup(func() { update.BaseURL = old })
}

func TestManualUpdateCheck(t *testing.T) {
	h := newHarness(t)
	h.m.opts.Version = "2026.09.26.1"
	fakeLatest(t, "2026.09.26.2")
	h.keys("?")
	h.typeText("update buti")
	h.keys("enter")
	if _, ok := h.m.modal.(*confirmModal); !ok {
		t.Fatalf("expected an update prompt, got %T", h.m.modal)
	}
	if !strings.Contains(h.screen(), "2026.09.26.2 is available") {
		t.Fatal("prompt should name the new version")
	}
}

func TestManualUpdateCheckUpToDate(t *testing.T) {
	h := newHarness(t)
	h.m.opts.Version = "2026.09.26.2"
	fakeLatest(t, "2026.09.26.2")
	h.keys("?")
	h.typeText("update buti")
	h.keys("enter")
	if h.m.modal != nil {
		t.Fatalf("no prompt expected when up to date, got %T", h.m.modal)
	}
	if !strings.Contains(h.screen(), "2026.09.26.2 is the latest version") {
		t.Fatal("should say buti is up to date")
	}
}

func TestManualUpdateCheckDevBuild(t *testing.T) {
	h := newHarness(t)
	h.keys("?")
	h.typeText("update buti")
	h.keys("enter")
	if !strings.Contains(h.screen(), "Development builds can't update themselves") {
		t.Fatal("a dev build should explain it can't update")
	}
}

func TestShowVersion(t *testing.T) {
	for _, tc := range []struct{ version, want string }{
		{"2026.09.26.1", "buti 2026.09.26.1"},
		{"dev", "buti development build (dev)"},
	} {
		h := newHarness(t)
		h.m.opts.Version = tc.version
		h.keys("?")
		h.typeText("version")
		h.keys("enter")
		if !strings.Contains(h.screen(), tc.want) {
			t.Errorf("version %q: want %q on screen", tc.version, tc.want)
		}
	}
}
