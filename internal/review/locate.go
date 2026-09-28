package review

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/bartinthefield/buti/internal/but"
)

// TargetUnassigned is the shortcode of the unassigned changes.
const TargetUnassigned = "zz"

// Differ fetches `but diff --json` for a cli id; *but.Client is one.
type Differ interface {
	Diff(ctx context.Context, cliID string) (*but.Diff, error)
}

// Source is what re-anchoring reads from GitButler; *but.Client is one.
type Source interface {
	Differ
	Status(ctx context.Context) (*but.Status, error)
}

// Located is a comment mapped onto the current workspace.
type Located struct {
	Comment Comment // as stored

	// Status is the comment's status, or StatusOutdated / StatusOrphaned for an open comment whose line or commit
	// is gone.
	Status Status

	// Anchor is where the comment points now. It differs from Comment.Anchor when the line moved or the change was
	// committed; for an outdated or orphaned comment it is the stored anchor, with the original line and text.
	Anchor Anchor

	// Target is the current shortcode of what the comment is on: TargetUnassigned, the stack's cli id for assigned
	// changes, or the commit's cli id. It is empty for an orphaned comment.
	Target string

	// FileID is the current cli id of the file within Target, empty when the file is not there.
	FileID string
}

// Moved reports whether the comment's anchor changed and should be saved.
func (l Located) Moved() bool {
	return l.Anchor != l.Comment.Anchor
}

