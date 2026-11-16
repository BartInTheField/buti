package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
)

// worktreeStatus is testStatus with a worktree, search, whose stack is ranking on top of search. A worktree's CLI id
// is its top branch's, and its name is its bottom branch's.
func worktreeStatus() *but.Status {
	s := testStatus()
	s.Worktrees = []but.Worktree{{CliID: "ra", Name: "search", Reference: "refs/heads/search",
		UncommittedChanges: []but.Change{{CliID: "q1", FilePath: "query.go", ChangeType: "added"}},
		Branches: []but.Branch{
			{CliID: "ra", Name: "ranking", BranchStatus: "completelyUnpushed"},
			{CliID: "se", Name: "search", BranchStatus: "completelyUnpushed",
				Commits: []but.Commit{{CliID: "c9", ChangeID: "chg9", CommitID: "999999999", Message: "Add search"}}},
		}}}
	return s
}

// A worktree's branches go to `but` by CLI id: with ranking stacked on it, `but` finds the name search ambiguous.
func TestWorktreeBranchesByID(t *testing.T) {
	h := newHarnessWith(t, worktreeStatus())
	h.wantOnScreen("⌂ 1 worktree →") // at 140 columns its lane is past the right edge
	h.send(tea.WindowSizeMsg{Width: 220, Height: 36})
	h.wantOnScreen("Worktrees", "search", "ranking", "query.go", "Add search")

	h.hover("branch:search")
	h.keys("P")
	h.but.expect("push se", "status --json -f --refresh-prs")
}

func TestCommitInWorktree(t *testing.T) {
	h := newHarnessWith(t, worktreeStatus())
	h.send(tea.WindowSizeMsg{Width: 220, Height: 36})
	h.selectText("query.go")
	if got := h.selectedKey(); got != "wt:search:file:query.go" {
		t.Fatalf("selected %q", got)
	}
	h.keys("c")
	// The worktree's change can't go to the workspace's branches, nor the workspace's into the worktree's.
	h.hover("branch:branch-2")
	h.keys("enter")
	if h.m.modal != nil {
		t.Fatal("committing a worktree's change to a workspace branch was allowed")
	}
	h.hover("branch:search")
	h.keys("enter")
	h.typeText("Add queries")
	h.keys("enter")
	h.but.expect("commit --message Add queries q1 --branch se")

	h.selectText("go.mod")
	h.keys("c")
	h.hover("branch:search")
	h.keys("enter")
	if h.m.modal != nil {
		t.Fatal("committing a workspace change to a worktree branch was allowed")
	}
}

// b on a worktree stacks on its top branch: `but` refuses a new unstacked branch in a worktree.
func TestNewBranchInWorktree(t *testing.T) {
	h := newHarnessWith(t, worktreeStatus())
	h.hover("worktree:search")
	h.keys("b")
	h.typeText("tuning")
	h.keys("enter")
	h.but.expect("branch new --above ra tuning")
}

// Dropping a worktree's commit on its card uncommits it, and the selection stays on the worktree.
func TestUncommitInWorktree(t *testing.T) {
	h := newHarnessWith(t, worktreeStatus())
	h.send(tea.WindowSizeMsg{Width: 220, Height: 36})
	h.dragTo("Add search", "⌂ search")
	h.but.expect("uncommit c9")
	if got := h.selectedKey(); got != "worktree:search" {
		t.Fatalf("selected %q after uncommitting, want the worktree", got)
	}
}

// Only the divider where the worktrees start is drawn in the worktree colour: without worktrees there is none.
func TestWorktreeDividerOnlyWithWorktrees(t *testing.T) {
	blue := "38;2;56;189;248" // colorWorktree, #38BDF8
	h := newHarness(t)
	if strings.Contains(h.m.View().Content, blue) {
		t.Fatalf("worktree colour on screen without worktrees:\n%s", h.screen())
	}
	h = newHarnessWith(t, worktreeStatus())
	h.send(tea.WindowSizeMsg{Width: 220, Height: 36})
	// Worktree cards have blue borders too, but only the divider reaches the bottom lane row.
	lines := strings.Split(h.m.View().Content, "\n")
	if bottom := lines[len(lines)-2]; !strings.Contains(bottom, blue+"m│") {
		t.Fatalf("no worktree divider on the bottom lane row:\n%s", h.screen())
	}
}
