package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

// lane is one column of the workspace: a stack of branches. items lists every
// selectable entity in top-to-bottom render order.
type lane struct {
	stack   but.Stack
	items   []entity
	phantom bool // the "new branch" drop lane
}

func buildLanes(s *but.Status, showFiles func(*but.Commit) bool) []lane {
	lanes := make([]lane, 0, len(s.Stacks))
	for _, st := range s.Stacks {
		l := lane{stack: st}
		for _, c := range st.AssignedChanges {
			l.items = append(l.items, entity{kind: entFile, id: c.CliID, label: c.FilePath, stack: st.CliID})
		}
		for _, b := range st.Branches {
			l.items = append(l.items, entity{kind: entBranch, id: b.CliID, label: b.Name, branch: b.Name, stack: st.CliID, status: b.BranchStatus})
			for ci := range b.Commits {
				c := &b.Commits[ci]
				l.items = append(l.items, entity{kind: entCommit, id: c.CliID, label: c.Subject(), branch: b.Name, stack: st.CliID, commit: c})
				if showFiles(c) {
					for _, f := range c.Changes {
						l.items = append(l.items, entity{kind: entCommittedFile, id: f.CliID, label: f.FilePath, branch: b.Name, stack: st.CliID, commit: c})
					}
				}
			}
		}
		lanes = append(lanes, l)
	}
	return lanes
}

func phantomLane() lane {
	return lane{phantom: true, items: []entity{{kind: entNewBranch, label: "new branch"}}}
}

// rowDeco is how the current mode wants a row drawn.
type rowDeco struct {
	marked bool
	dim    bool   // not a valid target in the current mode
	source bool   // the thing being committed/squashed/moved
	tag    string // operation label on a hovered target, e.g. "amend"
	insert int    // -1/+1: draw an insertion marker above/below the row
}

type decoFunc func(entity) rowDeco

// laneView is a rendered lane: its lines, the item index under each line
// (-1 for none, used for mouse hit testing) and the line of the selection.
type laneView struct {
	lines   []string
	items   []int
	selLine int
}

var (
	tagStyle    = lipgloss.NewStyle().Bold(true).Background(colorAccent).Foreground(colorText).Padding(0, 1)
	sourceStyle = lipgloss.NewStyle().Bold(true).Foreground(colorMod)
	markGlyph   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true).Render("✔")
)

// insertLine marks where a commit, move or pick will land.
func insertLine(width int, label string) string {
	l := " " + label + " "
	fill := max(width-ansi.StringWidth(l)-2, 0)
	return lipgloss.NewStyle().Foreground(colorAccent).Render("╶─" + l + strings.Repeat("─", fill))
}

// withTag appends a right-aligned tag pill (or source marker) to a row.
func withTag(line string, width int, d rowDeco) string {
	tag := ""
	switch {
	case d.tag != "":
		tag = tagStyle.Render(d.tag)
	case d.source:
		tag = sourceStyle.Render("◆ source")
	}
	if tag == "" {
		return line
	}
	avail := width - ansi.StringWidth(tag) - 1
	line = ansi.Truncate(line, max(avail, 0), "…")
	return line + strings.Repeat(" ", max(width-ansi.StringWidth(line)-ansi.StringWidth(tag), 1)) + tag
}

func dimmed(line string, d rowDeco) string {
	if d.dim {
		return mutedStyle.Faint(true).Render(ansi.Strip(line))
	}
	return line
}

