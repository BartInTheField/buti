package ui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/bartinthefield/buti/internal/but"
)

type verb int

const (
	verbCommit verb = iota
	verbSquash
	verbMove
	verbPick
)

func (v verb) String() string { return [...]string{"commit", "squash", "move", "pick"}[v] }

func (v verb) color() lipgloss.Style {
	c := [...]lipgloss.Style{
		lipgloss.NewStyle().Background(colorAdd),
		lipgloss.NewStyle().Background(lipgloss.Color("#60A5FA")),
		lipgloss.NewStyle().Background(colorPushed),
		lipgloss.NewStyle().Background(lipgloss.Color("#F472B6")),
	}[v]
	return c.Foreground(lipgloss.Color("#1C1917")).Bold(true).Padding(0, 1)
}

const (
	sideAbove = -1
	sideBelow = 1
)

// targetMode is active while the user picks where sources go.
type targetMode struct {
	verb      verb
	sources   []entity
	side      int
	useTarget bool // squash: keep the target's message
	emptyMsg  bool // commit: skip the message composer

	status *but.Status // kept current by the model, for branch commit lookups
}

// plan is what confirming a target would do.
type plan struct {
	label  string // short, for the target's tag
	desc   string // sentence for the status bar
	insert int    // insertion marker relative to the target row
	run    func(m *Model) tea.Cmd
}

// sourcesFor turns the subjects into sources for a verb, or explains why not.
func sourcesFor(v verb, subjects []entity) ([]entity, string) {
	if len(subjects) == 0 {
		return nil, "nothing selected"
	}
	switch v {
	case verbCommit:
		if allOf(subjects, entArea, entDir, entFile, entHunk) {
			return subjects, ""
		}
		// On a branch or commit, commit everything uncommitted (like `but tui`).
		return []entity{{kind: entArea, id: "zz", label: "all changes"}}, ""
	case verbSquash:
		if allOf(subjects, entArea, entDir, entFile, entHunk) || allOf(subjects, entCommit) ||
			allOf(subjects, entBranch) || allOf(subjects, entCommittedFile) {
			return subjects, ""
		}
		return nil, "can't squash " + subjects[0].kind.String() + "s"
	case verbMove:
		if allOf(subjects, entCommit) || (len(subjects) == 1 && subjects[0].kind == entBranch) {
			return subjects, ""
		}
		if allOf(subjects, entBranch) {
			return nil, "branches can only be moved one at a time"
		}
		return nil, "only commits and branches can be moved"
	case verbPick:
		if allOf(subjects, entCommit) {
			return subjects, ""
		}
		return nil, "only commits can be cherry-picked"
	}
	return nil, "unsupported"
}

func newTargetMode(v verb, sources []entity, status *but.Status) *targetMode {
	t := &targetMode{verb: v, sources: sources, side: sideBelow, status: status}
	if v == verbMove {
		t.side = sideAbove
	}
	return t
}

func (t *targetMode) isSource(e entity) bool {
	for _, s := range t.sources {
		if s.key() == e.key() {
			return true
		}
	}
	return false
}

// selfTargetOK reports whether a source may also be the target: squashing a
// branch into itself squashes all of its commits.
func (t *targetMode) selfTargetOK(e entity) bool {
	return t.verb == verbSquash && e.kind == entBranch
}

func (t *targetMode) sideWord() string {
	if t.side == sideAbove {
		return "above"
	}
	return "below"
}

func (t *targetMode) sourceIDs() []string {
	if len(t.sources) == 1 && t.sources[0].kind == entArea {
		return nil // no ids means "everything" to commit and amend
	}
	return collectIDs(t.sources)
}

