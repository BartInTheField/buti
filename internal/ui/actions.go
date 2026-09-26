package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
)

// action is a command available from a key, the palette, the help and the context menu.
type action struct {
	key    string
	title  string
	group  string
	global bool   // not about the selection; left out of the context menu
	hint   string // status bar label; set, the action is hinted before the others
	// resolving: also offered in edit mode, which hides the others (the
	// workspace is set aside, so they would act on nothing or get in the way).
	resolving bool
	when      func(m *Model, sel []entity) bool
	run       func(m *Model, sel []entity) tea.Cmd
}

// offers reports whether a is available for sel in the current mode.
func (m *Model) offers(a action, sel []entity) bool {
	return (a.resolving || !m.resolving()) && a.when(m, sel)
}

func always(*Model, []entity) bool { return true }

func one(kinds ...entityKind) func(*Model, []entity) bool {
	return func(m *Model, sel []entity) bool {
		return len(sel) == 1 && len(m.marks) == 0 && allOf(sel, kinds...)
	}
}

func any_(kinds ...entityKind) func(*Model, []entity) bool {
	return func(_ *Model, sel []entity) bool { return allOf(sel, kinds...) }
}

func hasUncommitted(m *Model) bool {
	if m.status == nil {
		return false
	}
	if len(m.status.UncommittedChanges) > 0 {
		return true
	}
	for _, st := range m.status.Stacks {
		if len(st.AssignedChanges) > 0 {
			return true
		}
	}
	return false
}

func verbAvailable(v verb) func(*Model, []entity) bool {
	return func(m *Model, sel []entity) bool {
		if _, why := sourcesFor(v, sel); why != "" {
			return false
		}
		return v != verbCommit || hasUncommitted(m)
	}
}

var actions []action

