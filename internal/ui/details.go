package ui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

const (
	detailsMinPct = 30
	detailsMaxPct = 90
	detailsTail   = 3 // blank lines after the diff, so the end reads as the end
)

// details is the diff/commit pane, split under the lanes or full screen.
type details struct {
	visible bool
	full    bool
	focused bool
	pct     int // share of the right-hand height when split

	ent     entity // what is shown
	reqKey  string // key of the entity last requested
	loading bool
	err     error
	data    *but.Diff
	header  []string
	base    diffLayout // data rendered at layoutW, reused while only the selection or comments change
	layout  diffLayout // base with the comment boxes
	layoutD *but.Diff
	layoutW int
	marks   map[string]bool // hunk marks last drawn
	doc     diffDoc
	drawn   int      // hunk drawn as selected in lines
	lines   []string // the viewport's content; doc.lines follow the header when patchable
	patch   bool     // whether lines hold the current layout, so hunks can be redrawn in place
	hunk    int      // selected hunk, when focused
	cursor  int      // layout row of the line cursor, inside the selected hunk; -1 for none
	anchor  int      // layout row where the range selection started; -1 for none
	vp      viewport.Model
	width   int // of the diff
	outerW  int // of the pane, tree included
	tree    fileTree

	// Review comments: every located comment, of which the pane draws those on what it shows.
	notes        []review.Located
	notesVer     int    // bumped when notes change
	notesDrawn   string // what the layout's comment boxes were drawn for
	hideResolved bool
	jump         string // id of a comment to put the cursor on once the diff arrives
	reveal       string // id of a comment to scroll into view once it is drawn
}

type detailsMsg struct {
	key  string
	diff *but.Diff
	err  error
}

func newDetails() details {
	vp := viewport.New()
	vp.MouseWheelDelta = 3
	return details{pct: 45, vp: vp, hunk: -1, cursor: -1, anchor: -1}
}

// diffTarget is the argument to `but diff` for an entity, and a path filter
// for directories (which `but diff` cannot address directly).
func diffTarget(e entity) (id, prefix string, ok bool) {
	switch e.kind {
	case entArea:
		return "", "", true
	case entDir:
		return "", e.label, true
	case entFile, entHunk, entBranch, entCommit, entCommittedFile:
		return e.id, "", e.id != ""
	}
	return "", "", false
}

// sync requests the diff for e if it changed; it returns nil when nothing is needed.
func (d *details) sync(c *but.Client, e entity, force bool) tea.Cmd {
	if !d.visible && !d.full {
		return nil
	}
	key := e.key()
	if conflicted(e) {
		key += ":conflicted" // resolving keeps the change id, but not the diff
	}
	if key == d.reqKey && !force {
		return nil
	}
	if key != d.reqKey {
		d.hunk, d.cursor, d.anchor, d.reveal = -1, -1, -1, ""
		d.vp.GotoTop()
	}
	d.reqKey, d.ent = key, e
	d.header = commitHeader(e)
	id, prefix, ok := diffTarget(e)
	if !ok {
		d.data, d.err, d.loading = nil, nil, false
		d.tree.build(nil)
		d.rerender()
		return nil
	}
	d.loading = true
	return func() tea.Msg {
		diff, err := c.Diff(context.Background(), id)
		if err == nil && prefix != "" {
			var kept []but.FileDiff
			for _, f := range diff.Changes {
				if strings.HasPrefix(f.Path, prefix) {
					kept = append(kept, f)
				}
			}
			diff.Changes = kept
		}
		return detailsMsg{key: key, diff: diff, err: err}
	}
}

func (d *details) receive(msg detailsMsg, marks map[string]bool) {
	if msg.key != d.reqKey {
		return
	}
	d.loading, d.data, d.err = false, msg.diff, msg.err
	d.tree.build(d.data)
	if d.hunk >= 0 && d.data != nil && d.hunk >= countHunks(d.data) {
		d.hunk = countHunks(d.data) - 1
	}
	d.rerender(marks)
}

func countHunks(diff *but.Diff) int {
	n := 0
	for _, f := range diff.Changes {
		n += len(f.Diff.Hunks)
	}
	return n
}

