package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

func testDiff(files, hunks int) *but.Diff {
	d := &but.Diff{}
	for f := range files {
		fd := but.FileDiff{ID: fmt.Sprintf("f%d", f), Path: fmt.Sprintf("file%d.go", f), Status: "modified"}
		fd.Diff.Type = "patch"
		for h := range hunks {
			start := h*10 + 1
			fd.Diff.Hunks = append(fd.Diff.Hunks, but.Hunk{
				OldStart: start, OldLines: 1, NewStart: start, NewLines: 1,
				Diff: fmt.Sprintf("@@ -%d,1 +%d,1 @@\n-old\n+new\n", start, start),
			})
		}
		d.Changes = append(d.Changes, fd)
	}
	return d
}

// Moving the hunk selection redraws in place; the viewport must show it.
func TestSelectHunkRedrawsInPlace(t *testing.T) {
	d := newDetails()
	d.visible, d.focused = true, true
	d.data = testDiff(3, 1)
	d.setSize(60, 20)
	d.selectHunk(0, nil)
	d.selectHunk(2, map[string]bool{"f0": true})

	want := renderDiff(d.data, 60, diffOpts{selected: 2, marked: map[string]bool{"f0": true}, cursor: d.cursor, lines: true}).String()
	if got := strings.Join(d.lines[:len(d.lines)-detailsTail], "\n"); got != want {
		t.Fatalf("redrawn content differs from a full render:\n%s\n--- want ---\n%s", got, want)
	}
	view := ansi.Strip(d.vp.View())
	if !strings.Contains(view, "✔ @@ -1,1") || !strings.Contains(view, "▶ @@ -1,1") {
		t.Fatalf("view does not show the mark and selection:\n%s", view)
	}
}

// Rendering the pane must not rebuild the diff: View runs on a copy of the model.
func TestViewDoesNotRerender(t *testing.T) {
	d := newDetails()
	d.visible = true
	d.data = testDiff(2, 2)
	d.setSize(59, 19)
	before := d.layoutD
	d.layoutD = nil // any rebuild would set it again
	d.view(60, 20)
	if d.layoutD != nil || before == nil {
		t.Fatal("view rebuilt the diff layout")
	}
}

// j and k move the line cursor through a hunk taller than the pane, scrolling with it.
func TestStepScrollsThroughTallHunk(t *testing.T) {
	h := newHarness(t)
	h.keys("D")
	var b strings.Builder
	b.WriteString("@@ -1,0 +1,88 @@\n")
	for i := 1; i <= 88; i++ {
		fmt.Fprintf(&b, "+line %d\n", i)
	}
	fd := but.FileDiff{ID: "x", Path: "CLAUDE.md", Status: "added"}
	fd.Diff.Type = "patch"
	fd.Diff.Hunks = []but.Hunk{{NewStart: 1, NewLines: 88, Diff: b.String()}}
	short := but.FileDiff{ID: "y", Path: "go.mod", Status: "modified"}
	short.Diff.Type = "patch"
	short.Diff.Hunks = []but.Hunk{{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Diff: "@@ -1,1 +1,1 @@\n-a\n+b\n"}}
	h.send(detailsMsg{key: h.m.det.reqKey, diff: &but.Diff{Changes: []but.FileDiff{fd, short}}})

	for range 88 { // the first j puts the cursor on line 1; 90 content lines in a 34 line pane
		h.keys("j")
	}
	if h.m.det.hunk != 0 || !strings.Contains(h.screen(), "line 88") {
		t.Fatalf("j did not scroll to the end of the tall hunk (hunk %d):\n%s", h.m.det.hunk, h.screen())
	}
	h.keys("j")
	if h.m.det.hunk != 1 {
		t.Fatalf("j at the end of a hunk selected hunk %d, want 1", h.m.det.hunk)
	}
	for range 10 {
		h.keys("J")
	}
	rows := strings.Split(h.screen(), "\n")
	for _, r := range rows[len(rows)-1-detailsTail : len(rows)-1] { // above the footer, next to the file tree
		if strings.Trim(r, " │") != "" {
			t.Fatalf("no blank space below the end of the diff:\n%s", h.screen())
		}
	}
	for range 90 {
		h.keys("k")
	}
	if h.m.det.hunk != 0 || !strings.Contains(h.screen(), "line 1 ") {
		t.Fatalf("k did not scroll back to the top of the tall hunk:\n%s", h.screen())
	}
}

