package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

func TestPRPill(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    but.Branch
		want string
	}{
		{"no pr", but.Branch{}, ""},
		{"no checks", but.Branch{ReviewID: "(#3)", CI: &but.CI{Status: "complete", Conclusion: "unknown"}}, "#3"},
		{"passing", but.Branch{ReviewID: "(#3)", CI: &but.CI{Status: "complete", Conclusion: "success"}}, "#3 ✓"},
		{"failing", but.Branch{ReviewID: "(#3)", CI: &but.CI{Status: "complete", FailingCheckTitles: []string{"test"}}}, "#3 ✗"},
		{"pending", but.Branch{ReviewID: "(#3)", CI: &but.CI{Status: "inProgress"}}, "#3 ●"},
		{"merged", but.Branch{ReviewID: "(#3)", BranchStatus: "integrated"}, "#3 merged"},
	} {
		if got := ansi.Strip(prPill(tc.b)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestBranchCardShowsPR(t *testing.T) {
	s := &but.Status{Stacks: []but.Stack{{CliID: "s1", Branches: []but.Branch{{CliID: "b1", Name: "feature",
		ReviewID: "(#12)", CI: &but.CI{Status: "complete", Conclusion: "success"}}}}}}
	l := buildLanes(s, func(*but.Commit) bool { return false })[0]
	v := l.render(40, -1, func(entity) rowDeco { return rowDeco{} })
	for _, line := range v.lines {
		if strings.Contains(line, "feature") {
			if plain := ansi.Strip(line); !strings.Contains(plain, "#12 ✓") || ansi.StringWidth(line) != 40 {
				t.Errorf("header %q: want #12 ✓ within 40 columns", plain)
			}
			return
		}
	}
	t.Error("no branch header rendered")
}