// rerender rebuilds the content at the current width, selection and marks,
// keeping the marks last drawn when none are given. Highlighting is only redone
// when the diff or the width changed.
func (d *details) rerender(marked ...map[string]bool) {
	if len(marked) > 0 {
		d.marks = marked[0]
	}
	lines := append([]string(nil), d.header...)
	d.patch = false
	switch {
	case d.err != nil:
		lines = append(lines, errorStyle.Render(d.err.Error()))
	case d.data == nil && d.loading:
		lines = append(lines, mutedStyle.Render("loading…"))
	case d.data == nil:
		if len(d.header) == 0 {
			lines = append(lines, mutedStyle.Render("Nothing to show"))
		}
	default:
		sel := -1
		if d.focused {
			sel = d.hunk
		}
		keep := d.rowIDs()
		w := max(d.width, 20)
		if d.layoutD != d.data || d.layoutW != w {
			d.base, d.layoutD, d.layoutW = layoutDiff(d.data, w), d.data, w
			d.layout, d.notesDrawn = d.base, ""
		}
		if k := fmt.Sprint(d.notesVer, d.reqKey, d.hideResolved); k != d.notesDrawn {
			d.layout, d.notesDrawn = d.base.withComments(d.notesShown(), w-2), k
			d.restoreRows(keep)
		}
		d.fixCursor()
		d.doc = d.layout.doc(d.opts(sel, d.marks))
		lines = append(lines, d.doc.lines...)
		lines = append(lines, make([]string, detailsTail)...)
		d.drawn, d.patch = sel, true
	}
	d.lines = lines
	d.vp.SetContentLines(lines)
	if d.jump != "" && d.data != nil && !d.loading {
		d.jumpTo(d.jump)
	}
	if d.reveal != "" {
		d.revealNote(d.reveal)
	}
}

// revealNote scrolls a comment's box into view, keeping the line cursor on screen.
func (d *details) revealNote(id string) {
	end := -1
	for i, r := range d.layout.rows {
		if r.note == id {
			end = i
		}
	}
	if end < 0 {
		return
	}
	d.reveal = ""
	y, top := len(d.header)+end, len(d.header)+max(d.cursor, 0)
	if y >= d.vp.YOffset()+d.vp.Height() {
		d.vp.SetYOffset(min(y-d.vp.Height()+1, top))
	}
}

// setNotes replaces the located review comments.
func (d *details) setNotes(ls []review.Located) {
	d.notes = ls
	d.notesVer++
}

// notesShown are the comments on what the pane shows: uncommitted comments on an uncommitted diff, a commit's
// comments on its diff, and on a branch's diff those of its commits. Orphaned comments have nowhere to go, and
// resolved ones can be hidden.
func (d *details) notesShown() []review.Located {
	var out []review.Located
	for _, l := range d.notes {
		a := l.Anchor
		switch {
		case l.Status == review.StatusOrphaned, d.hideResolved && !isOpen(l):
			continue
		case d.ent.kind == entBranch && a.Kind == review.KindCommit && a.Branch == d.ent.branch:
			// A commit numbers lines as the file is at that commit: show the comment where the branch diff has its
			// text, and leave out one on lines a later commit changed.
			if l.Status == review.StatusOutdated {
				out = append(out, l)
			} else if moved, ok := review.FindLines(d.fileDiff(a.Path), a); ok {
				l.Anchor = moved
				out = append(out, l)
			}
		case d.ent.kind.uncommitted() && a.Kind != review.KindCommit,
			d.ent.kind == entCommit && a.Kind == review.KindCommit && matchesCommit(a, d.ent.commit),
			d.ent.kind == entCommittedFile && a.Kind == review.KindCommit && matchesCommit(a, d.ent.commit) && a.Path == d.ent.label:
			out = append(out, l)
		}
	}
	return out
}

// fileDiff is the diff shown of path, or nil.
func (d *details) fileDiff(path string) *but.FileDiff {
	if d.data == nil {
		return nil
	}
	return d.data.File(path)
}

// cursorNote is the comment under the line cursor.
func (d *details) cursorNote() (review.Located, bool) {
	rows := d.layout.rows
	if !d.focused || d.cursor < 0 || d.cursor >= len(rows) || rows[d.cursor].note == "" {
		return review.Located{}, false
	}
	for _, l := range d.notes {
		if l.Comment.ID == rows[d.cursor].note {
			return l, true
		}
	}
	return review.Located{}, false
}

