package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

// fileDiff is a one hunk diff of path.
func fileDiff(id, path, hunk string) *but.Diff {
	fd := but.FileDiff{ID: id, Path: path, Status: "modified"}
	fd.Diff.Type = "patch"
	fd.Diff.Hunks = []but.Hunk{{OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 3, Diff: hunk}}
	return &but.Diff{Changes: []but.FileDiff{fd}}
}

const goModHunk = "@@ -1,3 +1,3 @@\n module x\n-go 1.21\n+go 1.22\n require y\n"

func (h *harness) comments(statuses ...review.Status) []review.Comment {
	h.t.Helper()
	cs, err := h.m.review.List(statuses...)
	if err != nil {
		h.t.Fatal(err)
	}
	return cs
}

func (h *harness) onlyComment() review.Comment {
	h.t.Helper()
	cs := h.comments()
	if len(cs) != 1 {
		h.t.Fatalf("%d comments, want 1: %+v", len(cs), cs)
	}
	return cs[0]
}

func (h *harness) wantScreen(texts ...string) {
	h.t.Helper()
	s := h.screen()
	for _, text := range texts {
		if !strings.Contains(s, text) {
			h.t.Fatalf("%q not on screen:\n%s", text, s)
		}
	}
}

// C on an uncommitted line opens the composer; saving stores the comment, which shows under the line.
func TestCommentOnUncommittedLine(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	h.selectText("go.mod")
	h.keys("D", "j", "j", "j") // module x, -go 1.21, +go 1.22
	h.wantScreen("C comment")
	h.keys("C")
	h.wantScreen("Comment", "on go.mod line 2")
	h.typeText("Bump to 1.23")
	h.keys("ctrl+s")

	c := h.onlyComment()
	want := review.Anchor{Kind: review.KindUnassigned, Path: "go.mod", Side: review.SideNew, Line: 2, EndLine: 2, LineText: "go 1.22"}
	if c.Anchor != want || c.Body != "Bump to 1.23" || c.Author != review.AuthorUser || c.Status != review.StatusOpen {
		t.Fatalf("stored %+v", c)
	}
	h.wantScreen("you · line 2", "Bump to 1.23", c.ID)
	lines := strings.Split(h.screen(), "\n")
	for i, l := range lines {
		if strings.Contains(l, "go 1.22") {
			if !strings.Contains(lines[i+1], "╭─ you") {
				t.Fatalf("the comment is not under its line:\n%s", h.screen())
			}
		}
	}
	h.keys("esc")
	if !strings.Contains(h.screen(), "go.mod") || !strings.Contains(h.screen(), "✎1") {
		t.Fatalf("no count badge on go.mod:\n%s", h.screen())
	}
}

// A comment on an assigned change is anchored by the stack's top branch.
func TestCommentOnAssignedChange(t *testing.T) {
	s := testStatus()
	s.UncommittedChanges = s.UncommittedChanges[:1]
	s.Stacks[1].AssignedChanges = []but.Change{{CliID: "a1", FilePath: "go.mod", ChangeType: "modified"}}
	h := newHarnessWith(t, s)
	h.but.setDiff(fileDiff("a1", "go.mod", goModHunk))
	h.selectText("go.mod")
	h.keys("D", "j", "C")
	h.typeText("Why x?")
	h.keys("ctrl+s")
	if a := h.onlyComment().Anchor; a.Kind != review.KindAssigned || a.Branch != "branch-2" || a.Line != 1 || a.LineText != "module x" {
		t.Fatalf("anchor %+v", a)
	}
}

// A range on a commit diff is anchored to the commit by change id, and counted on the commit.
func TestCommentOnCommitRange(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("c1:x", "x.go", "@@ -1,2 +1,3 @@\n package x\n+func A() {}\n+func B() {}\n"))
	h.selectText("first")
	h.keys("D", "j", "j", "v", "j", "C")
	h.wantScreen("on x.go lines 2–3")
	h.typeText("Split these")
	h.keys("ctrl+s")

	a := h.onlyComment().Anchor
	want := review.Anchor{Kind: review.KindCommit, ChangeID: "chg1", CommitID: "abcdef123", Branch: "branch-4",
		Path: "x.go", Side: review.SideNew, Line: 2, EndLine: 3, LineText: "func A() {}\nfunc B() {}"}
	if a != want {
		t.Fatalf("anchor %+v\nwant   %+v", a, want)
	}
	if h.m.det.anchor >= 0 {
		t.Fatal("saving did not end the range")
	}
	h.wantScreen("you · lines 2–3", "Split these")
	h.keys("esc")
	h.wantScreen("✎1 abcdef1")
	// Not on the other commit.
	if strings.Count(h.screen(), "✎") != 1 {
		t.Fatalf("badge on the wrong rows:\n%s", h.screen())
	}
}

