package review

import (
	"context"
	"fmt"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
)

// fakeBut serves a fixed status and file diffs by cli id, and counts the diffs asked for.
type fakeBut struct {
	st    *but.Status
	diffs map[string]*but.Diff
	calls int
}

func (f *fakeBut) Status(context.Context) (*but.Status, error) { return f.st, nil }

func (f *fakeBut) Diff(_ context.Context, id string) (*but.Diff, error) {
	f.calls++
	d, ok := f.diffs[id]
	if !ok {
		return nil, fmt.Errorf("no diff for %s", id)
	}
	return d, nil
}

func fileDiff(path, hunk string) *but.Diff {
	fd := but.FileDiff{Path: path}
	fd.Diff.Type = "patch"
	fd.Diff.Hunks = []but.Hunk{{Diff: hunk}}
	return &but.Diff{Changes: []but.FileDiff{fd}}
}

const (
	tokenV1 = "@@ -0,0 +1,3 @@\n+package auth\n+\n+func Token() string { return \"secret\" }\n"
	tokenV2 = "@@ -0,0 +1,5 @@\n+package auth\n+\n+// Token returns the token.\n+// It is a secret.\n+func Token() string { return \"secret\" }\n"
)

// workspace has a stack "auth" (cli id m0) with one commit adding token.go, a stack "docs" (k0) with README.md
// assigned, and README.md unassigned too.
func workspace(commitID, tokenHunk string) *fakeBut {
	return &fakeBut{
		st: &but.Status{
			UncommittedChanges: []but.Change{{CliID: "rl", FilePath: "README.md"}},
			Stacks: []but.Stack{
				{CliID: "m0", Branches: []but.Branch{{Name: "auth", Commits: []but.Commit{{
					CliID: "knl", ChangeID: "change-token", CommitID: commitID,
					Changes: []but.Change{{CliID: "k:m", FilePath: "src/auth/token.go"}},
				}}}}},
				{CliID: "k0", AssignedChanges: []but.Change{{CliID: "rd", FilePath: "docs/a.md"}},
					Branches: []but.Branch{{Name: "docs"}}},
			},
		},
		diffs: map[string]*but.Diff{
			"k:m": fileDiff("src/auth/token.go", tokenHunk),
			"rl":  fileDiff("README.md", "@@ -1,2 +1,3 @@\n # demo\n-old\n+new\n+more\n"),
			"rd":  fileDiff("docs/a.md", "@@ -0,0 +1,1 @@\n+hello\n"),
		},
	}
}

func tokenAnchor() Anchor {
	return Anchor{Kind: KindCommit, ChangeID: "change-token", CommitID: "c1", Branch: "auth",
		Path: "src/auth/token.go", Side: SideNew, Line: 3, EndLine: 3, LineText: `func Token() string { return "secret" }`}
}

func locateOne(t *testing.T, f *fakeBut, a Anchor) Located {
	t.Helper()
	ls, err := Locate(context.Background(), f.st, f, []Comment{{ID: "abc123", Status: StatusOpen, Anchor: a}})
	if err != nil {
		t.Fatal(err)
	}
	return ls[0]
}

func TestLocateCommitUnchanged(t *testing.T) {
	l := locateOne(t, workspace("c1", tokenV1), tokenAnchor())
	if l.Status != StatusOpen || l.Moved() || l.Target != "knl" || l.FileID != "k:m" {
		t.Fatalf("got %+v", l)
	}
}

func TestLocateCommitAfterAmendAndInsert(t *testing.T) {
	l := locateOne(t, workspace("c2", tokenV2), tokenAnchor())
	if l.Status != StatusOpen || !l.Moved() {
		t.Fatalf("got %+v", l)
	}
	if a := l.Anchor; a.Line != 5 || a.EndLine != 5 || a.CommitID != "c2" || a.ChangeID != "change-token" {
		t.Fatalf("anchor %+v", a)
	}
	if l.Target != "knl" || l.FileID != "k:m" {
		t.Fatalf("target %q file %q", l.Target, l.FileID)
	}
}

func TestLocateCommitByCommitID(t *testing.T) {
	a := tokenAnchor()
	a.ChangeID = "unknown"
	if l := locateOne(t, workspace("c1", tokenV1), a); l.Status != StatusOpen || l.Target != "knl" {
		t.Fatalf("got %+v", l)
	}
}

// goneAnchor is a comment on a commit that no longer exists, on a line no commit or uncommitted change has.
func goneAnchor() Anchor {
	a := tokenAnchor()
	a.ChangeID, a.CommitID, a.LineText = "gone", "gone", "func Gone() {}"
	return a
}