// jumpTo puts the line cursor on a comment.
func (d *details) jumpTo(id string) {
	d.jump = ""
	for i, r := range d.layout.rows {
		if r.note == id && r.top {
			d.focused, d.anchor = true, -1
			d.setCursor(i, d.marks)
			return
		}
	}
}

// rowID identifies a layout row across relayouts, which shift rows when comment boxes come and go.
type rowID struct {
	hunk     int
	sign     byte
	old, new int
	note     string
}

func (d *details) rowID(i int) (rowID, bool) {
	if i < 0 || i >= len(d.layout.rows) {
		return rowID{}, false
	}
	r := d.layout.rows[i]
	if r.note != "" {
		return rowID{hunk: r.hunk, note: r.note}, true
	}
	return rowID{hunk: r.hunk, sign: r.sign, old: r.old, new: r.new}, r.isLine()
}

// cursorRows remembers where the cursor and range are: each as its row, then the line above for a comment row,
// in case that comment goes away.
type cursorRows struct{ cursor, anchor []rowID }

func (d *details) rowIDs() cursorRows {
	ids := func(i int) []rowID {
		var out []rowID
		for ; i >= 0; i-- {
			id, ok := d.rowID(i)
			if ok {
				out = append(out, id)
				if id.note == "" {
					break
				}
			}
		}
		return out
	}
	return cursorRows{cursor: ids(d.cursor), anchor: ids(d.anchor)}
}

// restoreRows moves the cursor and range back onto the rows they were on.
func (d *details) restoreRows(k cursorRows) {
	find := func(ids []rowID, fallback int) int {
		for _, id := range ids {
			for i := range d.layout.rows {
				if got, ok := d.rowID(i); ok && got == id && (id.note == "" || d.layout.rows[i].top) {
					return i
				}
			}
		}
		return fallback
	}
	if len(k.cursor) > 0 {
		d.cursor = find(k.cursor, d.cursor)
	}
	if len(k.anchor) > 0 {
		d.anchor = find(k.anchor, d.anchor)
	}
}

// redraw updates the selection and marks by redrawing only the hunks that
// changed. It writes into the viewport's content in place: SetContentLines
// measures every line, which is slow for large diffs, and hunk gutters never
// change a line's width.
func (d *details) redraw(marks map[string]bool) {
	sel := -1
	if d.focused {
		sel = d.hunk
	}
	if !d.patch || len(d.lines) != len(d.header)+len(d.layout.rows)+detailsTail {
		d.rerender(marks)
		return
	}
	o := d.opts(sel, marks)
	dst := d.lines[len(d.header):]
	for i, h := range d.layout.hunks {
		if i == sel || i == d.drawn || (h.id != "" && marks[h.id] != d.marks[h.id]) {
			d.layout.drawHunk(dst, i, o)
		}
	}
	d.drawn, d.marks = sel, marks
}

// setSize sizes the pane; full screen, the file tree takes its share of the width.
func (d *details) setSize(w, h int) {
	d.outerW = w
	if d.treeShown() {
		w -= d.treeWidth() + 1
	}
	if w != d.width {
		d.width = w
		d.rerender()
	}
	d.vp.SetWidth(w)
	d.vp.SetHeight(max(h, 1))
	d.tree.height = h - 2 // its header and rule
	d.tree.scroll(0)      // back into range if the tree got taller
	if !d.tree.focused {
		d.tree.reveal(d.activeFile())
	}
}

// treeShown reports whether the file tree is drawn: full screen, unless hidden or the screen is narrow.
func (d *details) treeShown() bool {
	return d.full && !d.tree.hidden && d.outerW >= treeMinWidth
}

// treeWidth is the tree's width: the default share of the pane, or the width it was resized to.
func (d *details) treeWidth() int {
	if d.tree.width > 0 {
		return d.clampTreeWidth(d.tree.width)
	}
	return clamp(d.outerW/4, 24, 40)
}

// clampTreeWidth keeps a tree width readable and leaves the diff room.
func (d *details) clampTreeWidth(w int) int { return clamp(w, 12, max(d.outerW-40, 12)) }

