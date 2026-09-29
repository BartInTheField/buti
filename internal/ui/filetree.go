package ui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/bartinthefield/buti/internal/but"
)

// Narrower screens keep the whole width for the diff.
const treeMinWidth = 90

// fileTree lists the files of the diff shown full screen, next to it, to jump between them.
type fileTree struct {
	rows      []fileRow
	n         int // files listed, folded ones included
	collapsed map[string]bool
	cursor    int
	offset    int
	height    int // rows that fit
	width     int // set by dragging the divider or +/-; 0 for the default
	dragW     int // where the divider is being dragged to; 0 when it is not
	focused   bool
	hidden    bool   // switched off with T
	shown     string // the active file last scrolled into view, so the wheel can scroll away from it
}

// build lists the diff's files, one row per path, keeping the cursor where it was.
func (t *fileTree) build(d *but.Diff) {
	prev := ""
	if t.cursor >= 0 && t.cursor < len(t.rows) {
		prev = t.rows[t.cursor].path
	}
	var changes []but.Change
	seen := map[string]bool{}
	if d != nil {
		for _, f := range d.Changes {
			if !seen[f.Path] { // uncommitted diffs list one entry per hunk
				seen[f.Path] = true
				changes = append(changes, but.Change{FilePath: f.Path, ChangeType: f.Status})
			}
		}
	}
	if t.collapsed == nil {
		t.collapsed = map[string]bool{}
	}
	t.rows, t.n = buildFileRows(changes, t.collapsed), len(changes)
	t.cursor = t.rowOf(prev)
	t.clamp()
}

// rowOf is the row showing path (a file or a directory), or the folded directory holding the file, or -1.
func (t *fileTree) rowOf(path string) int {
	if path == "" {
		return -1
	}
	for i, r := range t.rows {
		if r.path == path || r.isDir && r.collapsed && strings.HasPrefix(path, r.path+"/") {
			return i
		}
	}
	return -1
}

// clamp keeps the cursor on a row and in view, a row clear of the "more" markers at the edges.
func (t *fileTree) clamp() {
	t.cursor = clamp(t.cursor, 0, max(len(t.rows)-1, 0))
	t.offset = t.showing(t.cursor)
}

// showing is the offset that shows row, a row clear of the markers.
func (t *fileTree) showing(row int) int {
	h, off := max(t.height, 1), t.offset
	if row < off+1 {
		off = row - 1
	}
	if row >= off+h-1 {
		off = row - h + 2
	}
	return clamp(off, 0, max(len(t.rows)-h, 0))
}

// reveal scrolls the active file into view when it changed, as the diff moves through the files.
func (t *fileTree) reveal(active string) {
	if active == t.shown {
		return
	}
	t.shown = active
	if i := t.rowOf(active); i >= 0 {
		t.offset = t.showing(i)
	}
}

// more reports how many rows are cut off above and below, each marker's own row included.
func (t *fileTree) more() (above, below int) {
	if above = t.offset; above > 0 {
		above++
	}
	if below = max(len(t.rows)-t.offset-max(t.height, 1), 0); below > 0 {
		below++
	}
	return above, below
}

func (t *fileTree) move(d int) {
	t.cursor += d
	t.clamp()
}

func (t *fileTree) scroll(d int) {
	t.offset = clamp(t.offset+d, 0, max(len(t.rows)-max(t.height, 1), 0))
}

// marker reports whether row is drawn as a "more" marker rather than a file.
func (t *fileTree) marker(row int) bool {
	above, below := t.more()
	return row == t.offset && above > 0 || row == t.offset+max(t.height, 1)-1 && below > 0
}

// focus gives the tree focus, with the cursor on the active file.
func (t *fileTree) focus(active string) {
	t.focused = true
	if i := t.rowOf(active); i >= 0 {
		t.cursor = i
	}
	t.clamp()
}

// file is the path under the cursor, when it is on a file.
func (t *fileTree) file() (string, bool) {
	if t.cursor < 0 || t.cursor >= len(t.rows) || t.rows[t.cursor].isDir {
		return "", false
	}
	return t.rows[t.cursor].path, true
}

// toggle folds or unfolds the directory under the cursor, reporting whether there was one.
func (t *fileTree) toggle(d *but.Diff) bool {
	if t.cursor < 0 || t.cursor >= len(t.rows) || !t.rows[t.cursor].isDir {
		return false
	}
	p := t.rows[t.cursor].path
	t.collapsed[p] = !t.collapsed[p]
	t.build(d)
	return true
}

// render draws the tree with a header, marking the active file (the one the diff is at).
func (t *fileTree) render(width, height int, active string) string {
	title := headerStyle.Render("Files")
	if t.focused {
		title = titleStyle.Render("Files")
	}
	lines := []string{" " + title + " " + countStyle.Render(itoa(t.n)), dividerStyle.Render(strings.Repeat("─", width))}
	act, end := t.rowOf(active), min(t.offset+height-len(lines), len(t.rows))
	above, below := t.more()
	for i := t.offset; i < end; i++ {
		switch r := t.rows[i]; {
		case i == t.offset && above > 0:
			lines = append(lines, mutedStyle.Render(" ↑ "+itoa(above)+" more"))
		case i == end-1 && below > 0:
			lines = append(lines, mutedStyle.Render(" ↓ "+itoa(below)+" more"))
		default:
			lines = append(lines, " "+r.render(width-1, t.focused && i == t.cursor, rowDeco{active: i == act}))
		}
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}
