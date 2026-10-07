package editor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
)

func hunk(os_, ol, ns, nl int, body string) but.Hunk {
	return but.Hunk{OldStart: os_, OldLines: ol, NewStart: ns, NewLines: nl, Diff: "@@ header @@\n" + body}
}

func TestOldContent(t *testing.T) {
	for _, tc := range []struct {
		name, new, old string
		hunks          []but.Hunk
	}{
		{"modified", "a\nB\nc\n", "a\nb\nc\n", []but.Hunk{hunk(1, 3, 1, 3, " a\n-b\n+B\n c\n")}},
		{"added file", "x\ny\n", "", []but.Hunk{hunk(0, 0, 1, 2, "+x\n+y\n")}},
		{"deleted file", "", "x\ny\n", []but.Hunk{hunk(1, 2, 0, 0, "-x\n-y\n")}},
		{"multi hunk", "1\nTWO\n3\n4\n5\n6\nSEVEN\n8\n", "1\n2\n3\n4\n5\n6\n7\n8\n", []but.Hunk{
			hunk(1, 3, 1, 3, " 1\n-2\n+TWO\n 3\n"), hunk(6, 3, 6, 3, " 6\n-7\n+SEVEN\n 8\n")}},
		{"hunks out of order", "A\nb\nc\nD\n", "a\nb\nc\nd\n", []but.Hunk{
			hunk(4, 1, 4, 1, "-d\n+D\n"), hunk(1, 1, 1, 1, "-a\n+A\n")}},
		{"pure deletion mid file", "a\nc\n", "a\nb\nc\n", []but.Hunk{hunk(2, 1, 1, 0, "-b\n")}},
		{"added line at end", "a\nb\n", "a\n", []but.Hunk{hunk(1, 1, 1, 2, " a\n+b\n")}},
		{"no newline added", "a\nb", "a\nb\n", []but.Hunk{hunk(2, 1, 2, 1, "-b\n+b\n\\ No newline at end of file\n")}},
		{"no newline removed", "a\nb\n", "a\nb", []but.Hunk{hunk(2, 1, 2, 1, "-b\n\\ No newline at end of file\n+b\n")}},
		{"no newline context", "a\nB\nc", "a\nb\nc", []but.Hunk{hunk(1, 3, 1, 3, " a\n-b\n+B\n c\n\\ No newline at end of file\n")}},
		{"no hunks", "same\n", "same\n", nil},
	} {
		got, err := OldContent(tc.new, tc.hunks)
		if err != nil || got != tc.old {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.old)
		}
	}
}

func TestOldContentStale(t *testing.T) {
	for name, tc := range map[string]struct {
		new   string
		hunks []but.Hunk
	}{
		"changed line":    {"a\nX\nc\n", []but.Hunk{hunk(1, 3, 1, 3, " a\n-b\n+B\n c\n")}},
		"file too short":  {"a\n", []but.Hunk{hunk(5, 1, 5, 1, "-b\n+B\n")}},
		"newline differs": {"a\nb\n", []but.Hunk{hunk(2, 1, 2, 1, "-b\n+b\n\\ No newline at end of file\n")}},
		"count":           {"a\nB\n", []but.Hunk{hunk(1, 1, 1, 2, "-a\n+B\n")}},
	} {
		if got, err := OldContent(tc.new, tc.hunks); err == nil {
			t.Errorf("%s: want an error, got %q", name, got)
		}
	}
}

func TestFiles(t *testing.T) {
	patch := func(path string, h ...but.Hunk) but.FileDiff {
		fd := but.FileDiff{Path: path}
		fd.Diff.Type, fd.Diff.Hunks = "patch", h
		return fd
	}
	bin := but.FileDiff{Path: "i.png"}
	bin.Diff.Type = "binary"
	d := &but.Diff{Changes: []but.FileDiff{patch("a", hunk(1, 1, 1, 1, "-x\n+y\n")), bin, patch("b", hunk(1, 1, 1, 1, "-x\n+y\n")), patch("a", hunk(5, 1, 5, 1, "-x\n+y\n"))}}
	got := Files(d)
	if len(got) != 2 || got[0].Path != "a" || len(got[0].Hunks) != 2 || got[1].Path != "b" {
		t.Errorf("got %+v", got)
	}
}