func init() {
	actions = []action{
		// Changes and commits.
		{key: "c", title: "Commit…", group: "Commit", when: verbAvailable(verbCommit),
			run: func(m *Model, sel []entity) tea.Cmd { return m.enterTarget(verbCommit, sel) }},
		{key: "r", title: "Squash / amend…", group: "Commit", when: verbAvailable(verbSquash),
			run: func(m *Model, sel []entity) tea.Cmd { return m.enterTarget(verbSquash, sel) }},
		{key: "R", title: "Amend all changes into this", group: "Commit",
			when: func(m *Model, sel []entity) bool {
				return one(entCommit, entBranch)(m, sel) && hasUncommitted(m) && (sel[0].kind == entCommit || m.branchHasCommits(sel[0]))
			},
			run: func(m *Model, sel []entity) tea.Cmd {
				t := sel[0]
				return m.runOp("Amend all changes into "+t.describe(), keepSelection, func(ctx context.Context, c *but.Client) error {
					return c.Amend(ctx, t.id, nil)
				})
			}},
		{key: "A", title: "Absorb into matching commits", group: "Commit",
			when: func(m *Model, sel []entity) bool {
				return hasUncommitted(m) && any_(entArea, entDir, entFile, entHunk)(m, sel)
			},
			run: (*Model).absorb},
		{key: "n", title: "Insert empty commit", group: "Commit", when: one(entBranch, entCommit),
			run: func(m *Model, sel []entity) tea.Cmd {
				at := but.Placement{Branch: sel[0].branch}
				if sel[0].kind == entCommit {
					at = but.Placement{Above: sel[0].id}
				}
				return m.runOp("Insert empty commit", selectNew, func(ctx context.Context, c *but.Client) error {
					return c.EmptyCommit(ctx, "", at)
				})
			}},
		{key: "enter", title: "Reword / rename", group: "Commit", when: one(entCommit, entBranch), run: (*Model).reword},
		{key: "M", title: "Reword in $EDITOR", group: "Commit", when: one(entCommit),
			run: func(m *Model, sel []entity) tea.Cmd {
				return m.execInteractive("Reword "+sel[0].describe(), keepSelection, m.client.Command("reword", sel[0].id))
			}},
		{key: "x", title: "Discard…", group: "Commit",
			when: func(m *Model, sel []entity) bool {
				return len(sel) > 0 && !allOf(sel, entNewBranch) && (sel[0].kind != entArea || hasUncommitted(m))
			},
			run: (*Model).discard},

		// Conflicts: e enters edit mode on a conflicted commit, and saves and exits it.
		{key: "e", title: "Resolve in edit mode", group: "Conflicts", hint: "resolve",
			when: func(m *Model, sel []entity) bool { return one(entCommit)(m, sel) && conflicted(sel[0]) },
			run:  (*Model).resolve},
		{key: "e", title: "Save and exit", group: "Conflicts", hint: "save and exit", resolving: true,
			when: func(m *Model, _ []entity) bool { return m.resolving() }, run: (*Model).saveAndExit},
		{key: "o", title: "Open conflicted files", group: "Conflicts", hint: "open conflicted", resolving: true,
			when: func(m *Model, _ []entity) bool { return m.resolving() && len(m.status.Resolving.Conflicted) > 0 },
			run:  (*Model).openConflicted},
		{key: "x", title: "Cancel editing…", group: "Conflicts", hint: "cancel", resolving: true,
			when: func(m *Model, _ []entity) bool { return m.resolving() }, run: (*Model).cancelEdit},

		// Branches and history.
		{key: "m", title: "Move…", group: "Branch", when: verbAvailable(verbMove),
			run: func(m *Model, sel []entity) tea.Cmd { return m.enterTarget(verbMove, sel) }},
		{key: "p", title: "Cherry-pick…", group: "Branch", when: verbAvailable(verbPick),
			run: func(m *Model, sel []entity) tea.Cmd { return m.enterTarget(verbPick, sel) }},
		{key: "b", title: "New branch…", group: "Branch", when: always, run: (*Model).newBranch},
		{key: "B", title: "New branch below…", group: "Branch", when: one(entBranch),
			run: func(m *Model, sel []entity) tea.Cmd {
				return m.promptNewBranch(but.Placement{Below: sel[0].id}, "below "+sel[0].label)
			}},
		{key: "P", title: "Push branch", group: "Branch", when: one(entBranch, entCommit), run: (*Model).push},
		{key: "N", title: "Create pull request", group: "Branch", when: one(entBranch),
			run: func(m *Model, sel []entity) tea.Cmd {
				b := sel[0].branch
				return m.runOp("Create pull request for "+b, keepSelection, func(ctx context.Context, c *but.Client) error {
					return c.PRNew(ctx, b, "", false)
				})
			}},
		{key: "o", title: "Open pull request", group: "Branch", hint: "open PR",
			when: func(m *Model, sel []entity) bool { return one(entBranch)(m, sel) && m.branchPR(sel[0]) != "" },
			run:  (*Model).openPR},
		{key: "a", title: "Apply branch…", group: "Branch", global: true, when: always, run: (*Model).applyPicker},
		{key: "S", title: "Unapply stack", group: "Branch", when: one(entBranch, entCommit),
			run: func(m *Model, sel []entity) tea.Cmd {
				b := sel[0].branch
				return m.runOp("Unapply "+b, keepSelection, func(ctx context.Context, c *but.Client) error { return c.Unapply(ctx, b) })
			}},
		{key: "", title: "Land branch onto target…", group: "Branch", when: one(entBranch),
			run: func(m *Model, sel []entity) tea.Cmd {
				b := sel[0].branch
				m.openModal(&confirmModal{title: "Land " + b + "?", yesLabel: "land",
					body: "Merges " + b + " straight into the target branch and pushes it.",
					onYes: func(m *Model) tea.Cmd {
						return m.runOp("Land "+b, selectArea, func(ctx context.Context, c *but.Client) error {
							_, err := c.Exec(ctx, "land", b, "--yes")
							return err
						})
					}})
				return nil
			}},
		{key: "L", title: "Pull (update from upstream)", group: "Branch", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd {
				return m.runOp("Pull upstream changes", keepSelection, func(ctx context.Context, c *but.Client) error { return c.Pull(ctx) })
			}},
		{key: "", title: "Clean up empty branches", group: "Branch", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd {
				return m.runOpOut("Clean up empty branches", keepSelection, func(ctx context.Context, c *but.Client) (string, error) {
					return c.Exec(ctx, "clean")
				})
			}},
		{key: "u", title: "Undo", group: "History", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd {
				return m.runOpOut("Undo", keepSelection, func(ctx context.Context, c *but.Client) (string, error) { return c.Exec(ctx, "undo") })
			}},
		{key: "U", title: "Redo", group: "History", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd {
				return m.runOpOut("Redo", keepSelection, func(ctx context.Context, c *but.Client) (string, error) { return c.Exec(ctx, "redo") })
			}},
		{key: "H", title: "Operation history…", group: "History", global: true, when: always, run: (*Model).oplogPicker},

		// Selection and view.
		{key: "space", title: "Mark / unmark", group: "View", when: func(m *Model, _ []entity) bool { return m.selected().valid() }, run: (*Model).toggleMark},
		{key: "f", title: "Show files in commit", group: "View", when: one(entCommit, entCommittedFile), run: (*Model).toggleCommitFiles},
		{key: "F", title: "Show files in all commits", group: "View", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd { m.showAllFiles = !m.showAllFiles; m.rebuild(); return nil }},
		{key: "d", title: "Toggle details pane", group: "View", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd {
				m.det.visible = !m.det.visible
				m.det.focused = false
				return m.syncDetails(true)
			}},
		{key: "D", title: "Full-screen details", group: "View", global: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd { return m.setDetailsFull(!m.det.full) }},
		{key: "o", title: "Open in $EDITOR", group: "View", when: one(entFile, entCommittedFile, entHunk),
			run: func(m *Model, sel []entity) tea.Cmd {
				return m.execInteractive("Edit "+sel[0].label, keepSelection, editorCommand(m.pathOf(sel[0])))
			}},
		{key: "O", title: "Open with default app", group: "View", when: one(entFile, entCommittedFile, entHunk),
			run: func(m *Model, sel []entity) tea.Cmd {
				if err := openExternal(m.pathOf(sel[0])); err != nil {
					return m.notify(toastError, err.Error())
				}
				return m.notify(toastInfo, "Opened "+sel[0].label)
			}},
		{key: "y", title: "Copy", group: "View", when: one(entBranch, entCommit, entFile, entCommittedFile, entHunk), run: (*Model).copyQuick},
		{key: "Y", title: "Copy…", group: "View", when: one(entBranch, entCommit, entFile, entCommittedFile, entHunk), run: (*Model).copyPicker},
		{key: "/", title: "Go to…", group: "View", global: true, when: always, run: (*Model).gotoPicker},
		{key: "t", title: "Go to branch…", group: "View", global: true, when: always, run: (*Model).branchPicker},
		{key: ":", title: "Run a but command…", group: "View", global: true, resolving: true, when: always, run: (*Model).butPrompt},
		{key: "!", title: "Run a shell command…", group: "View", global: true, resolving: true, when: always, run: (*Model).shellPrompt},
		{key: "ctrl+r", title: "Reload", group: "View", global: true, resolving: true, when: always,
			run: func(m *Model, _ []entity) tea.Cmd { return tea.Batch(m.fetchStatus(), m.syncDetails(true)) }},
		{title: "Version", group: "View", global: true, resolving: true, when: always, run: (*Model).showVersion},
		{title: "Update buti", group: "View", global: true, resolving: true, when: always, run: (*Model).checkUpdateNow},
		{key: ".", title: "Actions for selection…", group: "View", global: true, resolving: true, when: always, run: (*Model).contextMenu},
		{key: "?", title: "Help & all commands", group: "View", global: true, resolving: true, when: always, run: (*Model).helpPalette},
		{key: "ctrl+p", title: "Command palette", group: "View", global: true, resolving: true, when: always, run: (*Model).palette},
	}
}

