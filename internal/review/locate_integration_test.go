package review

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/testrepo"
)

// TestIntegrationReanchor re-anchors comments in the test repository while the real `but` amends, commits and
// uncommits under them. It is opt-in: BUTI_INTEGRATION=1 go test ./internal/review
func TestIntegrationReanchor(t *testing.T) {
	if os.Getenv("BUTI_INTEGRATION") == "" {
		t.Skip("set BUTI_INTEGRATION=1 to run against the real but CLI")
	}
	if _, err := exec.LookPath("but"); err != nil {
		t.Skip("but not on PATH")
	}
	r, err := testrepo.Create(testrepo.TempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	r.Setenv(t.Setenv)
	ctx := context.Background()
	c := but.New(r.Dir)
	s, err := Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	must := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}
	status := func() *but.Status {
		t.Helper()
		st, err := c.Status(ctx)
		must("status", err)
		return st
	}
	commit := func(branch, subject string) but.Commit {
		t.Helper()
		for _, st := range status().Stacks {
			for _, b := range st.Branches {
				for _, cm := range b.Commits {
					if b.Name == branch && cm.Subject() == subject {
						return cm
					}
				}
			}
		}
		t.Fatalf("no commit %q on %s", subject, branch)
		return but.Commit{}
	}
	uncommitted := func(path string) string {
		t.Helper()
		for _, ch := range status().UncommittedChanges {
			if ch.FilePath == path {
				return ch.CliID
			}
		}
		t.Fatalf("%s is not uncommitted", path)
		return ""
	}
	write := func(name, content string) {
		t.Helper()
		must("write", os.WriteFile(filepath.Join(r.Dir, name), []byte(content), 0o644))
	}
	reanchor := func(id string) Located {
		t.Helper()
		ls, err := s.Reanchor(ctx, c)
		must("reanchor", err)
		for _, l := range ls {
			if l.Comment.ID == id {
				return l
			}
		}
		t.Fatalf("comment %s not listed", id)
		return Located{}
	}

	// A comment on the commit that adds token.go.
	token := commit("auth", "Add token auth")
	secret := `func Token() string { return "secret" }`
	onCommit, err := s.Add(Anchor{Kind: KindCommit, ChangeID: token.ChangeID, CommitID: token.CommitID, Branch: "auth",
		Path: "src/auth/token.go", Line: 3, LineText: secret}, "", "Don't hard-code the token")
	must("add", err)

	// A comment on uncommitted work.
	onReadme, err := s.Add(Anchor{Kind: KindUnassigned, Path: "README.md", Line: 5, LineText: "## Usage"}, "", "Say more")
	must("add", err)
	if l := reanchor(onReadme.ID); l.Status != StatusOpen || l.Target != TargetUnassigned || l.FileID != uncommitted("README.md") {
		t.Fatalf("uncommitted: %+v", l)
	}

	// Amending another file into the commit changes its commit id, not its change id.
	write("src/auth/extra.go", "package auth\n")
	must("amend", c.Amend(ctx, token.CliID, []string{uncommitted("src/auth/extra.go")}))
	amended := commit("auth", "Add token auth")
	if amended.CommitID == token.CommitID {
		t.Fatal("amend kept the commit id")
	}
	l := reanchor(onCommit.ID)
	if l.Status != StatusOpen || l.Target != amended.CliID || l.Anchor.Line != 3 || l.Anchor.CommitID != amended.CommitID {
		t.Fatalf("after amend: %+v", l)
	}

	// Lines inserted above the commented one, amended into the commit.
	write("src/auth/token.go", "package auth\n\n// Token returns the API token.\n// TODO: read it from the environment.\n"+secret+"\n")
	must("amend", c.Amend(ctx, amended.CliID, []string{uncommitted("src/auth/token.go")}))
	amended = commit("auth", "Add token auth")
	l = reanchor(onCommit.ID)
	if l.Status != StatusOpen || l.Target != amended.CliID || l.Anchor.Line != 5 || l.Anchor.EndLine != 5 {
		t.Fatalf("after inserting lines: %+v", l)
	}
	stored, err := s.Get(onCommit.ID)
	must("get", err)
	if stored.Anchor.Line != 5 || stored.Anchor.CommitID != amended.CommitID || stored.Anchor.ChangeID != token.ChangeID {
		t.Fatalf("stored anchor %+v", stored.Anchor)
	}

	// Committing the README moves the uncommitted comment onto the new commit.
	must("commit", c.Commit(ctx, []string{uncommitted("README.md")}, "Document usage", but.Placement{Branch: "empty"}))
	usage := commit("empty", "Document usage")
	l = reanchor(onReadme.ID)
	if l.Status != StatusOpen || l.Target != usage.CliID || l.Anchor.Kind != KindCommit || l.Anchor.ChangeID != usage.ChangeID ||
		l.Anchor.Branch != "empty" || l.Anchor.Line != 5 {
		t.Fatalf("after commit: %+v", l)
	}

	// Rewriting the line makes the comment outdated, with its original text.
	write("src/auth/token.go", "package auth\n\n// Token returns the API token.\n// TODO: read it from the environment.\nfunc Token() string { return os.Getenv(\"TOKEN\") }\n")
	must("amend", c.Amend(ctx, amended.CliID, []string{uncommitted("src/auth/token.go")}))
	if l = reanchor(onCommit.ID); l.Status != StatusOutdated || l.Anchor.LineText != secret || l.Anchor.Line != 5 {
		t.Fatalf("after rewrite: %+v", l)
	}

	// Uncommitting the commit moves a comment on a line it added into the uncommitted changes. The rewritten line
	// is nowhere, so that comment is orphaned.
	amended = commit("auth", "Add token auth")
	onDoc, err := s.Add(Anchor{Kind: KindCommit, ChangeID: amended.ChangeID, CommitID: amended.CommitID, Branch: "auth",
		Path: "src/auth/token.go", Line: 3, LineText: "// Token returns the API token."}, "", "Say which API")
	must("add", err)
	must("uncommit", c.Uncommit(ctx, []string{amended.CliID}))
	l = reanchor(onDoc.ID)
	if l.Status != StatusOpen || l.Anchor.Kind == KindCommit || l.Anchor.ChangeID != "" || l.Anchor.Line != 3 || l.FileID == "" {
		t.Fatalf("after uncommit: %+v", l)
	}
	if l.Anchor.Kind == KindUnassigned && l.Target != TargetUnassigned || l.Anchor.Kind == KindAssigned && l.Anchor.Branch != "auth" {
		t.Fatalf("after uncommit, target %q: %+v", l.Target, l.Anchor)
	}
	if stored, err = s.Get(onDoc.ID); err != nil || stored.Anchor != l.Anchor {
		t.Fatalf("stored after uncommit %+v %v", stored.Anchor, err)
	}
	if l = reanchor(onCommit.ID); l.Status != StatusOrphaned || l.Target != "" || l.Anchor.Path != "src/auth/token.go" {
		t.Fatalf("rewritten after uncommit: %+v", l)
	}
	if stored, err = s.Get(onCommit.ID); err != nil || stored.Status != StatusOpen {
		t.Fatalf("orphaned is derived, not stored: %+v %v", stored, err)
	}
}
