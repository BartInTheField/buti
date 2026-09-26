package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
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

	want := renderDiff(d.data, 60, diffOpts{selected: 2, marked: map[string]bool{"f0": true}}).String()
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

// j and k scroll through a hunk taller than the pane before moving on.
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

	for range 56 { // 90 content lines in a 34 line pane
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
	for _, r := range rows[len(rows)-1-detailsTail : len(rows)-1] { // above the footer
		if strings.TrimSpace(r) != "" {
			t.Fatalf("no blank space below the end of the diff:\n%s", h.screen())
		}
	}
	h.keys("k")
	for range 60 {
		h.keys("k")
	}
	if h.m.det.hunk != 0 || !strings.Contains(h.screen(), "line 1 ") {
		t.Fatalf("k did not scroll back to the top of the tall hunk:\n%s", h.screen())
	}
}