// navHelp documents keys handled directly by the model.
var navHelp = [][2]string{
	{"j/k ↑/↓", "move"}, {"h/l ←/→", "previous / next column"}, {"tab", "cycle sidebar, lanes, details"},
	{"J/K", "next / previous branch"}, {"g/G", "top / bottom"}, {"ctrl+d/u", "move 10 rows"},
	{"+/-", "resize details"}, {"esc", "back: clear marks, leave mode, close"}, {"q", "quit"},
	{"click", "select · double-click: diff · right-click: actions"}, {"drag", "drop onto a branch, commit or “new branch”"},
}

func (m *Model) actionFor(key string, sel []entity) (action, bool) {
	for _, a := range actions {
		if a.key == key && m.offers(a, sel) {
			return a, true
		}
	}
	return action{}, false
}

func (m *Model) available(sel []entity, includeGlobal bool) []action {
	var out []action
	for _, a := range actions {
		if (includeGlobal || !a.global) && m.offers(a, sel) {
			out = append(out, a)
		}
	}
	return out
}

func actionItems(as []action) []pickItem {
	items := make([]pickItem, len(as))
	for i, a := range as {
		items[i] = pickItem{label: a.title, detail: a.key, value: a}
	}
	return items
}

func (m *Model) runPicked(it pickItem) tea.Cmd {
	a, ok := it.value.(action)
	if !ok {
		return nil
	}
	sel := m.subjects()
	if !m.offers(a, sel) {
		return m.notify(toastInfo, a.title+" is not available for "+describeSel(sel))
	}
	return a.run(m, sel)
}

