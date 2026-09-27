package ui

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

// commentsMsg delivers the review comments located on the current status.
type commentsMsg struct {
	seq     int
	key     string // fingerprint of the comments and status they were located on
	located []review.Located
	err     error
}

// reviewDoneMsg reports a write to the comment store.
type reviewDoneMsg struct {
	text   string
	reveal string // a comment to scroll into view, such as the one just added
	err    error
}

// loadComments reads the comment store and locates the comments on the current status, off the update loop:
// locating runs `but diff` per file. It does nothing when neither the comments nor the status changed since the
// last load, unless force is set, so the refresh tick stays cheap while picking up an agent's changes.
func (m *Model) loadComments(force bool) tea.Cmd {
	if m.review == nil || m.status == nil || m.resolving() {
		return nil
	}
	m.commentsSeq++
	seq, store, st, client, prev := m.commentsSeq, m.review, m.status, m.client, m.commentsKey
	if force {
		prev = ""
	}
	return func() tea.Msg {
		cs, err := store.List()
		if err != nil {
			return commentsMsg{seq: seq, err: err}
		}
		key := fingerprint(cs, st)
		if key == prev {
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		ls, err := review.Locate(ctx, st, client, cs)
		if err != nil {
			return commentsMsg{seq: seq, err: err}
		}
		moved := false
		for _, l := range ls {
			moved = moved || l.Moved()
		}
		if moved {
			if err := store.Save(ls); err != nil {
				return commentsMsg{seq: seq, err: err}
			}
			// Saving changed the file; fingerprint it as saved so the next tick does not locate again.
			if cs, err := store.List(); err == nil {
				key = fingerprint(cs, st)
			}
		}
		return commentsMsg{seq: seq, key: key, located: ls}
	}
}

func fingerprint(cs []review.Comment, st *but.Status) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(cs)
	_ = json.NewEncoder(h).Encode(st)
	return hex.EncodeToString(h.Sum(nil))
}

func (m *Model) receiveComments(msg commentsMsg) tea.Cmd {
	if msg.seq < m.commentsGot {
		return nil // an older load finished after a newer one
	}
	m.commentsGot = msg.seq
	if msg.err != nil {
		// The tick retries every few seconds; say it once.
		if msg.err.Error() == m.commentsErr {
			return nil
		}
		m.commentsErr = msg.err.Error()
		return m.notify(toastError, "Review comments: "+firstLines(msg.err.Error(), 3))
	}
	m.commentsErr, m.commentsKey, m.comments = "", msg.key, msg.located
	m.det.setNotes(msg.located)
	m.det.rerender(m.hunkMarks())
	return nil
}

// reviewOp writes to the comment store in the background (a write can wait for the lock), then reloads.
func (m *Model) reviewOp(done string, fn func(*review.Store) error) tea.Cmd {
	store := m.review
	return func() tea.Msg {
		return reviewDoneMsg{text: done, err: fn(store)}
	}
}

func (m *Model) handleReviewDone(msg reviewDoneMsg) tea.Cmd {
	if msg.err != nil {
		return tea.Batch(m.notify(toastError, msg.err.Error()), m.loadComments(true))
	}
	m.det.reveal = msg.reveal
	return tea.Batch(m.notify(toastSuccess, msg.text), m.loadComments(true))
}

// isOpen reports whether a comment still wants an answer: open, even when its line or commit is gone.
func isOpen(l review.Located) bool { return l.Comment.Status == review.StatusOpen }

func matchesCommit(a review.Anchor, c *but.Commit) bool {
	return c != nil && (a.ChangeID != "" && a.ChangeID == c.ChangeID || a.CommitID != "" && a.CommitID == c.CommitID)
}

