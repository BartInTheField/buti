package ui

import (
	"context"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

var (
	colorWorktree = lipgloss.Color("#38BDF8")
	worktreeStyle = lipgloss.NewStyle().Foreground(colorWorktree).Bold(true)
	worktreeGlyph = "⌂"
)

// worktreeEntity is a worktree as a subject: its uncommitted changes, and the place to open a shell in. Its id is
// the worktree's CLI id, which `but` gives its top branch too, so commands get its directory instead (worktreeArg).
func worktreeEntity(wt *but.Worktree) entity {
	e := entity{kind: entWorktree, id: wt.CliID, label: wt.Name, worktree: wt.Name, dir: wt.Path, stack: wt.CliID}
	for _, c := range wt.UncommittedChanges {
		e.ids = append(e.ids, c.CliID)
	}
	return e
}

// worktreeBase says where a worktree's branch starts: behind the target, or on a branch of the workspace.
func worktreeBase(s *but.Status, wt *but.Worktree) string {
	switch {
	case wt.Base.Behind > 0:
		return "↓" + itoa(wt.Base.Behind) + " behind"
	case wt.Base.InWorkspace:
		for _, st := range s.Stacks {
			for _, b := range st.Branches {
				for _, c := range b.Commits {
					if c.CommitID == wt.Base.CommitID {
						return "on " + b.Name
					}
				}
			}
		}
	}
	return ""
}

func worktreeLanes(s *but.Status, showFiles func(*but.Commit) bool) []lane {
	var lanes []lane
	for i := range s.Worktrees {
		wt := &s.Worktrees[i]
		head := worktreeEntity(wt)
		base := entity{worktree: wt.Name, dir: wt.Path, stack: wt.CliID}
		l := lane{stack: but.Stack{CliID: wt.CliID, Branches: wt.Branches}, wt: wt, wtBase: worktreeBase(s, wt)}
		l.items = append(l.items, head)
		for _, c := range wt.UncommittedChanges {
			f := base
			f.kind, f.id, f.label = entFile, c.CliID, c.FilePath
			l.items = append(l.items, f)
		}
		l.addBranches(showFiles, base)
		lanes = append(lanes, l)
	}
	return lanes
}

// tildePath shortens a path under the home directory to ~/….
func tildePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// truncatePath shortens a path in the middle, keeping its start and the end that tells worktrees apart.
func truncatePath(s string, width int) string {
	if ansi.StringWidth(s) <= width {
		return s
	}
	head, _, _ := strings.Cut(s, "/")
	r := []rune(s)
	for len(r) > 0 && ansi.StringWidth(head)+2+ansi.StringWidth(string(r)) > width {
		r = r[1:]
	}
	return head + "/…" + string(r)
}

// worktreeCard is the top of a worktree's lane, where a stack has its commit box: the worktree's name, base and
// path, its uncommitted changes and a commit button. It returns the card, the item on each of its lines and the
// line of the selection (-1 when it is not in the card).
func (l lane) worktreeCard(width, sel int, deco decoFunc) (string, map[int]int, int) {
	inner := width - 4
	wt := l.wt
	head := l.items[0]
	hd := deco(head)
	selLine := -1
	lineItems := map[int]int{}

	name := worktreeStyle.Render(worktreeGlyph+" ") + headerStyle.Render(ansi.Truncate(wt.Name, max(inner-4-ansi.StringWidth(l.wtBase), 1), "…"))
	if sel == 0 {
		name = selectedStyle.Render(worktreeGlyph + " " + ansi.Truncate(wt.Name, max(inner-4-ansi.StringWidth(l.wtBase), 1), "…"))
		selLine = 1
	}
	header := name
	if l.wtBase != "" && hd.tag == "" && !hd.source {
		st := mutedStyle
		if wt.Base.Behind > 0 {
			st = errorStyle
		}
		header += strings.Repeat(" ", max(inner-ansi.StringWidth(header)-ansi.StringWidth(l.wtBase), 1)) + st.Render(l.wtBase)
	}
	body := []string{dimmed(withTag(header, inner, hd), hd)}
	lineItems[1] = 0
	path := "path unknown"
	if wt.Path != "" {
		path = tildePath(wt.Path)
	}
	body = append(body, mutedStyle.Render(truncatePath(path, inner)))
	lineItems[2] = 0

	if len(wt.UncommittedChanges) == 0 {
		body = append(body, mutedStyle.Render(ansi.Truncate("No uncommitted changes", inner, "…")))
	}
	for i, c := range wt.UncommittedChanges {
		item := 1 + i
		d := deco(l.items[item])
		if item == sel {
			selLine = 1 + len(body)
		}
		letter, st := changeTypeStyle(c.ChangeType)
		prefix := ""
		if d.marked {
			prefix = markGlyph + " "
		}
		lineItems[1+len(body)] = item
		body = append(body, dimmed(withTag(fileLine(prefix, c.FilePath, withCount(st.Render(letter), d.comments), inner, item == sel), inner, d), d))
	}
	body = append(body, buttonStyle.Width(inner).Align(lipgloss.Center).Render("Start a commit…"))
	lineItems[len(body)] = startCommitHit

	card := cardStyle.BorderForeground(colorWorktree)
	switch {
	case hd.tag != "":
		card = card.BorderForeground(colorAccent).BorderStyle(lipgloss.ThickBorder())
	case selLine >= 0:
		card = card.BorderForeground(colorAccent)
	}
	return card.Width(width).Render(strings.Join(body, "\n")), lineItems, selLine
}

// Lane captions: with worktrees, a line above the lanes says which are the workspace's and which are worktrees.

// firstWorktreeLane is the index of the first worktree lane, or len(m.lanes) when there is none.
func (m *Model) firstWorktreeLane() int {
	for i, l := range m.lanes {
		if l.wt != nil {
			return i
		}
	}
	return len(m.lanes)
}

// captionHeight is the lines the captions take above the lanes: 1 with worktrees, else 0.
func (m *Model) captionHeight() int {
	if m.firstWorktreeLane() < len(m.lanes) {
		return 1
	}
	return 0
}

// captions draws "Workspace ───" over the visible stacks and "⌂ Worktrees ───" over the visible worktrees.
func (m *Model) captions() string {
	lw, visible := m.laneLayout()
	f := m.firstWorktreeLane()
	end := min(m.laneOffset+visible, len(m.lanes))
	caption := func(text string, from, to int, st, rule lipgloss.Style) string {
		w := (to-from)*(lw+1) - 1
		label := st.Render(" " + text + " ")
		return label + rule.Render(strings.Repeat("─", max(w-ansi.StringWidth(label), 0))) + " "
	}
	var line string
	if m.laneOffset < f {
		line += caption("Workspace", m.laneOffset, min(f, end), mutedStyle.Bold(true), dividerStyle)
	}
	if f < end {
		line += caption(worktreeGlyph+" Worktrees", max(f, m.laneOffset), end, worktreeStyle, lipgloss.NewStyle().Foreground(colorWorktree).Faint(true))
	}
	// Worktrees past the right edge would otherwise go unnoticed with a few stacks open.
	if hidden := len(m.lanes) - max(end, f); hidden > 0 {
		what := pluralize(hidden, "worktree")
		if f < end {
			what = itoa(hidden) + " more"
		}
		hint := worktreeStyle.Render(worktreeGlyph + " " + what + " →")
		line = ansi.Truncate(line, max(m.rightWidth()-ansi.StringWidth(hint)-1, 0), "")
		line += strings.Repeat(" ", max(m.rightWidth()-ansi.StringWidth(line)-ansi.StringWidth(hint)-1, 0)) + hint
	}
	return line
}

// Actions.

func (m *Model) worktreeChanges(name string) []string {
	if m.status == nil {
		return nil
	}
	var ids []string
	if wt := m.status.Worktree(name); wt != nil {
		for _, c := range wt.UncommittedChanges {
			ids = append(ids, c.CliID)
		}
	}
	return ids
}

// uncommittedIn reports whether the workspace (wt "") or a worktree has uncommitted changes.
func uncommittedIn(m *Model, wt string) bool {
	if wt == "" {
		return hasUncommitted(m)
	}
	return len(m.worktreeChanges(wt)) > 0
}

func (m *Model) openWorktreeShell(sel []entity) tea.Cmd {
	e := sel[0]
	if e.dir == "" {
		return m.notify(toastError, "Don't know where worktree "+e.label+" is")
	}
	// `but` drops a worktree whose directory is gone on the next reload; until then, say so instead of failing to
	// start the shell.
	if _, err := os.Stat(e.dir); err != nil {
		return tea.Batch(m.notify(toastError, "Worktree "+e.label+" is gone: "+tildePath(e.dir)+" doesn't exist"), m.fetchStatus())
	}
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "sh"
	}
	cmd := exec.Command(sh)
	cmd.Dir = e.dir
	return m.execInteractive("Shell in "+e.label, keepSelection, cmd)
}