func describeSel(sel []entity) string {
	if len(sel) == 0 {
		return "nothing"
	}
	return describeAll(sel)
}

func (m *Model) palette(sel []entity) tea.Cmd {
	m.openModal(newPicker("Command palette", actionItems(m.available(sel, true)), (*Model).runPicked))
	return nil
}

func (m *Model) contextMenu(sel []entity) tea.Cmd {
	as := m.available(sel, false)
	if len(as) == 0 {
		return m.notify(toastInfo, "No actions for "+describeSel(sel))
	}
	p := newPicker("Actions · "+describeSel(sel), actionItems(as), (*Model).runPicked)
	p.maxRows = 20
	m.openModal(p)
	return nil
}

func (m *Model) helpPalette(sel []entity) tea.Cmd {
	var items []pickItem
	for _, g := range []string{"Commit", "Conflicts", "Branch", "History", "View"} {
		for _, a := range actions {
			if a.group != g {
				continue
			}
			items = append(items, pickItem{label: g + " · " + a.title, detail: a.key, value: a, dim: !m.offers(a, sel)})
		}
	}
	for _, n := range navHelp {
		items = append(items, pickItem{label: "Navigate · " + n[1], detail: n[0]})
	}
	p := newPicker("Help · type to search, enter runs", items, (*Model).runPicked)
	p.maxRows = 24
	m.openModal(p)
	return nil
}

// enterTarget starts picking a target for verb v with the given subjects.
func (m *Model) enterTarget(v verb, sel []entity) tea.Cmd {
	srcs, why := sourcesFor(v, sel)
	if why != "" {
		return m.notify(toastInfo, why)
	}
	m.target = newTargetMode(v, srcs, m.status)
	m.det.focused = false
	if m.det.full {
		m.det.full = false
		m.det.visible = true
	}
	if m.focus == focusFiles && len(m.lanes) > 0 {
		// Targets live in the lanes; start on the first branch.
		m.focus, m.laneCursor = focusLanes, 0
		for i, e := range m.lanes[m.lane].items {
			if e.kind == entBranch {
				m.laneCursor = i
				break
			}
		}
		m.clampLanes()
	}
	return nil
}

// branchPR is the pull request label ("#3") of a branch entity, or "".
func (m *Model) branchPR(e entity) string {
	if m.status == nil {
		return ""
	}
	for _, st := range m.status.Stacks {
		for _, b := range st.Branches {
			if b.Name == e.branch {
				return b.PR()
			}
		}
	}
	return ""
}

func (m *Model) openPR(sel []entity) tea.Cmd {
	client, branch, pr := m.client, sel[0].branch, m.branchPR(sel[0])
	return func() tea.Msg {
		url, err := client.ReviewURL(context.Background(), branch)
		return prURLMsg{branch: branch, pr: pr, url: url, err: err}
	}
}

type prURLMsg struct {
	branch, pr, url string
	err             error
}

func (m *Model) showPR(msg prURLMsg) tea.Cmd {
	switch {
	case msg.err != nil:
		return m.notify(toastError, msg.err.Error())
	case msg.url == "":
		return m.notify(toastError, "No pull request found for "+msg.branch)
	}
	if err := openURL(msg.url); err != nil {
		return m.notify(toastError, err.Error())
	}
	return m.notify(toastInfo, "Opened "+msg.pr+" in the browser")
}

func (m *Model) branchHasCommits(e entity) bool {
	return len((&targetMode{status: m.status}).branchCommits(e)) > 0
}

