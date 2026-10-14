package walkthrough

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bartinthefield/buti/internal/but"
)

// mermaidBody is a walkthrough as an agent writes one, with a Mermaid fence that has to survive the round trip.
const mermaidBody = "## Summary\n\nAdds token auth.\n\n```mermaid\nsequenceDiagram\n    Client->>API: POST /login\n    API-->>Client: 200 {\"token\": \"…\"}\n```\n\n| File | Change |\n|---|---|\n| `src/auth/token.go` | new |\n"

func newStore(t *testing.T) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), "buti", "walkthroughs.json"))
	s.now = func() time.Time { return time.Date(2026, 10, 1, 12, 0, 0, 0, time.FixedZone("CEST", 2*3600)) }
	return s
}

func TestEmptyStore(t *testing.T) {
	s := newStore(t)
	if ws, err := s.List(); err != nil || len(ws) != 0 {
		t.Fatalf("List() = %v, %v", ws, err)
	}
	if _, err := s.Get("auth"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on a missing file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if ws, err := s.List(); err != nil || len(ws) != 0 {
		t.Errorf("List() on an empty file = %v, %v", ws, err)
	}
}

func TestSetGetListDelete(t *testing.T) {
	s := newStore(t)
	w, err := s.Set(Walkthrough{Branch: " auth ", Title: "Token auth", Body: mermaidBody, Author: AuthorAgent, Head: "abc", ChangeID: "chg"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Branch != "auth" || w.Author != AuthorAgent || !w.UpdatedAt.Equal(s.now()) || w.UpdatedAt.Location() != time.UTC {
		t.Errorf("Set returned %+v", w)
	}
	if _, err := s.Set(Walkthrough{Branch: "api", Body: "# API"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Get("auth")
	if err != nil {
		t.Fatal(err)
	}
	if got != w {
		t.Errorf("Get = %+v, want %+v", got, w)
	}
	if got.Body != mermaidBody {
		t.Errorf("body changed in the round trip:\n%q", got.Body)
	}

	ws, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(ws) != 2 || ws[0].Branch != "api" || ws[1].Branch != "auth" || ws[0].Author != AuthorUser {
		t.Errorf("List = %+v", ws)
	}

	// Set replaces, rather than adding a second walkthrough for the branch.
	if _, err := s.Set(Walkthrough{Branch: "auth", Body: "v2"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get("auth"); got.Body != "v2" || got.Title != "" {
		t.Errorf("after replacing: %+v", got)
	}
	if ws, _ := s.List(); len(ws) != 2 {
		t.Errorf("%d walkthroughs after replacing, want 2", len(ws))
	}

	if err := s.Delete("auth"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("auth"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: %v", err)
	}
	if err := s.Delete("auth"); !errors.Is(err, ErrNotFound) {
		t.Errorf("second Delete: %v", err)
	}
}

func TestSetRejects(t *testing.T) {
	s := newStore(t)
	for _, w := range []Walkthrough{{Branch: "", Body: "x"}, {Branch: "auth", Body: " \n"}} {
		if _, err := s.Set(w); err == nil {
			t.Errorf("Set(%+v) succeeded", w)
		}
	}
}

func TestRename(t *testing.T) {
	s := newStore(t)
	_, _ = s.Set(Walkthrough{Branch: "auth", Body: "a"})
	_, _ = s.Set(Walkthrough{Branch: "api", Body: "b"})
	if _, err := s.Rename("auth", "api"); err == nil {
		t.Error("Rename onto a branch with a walkthrough succeeded")
	}
	w, err := s.Rename("auth", "login")
	if err != nil || w.Branch != "login" || w.Body != "a" {
		t.Fatalf("Rename = %+v, %v", w, err)
	}
	if _, err := s.Get("auth"); !errors.Is(err, ErrNotFound) {
		t.Errorf("old name still has one: %v", err)
	}
	if _, err := s.Rename("nope", "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Rename of a missing one: %v", err)
	}
}

// TestFileFormat pins the stored layout and that a newer schema is refused rather than overwritten.
func TestFileFormat(t *testing.T) {
	s := newStore(t)
	if _, err := s.Set(Walkthrough{Branch: "auth", Body: "x", Head: "abc"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(s.Path)
	for _, want := range []string{`"version": 1`, `"walkthroughs": [`, `"branch": "auth"`, `"updated_at": "2026-10-01T10:00:00Z"`, `"head": "abc"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("file lacks %s:\n%s", want, data)
		}
	}
	if err := os.WriteFile(s.Path, []byte(`{"version": 2, "walkthroughs": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.List(); err == nil || !strings.Contains(err.Error(), "schema version 2") {
		t.Errorf("List of a newer file: %v", err)
	}
	if _, err := s.Set(Walkthrough{Branch: "auth", Body: "x"}); err == nil {
		t.Error("Set overwrote a newer file")
	}
}

// TestConcurrentWriters runs writers in parallel, as the TUI and an agent do; the lock must keep every write.
func TestConcurrentWriters(t *testing.T) {
	s := newStore(t)
	const writers, each = 8, 10
	var wg sync.WaitGroup
	errs := make(chan error, writers*each)
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A store per writer, so nothing is shared but the file.
			s := New(s.Path)
			for i := range each {
				if _, err := s.Set(Walkthrough{Branch: fmt.Sprintf("b%d-%d", w, i), Body: mermaidBody}); err != nil {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if ws, err := s.List(); err != nil || len(ws) != writers*each {
		t.Errorf("%d walkthroughs, want %d (%v)", len(ws), writers*each, err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(s.Path), ".walkthroughs-*"))
	if len(leftovers) > 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}

// TestConcurrentProcesses checks the lock across processes, not just goroutines.
func TestConcurrentProcesses(t *testing.T) {
	if path := os.Getenv("BUTI_WALKTHROUGH_CHILD"); path != "" {
		s := New(path)
		for i := range 20 {
			if _, err := s.Set(Walkthrough{Branch: fmt.Sprintf("%d-%d", os.Getpid(), i), Body: "x"}); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	s := newStore(t)
	var cmds []*exec.Cmd
	for range 4 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "BUTI_WALKTHROUGH_CHILD="+s.Path)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if ws, err := s.List(); err != nil || len(ws) != 80 {
		t.Fatalf("%d walkthroughs, want 80 (%v)", len(ws), err)
	}
}

func TestLocate(t *testing.T) {
	st := &but.Status{Stacks: []but.Stack{{Branches: []but.Branch{
		{Name: "login", Commits: []but.Commit{{CommitID: "new", ChangeID: "c2"}, {CommitID: "old", ChangeID: "c1"}}},
		{Name: "api", Commits: []but.Commit{{CommitID: "a1", ChangeID: "ca"}}},
		{Name: "empty"},
	}}}}
	for _, tc := range []struct {
		name                  string
		w                     Walkthrough
		branch                string
		renamed, stale, found bool
	}{
		{"current", Walkthrough{Branch: "api", Head: "a1", ChangeID: "ca"}, "api", false, false, true},
		{"moved on", Walkthrough{Branch: "login", Head: "old", ChangeID: "c1"}, "login", false, true, true},
		{"renamed", Walkthrough{Branch: "auth", Head: "new", ChangeID: "c2"}, "login", true, false, true},
		{"renamed and moved on", Walkthrough{Branch: "auth", Head: "old", ChangeID: "c1"}, "login", true, true, true},
		{"empty branch", Walkthrough{Branch: "empty"}, "empty", false, false, true},
		{"gone", Walkthrough{Branch: "gone", Head: "x", ChangeID: "nope"}, "", false, true, false},
		{"empty branch renamed", Walkthrough{Branch: "was-empty"}, "", false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Locate(tc.w, st)
			if (got.Branch != nil) != tc.found || got.Renamed != tc.renamed || got.Stale != tc.stale {
				t.Fatalf("Locate = %+v", got)
			}
			if got.Branch != nil && got.Branch.Name != tc.branch {
				t.Errorf("branch %s, want %s", got.Branch.Name, tc.branch)
			}
		})
	}
}

// TestFollow re-keys a walkthrough whose branch was renamed, unless the new name has one of its own.
func TestFollow(t *testing.T) {
	s := newStore(t)
	st := &but.Status{Stacks: []but.Stack{{Branches: []but.Branch{
		{Name: "login", Commits: []but.Commit{{CommitID: "a2", ChangeID: "c2"}}},
		{Name: "http", Commits: []but.Commit{{CommitID: "p1", ChangeID: "cp"}}},
		{Name: "kept", Commits: []but.Commit{{CommitID: "k1", ChangeID: "ck"}}},
	}}}}
	for _, w := range []Walkthrough{
		{Branch: "auth", Body: "auth", Head: "a2", ChangeID: "c2"}, // renamed to login
		{Branch: "api", Body: "api", Head: "p1", ChangeID: "cp"},   // renamed to http, which has its own
		{Branch: "http", Body: "http", Head: "p1", ChangeID: "cp"},
		{Branch: "kept", Body: "kept", Head: "k1", ChangeID: "ck"},
		{Branch: "gone", Body: "gone", Head: "x", ChangeID: "cx"},
	} {
		if _, err := s.Set(w); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := s.Follow(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(moved) != 1 || moved["auth"] != "login" {
		t.Errorf("Follow moved %v, want auth to login", moved)
	}
	if w, err := s.Get("login"); err != nil || w.Body != "auth" {
		t.Errorf("Get(login) = %+v, %v", w, err)
	}
	for _, b := range []string{"api", "http", "kept", "gone"} {
		if w, err := s.Get(b); err != nil || w.Body != b {
			t.Errorf("Get(%s) = %+v, %v", b, w, err)
		}
	}
	before, _ := os.Stat(s.Path)
	if moved, err := s.Follow(st); err != nil || len(moved) != 0 {
		t.Errorf("second Follow = %v, %v", moved, err)
	}
	if after, _ := os.Stat(s.Path); !after.ModTime().Equal(before.ModTime()) {
		t.Error("a Follow that moved nothing rewrote the file")
	}
}

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The temp dir may sit behind a symlink (/var on macOS), so compare the resolved git dirs.
	got, _ := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(s.Path)))
	want, _ := filepath.EvalSymlinks(filepath.Join(dir, ".git"))
	if got != want || filepath.Base(s.Path) != "walkthroughs.json" {
		t.Errorf("Path = %s, want it in %s/buti", s.Path, want)
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("Open outside a repository succeeded")
	}
}