func TestLocateCommitOrphaned(t *testing.T) {
	a := goneAnchor()
	l := locateOne(t, workspace("c1", tokenV1), a)
	if l.Status != StatusOrphaned || l.Target != "" || l.Moved() {
		t.Fatalf("got %+v", l)
	}
}

func TestLocateCommitFollowsUncommit(t *testing.T) {
	// The commit was uncommitted: its lines are added in the unassigned changes now.
	f := workspace("c1", tokenV1)
	f.st.Stacks[0].Branches[0].Commits = nil
	f.st.UncommittedChanges = append(f.st.UncommittedChanges, but.Change{CliID: "tk", FilePath: "src/auth/token.go"})
	f.diffs["tk"] = fileDiff("src/auth/token.go", tokenV2)
	l := locateOne(t, f, tokenAnchor())
	if l.Status != StatusOpen || l.Target != "zz" || l.FileID != "tk" {
		t.Fatalf("got %+v", l)
	}
	want := Anchor{Kind: KindUnassigned, Path: "src/auth/token.go", Side: SideNew, Line: 5, EndLine: 5,
		LineText: tokenAnchor().LineText}
	if l.Anchor != want {
		t.Fatalf("anchor %+v", l.Anchor)
	}
	// Assigned to the stack of the comment's branch instead.
	f.st.UncommittedChanges = f.st.UncommittedChanges[:1]
	f.st.Stacks[0].AssignedChanges = []but.Change{{CliID: "tk", FilePath: "src/auth/token.go"}}
	if l := locateOne(t, f, tokenAnchor()); l.Status != StatusOpen || l.Target != "m0" ||
		l.Anchor.Kind != KindAssigned || l.Anchor.Branch != "auth" || l.Anchor.ChangeID != "" {
		t.Fatalf("assigned: %+v", l)
	}
	// Where the line is only context, the uncommit did not bring it back, so the comment stays orphaned.
	f.diffs["tk"] = fileDiff("src/auth/token.go", "@@ -1,3 +1,4 @@\n package auth\n \n func Token() string { return \"secret\" }\n+// end\n")
	if l := locateOne(t, f, tokenAnchor()); l.Status != StatusOrphaned || l.Moved() {
		t.Fatalf("context only: %+v", l)
	}
}

func TestLocateCommitDoesNotFollowPunctuation(t *testing.T) {
	// A lone brace is added in many places; following it would guess.
	f := workspace("c1", "@@ -0,0 +1,2 @@\n+x\n+}\n")
	a := goneAnchor()
	a.Line, a.EndLine, a.LineText = 2, 2, "}"
	if l := locateOne(t, f, a); l.Status != StatusOrphaned || l.Moved() {
		t.Fatalf("got %+v", l)
	}
}

func TestLocateCommitFollowsSquash(t *testing.T) {
	// The commit was squashed into the one that adds token.go now.
	a := tokenAnchor()
	a.ChangeID, a.CommitID = "squashed", "squashed"
	l := locateOne(t, workspace("c2", tokenV2), a)
	if l.Status != StatusOpen || l.Target != "knl" || l.Anchor.ChangeID != "change-token" || l.Anchor.CommitID != "c2" ||
		l.Anchor.Line != 5 {
		t.Fatalf("got %+v", l)
	}
}

func TestLocateOutdatedKeepsText(t *testing.T) {
	a := tokenAnchor()
	a.LineText = "func Token() string { return \"public\" }"
	l := locateOne(t, workspace("c2", tokenV2), a)
	if l.Status != StatusOutdated || l.Moved() || l.Anchor.Line != 3 || l.Target != "knl" || l.FileID != "k:m" {
		t.Fatalf("got %+v", l)
	}
	// The file left the commit.
	f := workspace("c1", tokenV1)
	a.Path = "src/auth/other.go"
	if l := locateOne(t, f, a); l.Status != StatusOutdated || l.Target != "knl" || l.FileID != "" {
		t.Fatalf("missing file: %+v", l)
	}
}

func TestLocateRangeKeepsLength(t *testing.T) {
	a := tokenAnchor()
	a.Line, a.EndLine, a.LineText = 1, 3, "package auth\n\nfunc Token() string { return \"secret\" }"
	// The range no longer holds together in v2 (comment lines in between), so it is outdated.
	if l := locateOne(t, workspace("c2", tokenV2), a); l.Status != StatusOutdated {
		t.Fatalf("broken range: %+v", l)
	}
	a.Line, a.EndLine, a.LineText = 1, 2, "// Token returns the token.\n// It is a secret.\n"
	l := locateOne(t, workspace("c2", tokenV2), a)
	if l.Status != StatusOpen || l.Anchor.Line != 3 || l.Anchor.EndLine != 4 {
		t.Fatalf("got %+v", l.Anchor)
	}
}

