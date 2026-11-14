package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
)

// worktreeStatus is testStatus with a linked worktree, search, that starts on branch-2's commit.
func worktreeStatus() *but.Status {
	s := testStatus()
	s.Worktrees = []but.Worktree{{CliID: "se", Name: "search",
		Base:               but.WorktreeBase{CommitID: "123456789", InWorkspace: true},
		UncommittedChanges: []but.Change{{CliID: "w1", FilePath: "search_test.go", ChangeType: "added"}},
		Branches: []but.Branch{{CliID: "se", Name: "search", BranchStatus: "completelyUnpushed",
			Commits: []but.Commit{{CliID: "c9", CommitID: "999999999", Message: "Add a search index"}}}},
	}}
	return s
}

func newWorktreeHarness(t *testing.T, opts Options) (*harness, string) {
	t.Helper()
	dir := t.TempDir()
	c, f := newFakeBut(t, worktreeStatus())
	f.setWorktrees(&but.WorktreeList{Active: []but.WorktreeListing{{ID: "se", Name: "search", Path: dir}}})
	if opts.Worktree != nil {
		opts.Worktree.Path = dir
	}
	h := &harness{t: t, m: New(c, opts), but: f, wait: 150 * time.Millisecond}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 36})
	h.run(h.m.fetchStatus())
	return h, dir
}

func (h *harness) offered(key string) bool {
	_, ok := h.m.actionFor(key, h.m.subjects())
	return ok
}

func TestWorktreeSwitch(t *testing.T) {
	h, dir := newWorktreeHarness(t, Options{})
	h.wantOnScreen("⌂ workspace", "2 stacks · 2 changes", "1 other worktree")

	h.keys("w")
	h.wantOnScreen("Switch worktree", "current · 2 stacks · 2 changes", "⑂ search", "1 commit · 1 change · on branch-2")
	h.keys("enter") // the cursor starts on the next worktree
	h.wantOnScreen("⑂ search", "Add a search index", "search_test.go")
	for _, gone := range []string{"second", "go.mod", "new branch"} { // branch-2's commit, a workspace file, the drop lane
		if strings.Contains(h.screen(), gone) {
			t.Errorf("%q still shows in the search worktree", gone)
		}
	}

	// What `but` refuses in a worktree, or would do to the workspace, isn't offered.
	if got := h.selectedKey(); got != "branch:search" {
		t.Fatalf("selected %q after switching, want branch:search", got)
	}
	if !h.offered("b") || h.offered("S") || h.offered("a") {
		t.Errorf("on the worktree's branch: b %v (want stacked), S %v, a %v (want neither)", h.offered("b"), h.offered("S"), h.offered("a"))
	}
	h.selectText("search_test.go")
	if h.offered("b") || h.offered("A") {
		t.Errorf("on a worktree file: b (a new lane) %v, A (absorb) %v, want neither", h.offered("b"), h.offered("A"))
	}

	h.keys("c")
	h.hover("branch:search")
	h.keys("enter")
	h.typeText("Test it")
	h.keys("enter")
	h.but.expect("commit --message Test it w1 --branch search")
	if got := h.but.ranIn(); len(got) != 1 || got[0] != dir {
		t.Errorf("commit ran in %q, want the worktree %s", got, dir)
	}

	h.keys("w")
	h.typeText("workspace")
	h.keys("enter")
	h.wantOnScreen("⌂ workspace", "second", "new branch")
	if !h.offered("a") {
		t.Error("a (apply) isn't offered back in the workspace")
	}
}

func TestStartInWorktree(t *testing.T) {
	h, _ := newWorktreeHarness(t, Options{Worktree: &but.WorktreeListing{Name: "search"}})
	h.wantOnScreen("⑂ search", "Add a search index", "search_test.go")
	if strings.Contains(h.screen(), "second") {
		t.Error("starting in the search worktree shows the workspace")
	}
}

func TestWorktreeGone(t *testing.T) {
	h, _ := newWorktreeHarness(t, Options{Worktree: &but.WorktreeListing{Name: "search"}})
	h.but.setStatus(testStatus()) // removed elsewhere, with `but worktree remove`
	h.run(h.m.fetchStatus())
	h.wantOnScreen("second", "Worktree search is gone. Viewing the workspace")
}
