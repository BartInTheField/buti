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
	layout  diffLayout // data rendered at layoutW, reused while only the selection changes
	layoutD *but.Diff
	layoutW int
	marks   map[string]bool // hunk marks last drawn
	doc     diffDoc
	drawn   int      // hunk drawn as selected in lines
	lines   []string // the viewport's content; doc.lines follow the header when patchable
	patch   bool     // whether lines hold the current layout, so hunks can be redrawn in place
	hunk    int      // selected hunk, when focused
	vp      viewport.Model
	width   int
}

type detailsMsg struct {
	key  string
	diff *but.Diff
	err  error
}

func newDetails() details {
	vp := viewport.New()
	vp.MouseWheelDelta = 3
	return details{pct: 45, vp: vp, hunk: -1}
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
		d.hunk = -1
		d.vp.GotoTop()
	}
	d.reqKey, d.ent = key, e
	d.header = commitHeader(e)
	id, prefix, ok := diffTarget(e)
	if !ok {
		d.data, d.err, d.loading = nil, nil, false
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
		if w := max(d.width, 20); d.layoutD != d.data || d.layoutW != w {
			d.layout, d.layoutD, d.layoutW = layoutDiff(d.data, w), d.data, w
		}
		d.doc = d.layout.doc(diffOpts{selected: sel, marked: d.marks})
		lines = append(lines, d.doc.lines...)
		lines = append(lines, make([]string, detailsTail)...)
		d.drawn, d.patch = sel, true
	}
	d.lines = lines
	d.vp.SetContentLines(lines)
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
	o := diffOpts{selected: sel, marked: marks}
	dst := d.lines[len(d.header):]
	for i, h := range d.layout.hunks {
		if i == sel || i == d.drawn || (h.id != "" && marks[h.id] != d.marks[h.id]) {
			d.layout.drawHunk(dst, i, o)
		}
	}
	d.drawn, d.marks = sel, marks
}

func (d *details) setSize(w, h int) {
	if w != d.width {
		d.width = w
		d.rerender()
	}
	d.vp.SetWidth(w)
	d.vp.SetHeight(max(h, 1))
}

// selectHunk moves the hunk selection and scrolls it into view.
func (d *details) selectHunk(i int, marks map[string]bool) {
	if len(d.doc.hunks) == 0 {
		return
	}
	d.hunk = clamp(i, 0, len(d.doc.hunks)-1)
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

// step moves the hunk selection by dir. While the selected hunk continues past
// the edge of the pane, it scrolls instead, so a hunk taller than the pane can be
// read to the end; past the first and last hunk it scrolls to the content beyond.
func (d *details) step(dir int, marks map[string]bool) {
	switch {
	case len(d.doc.hunks) > 0 && d.hunk < 0:
		d.selectHunk(0, marks)
		return
	case len(d.doc.hunks) == 0 && dir > 0:
		d.vp.ScrollDown(1)
		return
	case len(d.doc.hunks) == 0:
		d.vp.ScrollUp(1)
		return
	}
	h := d.doc.hunks[d.hunk]
	top, bottom := len(d.header)+h.line, len(d.header)+h.end
	y := d.vp.YOffset()
	visible := bottom > y && top < y+d.vp.Height()
	switch {
	case dir > 0 && visible && bottom > y+d.vp.Height(), dir > 0 && d.hunk == len(d.doc.hunks)-1:
		d.vp.ScrollDown(1)
	case dir < 0 && visible && top < y, dir < 0 && d.hunk == 0:
		d.vp.ScrollUp(1)
	default:
		d.selectHunk(d.hunk+dir, marks)
	}
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
	style := headerStyle
	if d.focused || d.full {
		style = titleStyle
	}
	bar := style.Render(" " + ansi.Truncate(title, max(width-12, 1), "…"))
	if d.full {
		bar += strings.Repeat(" ", max(width-ansi.StringWidth(bar)-9, 1)) + buttonStyle.Render("esc ✕")
	}
	return lipgloss.JoinVertical(lipgloss.Left, bar, lipgloss.NewStyle().PaddingLeft(1).Render(d.vp.View()))
}