func TestLocateNearestMatch(t *testing.T) {
	f := workspace("c1", "@@ -0,0 +1,6 @@\n+x\n+}\n+y\n+z\n+w\n+}\n")
	a := tokenAnchor()
	a.Line, a.EndLine, a.LineText = 5, 5, "}"
	if l := locateOne(t, f, a); l.Anchor.Line != 6 {
		t.Fatalf("nearest: %+v", l.Anchor)
	}
}

func TestLocateUnassigned(t *testing.T) {
	a := Anchor{Kind: KindUnassigned, Path: "README.md", Side: SideNew, Line: 2, EndLine: 2, LineText: "new"}
	l := locateOne(t, workspace("c1", tokenV1), a)
	if l.Status != StatusOpen || l.Moved() || l.Target != "zz" || l.FileID != "rl" {
		t.Fatalf("got %+v", l)
	}
	// The old side counts old line numbers.
	a.Side, a.LineText = SideOld, "old"
	if l := locateOne(t, workspace("c1", tokenV1), a); l.Status != StatusOpen || l.Anchor.Line != 2 {
		t.Fatalf("old side: %+v", l)
	}
	a.LineText = "new"
	if l := locateOne(t, workspace("c1", tokenV1), a); l.Status != StatusOutdated || l.Target != "zz" {
		t.Fatalf("old side mismatch: %+v", l)
	}
}

func TestLocateAssigned(t *testing.T) {
	a := Anchor{Kind: KindAssigned, Branch: "docs", Path: "docs/a.md", Side: SideNew, Line: 1, EndLine: 1, LineText: "hello"}
	l := locateOne(t, workspace("c1", tokenV1), a)
	if l.Status != StatusOpen || l.Moved() || l.Target != "k0" || l.FileID != "rd" {
		t.Fatalf("got %+v", l)
	}
	// Moved to the unassigned changes.
	f := workspace("c1", tokenV1)
	f.st.Stacks[1].AssignedChanges = nil
	f.st.UncommittedChanges = append(f.st.UncommittedChanges, but.Change{CliID: "da", FilePath: "docs/a.md"})
	f.diffs["da"] = fileDiff("docs/a.md", "@@ -0,0 +1,1 @@\n+hello\n")
	l = locateOne(t, f, a)
	if l.Status != StatusOpen || l.Anchor.Kind != KindUnassigned || l.Anchor.Branch != "" || l.Target != "zz" || l.FileID != "da" {
		t.Fatalf("unassigned: %+v", l)
	}
	// A branch whose stack is gone and a file that is nowhere.
	a.Branch = "nope"
	f.st.UncommittedChanges = nil
	if l := locateOne(t, f, a); l.Status != StatusOutdated || l.Target != "" {
		t.Fatalf("gone: %+v", l)
	}
}

func TestLocateFollowsCommittedFile(t *testing.T) {
	f := workspace("c1", tokenV1)
	// token.go was committed twice: first added, then shown as context in a later commit.
	later := but.Commit{CliID: "zzt", ChangeID: "change-later", CommitID: "c9",
		Changes: []but.Change{{CliID: "z:m", FilePath: "src/auth/token.go"}}}
	br := &f.st.Stacks[0].Branches[0]
	br.Commits = append([]but.Commit{later}, br.Commits...)
	f.diffs["z:m"] = fileDiff("src/auth/token.go", "@@ -1,3 +1,4 @@\n package auth\n \n func Token() string { return \"secret\" }\n+// end\n")
	a := Anchor{Kind: KindUnassigned, Path: "src/auth/token.go", Side: SideNew, Line: 3, EndLine: 3,
		LineText: `func Token() string { return "secret" }`}
	l := locateOne(t, f, a)
	if l.Status != StatusOpen || l.Target != "knl" || l.FileID != "k:m" {
		t.Fatalf("got %+v", l)
	}
	want := Anchor{Kind: KindCommit, ChangeID: "change-token", CommitID: "c1", Branch: "auth",
		Path: "src/auth/token.go", Side: SideNew, Line: 3, EndLine: 3, LineText: a.LineText}
	if l.Anchor != want {
		t.Fatalf("anchor %+v", l.Anchor)
	}
	// Only context: the later commit still wins over losing the comment.
	f.diffs["k:m"] = fileDiff("src/auth/token.go", "@@ -0,0 +1,1 @@\n+package auth\n")
	if l := locateOne(t, f, a); l.Status != StatusOpen || l.Target != "zzt" {
		t.Fatalf("context only: %+v", l)
	}
}