// resizeTree widens (or narrows) the tree by dw.
func (d *details) resizeTree(dw int) { d.tree.width = d.clampTreeWidth(d.treeWidth() + dw) }

// activeFile is the file the pane is at: the line cursor's while it is on screen, else the one at the top.
func (d *details) activeFile() string {
	rows := d.layout.rows
	if d.focused && d.cursor >= 0 && d.cursor < len(rows) && rows[d.cursor].hunk >= 0 {
		if y := len(d.header) + d.cursor; y >= d.vp.YOffset() && y < d.vp.YOffset()+d.vp.Height() {
			return d.layout.hunks[rows[d.cursor].hunk].path
		}
	}
	top, path := d.vp.YOffset()-len(d.header), ""
	for i, f := range d.layout.files {
		if i == 0 || f.row <= top {
			path = f.path
		}
	}
	return path
}

// jumpToFile scrolls the diff to a file's header, with the line cursor on its first hunk.
func (d *details) jumpToFile(path string, marks map[string]bool) {
	for _, f := range d.layout.files {
		if f.path != path {
			continue
		}
		if f.hunk >= 0 {
			d.selectHunk(f.hunk, marks)
		}
		d.vp.SetYOffset(len(d.header) + f.row)
		return
	}
}

// focus gives the pane focus, selecting the first hunk when none is.
func (d *details) focus(marks map[string]bool) {
	d.focused = true
	if d.hunk < 0 {
		d.selectHunk(0, marks)
	} else {
		d.redraw(marks)
	}
}

func (d *details) opts(sel int, marks map[string]bool) diffOpts {
	o := diffOpts{selected: sel, marked: marks, cursor: d.cursor, anchor: d.anchor,
		lines: d.focused && d.cursor >= 0, ranged: d.anchor >= 0}
	if d.cursor >= 0 && d.cursor < len(d.layout.rows) {
		o.note = d.layout.rows[d.cursor].note
	}
	return o
}

// fixCursor keeps the line cursor and range inside the selected hunk after
// the diff changed, moving the cursor to the hunk's first line when it fell out.
func (d *details) fixCursor() {
	rows := d.layout.rows
	if d.hunk < 0 || d.hunk >= len(d.layout.hunks) {
		d.cursor, d.anchor = -1, -1
		return
	}
	in := func(r int) bool {
		return r >= 0 && r < len(rows) && rows[r].hunk == d.hunk && (rows[r].isLine() || rows[r].isNote() && d.anchor < 0)
	}
	if !in(d.cursor) {
		d.cursor, d.anchor = d.firstLine(d.hunk), -1
	}
	if d.anchor >= 0 && !in(d.anchor) {
		d.anchor = -1
	}
}

// firstLine is the first row of hunk i the line cursor can stop on, or -1.
func (d *details) firstLine(i int) int {
	h := d.layout.hunks[i]
	for r := h.line; r < h.end; r++ {
		if d.layout.rows[r].isLine() {
			return r
		}
	}
	return -1
}

// nextLine is the closest row after (dir > 0) or before row that the line
// cursor can stop on (a line, or a comment), or -1. While a range is being
// selected it stays in the range's hunk and skips comments.
func (d *details) nextLine(row, dir int) int {
	rows := d.layout.rows
	for r := row + dir; r >= 0 && r < len(rows); r += dir {
		if d.anchor >= 0 && rows[r].hunk != rows[d.anchor].hunk {
			return -1
		}
		if rows[r].isLine() || rows[r].isNote() && d.anchor < 0 {
			return r
		}
	}
	return -1
}

// setCursor moves the line cursor to a row, selecting its hunk, and scrolls it
// into view: with the hunk header when it is the hunk's first line, and a
// comment whole, with the line it is on.
func (d *details) setCursor(row int, marks map[string]bool) {
	rows := d.layout.rows
	d.cursor, d.hunk = row, rows[row].hunk
	d.redraw(marks)
	end := row
	for rows[row].note != "" && end+1 < len(rows) && rows[end+1].note == rows[row].note && !rows[end+1].top {
		end++
	}
	top, y := len(d.header)+row, len(d.header)+end
	if row > 0 && (rows[row-1].header || rows[row].isNote()) {
		top--
	}
	switch {
	case top < d.vp.YOffset():
		d.vp.SetYOffset(top)
	case y >= d.vp.YOffset()+d.vp.Height():
		d.vp.SetYOffset(min(y-d.vp.Height()+1, top))
	}
}

