package ui

import (
	"context"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

// buti views one worktree at a time: the workspace (m.worktree == "") or a linked worktree, which `but` runs in.
// Every `but status` reports all of them, so m.all keeps that, and m.status is the part being viewed.

type (
	// worktreePathsMsg brings where the worktrees are checked out, and then switches to the one named open.
	worktreePathsMsg struct {
		paths map[string]string
		err   error
		open  string
	}
	worktreeCreatedMsg struct {
		wt  *but.WorktreeListing
		err error
	}
	// worktreeChoice is a row of the worktree picker; an empty name is the workspace.
	worktreeChoice struct {
		name   string
		create bool
	}
)

// viewStatus keeps s and returns the part of it buti shows. ok is false when the worktree being viewed is gone.
func (m *Model) viewStatus(s *but.Status) (view *but.Status, ok bool) {
	m.all = s
	if m.worktree == "" {
		return s, true
	}
	view = s.InWorktree(m.worktree)
	return view, view != nil
}

// locateWorktrees asks `but worktree list` where worktrees are checked out, which `but status` doesn't say, when
// status shows other worktrees than last time. Then the picker opens at once.
func (m *Model) locateWorktrees() tea.Cmd {
	if m.all == nil {
		return nil
	}
	var names []string
	for _, wt := range m.all.Worktrees {
		names = append(names, wt.Name)
	}
	key := strings.Join(names, "\x00")
	if key == m.wtLocated {
		return nil
	}
	m.wtLocated = key
	return m.fetchWorktreePaths("")
}

func (m *Model) fetchWorktreePaths(open string) tea.Cmd {
	client := m.root
	return func() tea.Msg {
		l, err := client.Worktrees(context.Background())
		paths := map[string]string{}
		if err == nil {
			for _, w := range append(l.Active, l.Archived...) {
				paths[w.Name] = w.Path
			}
		}
		return worktreePathsMsg{paths: paths, err: err, open: open}
	}
}

func (m *Model) receiveWorktreePaths(msg worktreePathsMsg) tea.Cmd {
	if msg.err != nil {
		if msg.open == "" {
			m.wtLocated = "" // a background look: the next status tries again
			return nil
		}
		return m.notify(toastError, msg.err.Error())
	}
	m.wtPaths = msg.paths
	if msg.open == "" {
		return nil
	}
	if p := m.wtPaths[msg.open]; p != "" {
		return m.switchWorktree(msg.open, p)
	}
	return m.notify(toastError, "Can't find where worktree "+msg.open+" is checked out")
}

// worktreePicker lists the workspace and the worktrees to switch to.
func (m *Model) worktreePicker([]entity) tea.Cmd {
	current := 0
	items := []pickItem{{label: "⌂ workspace", detail: m.workspaceSummary(), value: worktreeChoice{}}}
	if m.all != nil {
		for _, wt := range m.all.Worktrees {
			if wt.Name == m.worktree {
				current = len(items)
			}
			items = append(items, pickItem{label: "⑂ " + wt.Name, detail: m.worktreeSummary(&wt), value: worktreeChoice{name: wt.Name}})
		}
	}
	items[current].detail = "current · " + items[current].detail
	items = append(items, pickItem{label: "+ New worktree…", detail: "but worktree new", value: worktreeChoice{create: true}})
	p := newPicker("Switch worktree", items, func(m *Model, it pickItem) tea.Cmd {
		c := it.value.(worktreeChoice)
		switch {
		case c.create:
			return m.promptNewWorktree()
		case c.name == m.worktree:
			return nil
		case m.busy != "":
			return m.notify(toastError, "Still working on: "+m.busy)
		case c.name != "" && m.wtPaths[c.name] == "":
			return m.fetchWorktreePaths(c.name) // created since the last look
		}
		return m.switchWorktree(c.name, m.wtPaths[c.name])
	})
	// Start on the next worktree, so w enter flips between two, or on New worktree when there is no other.
	p.cursor = len(items) - 1
	if n := len(items) - 1; n > 1 {
		p.cursor = (current + 1) % n
	}
	m.openModal(p)
	return nil
}

func (m *Model) promptNewWorktree() tea.Cmd {
	m.openModal(newPrompt("New worktree", "", "branch name, e.g. fix-login",
		"Checks the branch out in its own directory (but worktree new), then switches to it.",
		func(m *Model, name string) tea.Cmd {
			name = strings.TrimSpace(name)
			if name == "" {
				return nil
			}
			if m.busy != "" {
				return m.notify(toastError, "Still working on: "+m.busy)
			}
			m.busy = "New worktree " + name
			client := m.root
			return tea.Batch(m.spinner.Tick, func() tea.Msg {
				wt, err := client.NewWorktree(context.Background(), name)
				return worktreeCreatedMsg{wt: wt, err: err}
			})
		}))
	return nil
}

func (m *Model) handleWorktreeCreated(msg worktreeCreatedMsg) tea.Cmd {
	m.busy = ""
	if msg.err != nil {
		return m.notify(toastError, firstLines(msg.err.Error(), 6))
	}
	m.wtPaths[msg.wt.Name] = msg.wt.Path
	return m.switchWorktree(msg.wt.Name, msg.wt.Path)
}

// switchWorktree points buti at the worktree named name, checked out at path, or at the workspace when name is "".
func (m *Model) switchWorktree(name, path string) tea.Cmd {
	return m.switchWorktreeSaying(name, path, "")
}

// switchWorktreeSaying is switchWorktree with why it switched, for the toast.
func (m *Model) switchWorktreeSaying(name, path, why string) tea.Cmd {
	m.worktree = name
	m.client = m.root
	if name != "" {
		m.client = &but.Client{Dir: path, Bin: m.root.Bin}
	}
	m.marks, m.target = map[string]entity{}, nil
	m.focus, m.lane, m.laneCursor, m.laneOffset, m.fileCursor, m.fileOffset = focusLanes, 0, 0, 0, -1, 0
	m.det.focused, m.det.full = false, false
	m.commentsKey = ""
	// Show it at once from the status we have; the fetch brings its diffs and anything newer.
	if m.all != nil {
		if view, ok := m.viewStatus(m.all); ok {
			m.applyStatus(view)
		}
	}
	return tea.Batch(m.fetchStatus(), m.syncDetails(true), m.notify(toastInfo, why+"Viewing "+m.worktreeLabel()))
}

func (m *Model) worktreeLabel() string {
	if m.worktree == "" {
		return "the workspace"
	}
	return "worktree " + m.worktree
}

// headerHeight is 1 while there are worktrees to switch between, for the header that says which one is shown.
func (m *Model) headerHeight() int {
	if m.all == nil || m.resolving() || (len(m.all.Worktrees) == 0 && m.worktree == "") {
		return 0
	}
	return 1
}

func (m *Model) header() string {
	var chip, info string
	if m.worktree == "" {
		chip = lipgloss.NewStyle().Background(colorSurface).Foreground(colorText).Bold(true).Padding(0, 1).Render("⌂ workspace")
		info = m.workspaceSummary()
	} else {
		chip = lipgloss.NewStyle().Background(colorAccent).Foreground(colorText).Bold(true).Padding(0, 1).Render("⑂ " + m.worktree)
		if wt := m.all.Worktree(m.worktree); wt != nil {
			info = m.worktreeSummary(wt)
		}
		info += " · " + homeRel(m.client.Dir)
	}
	others := len(m.all.Worktrees) // the linked ones, plus the workspace, minus this one
	right := mutedStyle.Render(plural(others, "other worktree", "other worktrees")) + "  " + keyHints("w", "switch") + " "
	left := chip + " " + mutedStyle.Render(info)
	left = ansi.Truncate(left, max(m.width-ansi.StringWidth(right)-1, 0), "…")
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// workspaceSummary describes the workspace for the header and the picker, e.g. "3 stacks · 4 changes".
func (m *Model) workspaceSummary() string {
	if m.all == nil {
		return ""
	}
	return plural(len(m.all.Stacks), "stack", "stacks") + " · " + plural(len(m.all.UncommittedChanges), "change", "changes")
}

// worktreeSummary describes a worktree, e.g. "1 commit · 1 change · on api".
func (m *Model) worktreeSummary(wt *but.Worktree) string {
	commits := 0
	for _, b := range wt.Branches {
		commits += len(b.Commits)
	}
	parts := []string{plural(commits, "commit", "commits"), plural(len(wt.UncommittedChanges), "change", "changes")}
	switch {
	case wt.Base.InWorkspace:
		parts = append(parts, "on "+m.branchWithCommit(wt.Base.CommitID))
	case wt.Base.Behind > 0:
		parts = append(parts, "↓"+itoa(wt.Base.Behind)+" behind")
	}
	return strings.Join(parts, " · ")
}

// branchWithCommit names the workspace branch holding a commit, for where a worktree starts.
func (m *Model) branchWithCommit(id string) string {
	for _, st := range m.all.Stacks {
		for _, b := range st.Branches {
			for _, c := range b.Commits {
				if c.CommitID == id {
					return b.Name
				}
			}
		}
	}
	return "the workspace"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

// homeRel shortens a path under the home directory to ~/….
func homeRel(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}