func (m *Model) reword(sel []entity) tea.Cmd {
	e := sel[0]
	if e.kind == entBranch {
		m.openModal(newPrompt("Rename branch", e.branch, "branch-name", "Spaces become dashes.", func(m *Model, name string) tea.Cmd {
			name = strings.Join(strings.Fields(name), "-")
			if name == "" || name == e.branch {
				return nil
			}
			return m.runOp("Rename "+e.branch+" → "+name, keepSelection, func(ctx context.Context, c *but.Client) error {
				return c.Reword(ctx, e.id, name)
			})
		}))
		m.pendingKey = "branch:" + e.branch // updated on submit
		return nil
	}
	return m.openComposer(composeReword, "Reword "+e.describe(), e.commit.Message, func(m *Model, msg string) tea.Cmd {
		if strings.TrimSpace(msg) == strings.TrimSpace(e.commit.Message) {
			return nil
		}
		if strings.TrimSpace(msg) == "" {
			return m.notify(toastError, "A commit message can't be empty; use Discard to drop the commit.")
		}
		return m.runOp("Reword commit", keepSelection, func(ctx context.Context, c *but.Client) error {
			return c.Reword(ctx, e.id, msg)
		})
	})
}

func (m *Model) discard(sel []entity) tea.Cmd {
	if !allOf(sel, entNewBranch) && len(sel) > 1 {
		for _, e := range sel[1:] {
			if e.kind.markClass() != sel[0].kind.markClass() {
				return m.notify(toastError, "Can't discard a mix of "+sel[0].kind.String()+"s and "+e.kind.String()+"s")
			}
		}
	}
	var title, body string
	ids := collectIDs(sel)
	e := sel[0]
	switch {
	case e.kind == entArea:
		title, body, ids = "Discard all uncommitted changes?", "Every uncommitted change in the workspace is thrown away.", nil
	case len(sel) > 1:
		title, body = "Discard "+describeAll(sel)+"?", listLabels(sel)
	case e.kind == entBranch:
		title, body = "Discard branch "+e.branch+"?", "The branch and all of its commits are removed from the workspace."
	case e.kind == entCommit:
		title, body = "Discard commit "+e.commit.ShortID()+"?", "“"+e.label+"” and its changes are removed."
	case e.kind == entCommittedFile:
		title, body = "Discard changes to "+e.label+"?", "The changes to this file are removed from commit "+e.commit.ShortID()+"."
	default:
		title, body = "Discard "+e.describe()+"?", "The uncommitted changes are thrown away."
	}
	body += "\n\n" + mutedStyle.Render("You can bring it back with undo (u).")
	m.openModal(&confirmModal{title: title, body: body, yesLabel: "discard", onYes: func(m *Model) tea.Cmd {
		return m.runOp(strings.TrimSuffix(title, "?"), keepSelection, func(ctx context.Context, c *but.Client) error {
			return c.Discard(ctx, ids)
		})
	}})
	return nil
}

func listLabels(es []entity) string {
	var lines []string
	for i, e := range es {
		if i == 8 {
			lines = append(lines, fmt.Sprintf("… and %d more", len(es)-i))
			break
		}
		lines = append(lines, "• "+e.label)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) absorb(sel []entity) tea.Cmd {
	var ids []string
	if len(sel) != 1 || sel[0].kind != entArea {
		ids = collectIDs(sel)
	}
	return m.runOpOut("Absorb "+describeSel(sel), keepSelection, func(ctx context.Context, c *but.Client) (string, error) {
		if len(ids) == 0 {
			return "", c.Absorb(ctx, "")
		}
		for _, id := range ids {
			if err := c.Absorb(ctx, id); err != nil {
				return "", err
			}
		}
		return "", nil
	})
}

func (m *Model) newBranch(sel []entity) tea.Cmd {
	if len(sel) == 1 && len(m.marks) == 0 && (sel[0].kind == entBranch || sel[0].kind == entCommit) {
		b := sel[0].branch
		for _, l := range m.lanes {
			for _, e := range l.items {
				if e.kind == entBranch && e.branch == b {
					return m.promptNewBranch(but.Placement{Above: e.id}, "stacked on "+b)
				}
			}
		}
	}
	return m.promptNewBranch(but.Placement{}, "as a new lane")
}

func (m *Model) promptNewBranch(at but.Placement, where string) tea.Cmd {
	m.openModal(newPrompt("New branch "+where, "", "leave empty for a generated name", "Spaces become dashes.", func(m *Model, name string) tea.Cmd {
		name = strings.Join(strings.Fields(name), "-")
		return m.runOp("Create branch "+name, selectNew, func(ctx context.Context, c *but.Client) error {
			return c.BranchNew(ctx, name, at)
		})
	}))
	return nil
}

func (m *Model) push(sel []entity) tea.Cmd {
	b := sel[0].branch
	status := ""
	for _, l := range m.lanes {
		for _, e := range l.items {
			if e.kind == entBranch && e.branch == b {
				status = e.status
			}
		}
	}
	if status == "nothingToPush" || status == "integrated" {
		return m.notify(toastInfo, b+" has nothing to push")
	}
	run := func(m *Model, force bool) tea.Cmd {
		return m.runOp("Push "+b, keepSelection, func(ctx context.Context, c *but.Client) error { return c.Push(ctx, b, force) })
	}
	if status == "unpushedCommitsRequiringForce" {
		m.openModal(&confirmModal{title: "Force push " + b + "?", yesLabel: "force push",
			body: "The remote branch has diverged; pushing rewrites it.", onYes: func(m *Model) tea.Cmd { return run(m, true) }})
		return nil
	}
	return run(m, false)
}

func (m *Model) applyPicker([]entity) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		bs, err := client.Branches(context.Background())
		return branchesMsg{branches: bs, err: err}
	}
}