func TestLocateKeepsClosedStatus(t *testing.T) {
	f := workspace("c1", tokenV1)
	a := tokenAnchor()
	a.ChangeID, a.CommitID = "gone", "gone"
	ls, err := Locate(context.Background(), f.st, f, []Comment{{ID: "a", Status: StatusResolved, Anchor: a}})
	if err != nil || ls[0].Status != StatusResolved {
		t.Fatalf("%+v %v", ls, err)
	}
}

func TestLocateCachesDiffs(t *testing.T) {
	f := workspace("c1", tokenV1)
	cs := []Comment{{ID: "a", Status: StatusOpen, Anchor: tokenAnchor()}, {ID: "b", Status: StatusOpen, Anchor: tokenAnchor()}}
	if _, err := Locate(context.Background(), f.st, f, cs); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatalf("%d diff calls", f.calls)
	}
}

func TestReanchorSavesMoves(t *testing.T) {
	s := newStore(t)
	moved := must(t)(s.Add(tokenAnchor(), "", "why secret?"))
	gone := goneAnchor()
	orphan := must(t)(s.Add(gone, "", "orphan"))

	f := workspace("c2", tokenV2)
	ls, err := s.Reanchor(context.Background(), f, StatusOpen)
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) != 2 || ls[0].Status != StatusOpen || ls[1].Status != StatusOrphaned {
		t.Fatalf("%+v", ls)
	}
	if c := must(t)(s.Get(moved.ID)); c.Anchor.Line != 5 || c.Anchor.CommitID != "c2" || c.Status != StatusOpen {
		t.Fatalf("stored %+v", c)
	}
	if c := must(t)(s.Get(orphan.ID)); c.Anchor != gone || c.Status != StatusOpen {
		t.Fatalf("orphan stored %+v", c)
	}
}

func TestSaveSkipsConcurrentChange(t *testing.T) {
	s := newStore(t)
	c := must(t)(s.Add(tokenAnchor(), "", "x"))
	f := workspace("c2", tokenV2)
	ls, err := Locate(context.Background(), f.st, f, []Comment{c})
	if err != nil {
		t.Fatal(err)
	}
	must(t)(s.Update(c.ID, func(c *Comment) error { c.Anchor.Line = 1; return nil }))
	if err := s.Save(ls); err != nil {
		t.Fatal(err)
	}
	if got := must(t)(s.Get(c.ID)); got.Anchor.Line != 1 {
		t.Fatalf("overwrote a newer anchor: %+v", got.Anchor)
	}
}

// A line of a branch's diff goes on the newest commit that changes it, at that commit's line number; one that is
// only context goes on a commit that shows it.
func TestOnBranch(t *testing.T) {
	f := workspace("c1", tokenV1)
	b := &f.st.Stacks[0].Branches[0]
	b.Commits = append([]but.Commit{{CliID: "doc", ChangeID: "change-doc", CommitID: "c2",
		Changes: []but.Change{{CliID: "d:m", FilePath: "src/auth/token.go"}}}}, b.Commits...)
	f.diffs["d:m"] = fileDiff("src/auth/token.go", "@@ -1,3 +1,4 @@\n package auth\n \n+// Token returns the token.\n func Token() string { return \"secret\" }\n")
	ctx := context.Background()
	on := func(line int, text string) Anchor {
		a, err := OnBranch(ctx, f.st, f, "auth", Anchor{Path: "src/auth/token.go", Side: SideNew, Line: line, EndLine: line, LineText: text})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	if a := on(3, "// Token returns the token."); a.Kind != KindCommit || a.ChangeID != "change-doc" || a.Line != 3 || a.Branch != "auth" {
		t.Fatalf("doc line on %+v", a)
	}
	// Line 4 of the branch diff is line 3 of the commit that added it.
	if a := on(4, `func Token() string { return "secret" }`); a.ChangeID != "change-token" || a.CommitID != "c1" || a.Line != 3 || a.EndLine != 3 {
		t.Fatalf("func line on %+v", a)
	}
	if _, err := OnBranch(ctx, f.st, f, "auth", Anchor{Path: "src/auth/token.go", Side: SideNew, Line: 9, EndLine: 9, LineText: "nowhere"}); err == nil {
		t.Fatal("a line no commit has was placed")
	}
}