// toggleRange starts a range selection at the line cursor, or ends it.
func (d *details) toggleRange(marks map[string]bool) {
	switch {
	case d.anchor >= 0:
		d.anchor = -1
	case d.cursor >= 0:
		d.anchor = d.cursor
	}
	d.redraw(marks)
}

// cancelRange drops the range selection, reporting whether there was one.
func (d *details) cancelRange(marks map[string]bool) bool {
	if d.anchor < 0 {
		return false
	}
	d.anchor = -1
	d.redraw(marks)
	return true
}

// clickLine handles a click on content line y of the pane: it moves the line
// cursor there, extending the range when one is being selected or extend is
// set. A click on a hunk header selects the hunk.
func (d *details) clickLine(y int, extend bool, marks map[string]bool) {
	row := d.vp.YOffset() + y - len(d.header)
	if !d.patch || row < 0 || row >= len(d.layout.rows) {
		return
	}
	r := d.layout.rows[row]
	switch {
	case r.header:
		d.selectHunk(r.hunk, marks)
	case r.note != "":
		for !d.layout.rows[row].top {
			row--
		}
		d.anchor = -1
		d.setCursor(row, marks)
	case r.isLine():
		if d.anchor >= 0 && d.layout.rows[d.anchor].hunk != r.hunk {
			d.anchor = -1
		}
		if extend && d.anchor < 0 && d.cursor >= 0 && d.layout.rows[d.cursor].hunk == r.hunk {
			d.anchor = d.cursor
		}
		d.setCursor(row, marks)
	}
}

// lineSel is the line under the cursor, or the selected range, in the terms a
// review comment is anchored in.
type lineSel struct {
	path          string
	side          review.Side // old when every line is a removed one, new otherwise
	line, endLine int         // on side; equal for a single line
	text          string      // the lines on side, newline separated, without the diff signs
	hunk          hunkRef
}

// lineSelection reports the line cursor or range, when there is one.
func (d *details) lineSelection() (lineSel, bool) {
	rows := d.layout.rows
	if !d.focused || d.cursor < 0 || d.cursor >= len(rows) || !rows[d.cursor].isLine() {
		return lineSel{}, false
	}
	lo, hi := d.cursor, d.cursor
	if d.anchor >= 0 {
		lo, hi = min(d.cursor, d.anchor), max(d.cursor, d.anchor)
	}
	h := d.layout.hunks[rows[d.cursor].hunk]
	s := lineSel{path: h.path, side: review.SideOld, hunk: h}
	for _, r := range rows[lo : hi+1] {
		if r.isLine() && r.sign != '-' {
			s.side = review.SideNew
		}
	}
	var text []string
	for _, r := range rows[lo : hi+1] {
		n := r.new
		if s.side == review.SideOld {
			n = r.old
		}
		if !r.isLine() || n == 0 {
			continue // not a line, or a removed line inside a range on the new side
		}
		if s.line == 0 {
			s.line = n
		}
		s.endLine = n
		text = append(text, r.code)
	}
	s.text = strings.Join(text, "\n")
	return s, true
}

// selectHunk moves the hunk selection, with the line cursor on its first line,
// and scrolls it into view.
func (d *details) selectHunk(i int, marks map[string]bool) {
	if len(d.doc.hunks) == 0 {
		return
	}
	d.hunk = clamp(i, 0, len(d.doc.hunks)-1)
	d.cursor, d.anchor = d.firstLine(d.hunk), -1
	d.redraw(marks)
	h := d.doc.hunks[d.hunk]
	top, bottom := len(d.header)+h.line, len(d.header)+h.end
	switch {
	case top < d.vp.YOffset():
		d.vp.SetYOffset(top)
	case bottom > d.vp.YOffset()+d.vp.Height():
		d.vp.SetYOffset(min(top, bottom-d.vp.Height()))
	}
}

