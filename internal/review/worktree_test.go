package review

import "testing"

func TestWorktreeLines(t *testing.T) {
	file := []string{"package a", "", "func A() {}", "", "func B() {}", "", "func A() {}", "}"}
	anchor := func(kind Kind, side Side, line, end int, text string) Located {
		return Located{Anchor: Anchor{Kind: kind, Side: side, Line: line, EndLine: end, LineText: text}}
	}
	tests := []struct {
		name       string
		l          Located
		lines      []string
		start, end int
		exact      bool
	}{
		{"unassigned in place", anchor(KindUnassigned, SideNew, 3, 3, "func A() {}"), file, 3, 3, true},
		{"assigned range in place", anchor(KindAssigned, SideNew, 3, 5, "func A() {}\n\nfunc B() {}"), file, 3, 5, true},
		{"trailing whitespace ignored", anchor(KindUnassigned, SideNew, 5, 5, "func B() {}\r"), file, 5, 5, true},
		{"uncommitted line moved", anchor(KindUnassigned, SideNew, 4, 4, "func B() {}"), file, 5, 5, true},
		{"commit nearest occurrence", anchor(KindCommit, SideNew, 6, 6, "func A() {}"), file, 7, 7, true},
		{"commit keeps range length", anchor(KindCommit, SideNew, 1, 2, "func B() {}\n"), file, 5, 6, true},
		{"tie picks earlier", anchor(KindCommit, SideNew, 5, 5, "func A() {}"), file, 3, 3, true},
		{"old side searches", anchor(KindUnassigned, SideOld, 3, 3, "func B() {}"), file, 5, 5, true},
		{"range clamped to file", anchor(KindCommit, SideNew, 8, 10, "}"), file, 8, 8, true},
		{"not found", anchor(KindCommit, SideNew, 4, 6, "func C() {}"), file, 4, 4, false},
		{"not found past the end", anchor(KindUnassigned, SideNew, 40, 40, "gone"), file, 8, 8, false},
		{"empty file", anchor(KindUnassigned, SideNew, 3, 3, "gone"), nil, 1, 1, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, exact := WorktreeLines(tt.l, tt.lines)
			if start != tt.start || end != tt.end || exact != tt.exact {
				t.Fatalf("got %d-%d exact=%v, want %d-%d exact=%v", start, end, exact, tt.start, tt.end, tt.exact)
			}
		})
	}
}