// openComments counts the open comments on a file or commit, for its badge.
func (m *Model) openComments(e entity) int {
	n := 0
	for _, l := range m.comments {
		if !isOpen(l) || l.Status == review.StatusOrphaned {
			continue
		}
		a := l.Anchor
		switch e.kind {
		case entFile:
			if a.Kind != review.KindCommit && a.Path == e.label {
				n++
			}
		case entCommit:
			if a.Kind == review.KindCommit && matchesCommit(a, e.commit) {
				n++
			}
		case entCommittedFile:
			if a.Kind == review.KindCommit && matchesCommit(a, e.commit) && a.Path == e.label {
				n++
			}
		}
	}
	return n
}

var commentBadgeStyle = lipgloss.NewStyle().Foreground(colorMod).Bold(true)

// withCount puts a comment count badge before a row's right-hand tag.
func withCount(tag string, n int) string {
	if n == 0 {
		return tag
	}
	badge := commentBadgeStyle.Render("✎" + itoa(n))
	if tag == "" {
		return badge
	}
	return badge + " " + tag
}

// Actions.

func hasReview(m *Model, _ []entity) bool { return m.review != nil }

// canComment reports whether C can open the composer: a line cursor in the details pane, not on a comment.
func canComment(m *Model, sel []entity) bool {
	_, onNote := m.det.cursorNote()
	return m.review != nil && detailsLines(m, sel) && !onNote
}

func onComment(m *Model, _ []entity) bool {
	_, ok := m.det.cursorNote()
	return m.review != nil && ok
}

// anchorFor turns the line selection in the diff of e into a comment anchor.
func (m *Model) anchorFor(e entity, s lineSel) (review.Anchor, error) {
	a := review.Anchor{Path: s.path, Side: s.side, Line: s.line, EndLine: s.endLine, LineText: s.text}
	switch {
	case (e.kind == entCommit || e.kind == entCommittedFile) && e.commit != nil:
		a.Kind, a.ChangeID, a.CommitID, a.Branch = review.KindCommit, e.commit.ChangeID, e.commit.CommitID, e.branch
	case e.kind.uncommitted():
		a.Kind = review.KindUnassigned
		// Assigned changes are known by their stack's top branch: `but status` has no stable stack id.
		for _, st := range m.status.Stacks {
			if changeID(st.AssignedChanges, s.path) != "" && len(st.Branches) > 0 {
				a.Kind, a.Branch = review.KindAssigned, st.Branches[0].Name
			}
		}
	default:
		return a, errors.New("Comment on a commit or on uncommitted changes, not a branch")
	}
	return a, nil
}

func changeID(cs []but.Change, path string) string {
	for _, c := range cs {
		if c.FilePath == path {
			return c.CliID
		}
	}
	return ""
}

// where describes an anchor's lines: "a.go line 3", "a.go lines 3–5 (old)".
func where(a review.Anchor) string {
	s := a.Path + " line " + itoa(a.Line)
	if a.EndLine > a.Line {
		s = a.Path + " lines " + itoa(a.Line) + "–" + itoa(a.EndLine)
	}
	if a.Side == review.SideOld {
		s += " (old)"
	}
	return s
}

func (m *Model) startComment([]entity) tea.Cmd {
	s, ok := m.det.lineSelection()
	if !ok {
		return m.notify(toastInfo, "Put the line cursor on a line to comment on it")
	}
	a, err := m.anchorFor(m.det.ent, s)
	if err != nil {
		return m.notify(toastInfo, err.Error())
	}
	m.openModal(newCommentComposer("on "+where(a), "", func(m *Model, body string) tea.Cmd {
		if strings.TrimSpace(body) == "" {
			return nil
		}
		m.det.cancelRange(m.hunkMarks())
		store := m.review
		return func() tea.Msg {
			c, err := store.Add(a, review.AuthorUser, body)
			return reviewDoneMsg{text: "Comment added on " + where(a), reveal: c.ID, err: err}
		}
	}))
	return nil
}