type branchesMsg struct {
	branches *but.Branches
	err      error
}

func (m *Model) showApplyPicker(msg branchesMsg) tea.Cmd {
	if msg.err != nil {
		return m.notify(toastError, msg.err.Error())
	}
	list := slices.Clone(msg.branches.Branches)
	slices.SortFunc(list, func(a, b but.BranchListing) int { return int(b.LastCommitAt - a.LastCommitAt) })
	var items []pickItem
	for _, b := range list {
		where := "remote"
		if b.HasLocal {
			where = "local"
		}
		detail := where
		if b.LastCommitAt > 0 {
			detail += " · " + relTime(time.UnixMilli(b.LastCommitAt))
		}
		items = append(items, pickItem{label: b.Name, detail: detail, value: b.Name})
	}
	p := newPicker("Apply branch", items, func(m *Model, it pickItem) tea.Cmd {
		name := it.value.(string)
		return m.runOp("Apply "+name, selectNew, func(ctx context.Context, c *but.Client) error { return c.Apply(ctx, name) })
	})
	p.empty = "No unapplied branches"
	m.openModal(p)
	return nil
}

func relTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("2 Jan 2006")
}

type oplogMsg struct {
	entries []but.OplogEntry
	err     error
}

func (m *Model) oplogPicker([]entity) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		es, err := client.Oplog(context.Background())
		return oplogMsg{entries: es, err: err}
	}
}

func (m *Model) showOplog(msg oplogMsg) tea.Cmd {
	if msg.err != nil {
		return m.notify(toastError, msg.err.Error())
	}
	var items []pickItem
	for _, e := range msg.entries {
		title := e.Details.Title
		if title == "" {
			title = e.Details.Operation
		}
		items = append(items, pickItem{label: humanizeOp(title), detail: e.ID[:7] + " · " + relTime(time.UnixMilli(e.CreatedAt)), value: e})
	}
	p := newPicker("Operation history · enter restores", items, func(m *Model, it pickItem) tea.Cmd {
		e := it.value.(but.OplogEntry)
		m.openModal(&confirmModal{title: "Restore to before “" + humanizeOp(e.Details.Title) + "”?", yesLabel: "restore",
			body: "The workspace goes back to snapshot " + e.ID[:7] + ". This is itself recorded, so it can be undone.",
			onYes: func(m *Model) tea.Cmd {
				return m.runOp("Restore snapshot "+e.ID[:7], keepSelection, func(ctx context.Context, c *but.Client) error {
					return c.OplogRestore(ctx, e.ID)
				})
			}})
		return nil
	})
	p.empty = "No operations recorded yet"
	p.maxRows = 20
	m.openModal(p)
	return nil
}