// Reanchor maps the comments with the given statuses (all without any) onto the current status, saves the anchors
// that moved and returns the comments in their current place, oldest first.
func (s *Store) Reanchor(ctx context.Context, src Source, statuses ...Status) ([]Located, error) {
	cs, err := s.List(statuses...)
	if err != nil || len(cs) == 0 {
		return nil, err
	}
	st, err := src.Status(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := Locate(ctx, st, src, cs)
	if err != nil {
		return nil, err
	}
	return ls, s.Save(ls)
}

// Save stores the new anchors of the comments that moved. A comment that was changed or deleted since it was
// located is left alone, so a concurrent edit is never overwritten with a stale anchor.
func (s *Store) Save(ls []Located) error {
	if !slices.ContainsFunc(ls, Located.Moved) {
		return nil
	}
	return s.modify(func(f *File) error {
		for _, l := range ls {
			if !l.Moved() {
				continue
			}
			i := slices.IndexFunc(f.Comments, func(c Comment) bool { return c.ID == l.Comment.ID })
			if i >= 0 && f.Comments[i].Anchor == l.Comment.Anchor {
				f.Comments[i].Anchor = l.Anchor
			}
		}
		return nil
	})
}

// Locate maps comments onto st, the current `but status --json -f`, fetching file diffs with d. It changes nothing
// on disk, so a caller that already holds a status (the TUI) can locate without another status call.
//
//   - A commit comment follows its commit by change id, which survives amends, falling back to the commit id. When
//     the commit is gone, the comment follows its lines to where an uncommit or a squash put them: the uncommitted
//     changes, then another commit. Only a line that is added or removed there counts, not context. Otherwise the
//     comment is orphaned.
//   - An uncommitted comment looks for its file in its own place (`zz` or its branch's stack), then in the other
//     uncommitted places, then in the applied commits, so it follows the file when it is committed.
//   - In the file's diff, a line whose text moved is found again at the nearest place it now is. A line that is not
//     in the diff any more makes the comment outdated.
func Locate(ctx context.Context, st *but.Status, d Differ, cs []Comment) ([]Located, error) {
	l := &locator{ctx: ctx, st: st, d: d, diffs: map[string]*but.FileDiff{}}
	out := make([]Located, 0, len(cs))
	for _, c := range cs {
		loc, err := l.locate(c)
		if err != nil {
			return nil, err
		}
		if c.Status != StatusOpen {
			loc.Status = c.Status
		}
		out = append(out, loc)
	}
	return out, nil
}

type locator struct {
	ctx   context.Context
	st    *but.Status
	d     Differ
	diffs map[string]*but.FileDiff // by file cli id; nil for a file the diff does not have
}

func (l *locator) locate(c Comment) (Located, error) {
	if c.Anchor.Kind == KindCommit {
		return l.locateCommit(c)
	}
	return l.locateUncommitted(c)
}

// found returns c on a new place with the anchor moved to line.
func found(c Comment, a Anchor, line int, target, fileID string) Located {
	a.EndLine = line + (a.EndLine - a.Line)
	a.Line = line
	return Located{Comment: c, Status: StatusOpen, Anchor: a, Target: target, FileID: fileID}
}

func (l *locator) locateCommit(c Comment) (Located, error) {
	a := c.Anchor
	commit, branch := l.findCommit(a)
	if commit == nil {
		loc, ok, err := l.follow(c)
		if err != nil || ok {
			return loc, err
		}
		return Located{Comment: c, Status: StatusOrphaned, Anchor: a}, nil
	}
	moved := a
	moved.CommitID, moved.Branch = commit.CommitID, branch
	fileID := changeID(commit.Changes, a.Path)
	if fileID == "" {
		return Located{Comment: c, Status: StatusOutdated, Anchor: a, Target: commit.CliID}, nil
	}
	m, err := l.match(fileID, a)
	if err != nil {
		return Located{}, err
	}
	if m.line == 0 {
		return Located{Comment: c, Status: StatusOutdated, Anchor: a, Target: commit.CliID, FileID: fileID}, nil
	}
	return found(c, moved, m.line, commit.CliID, fileID), nil
}

// follow finds the changed lines of a comment whose commit is gone in the uncommitted changes, then in the applied
// commits. Lines without a letter or digit, like a blank line or a lone brace, are in too many places to follow.
func (l *locator) follow(c Comment) (Located, bool, error) {
	a := c.Anchor
	if !slices.ContainsFunc(anchorLines(a), hasWord) {
		return Located{}, false, nil
	}
	home := l.homeStack(a)
	for _, p := range l.uncommittedPlaces(a, home) {
		m, err := l.match(p.fileID, a)
		if err != nil {
			return Located{}, false, err
		}
		if m.line > 0 && m.changed {
			moved := a
			moved.Kind, moved.ChangeID, moved.CommitID, moved.Branch = p.kind, "", "", p.branch
			return found(c, moved, m.line, p.target, p.fileID), true, nil
		}
	}
	best, err := l.bestCommit(a, home)
	if err != nil || best == nil || !best.changed {
		return Located{}, false, err
	}
	return l.onCommit(c, best), true, nil
}

// findCommit finds the commit of a commit anchor in the applied stacks, and the branch it is on.
func (l *locator) findCommit(a Anchor) (*but.Commit, string) {
	for _, byChange := range []bool{true, false} {
		for si := range l.st.Stacks {
			for bi := range l.st.Stacks[si].Branches {
				b := &l.st.Stacks[si].Branches[bi]
				for ci := range b.Commits {
					cm := &b.Commits[ci]
					if byChange && a.ChangeID != "" && cm.ChangeID == a.ChangeID ||
						!byChange && a.CommitID != "" && cm.CommitID == a.CommitID {
						return cm, b.Name
					}
				}
			}
		}
	}
	return nil, ""
}

// place is where an uncommitted file lives: the unassigned changes or a stack.
type place struct {
	kind   Kind
	target string
	branch string
	fileID string
}

func (l *locator) locateUncommitted(c Comment) (Located, error) {
	a := c.Anchor
	home := l.homeStack(a)
	places := l.uncommittedPlaces(a, home)
	for _, p := range places {
		m, err := l.match(p.fileID, a)
		if err != nil {
			return Located{}, err
		}
		if m.line > 0 {
			moved := a
			moved.Kind, moved.Branch = p.kind, p.branch
			return found(c, moved, m.line, p.target, p.fileID), nil
		}
	}

	// Committed: prefer the commit that added the line over one that only shows it as context, then the commit
	// closest to the top of the comment's own stack.
	best, err := l.bestCommit(a, home)
	if err != nil {
		return Located{}, err
	}
	if best != nil {
		return l.onCommit(c, best), nil
	}

	loc := Located{Comment: c, Status: StatusOutdated, Anchor: a}
	switch {
	case len(places) > 0:
		loc.Target, loc.FileID = places[0].target, places[0].fileID
	case a.Kind == KindAssigned && home >= 0:
		loc.Target = l.st.Stacks[home].CliID
	case a.Kind == KindUnassigned:
		loc.Target = TargetUnassigned
	}
	return loc, nil
}

// homeStack is the index of the stack holding an anchor's branch, or -1.
func (l *locator) homeStack(a Anchor) int {
	if a.Branch == "" {
		return -1
	}
	return slices.IndexFunc(l.st.Stacks, func(s but.Stack) bool {
		return slices.ContainsFunc(s.Branches, func(b but.Branch) bool { return b.Name == a.Branch })
	})
}

// uncommittedPlaces lists where the anchor's file is uncommitted, the anchor's own place first.
func (l *locator) uncommittedPlaces(a Anchor, home int) []place {
	var ps []place
	if id := changeID(l.st.UncommittedChanges, a.Path); id != "" {
		ps = append(ps, place{kind: KindUnassigned, target: TargetUnassigned, fileID: id})
	}
	for i, s := range l.st.Stacks {
		id := changeID(s.AssignedChanges, a.Path)
		if id == "" || len(s.Branches) == 0 {
			continue
		}
		p := place{kind: KindAssigned, target: s.CliID, branch: s.Branches[0].Name, fileID: id}
		if i == home {
			p.branch = a.Branch
		}
		ps = append(ps, p)
	}
	own := slices.IndexFunc(ps, func(p place) bool {
		if a.Kind == KindUnassigned {
			return p.kind == KindUnassigned
		}
		return home >= 0 && p.target == l.st.Stacks[home].CliID
	})
	if own > 0 {
		ps[0], ps[own] = ps[own], ps[0]
	}
	return ps
}

type commitMatch struct {
	commit *but.Commit
	branch string
	fileID string
	match
}

// bestCommit finds an anchor's text in the applied commits: the commit that added the line wins over one that only
// shows it as context, then the commit closest to the top of the stack at index home. It is nil when no commit has it.
func (l *locator) bestCommit(a Anchor, home int) (*commitMatch, error) {
	var best *commitMatch
	for _, cm := range l.commitsWith(a.Path, home) {
		m, err := l.match(cm.fileID, a)
		if err != nil {
			return nil, err
		}
		if m.line > 0 && (best == nil || m.changed && !best.changed) {
			cm.match = m
			best = &cm
			if m.changed {
				break
			}
		}
	}
	return best, nil
}

// onCommit returns c moved onto the commit it was found in.
func (l *locator) onCommit(c Comment, cm *commitMatch) Located {
	moved := c.Anchor
	moved.Kind, moved.ChangeID, moved.CommitID, moved.Branch = KindCommit, cm.commit.ChangeID, cm.commit.CommitID, cm.branch
	return found(c, moved, cm.line, cm.commit.CliID, cm.fileID)
}

// OnBranch turns an anchor on the whole diff of branch into one on the branch's commit that has the lines: the
// newest one that changes them, else the newest that shows them as context. Comments live on commits, which
// re-anchoring and agents can find, rather than on a branch diff whose lines shift with every commit.
func OnBranch(ctx context.Context, st *but.Status, d Differ, branch string, a Anchor) (Anchor, error) {
	l := &locator{ctx: ctx, st: st, d: d, diffs: map[string]*but.FileDiff{}}
	var best *commitMatch
	for si := range st.Stacks {
		for bi := range st.Stacks[si].Branches {
			b := &st.Stacks[si].Branches[bi]
			if b.Name != branch {
				continue
			}
			for ci := range b.Commits {
				id := changeID(b.Commits[ci].Changes, a.Path)
				if id == "" {
					continue
				}
				m, err := l.match(id, a)
				if err != nil {
					return a, err
				}
				if m.line > 0 && (best == nil || m.changed && !best.changed) {
					best = &commitMatch{commit: &b.Commits[ci], branch: b.Name, fileID: id, match: m}
					if m.changed {
						break
					}
				}
			}
		}
	}
	if best == nil {
		return a, fmt.Errorf("No single commit on %s has these lines; comment on the commit instead", branch)
	}
	return l.onCommit(Comment{Anchor: a}, best).Anchor, nil
}

// FindLines finds an anchor's text in a file diff, as re-anchoring does: at its line, else at the nearest line
// that has it. It returns the anchor moved there, and false when the diff does not have the text.
func FindLines(fd *but.FileDiff, a Anchor) (Anchor, bool) {
	if fd == nil {
		return a, false
	}
	m := findLines(sideLines(fd, a.Side), a)
	if m.line == 0 {
		return a, false
	}
	a.EndLine = m.line + (a.EndLine - a.Line)
	a.Line = m.line
	return a, true
}

// commitsWith lists the applied commits that change path, newest first, with the stack at index home first.
func (l *locator) commitsWith(path string, home int) []commitMatch {
	order := make([]int, 0, len(l.st.Stacks))
	if home >= 0 {
		order = append(order, home)
	}
	for i := range l.st.Stacks {
		if i != home {
			order = append(order, i)
		}
	}
	var out []commitMatch
	for _, si := range order {
		for bi := range l.st.Stacks[si].Branches {
			b := &l.st.Stacks[si].Branches[bi]
			for ci := range b.Commits {
				if id := changeID(b.Commits[ci].Changes, path); id != "" {
					out = append(out, commitMatch{commit: &b.Commits[ci], branch: b.Name, fileID: id})
				}
			}
		}
	}
	return out
}

func changeID(cs []but.Change, path string) string {
	for _, c := range cs {
		if c.FilePath == path {
			return c.CliID
		}
	}
	return ""
}

// match is where an anchor's text was found in a file diff: its first line (0 when not found), and whether that
// line is changed on the anchor's side (added for new, removed for old) rather than context.
type match struct {
	line    int
	changed bool
}

func (l *locator) match(fileID string, a Anchor) (match, error) {
	fd, err := l.fileDiff(fileID, a.Path)
	if err != nil || fd == nil {
		return match{}, err
	}
	return findLines(sideLines(fd, a.Side), a), nil
}

// fileDiff returns the diff of the file with cli id fileID, nil when `but` shows none for path.
func (l *locator) fileDiff(fileID, path string) (*but.FileDiff, error) {
	if fd, ok := l.diffs[fileID]; ok {
		return fd, nil
	}
	d, err := l.d.Diff(l.ctx, fileID)
	if err != nil {
		return nil, fmt.Errorf("diff of %s: %w", path, err)
	}
	var fd *but.FileDiff
	for i := range d.Changes {
		if d.Changes[i].Path == path {
			fd = &d.Changes[i]
			break
		}
	}
	l.diffs[fileID] = fd
	return fd, nil
}

// diffLine is a line of a diff on one side.
type diffLine struct {
	text    string
	changed bool
}

// sideLines returns the lines a file diff shows on side, by line number.
func sideLines(fd *but.FileDiff, side Side) map[int]diffLine {
	lines := map[int]diffLine{}
	for _, h := range fd.Diff.Hunks {
		body := strings.Split(strings.TrimSuffix(h.Diff, "\n"), "\n")
		oldN, newN := h.OldStart, h.NewStart
		if len(body) > 0 && strings.HasPrefix(body[0], "@@") {
			if o, n, ok := hunkStarts(body[0]); ok {
				oldN, newN = o, n
			}
			body = body[1:]
		}
		for _, ln := range body {
			if ln == "" {
				ln = " " // some diffs drop the space of an empty context line
			}
			text := ln[1:]
			switch ln[0] {
			case ' ':
				if side == SideOld {
					lines[oldN] = diffLine{text: text}
				} else {
					lines[newN] = diffLine{text: text}
				}
				oldN++
				newN++
			case '+':
				if side != SideOld {
					lines[newN] = diffLine{text: text, changed: true}
				}
				newN++
			case '-':
				if side == SideOld {
					lines[oldN] = diffLine{text: text, changed: true}
				}
				oldN++
			}
		}
	}
	return lines
}

// hunkStarts reads the start lines of a "@@ -o,n +o,n @@" header.
func hunkStarts(header string) (oldStart, newStart int, ok bool) {
	f := strings.Fields(header)
	if len(f) < 3 {
		return 0, 0, false
	}
	start := func(s, sign string) (int, bool) {
		s, found := strings.CutPrefix(s, sign)
		s, _, _ = strings.Cut(s, ",")
		n, err := strconv.Atoi(s)
		return n, found && err == nil
	}
	o, ok1 := start(f[1], "-")
	n, ok2 := start(f[2], "+")
	return o, n, ok1 && ok2
}

// findLines finds the anchor's text in lines: at its line if it is still there, otherwise at the nearest line
// where the whole text starts, earlier lines winning a tie.
func findLines(lines map[int]diffLine, a Anchor) match {
	want := anchorLines(a)
	at := func(n int) bool {
		for i, w := range want {
			l, ok := lines[n+i]
			if !ok || trimEOL(l.text) != w {
				return false
			}
		}
		return true
	}
	if at(a.Line) {
		return match{line: a.Line, changed: lines[a.Line].changed}
	}
	best := 0
	for n := range lines {
		if !at(n) {
			continue
		}
		if best == 0 || abs(n-a.Line) < abs(best-a.Line) || abs(n-a.Line) == abs(best-a.Line) && n < best {
			best = n
		}
	}
	if best == 0 {
		return match{}
	}
	return match{line: best, changed: lines[best].changed}
}

// anchorLines splits an anchor's text into lines, which a range holds newline separated. An empty text is one
// blank line.
func anchorLines(a Anchor) []string {
	ls := strings.Split(a.LineText, "\n")
	if n := max(a.EndLine-a.Line+1, 1); len(ls) > n && ls[len(ls)-1] == "" {
		ls = ls[:len(ls)-1] // a trailing newline
	}
	for i := range ls {
		ls[i] = trimEOL(ls[i])
	}
	return ls
}

// trimEOL drops trailing whitespace, so a CRLF file or a stray space does not lose a comment.
func trimEOL(s string) string {
	return strings.TrimRight(s, " \t\r")
}

// hasWord reports whether s has a letter or a digit.
func hasWord(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