func (m *Model) editComment([]entity) tea.Cmd {
	l, _ := m.det.cursorNote()
	id := l.Comment.ID
	m.openModal(newCommentComposer("on "+where(l.Anchor), l.Comment.Body, func(m *Model, body string) tea.Cmd {
		switch {
		case strings.TrimSpace(body) == strings.TrimSpace(l.Comment.Body):
			return nil
		case strings.TrimSpace(body) == "":
			return m.notify(toastInfo, "A comment can't be empty; press d to delete it")
		}
		return m.reviewOp("Comment edited", func(st *review.Store) error {
			_, err := st.Edit(id, body)
			return err
		})
	}))
	return nil
}

func (m *Model) deleteComment([]entity) tea.Cmd {
	l, _ := m.det.cursorNote()
	id := l.Comment.ID
	m.openModal(&confirmModal{title: "Delete comment on " + where(l.Anchor) + "?", yesLabel: "delete",
		body: "“" + firstLines(l.Comment.Body, 3) + "”",
		onYes: func(m *Model) tea.Cmd {
			return m.reviewOp("Comment deleted", func(st *review.Store) error { return st.Delete(id) })
		}})
	return nil
}

func (m *Model) toggleCommentResolved([]entity) tea.Cmd {
	l, _ := m.det.cursorNote()
	id := l.Comment.ID
	if isOpen(l) {
		return m.reviewOp("Comment resolved", func(st *review.Store) error {
			_, err := st.Resolve(id, "")
			return err
		})
	}
	return m.reviewOp("Comment reopened", func(st *review.Store) error {
		_, err := st.Reopen(id)
		return err
	})
}

func (m *Model) toggleResolvedComments([]entity) tea.Cmd {
	m.det.hideResolved = !m.det.hideResolved
	m.det.rerender(m.hunkMarks())
	if m.det.hideResolved {
		return m.notify(toastInfo, "Resolved comments hidden")
	}
	return m.notify(toastInfo, "Resolved comments shown")
}

// commentsPicker lists the open comments; picking one jumps to its line.
func (m *Model) commentsPicker([]entity) tea.Cmd {
	var items []pickItem
	for _, l := range m.comments {
		if !isOpen(l) {
			continue
		}
		label := where(l.Anchor) + "  " + firstLines(l.Comment.Body, 1)
		detail := l.Target
		switch {
		case l.Status == review.StatusOrphaned:
			detail = "commit gone"
		case l.Status == review.StatusOutdated:
			detail = "outdated"
		case l.Anchor.Kind == review.KindCommit && len(l.Anchor.CommitID) >= 7:
			detail += " " + l.Anchor.CommitID[:7]
		}
		if who := authorLabel(l.Comment.Author); who != "you" {
			detail = who + " · " + detail
		}
		items = append(items, pickItem{label: label, detail: detail, value: l})
	}
	p := newPicker("Review comments · enter jumps to the line", items, func(m *Model, it pickItem) tea.Cmd {
		return m.jumpToComment(it.value.(review.Located))
	})
	p.empty = "No open comments"
	p.maxRows = 20
	m.openModal(p)
	return nil
}

// jumpToComment selects what a comment is on and opens its diff with the cursor on the comment.
func (m *Model) jumpToComment(l review.Located) tea.Cmd {
	if l.Status == review.StatusOrphaned {
		return m.notify(toastInfo, "The commit of this comment is gone")
	}
	var e entity
	if l.Anchor.Kind == review.KindCommit {
		for _, ln := range m.lanes {
			for _, it := range ln.items {
				if it.kind == entCommit && matchesCommit(l.Anchor, it.commit) {
					e = it
				}
			}
		}
	} else {
		e = entity{kind: entFile, label: l.Anchor.Path}
	}
	if !e.valid() || !m.selectKey(e.key()) {
		return m.notify(toastInfo, l.Anchor.Path+" is no longer in the workspace")
	}
	m.clampFiles()
	m.clampLanes()
	m.target = nil
	m.det.visible, m.det.focused = true, true
	m.det.jump = l.Comment.ID
	if cmd := m.syncDetails(false); cmd != nil {
		return cmd // the jump happens when the diff arrives
	}
	m.det.rerender(m.hunkMarks())
	return nil
}