// render draws the lane at the given width. sel is the selected item index, or -1
// when the lane has no selection.
func (l lane) render(width, sel int, deco decoFunc) laneView {
	if l.phantom {
		return l.renderPhantom(width, sel, deco)
	}
	selLine := -1
	inner := width - 4 // card border + padding
	item := 0
	var out []string
	var hits []int
	// add appends a block whose lines all belong to item hit; it returns the block's first line.
	add := func(block string, hit int) int {
		start := len(out)
		for _, line := range strings.Split(block, "\n") {
			out = append(out, line)
			hits = append(hits, hit)
		}
		return start
	}

	// Commit box: staged changes (or a hint) and the commit button.
	var box []string
	boxItems := map[int]int{}
	if len(l.stack.AssignedChanges) == 0 {
		box = append(box, dropStyle.Width(inner).Render("No staged changes"))
	}
	for _, c := range l.stack.AssignedChanges {
		e := l.items[item]
		d := deco(e)
		if item == sel {
			selLine = len(out) + 1 + len(box)
		}
		letter, st := changeTypeStyle(c.ChangeType)
		prefix := ""
		if d.marked {
			prefix = markGlyph + " "
		}
		boxItems[len(box)] = item
		box = append(box, dimmed(fileLine(prefix, c.FilePath, st.Render(letter), inner, item == sel), d))
		item++
	}
	box = append(box, buttonStyle.Width(inner).Align(lipgloss.Center).Render("Start a commit…"))
	start := add(cardStyle.Width(width).Render(strings.Join(box, "\n")), -1)
	for line, it := range boxItems {
		hits[start+1+line] = it
	}
	// The commit button starts a commit into this lane.
	buttonLine := 0
	for _, b := range box[:len(box)-1] {
		buttonLine += lipgloss.Height(b)
	}
	hits[start+1+buttonLine] = startCommitHit

	for bi, b := range l.stack.Branches {
		if bi > 0 {
			add(dividerStyle.Render("  │"), -1)
		}
		focused := false
		branchItem := item
		be := l.items[item]
		bd := deco(be)
		var body []string
		bodyItems := map[int]int{} // body line -> item

		badge := lipgloss.NewStyle().Background(branchColor(b.BranchStatus)).Foreground(colorText).Render(" ⑂ ")
		if bd.marked {
			badge = markGlyph + " " + badge
		}
		name := ansi.Truncate(b.Name, max(inner-6, 1), "…")
		if item == sel {
			focused, selLine = true, len(out)+1
			name = selectedStyle.Render(name)
		} else {
			name = headerStyle.Render(name)
		}
		body = append(body, dimmed(withTag(badge+" "+name, inner, bd), bd), "")
		item++

		if len(b.Commits) == 0 {
			body = append(body,
				mutedStyle.Render(ansi.Truncate("This is an empty branch.", inner, "…")),
				mutedStyle.Render(ansi.Truncate("Create commits here.", inner, "…")))
		}
		for ci := range b.Commits {
			c := &b.Commits[ci]
			ce := l.items[item]
			cd := deco(ce)
			if cd.insert < 0 {
				body = append(body, insertLine(inner, "here"))
			}
			if item == sel {
				focused, selLine = true, len(out)+1+len(body)
			}
			dot := lipgloss.NewStyle().Foreground(branchColor(b.BranchStatus)).Render("●")
			switch {
			case cd.marked:
				dot = markGlyph
			case c.Conflicted != nil && *c.Conflicted:
				dot = errorStyle.Render("✗")
			}
			subject := c.Subject()
			if subject == "" {
				subject = mutedStyle.Italic(true).Render("(no message)")
			}
			row := fileLine(dot+" ", subject, mutedStyle.Render(c.ShortID()), inner, item == sel)
			if cd.tag != "" || cd.source {
				row = withTag(fileLine(dot+" ", subject, "", inner-ansi.StringWidth(cd.tag)-3, item == sel), inner, cd)
			}
			bodyItems[len(body)] = item
			body = append(body, dimmed(row, cd))
			item++
			if cd.insert > 0 {
				body = append(body, insertLine(inner, "here"))
			}

			for item < len(l.items) && l.items[item].kind == entCommittedFile && l.items[item].commit == c {
				fe := l.items[item]
				fd := deco(fe)
				if item == sel {
					focused, selLine = true, len(out)+1+len(body)
				}
				letter, st := "~", lipgloss.NewStyle().Foreground(colorMod)
				for _, f := range c.Changes {
					if f.CliID == fe.id {
						letter, st = changeTypeStyle(f.ChangeType)
					}
				}
				prefix := "  "
				if fd.marked {
					prefix = " " + markGlyph
				}
				bodyItems[len(body)] = item
				body = append(body, dimmed(withTag(fileLine(prefix+" ", fe.label, st.Render(letter), inner, item == sel), inner, fd), fd))
				item++
			}
		}
		if n := len(b.UpstreamCommits); n > 0 {
			body = append(body, errorStyle.Render(ansi.Truncate("↓ "+pluralize(n, "upstream commit"), inner, "…")))
		}

		body = append(body, dividerStyle.Render(strings.Repeat("─", inner)))
		push := primaryBtnStyle.Render("Push ⇡")
		if !canPush(b.BranchStatus) {
			push = disabledBtnStyle.Render("Push ⇡")
		}
		gap := max(inner-ansi.StringWidth(push)-1, 1)
		body = append(body, push+strings.Repeat(" ", gap)+mutedStyle.Render("⋯"))
		pushLine := len(body) - 1

		card := cardStyle
		switch {
		case bd.tag != "":
			card = card.BorderForeground(colorAccent).BorderStyle(lipgloss.ThickBorder())
		case focused:
			card = card.BorderForeground(colorAccent)
		case bd.dim:
			card = card.BorderForeground(colorBorder)
		}
		start := add(card.Width(width).Render(strings.Join(body, "\n")), branchItem)
		for line, it := range bodyItems {
			hits[start+1+line] = it
		}
		hits[start+1+pushLine] = pushHitBase - branchItem
	}
	return laneView{lines: out, items: hits, selLine: selLine}
}

// Special hit values for buttons, below every item index.
const (
	startCommitHit = -2
	pushHitBase    = -1000 // pushHitBase - branchItem: the push row of that branch's card
)

func (l lane) renderPhantom(width, sel int, deco decoFunc) laneView {
	d := deco(l.items[0])
	text := "＋ Drop here for a new branch"
	st := dropStyle.Width(width).Padding(1, 0)
	if d.tag != "" || sel == 0 {
		st = st.BorderForeground(colorAccent).Foreground(colorText)
		if d.tag != "" {
			text = tagStyle.Render(d.tag)
		}
	}
	block := strings.Split(st.Render(text), "\n")
	hits := make([]int, len(block))
	selLine := -1
	if sel == 0 {
		selLine = 1
	}
	return laneView{lines: block, items: hits, selLine: selLine}
}

func canPush(status string) bool {
	return status != "nothingToPush" && status != "integrated"
}

func pluralize(n int, word string) string {
	s := itoa(n) + " " + word
	if n != 1 {
		s += "s"
	}
	return s
}
