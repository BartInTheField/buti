package ui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

var (
	colorAddBg = lipgloss.Color("#1E3A2B")
	colorDelBg = lipgloss.Color("#43232A")

	fileHeaderStyle = lipgloss.NewStyle().Bold(true).Background(colorSurface).Foreground(colorText)
	hunkHeaderStyle = lipgloss.NewStyle().Foreground(colorAccent)
	gutterStyle     = lipgloss.NewStyle().Foreground(colorMuted)
)

type diffLine struct {
	sign     byte   // '+', '-', ' ' or '\\' (no newline marker)
	old, new int    // line numbers; 0 when absent on that side
	code     string // the line as in the file, without the sign
	segs     []segment
}

// hunkRef locates one hunk inside a rendered diff document.
type hunkRef struct {
	line int    // first line (the @@ header) in the document
	end  int    // one past the last line
	id   string // CLI id for uncommitted hunks, empty otherwise
	path string
	text string // raw unified hunk, for copying
}

// diffDoc is a rendered diff plus the positions of its hunks.
type diffDoc struct {
	lines []string
	hunks []hunkRef
}

// diffOpts controls hunk highlighting while rendering.
type diffOpts struct {
	selected int             // hunk index drawn as selected, -1 for none
	marked   map[string]bool // hunk ids drawn as marked
	// The line cursor and range selection, as rows. They are only drawn
	// inside the selected hunk, and only when lines (and ranged) are set.
	cursor, anchor int
	lines, ranged  bool
	note           string // id of the comment under the cursor, drawn selected
}

// diffRow is one line of a laid out diff, before hunk selection and marks are drawn.
type diffRow struct {
	hunk   int    // index into the hunks, -1 for lines outside a hunk
	header bool   // the hunk's @@ line; text is unstyled and already truncated
	text   string // the rendered line, without the two column hunk gutter
	// The diff line the row shows, for the line cursor.
	sign     byte
	old, new int
	code     string
	// A row of an inline review comment box.
	note string // the comment's id, "" for other rows
	top  bool   // the box's first row, where the line cursor stops on the comment
	alt  string // text drawn while the comment is under the cursor
}

// isLine reports whether the row is a diff line the line cursor can stop on.
func (r diffRow) isLine() bool {
	return r.hunk >= 0 && !r.header && (r.sign == '+' || r.sign == '-' || r.sign == ' ')
}

// isNote reports whether the row is the top of a comment box, where the line cursor also stops.
func (r diffRow) isNote() bool { return r.note != "" && r.top }

// diffLayout is a diff highlighted and rendered at one width. Selection and
// marks are applied on top by doc, so moving between hunks stays cheap.
type diffLayout struct {
	rows  []diffRow
	hunks []hunkRef
	numW  int // width of a line number in the gutter
}

// renderDiff draws a syntax highlighted unified diff at the given width.
func renderDiff(d *but.Diff, width int, o diffOpts) diffDoc {
	return layoutDiff(d, width).doc(o)
}

// layoutDiff does the expensive part of rendering: parsing and highlighting.
func layoutDiff(d *but.Diff, width int) diffLayout {
	var l diffLayout
	if len(d.Changes) == 0 {
		l.rows = []diffRow{{hunk: -1, text: mutedStyle.Render("No changes")}}
		return l
	}
	numW := 1
	for _, f := range d.Changes {
		for _, h := range f.Diff.Hunks {
			numW = max(numW, len(strconv.Itoa(h.OldStart+h.OldLines)), len(strconv.Itoa(h.NewStart+h.NewLines)))
		}
	}
	l.numW = numW
	plain := func(text string) { l.rows = append(l.rows, diffRow{hunk: -1, text: text}) }
	prevPath := ""
	for _, f := range d.Changes {
		// Uncommitted diffs list one entry per hunk; group them under one file header.
		if f.Path != prevPath {
			if prevPath != "" {
				plain("")
			}
			letter, st := changeTypeStyle(f.Status)
			header := " " + st.Inherit(fileHeaderStyle).Render(letter) + fileHeaderStyle.Render(" "+f.Path)
			plain(fileHeaderStyle.Width(width).Render(ansi.Truncate(header, width, "…")))
		}
		prevPath = f.Path

		if f.Diff.Type != "patch" {
			plain(mutedStyle.Render(fmt.Sprintf("  (%s, no text diff)", f.Diff.Type)))
			continue
		}
		for _, h := range f.Diff.Hunks {
			id := f.ID
			if len(f.Diff.Hunks) > 1 {
				id = "" // ids address single-hunk entries only
			}
			ref := hunkRef{line: len(l.rows), id: id, path: f.Path, text: h.Diff}
			idx := len(l.hunks)
			header, lines := parseHunk(f.Path, h)
			l.rows = append(l.rows, diffRow{hunk: idx, header: true, text: ansi.Truncate(header, max(width-2, 1), "…")})
			for _, dl := range lines {
				l.rows = append(l.rows, diffRow{hunk: idx, text: renderDiffLine(dl, numW, width-2),
					sign: dl.sign, old: dl.old, new: dl.new, code: dl.code})
			}
			ref.end = len(l.rows)
			l.hunks = append(l.hunks, ref)
		}
	}
	return l
}

// doc draws the hunk gutters for the given selection and marks.
func (l diffLayout) doc(o diffOpts) diffDoc {
	doc := diffDoc{lines: make([]string, len(l.rows)), hunks: l.hunks}
	for i, r := range l.rows {
		if r.hunk < 0 {
			doc.lines[i] = r.text
		}
	}
	for i := range l.hunks {
		l.drawHunk(doc.lines, i, o)
	}
	return doc
}

