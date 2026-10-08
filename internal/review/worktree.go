package review

// WorktreeLines returns the 1-based lines a located comment covers in the working copy of its file, given as lines
// without EOLs: where an editor, which shows the working copy rather than a diff, should mark it.
//
// Uncommitted comments on the new side count lines in the working copy already. Others, and lines that moved, are
// found by the text of their first line, at the occurrence nearest the anchor's line. exact is false when the text is
// not in the file, and the comment falls back to its anchor's line.
func WorktreeLines(l Located, lines []string) (start, end int, exact bool) {
	a := l.Anchor
	span := max(a.EndLine-a.Line, 0)
	first := anchorLines(a)[0]
	at := func(n int) bool { return n >= 1 && n <= len(lines) && trimEOL(lines[n-1]) == first }

	if (a.Kind == KindUnassigned || a.Kind == KindAssigned) && a.Side != SideOld && at(a.Line) {
		return a.Line, min(a.Line+span, len(lines)), true
	}
	best := 0
	for n := 1; n <= len(lines); n++ {
		if at(n) && (best == 0 || abs(n-a.Line) < abs(best-a.Line)) {
			best = n
		}
	}
	if best > 0 {
		return best, min(best+span, len(lines)), true
	}
	start = min(max(a.Line, 1), max(len(lines), 1))
	return start, start, false
}
