package review

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

func newStore(t *testing.T) *Store {
	t.Helper()
	s := New(filepath.Join(t.TempDir(), "buti", "review.json"))
	now := time.Date(2026, 9, 27, 10, 12, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	return s
}

// writeRaw puts data in the store's file as another program might.
func writeRaw(t *testing.T, s *Store, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.Path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// must returns a function that fails the test on a store error, for calls whose comment isn't needed.
func must(t *testing.T) func(Comment, error) Comment {
	return func(c Comment, err error) Comment {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
}

func anchor() Anchor {
	return Anchor{Kind: KindUnassigned, Path: "internal/ui/diff.go", Line: 42, LineText: "for _, h := range f.Diff.Hunks {"}
}

func TestLoadMissingOrEmpty(t *testing.T) {
	s := newStore(t)
	cs, err := s.List()
	if err != nil || len(cs) != 0 {
		t.Fatalf("missing file: %v %v", cs, err)
	}
	writeRaw(t, s, "  \n")
	if cs, err := s.List(); err != nil || len(cs) != 0 {
		t.Fatalf("empty file: %v %v", cs, err)
	}
	if _, err := s.Add(anchor(), "", "on an empty file"); err != nil {
		t.Fatal(err)
	}
}

func TestLoadRejectsCorruptAndNewer(t *testing.T) {
	s := newStore(t)
	writeRaw(t, s, "{not json")
	if _, err := s.List(); err == nil {
		t.Fatal("corrupt file loaded")
	}
	if _, err := s.Add(anchor(), "", "x"); err == nil {
		t.Fatal("wrote over a corrupt file")
	}
	writeRaw(t, s, `{"version": 2, "comments": []}`)
	if _, err := s.List(); err == nil {
		t.Fatal("newer schema loaded")
	}
}

func TestAdd(t *testing.T) {
	s := newStore(t)
	c, err := s.Add(anchor(), "", "handle the empty-hunk case")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[0-9a-f]{6}$`).MatchString(c.ID) {
		t.Errorf("id %q is not 6 hex", c.ID)
	}
	if c.Status != StatusOpen || c.Author != AuthorUser || c.Anchor.Side != SideNew || c.Anchor.EndLine != 42 {
		t.Errorf("defaults not applied: %+v", c)
	}
	agent, _ := s.Add(anchor(), "agent", "from an agent")
	if agent.Author != "agent" {
		t.Errorf("author = %q", agent.Author)
	}
	got, err := s.Get(c.ID)
	if err != nil || got.Body != c.Body {
		t.Fatalf("get: %+v %v", got, err)
	}

	bad := []Anchor{
		{Kind: "nope", Path: "a", Line: 1},
		{Kind: KindUnassigned, Line: 1},
		{Kind: KindUnassigned, Path: "a"},
		{Kind: KindUnassigned, Path: "a", Line: 5, EndLine: 3},
		{Kind: KindUnassigned, Path: "a", Line: 1, Side: "middle"},
		{Kind: KindAssigned, Path: "a", Line: 1},
		{Kind: KindCommit, Path: "a", Line: 1},
	}
	for _, a := range bad {
		if _, err := s.Add(a, "", "x"); err == nil {
			t.Errorf("accepted anchor %+v", a)
		}
	}
	if _, err := s.Add(anchor(), "", "  "); err == nil {
		t.Error("accepted an empty body")
	}
}

func TestList(t *testing.T) {
	s := newStore(t)
	a, _ := s.Add(anchor(), "", "one")
	b, _ := s.Add(anchor(), "", "two")
	c, _ := s.Add(anchor(), "", "three")
	must(t)(s.Resolve(b.ID, "done"))
	must(t)(s.Dismiss(c.ID, "won't fix"))

	ids := func(cs []Comment, err error) string {
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return strings.Join(out, ",")
	}
	if got, want := ids(s.List()), a.ID+","+b.ID+","+c.ID; got != want {
		t.Errorf("all = %s, want %s", got, want)
	}
	if got := ids(s.List(StatusOpen)); got != a.ID {
		t.Errorf("open = %s, want %s", got, a.ID)
	}
	if got, want := ids(s.List(StatusResolved, StatusDismissed)), b.ID+","+c.ID; got != want {
		t.Errorf("closed = %s, want %s", got, want)
	}
}

func TestResolveDismissReopen(t *testing.T) {
	s := newStore(t)
	c, _ := s.Add(anchor(), "", "rename this")
	r, err := s.Resolve(c.ID, "renamed to hunkIndex")
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != StatusResolved || r.Resolution.Summary != "renamed to hunkIndex" || r.Resolution.At == nil {
		t.Errorf("resolve: %+v", r)
	}
	if got, _ := s.Get(c.ID); got.Status != StatusResolved {
		t.Errorf("resolution not saved: %+v", got)
	}
	o, _ := s.Reopen(c.ID)
	if o.Status != StatusOpen || o.Resolution.At != nil || o.Resolution.Summary != "" {
		t.Errorf("reopen: %+v", o)
	}
	d, _ := s.Dismiss(c.ID, "intended")
	if d.Status != StatusDismissed || d.Resolution.Summary != "intended" {
		t.Errorf("dismiss: %+v", d)
	}
	if _, err := s.Resolve("ffffff", ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("resolve unknown: %v", err)
	}
}

func TestReplyEditDelete(t *testing.T) {
	s := newStore(t)
	c, _ := s.Add(anchor(), "", "why?")
	must(t)(s.Reply(c.ID, "agent", "because"))
	r, err := s.Reply(c.ID, "", "ok")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Replies) != 2 || r.Replies[0].Author != "agent" || r.Replies[1].Author != AuthorUser ||
		r.Replies[1].Body != "ok" || r.Replies[1].CreatedAt.IsZero() {
		t.Errorf("replies: %+v", r.Replies)
	}
	if _, err := s.Reply(c.ID, "", ""); err == nil {
		t.Error("accepted an empty reply")
	}
	if e, _ := s.Edit(c.ID, "why this?"); e.Body != "why this?" || len(e.Replies) != 2 {
		t.Errorf("edit: %+v", e)
	}
	if err := s.Delete(c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(c.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted comment found: %v", err)
	}
}

func TestUpdate(t *testing.T) {
	s := newStore(t)
	c, _ := s.Add(anchor(), "", "x")
	u, err := s.Update(c.ID, func(c *Comment) error {
		c.Anchor.Line, c.Anchor.EndLine = 45, 45
		return nil
	})
	if err != nil || u.Anchor.Line != 45 {
		t.Fatalf("update: %+v %v", u, err)
	}
	if _, err := s.Update(c.ID, func(c *Comment) error { c.Status = StatusOutdated; return nil }); err == nil {
		t.Error("stored a derived status")
	}
	if _, err := s.Update(c.ID, func(c *Comment) error { c.ID = "000000"; return nil }); err == nil {
		t.Error("changed an id")
	}
	boom := errors.New("boom")
	if _, err := s.Update(c.ID, func(c *Comment) error { c.Body = "lost"; return boom }); !errors.Is(err, boom) {
		t.Errorf("fn error: %v", err)
	}
	if got, _ := s.Get(c.ID); got.Body != "x" || got.Anchor.Line != 45 {
		t.Errorf("after failed update: %+v", got)
	}
}

func TestClear(t *testing.T) {
	s := newStore(t)
	a, _ := s.Add(anchor(), "", "one")
	b, _ := s.Add(anchor(), "", "two")
	must(t)(s.Add(anchor(), "", "three"))
	must(t)(s.Resolve(a.ID, ""))
	must(t)(s.Resolve(b.ID, ""))
	n, err := s.Clear(StatusResolved)
	if err != nil || n != 2 {
		t.Fatalf("clear = %d %v", n, err)
	}
	if cs, _ := s.List(); len(cs) != 1 || cs[0].Body != "three" {
		t.Errorf("left: %+v", cs)
	}
}

func TestPrefixMatch(t *testing.T) {
	s := newStore(t)
	f := File{Version: Version, Comments: []Comment{
		{ID: "a1b2c3", Status: StatusOpen, Body: "one"},
		{ID: "a1f000", Status: StatusOpen, Body: "two"},
		{ID: "a1", Status: StatusOpen, Body: "exact"},
	}}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.write(f); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]string{"a1b": "one", "A1F": "two", "a1b2c3": "one", "a1": "exact"} {
		if c, err := s.Get(id); err != nil || c.Body != want {
			t.Errorf("Get(%q) = %q %v, want %q", id, c.Body, err, want)
		}
	}
	if _, err := s.Get("a"); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("Get(a): %v", err)
	}
	for _, id := range []string{"b", ""} {
		if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
			t.Errorf("Get(%q): %v", id, err)
		}
	}
}

// TestConcurrentWriters runs writers in parallel, as the TUI and an agent do; the lock must keep every write.
func TestConcurrentWriters(t *testing.T) {
	s := newStore(t)
	seed, _ := s.Add(anchor(), "", "seed")
	const writers, each = 8, 10
	var wg sync.WaitGroup
	errs := make(chan error, writers*each*2)
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// A store per writer, so nothing is shared but the file.
			s := New(s.Path)
			for i := range each {
				if _, err := s.Add(anchor(), "", fmt.Sprintf("w%d-%d", w, i)); err != nil {
					errs <- err
				}
				if _, err := s.Reply(seed.ID, "", fmt.Sprintf("r%d-%d", w, i)); err != nil {
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
	cs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1+writers*each {
		t.Errorf("%d comments, want %d", len(cs), 1+writers*each)
	}
	seen := map[string]bool{}
	for _, c := range cs {
		if seen[c.ID] {
			t.Errorf("duplicate id %s", c.ID)
		}
		seen[c.ID] = true
	}
	if got, _ := s.Get(seed.ID); len(got.Replies) != writers*each {
		t.Errorf("%d replies, want %d", len(got.Replies), writers*each)
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(s.Path), ".review-*"))
	if len(leftovers) > 0 {
		t.Errorf("temp files left: %v", leftovers)
	}
}

// TestConcurrentProcesses checks the lock across processes, not just goroutines.
func TestConcurrentProcesses(t *testing.T) {
	if path := os.Getenv("BUTI_REVIEW_CHILD"); path != "" {
		s := New(path)
		for i := range 20 {
			if _, err := s.Add(anchor(), "", fmt.Sprint(i)); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	s := newStore(t)
	var cmds []*exec.Cmd
	for range 4 {
		cmd := exec.Command(os.Args[0], "-test.run=^TestConcurrentProcesses$")
		cmd.Env = append(os.Environ(), "BUTI_REVIEW_CHILD="+s.Path)
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
	if cs, err := s.List(); err != nil || len(cs) != 80 {
		t.Fatalf("%d comments, want 80 (%v)", len(cs), err)
	}
}

// TestRoundTrip reads the schema from the issue, writes it back and checks that nothing is lost.
func TestRoundTrip(t *testing.T) {
	const doc = `{
  "version": 1,
  "comments": [
    {
      "id": "a1b2c3",
      "status": "resolved",
      "author": "user",
      "body": "This should handle the empty-hunk case",
      "created_at": "2026-09-27T10:12:00Z",
      "anchor": {
        "kind": "commit",
        "change_id": "wzqkxlsu",
        "commit_id": "3f9e1a0",
        "branch": "auth",
        "path": "internal/ui/diff.go",
        "side": "new",
        "line": 42,
        "end_line": 44,
        "line_text": "for _, h := range f.Diff.Hunks {"
      },
      "resolution": {
        "summary": "Skipped empty hunks",
        "at": "2026-09-27T11:00:00Z"
      },
      "replies": [
        {
          "author": "agent",
          "body": "Done",
          "created_at": "2026-09-27T10:59:00Z"
        }
      ]
    },
    {
      "id": "d4e5f6",
      "status": "open",
      "author": "agent",
      "body": "[nit] typo",
      "created_at": "2026-09-27T10:13:00Z",
      "anchor": {
        "kind": "assigned",
        "branch": "api",
        "path": "README.md",
        "side": "old",
        "line": 3,
        "end_line": 3,
        "line_text": "teh"
      },
      "resolution": {
        "summary": "",
        "at": null
      },
      "replies": []
    }
  ]
}
`
	s := newStore(t)
	writeRaw(t, s, doc)
	f, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	c := f.Comments[0]
	if c.Anchor.Kind != KindCommit || c.Anchor.ChangeID != "wzqkxlsu" || c.Anchor.EndLine != 44 ||
		c.Resolution.At == nil || len(c.Replies) != 1 || f.Comments[1].Anchor.Side != SideOld {
		t.Fatalf("decoded: %+v", f)
	}
	if err := s.write(f); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(s.Path)
	if string(got) != doc {
		t.Errorf("round trip changed the file:\n%s", got)
	}
	var generic map[string]any
	if err := json.Unmarshal(got, &generic); err != nil {
		t.Fatal(err)
	}

	// A file without authors, as the issue's draft has, loads with the user as the author.
	writeRaw(t, s, `{"version":1,"comments":[{"id":"abcdef","status":"open","body":"b","anchor":{"kind":"unassigned","path":"a","line":1}}]}`)
	r, err := s.Reply("abc", "", "ok")
	if err != nil || len(r.Replies) != 1 || r.Author != AuthorUser {
		t.Fatalf("reply on a minimal file: %+v %v", r, err)
	}
}

func TestOpen(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "repo")
	dir = filepath.Join(dir, "repo")
	run("commit", "-q", "--allow-empty", "-m", "init")
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	run("worktree", "add", "-q", filepath.Join(dir, "..", "wt"))

	want, _ := filepath.EvalSymlinks(filepath.Join(dir, ".git"))
	for _, from := range []string{dir, filepath.Join(dir, "sub"), filepath.Join(dir, "..", "wt")} {
		s, err := Open(from)
		if err != nil {
			t.Fatal(err)
		}
		got, _ := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(s.Path)))
		if got != want || filepath.Base(s.Path) != "review.json" {
			t.Errorf("Open(%s) = %s, want under %s/buti", from, s.Path, want)
		}
	}
	if _, err := Open(t.TempDir()); err == nil {
		t.Error("Open outside a repository succeeded")
	}
}