// C on a branch's diff puts the comment on the branch's commit that has the line; it shows in the branch diff and
// in the commit's.
func TestCommentOnBranch(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("c1:x", "x.go", "@@ -1,1 +1,2 @@\n package x\n+func A() {}\n"))
	h.selectText("branch-4")
	h.keys("D", "j", "j", "C")
	h.wantScreen("on x.go line 2 in abcdef1")
	h.typeText("Name it")
	h.keys("ctrl+s")

	a := h.onlyComment().Anchor
	want := review.Anchor{Kind: review.KindCommit, ChangeID: "chg1", CommitID: "abcdef123", Branch: "branch-4",
		Path: "x.go", Side: review.SideNew, Line: 2, EndLine: 2, LineText: "func A() {}"}
	if a != want {
		t.Fatalf("anchor %+v\nwant   %+v", a, want)
	}
	h.wantScreen("you · line 2", "Name it")
	h.keys("esc")
	h.wantScreen("✎1 abcdef1")
}

// A line no commit of the branch has, such as one of a file no commit changes, gets no comment.
func TestCommentOnBranchNoCommit(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("b4:y", "y.go", "@@ -1,1 +1,2 @@\n package y\n+func A() {}\n"))
	h.selectText("branch-4")
	h.keys("D", "j", "j", "C")
	if len(h.comments()) != 0 || h.m.modal != nil {
		t.Fatal("C opened the composer on lines no commit has")
	}
	h.wantScreen("No single commit on branch-4")
}

// Seeds a comment on go.mod line 2 and opens its diff with the cursor on it.
func seededComment(t *testing.T) (*harness, review.Comment) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	c, err := h.m.review.Add(review.Anchor{Kind: review.KindUnassigned, Path: "go.mod", Line: 2, LineText: "go 1.22"}, "", "Bump it")
	if err != nil {
		t.Fatal(err)
	}
	h.run(h.m.loadComments(true))
	h.selectText("go.mod")
	h.keys("D", "j", "j", "j", "j") // past +go 1.22 onto the comment
	if l, ok := h.m.det.cursorNote(); !ok || l.Comment.ID != c.ID {
		t.Fatalf("cursor not on the comment:\n%s", h.screen())
	}
	return h, c
}

// On a comment, x resolves and reopens it, e edits it and d deletes it after confirming.
func TestCommentKeys(t *testing.T) {
	h, c := seededComment(t)
	h.wantScreen("e edit", "d delete", "x resolve")
	if _, ok := h.m.det.lineSelection(); ok {
		t.Fatal("a comment is not a line to comment on")
	}

	h.keys("x")
	if got := h.onlyComment(); got.Status != review.StatusResolved {
		t.Fatalf("x left it %s", got.Status)
	}
	h.wantScreen("✓ resolved · Bump it", "x reopen")
	h.keys("z")
	if strings.Contains(h.screen(), "Bump it") {
		t.Fatalf("z did not hide the resolved comment:\n%s", h.screen())
	}
	h.keys("z", "j") // the cursor went to the line above
	h.wantScreen("✓ resolved · Bump it")
	if l, ok := h.m.det.cursorNote(); !ok || l.Comment.ID != c.ID {
		t.Fatalf("cursor left the comment:\n%s", h.screen())
	}
	h.keys("x")
	if got := h.onlyComment(); got.Status != review.StatusOpen {
		t.Fatalf("x did not reopen it: %s", got.Status)
	}

	h.keys("e")
	h.wantScreen("Bump it")
	h.typeText(" to 1.23")
	h.keys("ctrl+s")
	if got := h.onlyComment(); got.Body != "Bump it to 1.23" {
		t.Fatalf("edited body %q", got.Body)
	}
	h.wantScreen("Bump it to 1.23")

	h.keys("d")
	h.wantScreen("Delete comment on go.mod line 2?")
	h.keys("n")
	if len(h.comments()) != 1 || !h.m.det.full {
		t.Fatal("n deleted the comment or closed the pane")
	}
	h.keys("d", "y")
	if len(h.comments()) != 0 {
		t.Fatal("d y did not delete the comment")
	}
	if strings.Contains(h.screen(), "Bump it") {
		t.Fatalf("deleted comment still shown:\n%s", h.screen())
	}
	if s, ok := h.m.det.lineSelection(); !ok || s.line != 2 {
		t.Fatalf("cursor did not go back to the line: %+v %v", s, ok)
	}
	// Off a comment, d is the pane's again.
	h.keys("d")
	if h.m.det.full {
		t.Fatal("d did not leave full screen")
	}
}