// commitDiff is a two file commit diff: a.go with a context line, a removal and
// two additions; b.go with one hunk after an unchanged stretch.
func commitDiff() *but.Diff {
	a := but.FileDiff{Path: "a.go", Status: "modified"}
	a.Diff.Type = "patch"
	a.Diff.Hunks = []but.Hunk{{OldStart: 3, OldLines: 2, NewStart: 3, NewLines: 3,
		Diff: "@@ -3,2 +3,3 @@\n ctx\n-gone\n+one\n+two\n"}}
	b := but.FileDiff{Path: "b.go", Status: "modified"}
	b.Diff.Type = "patch"
	b.Diff.Hunks = []but.Hunk{{OldStart: 10, OldLines: 1, NewStart: 10, NewLines: 1,
		Diff: "@@ -10,1 +10,1 @@\n-x := 1\n+x := 2\n"}}
	return &but.Diff{Changes: []but.FileDiff{a, b}}
}

func (h *harness) lineSel() lineSel {
	h.t.Helper()
	s, ok := h.m.det.lineSelection()
	if !ok {
		h.t.Fatalf("no line selection:\n%s", h.screen())
	}
	return s
}

// j and k move the line cursor line by line, across files, and report where it is.
func TestLineCursorLocation(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("D")
	h.send(detailsMsg{key: h.m.det.reqKey, diff: commitDiff()})
	h.keys("j") // the cursor starts on the first line
	want := []struct {
		path string
		side review.Side
		line int
		text string
	}{
		{"a.go", review.SideNew, 3, "ctx"},
		{"a.go", review.SideOld, 4, "gone"},
		{"a.go", review.SideNew, 4, "one"},
		{"a.go", review.SideNew, 5, "two"},
		{"b.go", review.SideOld, 10, "x := 1"},
		{"b.go", review.SideNew, 10, "x := 2"},
	}
	check := func(i int) {
		t.Helper()
		w, s := want[i], h.lineSel()
		if s.path != w.path || s.side != w.side || s.line != w.line || s.endLine != w.line || s.text != w.text {
			t.Fatalf("line %d: got %s %s %d-%d %q, want %s %s %d %q", i, s.path, s.side, s.line, s.endLine, s.text,
				w.path, w.side, w.line, w.text)
		}
	}
	for i := range want {
		if i > 0 {
			h.keys("j")
		}
		check(i)
	}
	h.keys("j") // past the last line it scrolls; the cursor stays
	check(len(want) - 1)
	for i := len(want) - 2; i >= 0; i-- {
		h.keys("k")
		check(i)
	}
	if h.m.det.hunk != 0 {
		t.Fatalf("hunk %d, want 0", h.m.det.hunk)
	}
	h.keys("]")
	check(4)
	h.keys("[")
	check(0)
}

// v selects a range that moving the cursor extends, within the hunk; esc cancels it.
func TestLineRangeSelection(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("D")
	h.send(detailsMsg{key: h.m.det.reqKey, diff: commitDiff()})
	h.keys("j", "v", "j", "j")
	s := h.lineSel()
	if s.path != "a.go" || s.side != review.SideNew || s.line != 3 || s.endLine != 4 || s.text != "ctx\none" {
		t.Fatalf("range with a removed line: %+v", s)
	}
	h.keys("j", "j", "j") // stops at the end of the hunk
	if s := h.lineSel(); s.path != "a.go" || s.line != 3 || s.endLine != 5 {
		t.Fatalf("range left the hunk: %+v", s)
	}
	if !strings.Contains(h.screen(), "▌┃") || !strings.Contains(h.screen(), "esc cancel range") {
		t.Fatalf("range not drawn:\n%s", h.screen())
	}
	h.keys("esc")
	if s := h.lineSel(); s.line != 5 || s.endLine != 5 || !h.m.det.focused {
		t.Fatalf("esc did not just cancel the range: %+v focused %v", s, h.m.det.focused)
	}
	h.keys("k", "k", "v", "k") // old lines only: the range is on the old side
	h.keys("j")
	if s := h.lineSel(); s.side != review.SideOld || s.line != 4 || s.endLine != 4 || s.text != "gone" {
		t.Fatalf("range on a removed line: %+v", s)
	}
	h.keys("v")
	if h.m.det.anchor >= 0 {
		t.Fatal("v did not end the range")
	}
}

