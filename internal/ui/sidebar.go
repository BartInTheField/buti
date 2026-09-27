package ui

import (
	"path"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

// fileRow is one visible line of the unstaged file tree.
type fileRow struct {
	depth     int
	name      string
	path      string // directory path, or file path for files
	isDir     bool
	collapsed bool
	change    but.Change
	ids       []string // for directories: CLI ids of every file below
}

// Change types of the rows for conflicted files, which have no CLI id.
const (
	changeConflicted = "conflicted"
	changeResolved   = "resolved"
)

func (r fileRow) entity() entity {
	if r.isDir {
		return entity{kind: entDir, ids: r.ids, label: r.path + "/"}
	}
	if t := r.change.ChangeType; t == changeConflicted || t == changeResolved {
		return entity{kind: entConflict, label: r.path, status: t}
	}
	return entity{kind: entFile, id: r.change.CliID, label: r.path}
}

type dirNode struct {
	dirs  map[string]*dirNode
	files []but.Change
}

func newDirNode() *dirNode { return &dirNode{dirs: map[string]*dirNode{}} }

func (n *dirNode) allIDs() []string {
	var ids []string
	for _, f := range n.files {
		ids = append(ids, f.CliID)
	}
	for _, d := range n.dirs {
		ids = append(ids, d.allIDs()...)
	}
	return ids
}

// buildFileRows turns flat file changes into a directory tree, directories first,
// compacting single-child directory chains the way the desktop app does ("cmd/buti").
func buildFileRows(changes []but.Change, collapsed map[string]bool) []fileRow {
	root := newDirNode()
	for _, c := range changes {
		parts := strings.Split(c.FilePath, "/")
		n := root
		for _, p := range parts[:len(parts)-1] {
			child, ok := n.dirs[p]
			if !ok {
				child = newDirNode()
				n.dirs[p] = child
			}
			n = child
		}
		n.files = append(n.files, c)
	}

	var rows []fileRow
	var walk func(n *dirNode, prefix string, depth int)
	walk = func(n *dirNode, prefix string, depth int) {
		names := make([]string, 0, len(n.dirs))
		for name := range n.dirs {
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			d, label := n.dirs[name], name
			for len(d.files) == 0 && len(d.dirs) == 1 {
				for k, only := range d.dirs {
					label += "/" + k
					d = only
				}
			}
			p := prefix + label
			rows = append(rows, fileRow{depth: depth, name: label, path: p, isDir: true, collapsed: collapsed[p], ids: d.allIDs()})
			if !collapsed[p] {
				walk(d, p+"/", depth+1)
			}
		}
		slices.SortFunc(n.files, func(a, b but.Change) int { return strings.Compare(a.FilePath, b.FilePath) })
		for _, f := range n.files {
			rows = append(rows, fileRow{depth: depth, name: path.Base(f.FilePath), path: f.FilePath, change: f})
		}
	}
	walk(root, "", 0)
	return rows
}

// conflictRows lists the files of a commit in edit mode, conflicted first.
func conflictRows(conflicted, resolved []string) []fileRow {
	var rows []fileRow
	for _, g := range []struct {
		paths []string
		typ   string
	}{{conflicted, changeConflicted}, {resolved, changeResolved}} {
		for _, p := range slices.Sorted(slices.Values(g.paths)) {
			rows = append(rows, fileRow{name: p, path: p, change: but.Change{FilePath: p, ChangeType: g.typ}})
		}
	}
	return rows
}

func (r fileRow) render(width int, selected bool, d rowDeco) string {
	indent := strings.Repeat("  ", r.depth)
	if r.isDir {
		arrow := mutedStyle.Render("▾")
		if r.collapsed {
			arrow = mutedStyle.Render("▸")
		}
		if d.marked {
			arrow = markGlyph
		}
		line := ansi.Truncate(indent+arrow+" "+r.name+"/", width, "…")
		return dimmed(pad(line, width, selected), d)
	}
	letter, st := changeTypeStyle(r.change.ChangeType)
	lead := "  "
	if d.marked {
		lead = markGlyph + " "
	}
	return dimmed(fileLine(indent+lead, r.name, withCount(st.Render(letter), d.comments), width, selected), d)
}

// fileLine renders "<prefix><name> … <tag>" with the tag flush right.
func fileLine(prefix, name, tag string, width int, selected bool) string {
	avail := width - ansi.StringWidth(prefix) - ansi.StringWidth(tag) - 1
	name = ansi.Truncate(name, max(avail, 1), "…")
	gap := max(width-ansi.StringWidth(prefix)-ansi.StringWidth(name)-ansi.StringWidth(tag), 1)
	return pad(prefix+name+strings.Repeat(" ", gap)+tag, width, selected)
}

// pad fills a line to width; a selected line is rendered inverted.
func pad(line string, width int, selected bool) string {
	if w := ansi.StringWidth(line); w < width {
		line += strings.Repeat(" ", width-w)
	}
	if selected {
		return selectedStyle.Render(ansi.Strip(line))
	}
	return line
}
