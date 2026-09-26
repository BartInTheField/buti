package ui

import (
	"context"
	"path"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

// resolving reports whether a conflicted commit is checked out in edit mode.
// `but status` then shows only its files, so editView replaces the workspace.
func (m *Model) resolving() bool { return m.status != nil && m.status.Resolving != nil }

func conflicted(e entity) bool {
	return e.kind == entCommit && e.commit != nil && e.commit.Conflicted != nil && *e.commit.Conflicted
}

// resolve checks a conflicted commit out in edit mode, to fix its files in an editor.
func (m *Model) resolve(sel []entity) tea.Cmd {
	e := sel[0]
	c := *e.commit
	m.editing = &c
	return m.runOp("Edit "+e.describe(), keepSelection, func(ctx context.Context, c *but.Client) error {
		return c.ResolveStart(ctx, e.id)
	})
}

func (m *Model) openConflicted([]entity) tea.Cmd {
	var paths []string
	for _, p := range m.status.Resolving.Conflicted {
		paths = append(paths, m.pathOf(entity{kind: entConflict, label: p}))
	}
	return m.execInteractive("Open conflicted files", keepSelection, editorCommand(paths...))
}

func (m *Model) saveAndExit([]entity) tea.Cmd {
	save := func(m *Model) tea.Cmd {
		return m.runOp("Save and exit edit mode", keepSelection, func(ctx context.Context, c *but.Client) error {
			return c.ResolveFinish(ctx)
		})
	}
	if left := m.status.Resolving.Conflicted; len(left) > 0 {
		var es []entity
		for _, p := range left {
			es = append(es, entity{kind: entConflict, label: p})
		}
		m.openModal(&confirmModal{title: "Save with conflict markers left?", yesLabel: "save and exit",
			body:  "These files still have markers, which would be committed as they are:\n\n" + listLabels(es),
			onYes: save})
		return nil
	}
	return save(m)
}

func (m *Model) cancelEdit([]entity) tea.Cmd {
	m.openModal(&confirmModal{title: "Leave edit mode without saving?", yesLabel: "leave",
		body: "The commit stays conflicted, and edits to its files are dropped.",
		onYes: func(m *Model) tea.Cmd {
			return m.runOp("Cancel edit mode", keepSelection, func(ctx context.Context, c *but.Client) error {
				return c.ResolveCancel(ctx, true)
			})
		}})
	return nil
}

// editLayout is the edit mode screen, with what is clickable where.
type editLayout struct {
	lines   []string
	left    int         // column the content starts at
	files   map[int]int // line -> file row
	buttons []editButton
}

type editButton struct {
	y, x0, x1 int
	key       string // the action's key
}

// editView lays out edit mode like the desktop app: the commit, its files, and
// the ways out.
func (m *Model) editView() editLayout {
	w := min(m.width-4, 80)
	inner := w - 4 // card border and padding
	l := editLayout{left: max((m.width-w)/2, 0), files: map[int]int{}}
	add := func(block string) int {
		start := len(l.lines)
		l.lines = append(l.lines, strings.Split(block, "\n")...)
		return start
	}

	add("")
	title := "You are editing a conflicted commit"
	if c := m.editing; c != nil {
		title = "You are editing commit " + buttonStyle.Render(c.ShortID())
	}
	add(headerStyle.Render(title))
	add("")
	if c := m.editing; c != nil {
		subject := c.Subject()
		if subject == "" {
			subject = mutedStyle.Italic(true).Render("(no message)")
		}
		add(cardStyle.Width(w).Render(ansi.Truncate(subject, inner, "…") + "\n" +
			mutedStyle.Render(ansi.Truncate(c.ShortID()+" · "+c.AuthorName, inner, "…"))))
	}

	pill := func(label string, bg lipgloss.Style) string {
		return bg.Foreground(lipgloss.Color("#1C1917")).Padding(0, 1).Render(label)
	}
	body := []string{headerStyle.Render("Commit files") + " " + countStyle.Render(itoa(len(m.files))), ""}
	for i, f := range m.files {
		tag := pill("Conflicted", lipgloss.NewStyle().Background(colorDel))
		if f.change.ChangeType == changeResolved {
			tag = pill("Resolved", lipgloss.NewStyle().Background(colorAdd))
		}
		name := path.Base(f.path)
		if dir := path.Dir(f.path); dir != "." {
			name += " " + mutedStyle.Render(dir)
		}
		if m.fileCursor == i {
			name = selectedStyle.Render(ansi.Strip(name))
		}
		body = append(body, fileLine("", name, tag, inner, false))
	}
	start := add(cardStyle.Width(w).Render(strings.Join(body, "\n")))
	for i := range m.files {
		l.files[start+3+i] = i // past the border, the heading and the blank line
	}

	add("")
	add(mutedStyle.Render("Fix the files in your editor; this view updates as their markers go."))
	add(mutedStyle.Render("To exit edit mode, save and exit or cancel."))
	add("")
	btns := []struct {
		key, label string
		st         lipgloss.Style
	}{
		{"x", "Cancel", buttonStyle},
		{"o", "Open conflicted files", buttonStyle.Background(colorBorder)},
		{"e", "Save and exit ✓", lipgloss.NewStyle().Background(colorPushed).Foreground(lipgloss.Color("#1C1917")).Bold(true).Padding(0, 1)},
	}
	if len(m.status.Resolving.Conflicted) == 0 {
		btns = append(btns[:1], btns[2]) // nothing left to open
	}
	var parts []string
	x := 0
	for _, b := range btns {
		p := b.st.Render(b.key + " " + b.label)
		parts = append(parts, p)
		l.buttons = append(l.buttons, editButton{key: b.key, x0: x, x1: x + ansi.StringWidth(p)})
		x += ansi.StringWidth(p) + 1
	}
	row := strings.Join(parts, " ")
	shift := max(w-ansi.StringWidth(row), 0) // right-aligned under the cards
	y := add(strings.Repeat(" ", shift) + row)
	for i := range l.buttons {
		b := &l.buttons[i]
		b.y, b.x0, b.x1 = y, b.x0+shift, b.x1+shift
	}
	return l
}

func (l editLayout) render(width, height int) string {
	margin := strings.Repeat(" ", l.left)
	lines := make([]string, len(l.lines))
	for i, s := range l.lines {
		lines[i] = margin + s
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

// hit maps a screen position to a file row or a button's key.
func (l editLayout) hit(x, y int) (row int, key string) {
	if r, ok := l.files[y]; ok {
		return r, ""
	}
	for _, b := range l.buttons {
		if y == b.y && x >= l.left+b.x0 && x < l.left+b.x1 {
			return -1, b.key
		}
	}
	return -1, ""
}