func TestWorktreeAndCommit(t *testing.T) {
	dir, tmp := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a.go"), []byte("a\nB\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []File{
		{"pkg/a.go", []but.Hunk{hunk(1, 2, 1, 2, " a\n-b\n+B\n")}},
		{"gone.txt", []but.Hunk{hunk(1, 1, 0, 0, "-bye\n")}},
	}
	pairs, err := Worktree(tmp, dir, files)
	if err != nil {
		t.Fatal(err)
	}
	if pairs[0][1] != filepath.Join(dir, "pkg", "a.go") || !strings.HasSuffix(pairs[0][0], "old/pkg/a.go") {
		t.Errorf("pairs %v", pairs)
	}
	if b, _ := os.ReadFile(pairs[0][0]); string(b) != "a\nb\n" {
		t.Errorf("old a.go = %q", b)
	}
	if b, _ := os.ReadFile(pairs[1][0]); string(b) != "bye\n" {
		t.Errorf("old gone.txt = %q", b)
	}
	if b, err := os.ReadFile(pairs[1][1]); err != nil || len(b) != 0 {
		t.Errorf("deleted file's right side = %q, %v; want empty", b, err)
	}
	if got := ZedArgs(pairs[:1]); len(got) != 3 || got[0] != "--diff" {
		t.Errorf("args %v", got)
	}

	od, nd, err := Commit(tmp, "abc", files[:1], func(string) (string, error) { return "a\nB\n", nil })
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(od, "pkg/a.go")); string(b) != "a\nb\n" {
		t.Errorf("old = %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(nd, "pkg/a.go")); string(b) != "a\nB\n" {
		t.Errorf("new = %q", b)
	}
}

func TestGitShow(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput()
		if err != nil {
			t.Skipf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "a.txt")
	git("commit", "-q", "-m", "x")
	sha := git("rev-parse", "HEAD")
	ctx := context.Background()
	if got, err := GitShow(ctx, dir, sha, "a.txt"); err != nil || got != "one\n" {
		t.Errorf("got %q, %v", got, err)
	}
	if got, err := GitShow(ctx, dir, sha, "missing.txt"); err != nil || got != "" {
		t.Errorf("missing path: got %q, %v; want empty", got, err)
	}
}

// The copies go in the repository's git dir, where the editor's language servers find the project's toolchain.
func TestRootInGitDir(t *testing.T) {
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	got, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(Root(filepath.Join(repo)))))
	want, _ := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	if err != nil || got != want || filepath.Base(Root(repo)) != "diff" {
		t.Errorf("Root = %q, want under %s/buti", Root(repo), want)
	}
	if got := Root(t.TempDir()); got != filepath.Join(os.TempDir(), "buti-diff") {
		t.Errorf("outside a repository: %q", got)
	}
}

func TestFromCopy(t *testing.T) {
	tmp := t.TempDir()
	if err := WriteSource(tmp, Source{Branch: "feature/x"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, rel, side string
		ok              bool
	}{
		{"feature_x/new/src/a.go", "src/a.go", "new", true},
		{"feature_x/old/src/a.go", "src/a.go", "old", true},
		{"old/README.md", "README.md", "old", true},
		{"empty/gone.md", "gone.md", "new", true},
		{"stray.go", "", "", false},
	} {
		src, rel, side, ok := FromCopy(filepath.Join(tmp, tc.path))
		if ok != tc.ok || rel != tc.rel || side != tc.side || ok && src.Branch != "feature/x" {
			t.Errorf("%s: got %+v %q %q %v", tc.path, src, rel, side, ok)
		}
	}
	if _, _, _, ok := FromCopy(filepath.Join(t.TempDir(), "a", "new", "b.go")); ok {
		t.Error("a file outside a diff directory is not a copy")
	}
}