// step moves the line cursor by dir, across hunks and files. Past the first and
// last line it scrolls to the content beyond; while selecting a range it stops
// at the edge of the hunk.
func (d *details) step(dir int, marks map[string]bool) {
	if len(d.doc.hunks) > 0 && (d.hunk < 0 || d.cursor < 0) {
		d.selectHunk(max(d.hunk, 0), marks)
		return
	}
	next := -1
	if len(d.doc.hunks) > 0 {
		next = d.nextLine(d.cursor, dir)
	}
	switch {
	case next >= 0:
		d.setCursor(next, marks)
	case d.anchor >= 0:
	case dir > 0:
		d.vp.ScrollDown(1)
	default:
		d.vp.ScrollUp(1)
	}
}

// stepHunk moves the hunk selection, and the line cursor with it, by dir.
func (d *details) stepHunk(dir int, marks map[string]bool) {
	if d.hunk < 0 {
		dir = 0
	}
	d.selectHunk(d.hunk+dir, marks)
}

func (d *details) selectedHunk() (hunkRef, bool) {
	if d.hunk < 0 || d.hunk >= len(d.doc.hunks) {
		return hunkRef{}, false
	}
	return d.doc.hunks[d.hunk], true
}

// hunkEntity is the selected hunk as an operation subject.
func (d *details) hunkEntity() (entity, bool) {
	h, ok := d.selectedHunk()
	if !ok || h.id == "" || !d.ent.kind.uncommitted() {
		return entity{}, false
	}
	return entity{kind: entHunk, id: h.id, label: h.path + " " + strings.SplitN(h.text, "\n", 2)[0]}, true
}

func commitHeader(e entity) []string {
	if e.kind != entCommit || e.commit == nil {
		return nil
	}
	c := e.commit
	label := mutedStyle.Width(12).Render
	lines := []string{
		label("Commit") + c.CommitID,
	}
	if c.ChangeID != "" {
		lines = append(lines, label("Change")+c.ChangeID)
	}
	lines = append(lines,
		label("Author")+fmt.Sprintf("%s <%s>", c.AuthorName, c.AuthorEmail),
		label("Date")+c.CreatedAt.Local().Format("Mon 2 Jan 2006 15:04"),
		"",
	)
	msg := strings.TrimSpace(c.Message)
	if msg == "" {
		msg = mutedStyle.Italic(true).Render("(no message)")
	}
	for _, l := range strings.Split(msg, "\n") {
		lines = append(lines, "  "+l)
	}
	if conflicted(e) {
		return append(lines, "", errorStyle.Render("✗ Conflicted")+mutedStyle.Render(" · e resolves it in edit mode"), "")
	}
	return append(lines, "", mutedStyle.Render(pluralize(len(c.Changes), "file")+" changed"), "")
}

// view renders the pane with a title bar. The pane must already be sized by
// setSize: View works on a copy of the model, so sizing here would be lost and
// the diff re-rendered on every frame.
func (d *details) view(width, height int) string {
	title := "Details"
	if d.ent.valid() {
		title += " · " + d.ent.describe()
	}
	if d.loading {
		title += " …"
	}
	tree := d.treeShown()
	style := headerStyle
	if (d.focused || d.full) && (!tree || !d.tree.focused) {
		style = titleStyle
	}
	bar := style.Render(" " + ansi.Truncate(title, max(width-12, 1), "…"))
	if d.full {
		bar += strings.Repeat(" ", max(width-ansi.StringWidth(bar)-9, 1)) + buttonStyle.Render("esc ✕")
	}
	body := lipgloss.NewStyle().PaddingLeft(1).Render(d.vp.View())
	if tree {
		tw, divStyle := d.treeWidth(), dividerStyle
		if d.tree.dragW > 0 {
			// The divider follows the pointer; the diff is laid out at its new width once it is dropped.
			tw, divStyle = d.clampTreeWidth(d.tree.dragW), titleStyle
			body = lipgloss.NewStyle().MaxWidth(max(width-tw-1, 1)).Render(body)
		}
		vdiv := divStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", height-1), "\n"))
		body = lipgloss.JoinHorizontal(lipgloss.Top, d.tree.render(tw, height-1, d.activeFile()), vdiv, body)
	}
	return lipgloss.JoinVertical(lipgloss.Left, bar, body)
}