// Inline comment boxes.

var (
	noteBorderStyle    = lipgloss.NewStyle().Foreground(colorMod)
	noteSelBorderStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	noteAuthorStyle    = lipgloss.NewStyle().Foreground(colorText).Bold(true)
	noteAgentStyle     = lipgloss.NewStyle().Foreground(colorPushed).Bold(true)

	// severityStyles colour the tags /buti-review starts a comment with.
	severityStyles = map[string]lipgloss.Style{
		"[must-fix]":   lipgloss.NewStyle().Foreground(colorDel).Bold(true),
		"[suggestion]": lipgloss.NewStyle().Foreground(colorMod).Bold(true),
		"[nit]":        mutedStyle.Bold(true),
		"[question]":   lipgloss.NewStyle().Foreground(colorAccent).Bold(true),
	}
)

// authorLabel is how a comment's author is shown: "you" for the user, the stored name (such as "agent") otherwise.
func authorLabel(author string) string {
	if author == review.AuthorUser || author == "" {
		return "you"
	}
	return author
}

// authorStyle sets comments from someone other than the user, such as a reviewing agent, apart.
func authorStyle(author string) lipgloss.Style {
	if author == review.AuthorUser || author == "" {
		return noteAuthorStyle
	}
	return noteAgentStyle
}

// styleSeverity colours a leading severity tag, such as "[must-fix]", in a line of a comment's text.
func styleSeverity(line string) string {
	for tag, st := range severityStyles {
		if rest, ok := strings.CutPrefix(line, tag); ok {
			return st.Render(tag) + rest
		}
	}
	return line
}

// noteRows renders a comment as the rows of the box drawn under its line, once as is and once selected. width is
// the width of a diff line; the box is indented past the line number gutter by indent.
func noteRows(l review.Located, outdated bool, indent, width int) (plain, sel []string) {
	const minW = 24
	bw := width - indent - 1
	if bw < minW {
		indent = max(width-minW-1, 0)
		bw = max(width-indent-1, 8)
	}
	pre := strings.Repeat(" ", indent)
	c := l.Comment

	if !isOpen(l) {
		label := "✓ resolved"
		if c.Status == review.StatusDismissed {
			label = "✕ dismissed"
		}
		text := "╶ " + label + " · " + firstLines(c.Body, 1)
		if s := firstLines(c.Resolution.Summary, 1); s != "" {
			text += " — " + s
		}
		text = ansi.Truncate(text, bw, "…")
		return []string{pre + mutedStyle.Render(text)}, []string{pre + noteSelBorderStyle.Render(text)}
	}

	author := authorStyle(c.Author).Render(authorLabel(c.Author))
	at := "line " + itoa(l.Anchor.Line)
	if l.Anchor.EndLine > l.Anchor.Line {
		at = "lines " + itoa(l.Anchor.Line) + "–" + itoa(l.Anchor.EndLine)
	}
	if l.Anchor.Side == review.SideOld {
		at += " (old)"
	}
	if outdated {
		at = "outdated · was " + at
	}
	inner := bw - 4
	var body []string
	if outdated {
		was := strings.Split(strings.TrimSuffix(l.Anchor.LineText, "\n"), "\n")
		for _, t := range was[:min(len(was), 3)] {
			body = append(body, mutedStyle.Render(ansi.Truncate("> "+expandTabs(t), inner, "…")))
		}
	}
	wrap := func(prefix, s string) {
		for _, p := range strings.Split(strings.TrimSpace(s), "\n") {
			for _, w := range strings.Split(ansi.Wrap(expandTabs(p), max(inner-ansi.StringWidth(prefix), 1), ""), "\n") {
				body = append(body, prefix+w)
				prefix = strings.Repeat(" ", ansi.StringWidth(prefix))
			}
		}
	}
	first := len(body)
	wrap("", c.Body)
	if first < len(body) {
		body[first] = styleSeverity(body[first])
	}
	for _, r := range c.Replies {
		wrap(mutedStyle.Render("↳ ")+authorStyle(r.Author).Render(authorLabel(r.Author))+mutedStyle.Render(": "), r.Body)
	}

	draw := func(border lipgloss.Style) []string {
		head := author + mutedStyle.Render(" · "+at)
		left := border.Render("╭─ ") + head + " "
		right := " " + mutedStyle.Render(c.ID) + border.Render(" ─╮")
		fill := bw - ansi.StringWidth(left) - ansi.StringWidth(right)
		if fill < 1 {
			right = border.Render("─╮")
			left = ansi.Truncate(left, bw-ansi.StringWidth(right)-1, "…") + " "
			fill = max(bw-ansi.StringWidth(left)-ansi.StringWidth(right), 0)
		}
		out := []string{pre + left + border.Render(strings.Repeat("─", fill)) + right}
		for _, b := range body {
			b = ansi.Truncate(b, inner, "…")
			out = append(out, pre+border.Render("│ ")+b+strings.Repeat(" ", max(inner-ansi.StringWidth(b), 0))+border.Render(" │"))
		}
		return append(out, pre+border.Render("╰"+strings.Repeat("─", max(bw-2, 0))+"╯"))
	}
	st := noteBorderStyle
	if outdated {
		st = mutedStyle
	}
	return draw(st), draw(noteSelBorderStyle)
}

