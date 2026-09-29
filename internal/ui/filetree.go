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
	focused   bool
	hidden    bool // switched off with T
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

func (t *fileTree) clamp() {
	t.cursor = clamp(t.cursor, 0, max(len(t.rows)-1, 0))
	h := max(t.height, 1)
	if t.cursor < t.offset {
		t.offset = t.cursor
	}
	if t.cursor >= t.offset+h {
		t.offset = t.cursor - h + 1
	}
	t.offset = clamp(t.offset, 0, max(len(t.rows)-h, 0))
}

func (t *fileTree) move(d int) {
	t.cursor += d
	t.clamp()
}

func (t *fileTree) scroll(d int) {
	t.offset = clamp(t.offset+d, 0, max(len(t.rows)-max(t.height, 1), 0))
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
	act := t.rowOf(active)
	for i := t.offset; i < min(t.offset+height-len(lines), len(t.rows)); i++ {
		r := t.rows[i]
		lines = append(lines, " "+r.render(width-1, t.focused && i == t.cursor, rowDeco{active: i == act}))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}