// humanizeOp turns "CreateCommit" into "Create commit".
func humanizeOp(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteRune(' ')
			r += 'a' - 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (m *Model) pathOf(e entity) string {
	p := e.label
	if e.kind == entHunk {
		p, _, _ = strings.Cut(e.label, " ")
	}
	if m.client.Dir == "" || strings.HasPrefix(p, "/") {
		return p
	}
	return m.client.Dir + "/" + p
}

func (m *Model) copyQuick(sel []entity) tea.Cmd {
	e := sel[0]
	text := e.label
	switch e.kind {
	case entBranch:
		text = e.branch
	case entCommit:
		text = e.commit.ShortID()
		if len(e.commit.ChangeID) >= 8 {
			text = e.commit.ChangeID[:8]
		}
	case entHunk:
		if h, ok := m.det.selectedHunk(); ok {
			text = h.text
		}
	}
	return tea.Batch(tea.SetClipboard(text), m.notify(toastSuccess, "Copied "+firstLines(text, 1)))
}

func (m *Model) copyPicker(sel []entity) tea.Cmd {
	e := sel[0]
	var items []pickItem
	add := func(label, value string) {
		if value != "" {
			items = append(items, pickItem{label: label, detail: firstLines(value, 1), value: value})
		}
	}
	switch e.kind {
	case entCommit:
		c := e.commit
		add("Commit ID", c.CommitID)
		add("Short commit ID", c.ShortID())
		add("Change ID", c.ChangeID)
		add("Message title", c.Subject())
		add("Whole message", strings.TrimSpace(c.Message))
		add("Author", c.AuthorName+" <"+c.AuthorEmail+">")
	case entBranch:
		add("Branch name", e.branch)
		add("Short ID", e.id)
	default:
		add("File path", m.pathOf(e))
		add("Relative path", e.label)
		add("Short ID", e.id)
	}
	m.openModal(newPicker("Copy", items, func(m *Model, it pickItem) tea.Cmd {
		return tea.Batch(tea.SetClipboard(it.value.(string)), m.notify(toastSuccess, "Copied "+strings.ToLower(it.label)))
	}))
	return nil
}

// gotoPicker is a fuzzy "go to anything" over files, branches and commits.
func (m *Model) gotoPicker([]entity) tea.Cmd {
	var items []pickItem
	if m.status != nil {
		items = append(items, pickItem{label: "Unstaged changes", detail: "zz", value: entity{kind: entArea}})
		for _, r := range buildFileRows(m.status.UncommittedChanges, map[string]bool{}) {
			if !r.isDir {
				items = append(items, pickItem{label: "  " + r.path, detail: r.change.CliID, value: r.entity()})
			}
		}
	}
	for _, l := range m.lanes {
		for _, e := range l.items {
			switch e.kind {
			case entBranch:
				items = append(items, pickItem{label: "⑂ " + e.branch, detail: e.id, value: e})
			case entCommit:
				items = append(items, pickItem{label: "  ● " + e.label, detail: e.id + " " + e.commit.ShortID(), value: e})
			case entFile:
				items = append(items, pickItem{label: "  " + e.label, detail: e.id, value: e})
			}
		}
	}
	m.openModal(newPicker("Go to", items, func(m *Model, it pickItem) tea.Cmd { return m.selectEntity(it.value.(entity)) }))
	return nil
}

func (m *Model) branchPicker([]entity) tea.Cmd {
	items := []pickItem{{label: "Unstaged changes", detail: "zz", value: entity{kind: entArea}}}
	for _, l := range m.lanes {
		for _, e := range l.items {
			if e.kind == entBranch {
				items = append(items, pickItem{label: e.branch, detail: humanizeOp(strings.ToUpper(e.status[:1]) + e.status[1:]), value: e})
			}
		}
	}
	m.openModal(newPicker("Go to branch", items, func(m *Model, it pickItem) tea.Cmd { return m.selectEntity(it.value.(entity)) }))
	return nil
}

func (m *Model) butPrompt([]entity) tea.Cmd {
	m.openModal(newPrompt("Run a but command", "", "e.g. branch list", "Runs in the repository; output is shown afterwards.", func(m *Model, line string) tea.Cmd {
		args, err := splitArgs(strings.TrimPrefix(strings.TrimSpace(line), "but "))
		if err != nil {
			return m.notify(toastError, err.Error())
		}
		if len(args) == 0 {
			return nil
		}
		client := m.client
		return func() tea.Msg {
			out, err := client.Exec(context.Background(), args...)
			return commandMsg{title: "but " + strings.Join(args, " "), output: out, err: err}
		}
	}))
	return nil
}

type commandMsg struct {
	title, output string
	err           error
}

func (m *Model) shellPrompt([]entity) tea.Cmd {
	m.openModal(newPrompt("Run a shell command", "", "e.g. git log --oneline -5", "The TUI is suspended while it runs.", func(m *Model, line string) tea.Cmd {
		if strings.TrimSpace(line) == "" {
			return nil
		}
		return m.execInteractive("$ "+line, keepSelection, shellCommand(m.client.Dir, line))
	}))
	return nil
}