// withComments returns the layout with comment boxes under the lines they are on. A comment whose line is not in
// the diff (outdated) goes after the last line of its file; one on a file the diff does not show is left out.
func (l diffLayout) withComments(notes []review.Located, width int) diffLayout {
	if len(notes) == 0 {
		return l
	}
	after := map[int][]int{} // row -> notes
	outdated := map[int]bool{}
	for i, n := range notes {
		row, gone := l.noteRow(n)
		if row < 0 {
			continue
		}
		after[row] = append(after[row], i)
		outdated[i] = gone
	}
	if len(after) == 0 {
		return l
	}
	out := diffLayout{numW: l.numW, hunks: append([]hunkRef(nil), l.hunks...)}
	indent := 2*l.numW + 3
	for i, r := range l.rows {
		out.rows = append(out.rows, r)
		for _, ni := range after[i] {
			plain, sel := noteRows(notes[ni], outdated[ni], indent, width)
			for k := range plain {
				out.rows = append(out.rows, diffRow{hunk: r.hunk, text: plain[k], alt: sel[k], note: notes[ni].Comment.ID, top: k == 0})
			}
		}
	}
	for i := range out.hunks {
		out.hunks[i].line = -1
	}
	for i, r := range out.rows {
		if r.hunk >= 0 {
			h := &out.hunks[r.hunk]
			if h.line < 0 {
				h.line = i
			}
			h.end = i + 1
		}
	}
	return out
}

// noteRow is the row a comment goes under, and whether it is outdated there: its last line, or else the last line
// of its file. It is -1 when the diff does not show the file.
func (l diffLayout) noteRow(n review.Located) (int, bool) {
	a, last := n.Anchor, -1
	for _, h := range l.hunks {
		if h.path != a.Path {
			continue
		}
		for r := h.line; r < h.end; r++ {
			row := l.rows[r]
			if !row.isLine() {
				continue
			}
			last = r
			if n.Status == review.StatusOutdated {
				continue
			}
			end := max(a.EndLine, a.Line)
			if a.Side == review.SideOld && row.old == end && row.sign != '+' ||
				a.Side != review.SideOld && row.new == end && row.sign != '-' {
				return r, false
			}
		}
	}
	return last, true
}