// plan resolves what confirming target would do. ok is false for invalid targets.
func (t *targetMode) plan(target entity) (p plan, ok bool) {
	if !target.valid() || t.isSource(target) && !t.selfTargetOK(target) {
		return plan{}, false
	}
	src := t.sources[0]
	what := describeAll(t.sources)
	ids := t.sourceIDs()
	hasCommits := func(e entity) bool { return e.kind == entBranch && len(t.branchCommits(e)) > 0 }

	switch t.verb {
	case verbCommit:
		var at but.Placement
		switch target.kind {
		case entBranch:
			at, p.label, p.desc = but.Placement{Branch: target.branch}, "commit here", "Commit "+what+" to "+target.describe()
		case entCommit:
			at, p.insert = placeRelative(t.side, target.id), t.side
			p.label, p.desc = "commit "+t.sideWord(), "Commit "+what+" "+t.sideWord()+" "+target.describe()
		case entNewBranch:
			at, p.label, p.desc = but.Placement{NewBranch: true}, "new branch", "Commit "+what+" to a new branch"
		default:
			return plan{}, false
		}
		p.run = func(m *Model) tea.Cmd { return m.startCommit(ids, at, t.emptyMsg, p.desc) }
		return p, true

	case verbSquash:
		switch {
		case src.kind.uncommitted():
			if target.kind != entCommit && !hasCommits(target) {
				return plan{}, false
			}
			p.label, p.desc = "amend", "Amend "+what+" into "+target.describe()
			tid := target.id
			p.run = func(m *Model) tea.Cmd {
				return m.runOp(p.desc, keepSelection, func(ctx context.Context, c *but.Client) error {
					return c.Amend(ctx, tid, ids)
				})
			}
			return p, true

		case target.kind == entArea:
			p.label, p.desc = "uncommit", "Uncommit "+what
			p.run = func(m *Model) tea.Cmd {
				return m.runOp(p.desc, selectArea, func(ctx context.Context, c *but.Client) error {
					return c.Uncommit(ctx, ids)
				})
			}
			return p, true

		case src.kind == entBranch && target.kind == entBranch && t.isSource(target):
			if len(t.sources) > 1 || len(t.branchCommits(target)) < 2 {
				return plan{}, false
			}
			p.label, p.desc = "squash all", "Squash all commits of "+target.describe()
			return t.squashPlan(p, ids, target, ""), true

		case target.kind == entCommit || hasCommits(target):
			if src.kind == entCommittedFile && target.kind == entCommit && target.commit == src.commit {
				return plan{}, false
			}
			p.label = "squash into"
			if src.kind == entCommittedFile {
				p.label = "move into"
			}
			p.desc = strings.ToUpper(p.label[:1]) + p.label[1:] + " " + target.describe() + ": " + what
			return t.squashPlan(p, ids, target, target.id), true
		}
		return plan{}, false

	case verbMove:
		var at but.Placement
		switch {
		case src.kind == entCommit && target.kind == entCommit:
			at, p.insert = placeRelative(t.side, target.id), t.side
			p.label, p.desc = "move "+t.sideWord(), "Move "+what+" "+t.sideWord()+" "+target.describe()
		case src.kind == entCommit && target.kind == entBranch:
			at, p.label, p.desc = but.Placement{Branch: target.branch}, "move here", "Move "+what+" to "+target.describe()
		case src.kind == entBranch && target.kind == entBranch:
			at, p.label, p.desc = but.Placement{Branch: target.branch}, "stack onto", "Stack "+what+" onto "+target.describe()
		case target.kind == entNewBranch:
			at, p.label = but.Placement{NewBranch: true}, "new branch"
			p.desc = "Move " + what + " to a new branch"
			if src.kind == entBranch {
				p.label, p.desc = "unstack", "Unstack "+what
			}
		default:
			return plan{}, false
		}
		p.run = func(m *Model) tea.Cmd {
			sel := selectNew
			if src.kind == entBranch {
				sel = keepSelection
			}
			return m.runOp(p.desc, sel, func(ctx context.Context, c *but.Client) error { return c.Move(ctx, ids, at) })
		}
		return p, true

	case verbPick:
		var at but.Placement
		switch target.kind {
		case entBranch:
			at, p.label, p.desc = but.Placement{Branch: target.branch}, "pick here", "Cherry-pick "+what+" to "+target.describe()
		case entCommit:
			at, p.insert = placeRelative(t.side, target.id), t.side
			p.label, p.desc = "pick "+t.sideWord(), "Cherry-pick "+what+" "+t.sideWord()+" "+target.describe()
		case entNewBranch:
			at, p.label, p.desc = but.Placement{NewBranch: true}, "new branch", "Cherry-pick "+what+" to a new branch"
		default:
			return plan{}, false
		}
		p.run = func(m *Model) tea.Cmd {
			return m.runOp(p.desc, selectNew, func(ctx context.Context, c *but.Client) error { return c.Pick(ctx, ids, at) })
		}
		return p, true
	}
	return plan{}, false
}