// worktreeArg names a worktree to `but worktree`: by its directory, which can't be mistaken for a branch.
func worktreeArg(e entity) string {
	if e.dir != "" {
		return e.dir
	}
	return e.label
}

func (m *Model) archiveWorktree(sel []entity) tea.Cmd {
	name, arg := sel[0].label, worktreeArg(sel[0])
	return m.runOp("Archive worktree "+name, keepSelection, func(ctx context.Context, c *but.Client) error {
		return c.WorktreeArchive(ctx, arg)
	})
}

func (m *Model) removeWorktree(sel []entity) tea.Cmd {
	e := sel[0]
	body := "Deletes " + tildePath(e.dir) + ". Its branch stays."
	force := len(e.ids) > 0
	if force {
		body += "\n\n" + errorStyle.Render(pluralize(len(e.ids), "uncommitted change")+" in it are thrown away.")
	}
	m.openModal(&confirmModal{title: "Remove worktree " + e.label + "?", body: body, yesLabel: "remove", onYes: func(m *Model) tea.Cmd {
		return m.runOp("Remove worktree "+e.label, keepSelection, func(ctx context.Context, c *but.Client) error {
			return c.WorktreeRemove(ctx, worktreeArg(e), force)
		})
	}})
	return nil
}

// newWorktree checks a new branch out in a worktree: on top of the selected workspace branch, or at the base.
func (m *Model) newWorktree(sel []entity) tea.Cmd {
	above, where := "", "at the workspace base"
	if len(sel) == 1 && sel[0].kind == entBranch && sel[0].worktree == "" {
		above, where = sel[0].branch, "on top of "+sel[0].branch
	}
	m.openModal(newPrompt("New worktree "+where, "", "branch name, empty for a generated one", "Checks the branch out in its own directory, next to the workspace.", func(m *Model, name string) tea.Cmd {
		name = strings.Join(strings.Fields(name), "-")
		if name != "" {
			m.pendingKey = "worktree:" + name
		}
		return m.runOp("Create worktree "+name, keepSelection, func(ctx context.Context, c *but.Client) error {
			return c.WorktreeNew(ctx, name, above)
		})
	}))
	return nil
}
