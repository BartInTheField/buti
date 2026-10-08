package ui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/editor"
)

// detached stubs startDetached, recording what would have been started. A `zed` on the PATH stands in for Zed's
// CLI, whether or not the machine has Zed.
func detached(t *testing.T) *[][]string {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "zed"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var started [][]string
	startDetached = func(cmd *exec.Cmd) error { started = append(started, cmd.Args); return nil }
	t.Cleanup(func() { startDetached = defaultStartDetached })
	return &started
}

func setEditor(t *testing.T, visual, editor, term string) {
	t.Helper()
	t.Setenv("VISUAL", visual)
	t.Setenv("EDITOR", editor)
	t.Setenv("TERM_PROGRAM", term)
}

func TestOpenAtCursorInZed(t *testing.T) {
	started := detached(t)
	setEditor(t, "", "zed --wait", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	h.selectText("go.mod")
	h.keys("D", "j", "j", "j") // the cursor is on "+go 1.22", line 2
	h.keys("o")

	want := []string{"sh", "-c", `zed "$@"`, "editor", h.m.client.Dir + "/go.mod:2"}
	if len(*started) != 1 || strings.Join((*started)[0], "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("started %q, want %q", *started, want)
	}
	if h.m.busy != "" {
		t.Errorf("the TUI was suspended: busy %q", h.m.busy)
	}
	h.wantScreen("Opened go.mod")
}

func TestOpenOnRemovedLineUsesNextLine(t *testing.T) {
	started := detached(t)
	setEditor(t, "", "code -w", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	h.selectText("go.mod")
	h.keys("D", "j", "j") // "-go 1.21": the next line on the new side is 2
	h.keys("o")
	if len(*started) != 1 || (*started)[0][4] != "-g" || !strings.HasSuffix((*started)[0][5], "/go.mod:2") {
		t.Fatalf("started %q", *started)
	}
}

func TestOpenInTerminalEditorSuspends(t *testing.T) {
	started := detached(t)
	setEditor(t, "vim", "", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	h.selectText("go.mod")
	h.keys("D", "j", "j", "j", "o")
	if len(*started) != 0 {
		t.Errorf("a terminal editor was started detached: %q", *started)
	}
	if !strings.HasPrefix(h.m.busy, "Edit go.mod") {
		t.Errorf("busy %q: the TUI should hand over the terminal", h.m.busy)
	}
	if got := h.m.openLine(h.m.selected()); got != 2 {
		t.Errorf("line %d, want 2 (vim gets +2)", got)
	}
}

func TestOpenHunkWithoutCursorUsesFirstAddedLine(t *testing.T) {
	h := newHarness(t)
	e := entity{kind: entHunk, id: "f2:0", label: "go.mod @@ -1,3 +1,3 @@"}
	h.m.det.doc.hunks = []hunkRef{{id: "f2:0", path: "go.mod", text: goModHunk}}
	if got := h.m.openLine(e); got != 2 {
		t.Errorf("line %d, want 2", got)
	}
	if got := h.m.openLine(entity{kind: entFile, label: "go.mod"}); got != 0 {
		t.Errorf("line %d for a file, want 0", got)
	}
}

func TestZedDiffOfUncommittedFile(t *testing.T) {
	started := detached(t)
	setEditor(t, "", "zed", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	work := filepath.Join(h.m.client.Dir, "go.mod")
	if err := os.WriteFile(work, []byte("module x\ngo 1.22\nrequire y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.selectText("go.mod")
	h.keys("Z")

	if len(*started) != 1 {
		t.Fatalf("started %q", *started)
	}
	args := (*started)[0]
	if len(args) != 4 || args[0] != "zed" || args[1] != "--diff" || args[3] != work {
		t.Fatalf("args %q, want zed --diff <old> %s", args, work)
	}
	if !strings.HasSuffix(args[2], "/old/go.mod") {
		t.Errorf("old file %q", args[2])
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(filepath.Dir(args[2]))) })
	if b, _ := os.ReadFile(args[2]); string(b) != "module x\ngo 1.21\nrequire y\n" {
		t.Errorf("old content %q", b)
	}
	if h.m.busy != "" {
		t.Errorf("busy %q", h.m.busy)
	}
	h.wantScreen("Opened the diff of go.mod in Zed")
}

func TestZedDiffOfCommit(t *testing.T) {
	started := detached(t)
	setEditor(t, "", "zed", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("c1:x", "x.go", "@@ -0,0 +1,2 @@\n+package x\n+func A() {}\n"))
	h.selectText("first")
	h.keys("Z")
	// The test repository has no such commit, so x.go reads as empty and the diff does not fit: an error toast,
	// not a start.
	if len(*started) != 0 {
		t.Fatalf("started %q", *started)
	}
	h.wantScreen("x.go")
}

func TestZedDiffMissingZed(t *testing.T) {
	detached(t)
	setEditor(t, "", "vim", "")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	h.selectText("go.mod")
	t.Setenv("PATH", t.TempDir()) // after the harness: the fake `but` needs cat
	apps := editor.ZedApps
	editor.ZedApps = func() []string { return nil } // nor an installed Zed
	t.Cleanup(func() { editor.ZedApps = apps })
	h.keys("Z")
	h.wantScreen("cli: install")
}

// In Zed's terminal, D opens the diff in Zed rather than full screen; elsewhere it stays full screen.
func TestFullDetailsInZed(t *testing.T) {
	started := detached(t)
	setEditor(t, "", "", "zed")
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	if err := os.WriteFile(filepath.Join(h.m.client.Dir, "go.mod"), []byte("module x\ngo 1.22\nrequire y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.selectText("go.mod")
	h.keys("D")
	if len(*started) != 1 || (*started)[0][1] != "--diff" {
		t.Fatalf("started %q, want zed --diff", *started)
	}
	t.Cleanup(func() { _ = os.RemoveAll(filepath.Dir(filepath.Dir((*started)[0][2]))) })
	if h.m.det.full {
		t.Error("D went full screen in Zed")
	}

	t.Setenv("TERM_PROGRAM", "")
	h.keys("D")
	if len(*started) != 1 || !h.m.det.full {
		t.Fatalf("outside Zed: started %q, full %v", *started, h.m.det.full)
	}
}