// newBranchPlan is `b` in commit and pick mode: a new branch stacked above the target branch.
func (t *targetMode) newBranchPlan(target entity) (plan, bool) {
	if target.kind != entBranch || (t.verb != verbCommit && t.verb != verbPick) {
		return plan{}, false
	}
	ids := t.sourceIDs()
	at := but.Placement{Above: target.id}
	if t.verb == verbCommit {
		desc := "Commit " + describeAll(t.sources) + " to a new branch above " + target.label
		return plan{desc: desc, run: func(m *Model) tea.Cmd { return m.startCommit(ids, at, t.emptyMsg, desc) }}, true
	}
	desc := "Cherry-pick " + describeAll(t.sources) + " to a new branch above " + target.label
	return plan{desc: desc, run: func(m *Model) tea.Cmd {
		return m.runOp(desc, selectNew, func(ctx context.Context, c *but.Client) error { return c.Pick(ctx, ids, at) })
	}}, true
}

func placeRelative(side int, id string) but.Placement {
	if side == sideAbove {
		return but.Placement{Above: id}
	}
	return but.Placement{Below: id}
}

// branchCommits finds a branch's commits through the source/target entity.
func (t *targetMode) branchCommits(e entity) []but.Commit {
	if e.kind != entBranch || t.status == nil {
		return nil
	}
	for _, st := range t.status.Stacks {
		for _, b := range st.Branches {
			if b.Name == e.branch {
				return b.Commits
			}
		}
	}
	return nil
}

// squashPlan fills in how messages combine: keep the target's with `u`, reuse the
// only non-empty one, or open the composer with all of them.
func (t *targetMode) squashPlan(p plan, ids []string, target entity, targetID string) plan {
	var msgs []string
	add := func(msg string) {
		if msg = strings.TrimSpace(msg); msg != "" {
			msgs = append(msgs, msg)
		}
	}
	targetMsg := ""
	if target.kind == entCommit {
		targetMsg = target.commit.Message
	} else if cs := t.branchCommits(target); len(cs) > 0 && targetID != "" {
		targetMsg = cs[0].Message
	}
	add(targetMsg)
	for _, s := range t.sources {
		switch s.kind {
		case entCommit:
			add(s.commit.Message)
		case entBranch:
			for _, c := range t.branchCommits(s) {
				add(c.Message)
			}
		}
	}
	mode := but.SquashUseTargetMessage
	if !t.useTarget && strings.TrimSpace(targetMsg) == "" && len(msgs) == 1 && t.sources[0].kind == entCommit {
		mode = but.SquashUseSourceMessage
	}
	if t.useTarget || len(msgs) <= 1 {
		if t.useTarget {
			p.label += " (keep msg)"
		}
		p.run = func(m *Model) tea.Cmd {
			return m.runOp(p.desc, selectNew, func(ctx context.Context, c *but.Client) error {
				return c.Squash(ctx, ids, targetID, mode, "")
			})
		}
		return p
	}
	combined := strings.Join(msgs, "\n\n")
	p.run = func(m *Model) tea.Cmd {
		return m.openComposer(composeSquash, p.desc, combined, func(msg string) tea.Cmd {
			return m.runOp(p.desc, selectNew, func(ctx context.Context, c *but.Client) error {
				return c.Squash(ctx, ids, targetID, but.SquashCombineMessages, msg)
			})
		})
	}
	return p
}

// dropVerb picks the natural operation for a mouse drag from source onto target.
func dropVerb(source, target entity) (verb, bool) {
	switch {
	case source.kind.uncommitted():
		switch target.kind {
		case entBranch, entNewBranch:
			return verbCommit, true
		case entCommit:
			return verbSquash, true // amend
		}
	case source.kind == entCommit:
		switch target.kind {
		case entCommit, entArea:
			return verbSquash, true
		case entBranch, entNewBranch:
			return verbMove, true
		}
	case source.kind == entBranch:
		switch target.kind {
		case entBranch, entNewBranch:
			return verbMove, true
		case entArea, entCommit:
			return verbSquash, true
		}
	case source.kind == entCommittedFile:
		switch target.kind {
		case entCommit, entBranch, entArea:
			return verbSquash, true
		}
	}
	return 0, false
}