// A resolution written by the agent shows up on the next refresh.
func TestCommentsReloadOnTick(t *testing.T) {
	h, c := seededComment(t)
	if _, err := h.m.review.Resolve(c.ID, "Bumped to 1.23"); err != nil {
		t.Fatal(err)
	}
	h.send(tickMsg{})
	h.wantScreen("✓ resolved · Bump it — Bumped to 1.23")
}

// The palette lists open comments, and picking one opens its diff on it.
func TestCommentsPalette(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("c1:x", "x.go", "@@ -1,2 +1,3 @@\n package x\n+func A() {}\n+func B() {}\n"))
	c, err := h.m.review.Add(review.Anchor{Kind: review.KindCommit, ChangeID: "chg1", Path: "x.go", Line: 3, LineText: "func B() {}"}, "", "Name it better")
	if err != nil {
		t.Fatal(err)
	}
	done, _ := h.m.review.Add(review.Anchor{Kind: review.KindCommit, ChangeID: "chg1", Path: "x.go", Line: 2, LineText: "func A() {}"}, "", "Done already")
	if _, err := h.m.review.Resolve(done.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.loadComments(true))

	h.keys("ctrl+p")
	h.typeText("review comments")
	h.keys("enter")
	h.wantScreen("Review comments", "x.go line 3  Name it better", "c1 abcdef1")
	if strings.Contains(h.screen(), "Done already") {
		t.Fatalf("the palette lists a resolved comment:\n%s", h.screen())
	}
	h.keys("enter")
	if got := h.selectedKey(); got != "commit:chg1" {
		t.Fatalf("selected %q", got)
	}
	if l, ok := h.m.det.cursorNote(); !ok || l.Comment.ID != c.ID || !h.m.det.focused {
		t.Fatalf("not on the comment:\n%s", h.screen())
	}
	h.wantScreen("Name it better", "✓ resolved · Done already")
}

// A comment whose line is gone is drawn at the end of its file, with the text it was on.
func TestOutdatedComment(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	if _, err := h.m.review.Add(review.Anchor{Kind: review.KindUnassigned, Path: "go.mod", Line: 7, LineText: "toolchain go1.22"}, "", "Drop this"); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.loadComments(true))
	h.selectText("go.mod")
	h.keys("D")
	h.wantScreen("outdated · was line 7", "> toolchain go1.22", "Drop this")
}

// Clicking a comment box puts the cursor on the comment.
func TestClickComment(t *testing.T) {
	h, c := seededComment(t)
	h.click("module x")
	if _, ok := h.m.det.cursorNote(); ok {
		t.Fatal("cursor still on the comment")
	}
	x, y := h.find("Bump it")
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	if l, ok := h.m.det.cursorNote(); !ok || l.Comment.ID != c.ID {
		t.Fatalf("click did not select the comment:\n%s", h.screen())
	}
}

// A comment an agent left with `buti review comment` shows its author, its severity tag and replies by either side.
func TestAgentComment(t *testing.T) {
	h := newHarness(t)
	h.but.setDiff(fileDiff("f2", "go.mod", goModHunk))
	c, err := h.m.review.Add(review.Anchor{Kind: review.KindUnassigned, Path: "go.mod", Line: 2, LineText: "go 1.22"},
		"agent", "[must-fix] 1.22 is out of support")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.review.Reply(c.ID, review.AuthorUser, "Which one then?"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.review.Reply(c.ID, "agent", "1.23"); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.loadComments(true))
	h.selectText("go.mod")
	h.keys("D")
	h.wantScreen("╭─ agent · line 2", "[must-fix] 1.22 is out of support", "↳ you: Which one then?", "↳ agent: 1.23")
	raw := h.m.View().Content
	if !strings.Contains(raw, severityStyles["[must-fix]"].Render("[must-fix]")) {
		t.Errorf("the severity tag is not coloured:\n%q", raw)
	}
	if !strings.Contains(raw, noteAgentStyle.Render("agent")) {
		t.Errorf("the agent is not set apart:\n%q", raw)
	}

	h.keys("esc", "ctrl+p")
	h.typeText("review comments")
	h.keys("enter")
	h.wantScreen("go.mod line 2  [must-fix] 1.22 is out of support", "agent · zz")
}