var (
	selBarGlyph    = lipgloss.NewStyle().Foreground(colorAccent).Render("▶ ")
	selGutterGlyph = lipgloss.NewStyle().Foreground(colorAccent).Render("▌ ")
	selHunkHeader  = hunkHeaderStyle.Bold(true).Reverse(true)

	// The line cursor and range sit in the second gutter column, next to the hunk bar.
	selBar           = lipgloss.NewStyle().Foreground(colorAccent).Render("▌")
	cursorGlyph      = selBar + lipgloss.NewStyle().Foreground(colorText).Bold(true).Render("▶")
	rangeGlyph       = selBar + lipgloss.NewStyle().Foreground(colorMod).Render("┃")
	rangeCursorGlyph = selBar + lipgloss.NewStyle().Foreground(colorMod).Bold(true).Render("▶")
)

// drawHunk writes hunk i's lines into dst, which is indexed like the rows.
func (l diffLayout) drawHunk(dst []string, i int, o diffOpts) {
	ref := l.hunks[i]
	sel := i == o.selected
	bar, hh, gutter := "  ", hunkHeaderStyle, "  "
	switch {
	case ref.id != "" && o.marked[ref.id]:
		bar = markGlyph + " "
	case sel:
		bar = selBarGlyph
	}
	if sel {
		hh, gutter = selHunkHeader, selGutterGlyph
	}
	lo, hi := o.cursor, o.cursor
	if o.ranged {
		lo, hi = min(o.cursor, o.anchor), max(o.cursor, o.anchor)
	}
	for j := ref.line; j < ref.end; j++ {
		r := l.rows[j]
		switch {
		case r.header:
			dst[j] = bar + hh.Render(r.text)
		case r.note != "" && sel && o.lines && r.note == o.note:
			glyph := selBar + " "
			if j == o.cursor {
				glyph = cursorGlyph
			}
			dst[j] = glyph + r.alt
		case r.note != "":
			dst[j] = gutter + r.text
		case !sel || !o.lines || j < lo || j > hi:
			dst[j] = gutter + r.text
		case j == o.cursor && o.ranged:
			dst[j] = rangeCursorGlyph + r.text
		case j == o.cursor:
			dst[j] = cursorGlyph + r.text
		default:
			dst[j] = rangeGlyph + r.text
		}
	}
}

func (d diffDoc) String() string { return strings.Join(d.lines, "\n") }

// parseHunk splits a hunk into lines, highlighting the old and new sides
// separately so each lexer sees coherent source.
func parseHunk(path string, h but.Hunk) (string, []diffLine) {
	raw := strings.Split(strings.TrimSuffix(h.Diff, "\n"), "\n")
	header, raw := raw[0], raw[1:]

	var oldSrc, newSrc []string
	for _, r := range raw {
		if r == "" {
			r = " "
		}
		code := expandTabs(r[1:])
		switch r[0] {
		case '-':
			oldSrc = append(oldSrc, code)
		case '+':
			newSrc = append(newSrc, code)
		case ' ':
			oldSrc = append(oldSrc, code)
			newSrc = append(newSrc, code)
		}
	}
	oldHL := highlight(path, strings.Join(oldSrc, "\n"))
	newHL := highlight(path, strings.Join(newSrc, "\n"))
	at := func(hl [][]segment, i int) []segment {
		if i < len(hl) {
			return hl[i]
		}
		return nil
	}

	lines := make([]diffLine, 0, len(raw))
	oi, ni := 0, 0
	for _, r := range raw {
		if r == "" {
			r = " "
		}
		l := diffLine{sign: r[0], code: r[1:]}
		switch r[0] {
		case '-':
			l.old, l.segs = h.OldStart+oi, at(oldHL, oi)
			oi++
		case '+':
			l.new, l.segs = h.NewStart+ni, at(newHL, ni)
			ni++
		case ' ':
			l.old, l.new, l.segs = h.OldStart+oi, h.NewStart+ni, at(newHL, ni)
			oi++
			ni++
		default:
			l.segs = []segment{{text: r, style: mutedStyle}}
		}
		lines = append(lines, l)
	}
	return header, lines
}

func renderDiffLine(l diffLine, numW, width int) string {
	var bg lipgloss.Style
	switch l.sign {
	case '+':
		bg = lipgloss.NewStyle().Background(colorAddBg)
	case '-':
		bg = lipgloss.NewStyle().Background(colorDelBg)
	}
	num := func(n int) string {
		if n == 0 {
			return strings.Repeat(" ", numW)
		}
		return fmt.Sprintf("%*d", numW, n)
	}
	sign := string(l.sign)
	if l.sign == '\\' {
		sign = " "
	}
	gutter := gutterStyle.Render(num(l.old)+" "+num(l.new)+" │") + bg.Render(sign+" ")

	var code strings.Builder
	used := 0
	avail := max(width-ansi.StringWidth(gutter), 0)
	for _, s := range l.segs {
		if used >= avail {
			break
		}
		text := ansi.Truncate(s.text, avail-used, "")
		used += ansi.StringWidth(text)
		code.WriteString(s.style.Inherit(bg).Render(text))
	}
	if used < avail {
		code.WriteString(bg.Render(strings.Repeat(" ", avail-used)))
	}
	return gutter + code.String()
}