// Clicking a line moves the cursor there; shift-click extends a range.
func TestClickLine(t *testing.T) {
	h := newHarness(t)
	h.selectText("first")
	h.keys("D")
	h.send(detailsMsg{key: h.m.det.reqKey, diff: commitDiff()})
	h.click("x := 2")
	if s := h.lineSel(); s.path != "b.go" || s.side != review.SideNew || s.line != 10 {
		t.Fatalf("click on b.go line 10: %+v", s)
	}
	h.click("ctx")
	x, y := h.find("two")
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft, Mod: tea.ModShift})
	if s := h.lineSel(); s.path != "a.go" || s.line != 3 || s.endLine != 5 {
		t.Fatalf("shift-click range: %+v", s)
	}
}

// treeDiff is a diff of files in two directories, for the file tree.
func treeDiff() *but.Diff {
	d := testDiff(3, 1)
	for i, p := range []string{"pkg/a.go", "pkg/b.go", "main.go"} {
		d.Changes[i].Path = p
	}
	return d
}

// Full screen, the details pane lists its files in a tree, and moving through it jumps the diff.
func TestFullDetailsFileTree(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(treeDiff())
	h.selectText("Unstaged")
	h.keys("D")
	for _, want := range []string{"Files", "pkg/", "a.go", "b.go", "main.go"} {
		if !strings.Contains(h.screen(), want) {
			t.Fatalf("%q not on the full screen:\n%s", want, h.screen())
		}
	}
	if got := h.m.det.activeFile(); got != "pkg/a.go" {
		t.Fatalf("active file %q, want pkg/a.go", got)
	}

	h.keys("tab") // the tree takes focus, on the active file
	if !h.m.det.tree.focused || h.m.det.tree.cursor != 1 {
		t.Fatalf("tab should focus the tree on pkg/a.go: focused=%v cursor=%d", h.m.det.tree.focused, h.m.det.tree.cursor)
	}
	h.keys("j")
	if got := h.m.det.activeFile(); got != "pkg/b.go" || h.m.det.hunk != 1 {
		t.Fatalf("j should jump to pkg/b.go: active %q, hunk %d", got, h.m.det.hunk)
	}
	h.keys("G", "enter") // main.go, and back to the diff
	if got := h.m.det.activeFile(); got != "main.go" || h.m.det.tree.focused {
		t.Fatalf("enter should jump to main.go and leave the tree: active %q, focused %v", got, h.m.det.tree.focused)
	}

	h.keys("tab", "g", "enter") // fold pkg/
	if len(h.m.det.tree.rows) != 2 || !strings.Contains(h.screen(), "▸ pkg/") {
		t.Fatalf("enter on a directory should fold it:\n%s", h.screen())
	}
	h.click("main.go")
	if got := h.m.det.activeFile(); got != "main.go" {
		t.Fatalf("clicking a file should jump to it: active %q", got)
	}

	before := h.m.det.treeWidth()
	h.keys("-")
	if h.m.det.treeWidth() != before-4 || h.m.det.width != 139-(before-4)-1 { // the pane is a column narrower than the screen
		t.Fatalf("- should narrow the tree by 4: tree %d, diff %d", h.m.det.treeWidth(), h.m.det.width)
	}
	x, y := h.find("│")
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.send(tea.MouseMotionMsg{X: 50, Y: y, Button: tea.MouseLeft})
	if h.m.det.treeWidth() != before-4 {
		t.Fatal("the diff should not be re-laid out while the divider is dragged")
	}
	h.send(tea.MouseReleaseMsg{X: 50, Y: y, Button: tea.MouseLeft})
	if h.m.det.treeWidth() != 50 || h.m.det.width != 139-50-1 {
		t.Fatalf("dragging the divider to column 50 gave tree %d, diff %d", h.m.det.treeWidth(), h.m.det.width)
	}

	h.keys("T")
	if strings.Contains(h.screen(), "Files") {
		t.Fatal("T should hide the tree")
	}
	h.keys("T")
	h.send(tea.WindowSizeMsg{Width: 80, Height: 36})
	if strings.Contains(h.screen(), "Files") {
		t.Fatal("a narrow screen should hide the tree")
	}
	h.keys("esc")
	if h.m.det.full {
		t.Fatal("esc should leave full screen")
	}
}
