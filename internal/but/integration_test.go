package but

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestIntegration runs every operation against the real `but` CLI in a scratch
// repository. It is opt-in: BUTI_INTEGRATION=1 go test ./internal/but
func TestIntegration(t *testing.T) {
	if os.Getenv("BUTI_INTEGRATION") == "" {
		t.Skip("set BUTI_INTEGRATION=1 to run against the real but CLI")
	}
	if _, err := exec.LookPath("but"); err != nil {
		t.Skip("but not on PATH")
	}
	dir := t.TempDir()
	sh := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sh("git", "init", "-q", "-b", "main")
	write("base.txt", "base\n")
	sh("git", "add", ".")
	sh("git", "commit", "-qm", "init")
	sh("git", "remote", "add", "origin", "https://example.invalid/x.git")
	sh("git", "update-ref", "refs/remotes/origin/main", "HEAD")
	sh("but", "setup")

	ctx := context.Background()
	c := New(dir)
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	status := func() *Status {
		t.Helper()
		s, err := c.Status(ctx)
		must("status", err)
		return s
	}
	branch := func(s *Status, name string) *Branch {
		for i := range s.Stacks {
			for j := range s.Stacks[i].Branches {
				if b := &s.Stacks[i].Branches[j]; b.Name == name {
					return b
				}
			}
		}
		t.Fatalf("no branch %q", name)
		return nil
	}
	fileID := func(s *Status, path string) string {
		for _, f := range s.UncommittedChanges {
			if f.FilePath == path {
				return f.CliID
			}
		}
		t.Fatalf("no uncommitted %q", path)
		return ""
	}

	must("branch new", c.BranchNew(ctx, "alpha", Placement{}))
	write("a.txt", "a\n")
	write("b.txt", "b\n")
	s := status()
	must("commit to branch", c.Commit(ctx, []string{fileID(s, "a.txt")}, "add a", Placement{Branch: "alpha"}))
	s = status()
	if n := len(branch(s, "alpha").Commits); n != 1 {
		t.Fatalf("alpha has %d commits", n)
	}

	// Commit to a new branch: the bare --branch must come last.
	must("commit to new branch", c.Commit(ctx, []string{fileID(s, "b.txt")}, "add b", Placement{NewBranch: true}))
	s = status()
	if len(s.Stacks) != 2 {
		t.Fatalf("expected a second stack, got %d", len(s.Stacks))
	}

	// Amend a new change into alpha's commit.
	write("a.txt", "a\na2\n")
	s = status()
	alphaCommit := branch(s, "alpha").Commits[0].CliID
	must("amend", c.Amend(ctx, alphaCommit, []string{fileID(s, "a.txt")}))
	if s = status(); len(s.UncommittedChanges) != 0 {
		t.Fatalf("amend left %v", s.UncommittedChanges)
	}

	// Empty commit above, reword it, then squash it into alpha's commit keeping the target message.
	must("empty commit", c.EmptyCommit(ctx, "", Placement{Branch: "alpha"}))
	s = status()
	top := branch(s, "alpha").Commits[0]
	must("reword", c.Reword(ctx, top.CliID, "empty one"))
	s = status()
	top, bottom := branch(s, "alpha").Commits[0], branch(s, "alpha").Commits[1]
	if top.Subject() != "empty one" {
		t.Fatalf("reword: %q", top.Subject())
	}
	must("squash", c.Squash(ctx, []string{top.CliID}, bottom.CliID, SquashUseTargetMessage, ""))
	s = status()
	if cs := branch(s, "alpha").Commits; len(cs) != 1 || cs[0].Subject() != "add a" {
		t.Fatalf("after squash: %+v", cs)
	}

	// Move the other stack's commit onto alpha, then to a new branch.
	var other string
	for _, st := range s.Stacks {
		if st.Branches[0].Name != "alpha" {
			other = st.Branches[0].Name
		}
	}
	oc := branch(s, other).Commits[0].CliID
	must("move to branch", c.Move(ctx, []string{oc}, Placement{Branch: "alpha"}))
	s = status()
	if n := len(branch(s, "alpha").Commits); n != 2 {
		t.Fatalf("alpha has %d commits after move", n)
	}
	must("move to new branch", c.Move(ctx, []string{branch(s, "alpha").Commits[0].CliID}, Placement{NewBranch: true}))

	// Stacked branch, rename, uncommit, discard, undo/redo, oplog, branch list.
	s = status()
	must("stacked branch", c.BranchNew(ctx, "beta", Placement{Above: branch(s, "alpha").CliID}))
	s = status()
	must("rename", c.Reword(ctx, branch(s, "beta").CliID, "beta-2"))
	s = status()
	branch(s, "beta-2")
	must("uncommit", c.Uncommit(ctx, []string{branch(s, "alpha").Commits[0].CliID}))
	s = status()
	if len(s.UncommittedChanges) == 0 {
		t.Fatal("uncommit left nothing uncommitted")
	}
	must("discard", c.Discard(ctx, []string{s.UncommittedChanges[0].CliID}))
	must("undo", c.Undo(ctx))
	if s = status(); len(s.UncommittedChanges) == 0 {
		t.Fatal("undo did not restore the discarded change")
	}
	must("redo", c.Redo(ctx))
	entries, err := c.Oplog(ctx)
	must("oplog", err)
	if len(entries) == 0 || entries[0].ID == "" {
		t.Fatalf("oplog: %+v", entries)
	}
	_, err = c.Branches(ctx)
	must("branches", err)

	// Unapply and apply again.
	must("unapply", c.Unapply(ctx, "beta-2"))
	must("apply", c.Apply(ctx, "beta-2"))

	// Diff of uncommitted hunks carries ids.
	write("base.txt", "base\nmore\n")
	d, err := c.Diff(ctx, "")
	must("diff", err)
	if len(d.Changes) == 0 || !strings.Contains(d.Changes[0].ID, ":") {
		t.Fatalf("diff ids: %+v", d.Changes)
	}
	must("restore", c.OplogRestore(ctx, entries[len(entries)-1].ID))
}
