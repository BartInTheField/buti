// Package review stores the review comments left on diffs, shared by the TUI and the `buti review` CLI.
//
// The comments live in one JSON file per repository that is never committed. Every change is a locked
// read-modify-write of that file, so the TUI and a coding agent can write at the same time. The file layout is an
// implementation detail: agents read comments through the CLI, not this file.
package review

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Version is the schema version written to the file.
const Version = 1

// Kind says what a comment is anchored to.
type Kind string

const (
	KindUnassigned Kind = "unassigned" // uncommitted work not assigned to a stack (`zz`)
	KindAssigned   Kind = "assigned"   // uncommitted work assigned to the stack of Anchor.Branch
	KindCommit     Kind = "commit"     // a commit, found by Anchor.ChangeID
)

// Status is where a comment is in its life.
type Status string

const (
	StatusOpen      Status = "open"
	StatusResolved  Status = "resolved"
	StatusDismissed Status = "dismissed"

	// StatusOutdated and StatusOrphaned are never stored: re-anchoring against the current status derives them for
	// an open comment whose line is gone, or whose commit no longer exists.
	StatusOutdated Status = "outdated"
	StatusOrphaned Status = "orphaned"
)

// Stored reports whether s may be written to the file.
func (s Status) Stored() bool {
	return s == StatusOpen || s == StatusResolved || s == StatusDismissed
}

// Side is the side of the diff a line number counts on.
type Side string

const (
	SideNew Side = "new"
	SideOld Side = "old"
)

// AuthorUser is the default author; agent-written comments set another one.
const AuthorUser = "user"

// Anchor is the stable place a comment points at. It holds no `but` cli ids, since those change between status
// calls: shortcodes are derived from the anchor when comments are listed.
type Anchor struct {
	Kind     Kind   `json:"kind"`
	ChangeID string `json:"change_id,omitempty"` // commit: stable across amends
	CommitID string `json:"commit_id,omitempty"` // commit: fallback when the change id is not found
	Branch   string `json:"branch,omitempty"`    // assigned: the stack's branch; commit: the branch it was on
	Path     string `json:"path"`
	Side     Side   `json:"side"`
	Line     int    `json:"line"`
	EndLine  int    `json:"end_line"`
	LineText string `json:"line_text"` // the text of Line, to find it again after the file moves
}

// Resolution records how a comment was closed; for a dismissed comment Summary is the reason.
type Resolution struct {
	Summary string     `json:"summary"`
	At      *time.Time `json:"at"`
}

type Reply struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type Comment struct {
	ID         string     `json:"id"`
	Status     Status     `json:"status"`
	Author     string     `json:"author"`
	Body       string     `json:"body"`
	CreatedAt  time.Time  `json:"created_at"`
	Anchor     Anchor     `json:"anchor"`
	Resolution Resolution `json:"resolution"`
	Replies    []Reply    `json:"replies"`
}

// File is the on-disk document.
type File struct {
	Version  int       `json:"version"`
	Comments []Comment `json:"comments"`
}

var (
	ErrNotFound  = errors.New("no such comment")
	ErrAmbiguous = errors.New("ambiguous comment id")
)

// Store reads and writes the comment file at Path.
type Store struct {
	Path string
	now  func() time.Time
}

// New returns a store for the file at path.
func New(path string) *Store {
	return &Store{Path: path, now: time.Now}
}

// Open returns the store of the repository containing dir, at <git-common-dir>/buti/review.json.
//
// The common dir, rather than --git-dir, is shared by all worktrees of a repository. GitButler keeps its own state
// there too, and commit change ids are repository-wide, so every worktree sees the same comments. Living inside the
// git dir keeps the file out of commits and out of the way of `but` operations.
func Open(dir string) (*Store, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("finding the git dir: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("finding the git dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	return New(filepath.Join(gitDir, "buti", "review.json")), nil
}

// Load reads the file. A missing or empty file is an empty store.
func (s *Store) Load() (File, error) {
	f := File{Version: Version}
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return f, nil
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, fmt.Errorf("reading %s: %w", s.Path, err)
	}
	if f.Version > Version {
		return f, fmt.Errorf("%s has schema version %d; this buti reads up to %d", s.Path, f.Version, Version)
	}
	f.Version = Version
	for i := range f.Comments {
		defaultAuthors(&f.Comments[i])
	}
	return f, nil
}

// defaultAuthors fills in AuthorUser where a file written before authors existed has none.
func defaultAuthors(c *Comment) {
	if c.Author == "" {
		c.Author = AuthorUser
	}
	for i := range c.Replies {
		if c.Replies[i].Author == "" {
			c.Replies[i].Author = AuthorUser
		}
	}
}

// List returns the comments with one of the given statuses, or all of them without any, oldest first.
func (s *Store) List(statuses ...Status) ([]Comment, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}
	if len(statuses) == 0 {
		return f.Comments, nil
	}
	var out []Comment
	for _, c := range f.Comments {
		if slices.Contains(statuses, c.Status) {
			out = append(out, c)
		}
	}
	return out, nil
}

// Get returns the comment whose id is id or starts with it.
func (s *Store) Get(id string) (Comment, error) {
	f, err := s.Load()
	if err != nil {
		return Comment{}, err
	}
	i, err := find(f.Comments, id)
	if err != nil {
		return Comment{}, err
	}
	return f.Comments[i], nil
}

// Add stores a new open comment on anchor and returns it with its id. An empty author is AuthorUser, and an unset
// end line is the start line.
func (s *Store) Add(anchor Anchor, author, body string) (Comment, error) {
	if strings.TrimSpace(body) == "" {
		return Comment{}, errors.New("empty comment")
	}
	if err := anchor.validate(); err != nil {
		return Comment{}, err
	}
	if anchor.Side == "" {
		anchor.Side = SideNew
	}
	if anchor.EndLine == 0 {
		anchor.EndLine = anchor.Line
	}
	if author == "" {
		author = AuthorUser
	}
	var c Comment
	err := s.modify(func(f *File) error {
		id, err := newID(f.Comments)
		if err != nil {
			return err
		}
		c = Comment{
			ID:        id,
			Status:    StatusOpen,
			Author:    author,
			Body:      body,
			CreatedAt: s.now().UTC(),
			Anchor:    anchor,
			Replies:   []Reply{},
		}
		f.Comments = append(f.Comments, c)
		return nil
	})
	return c, err
}

// Update applies fn to the comment matching id under the lock and saves it. Re-anchoring uses it to move a comment's
// line; fn may not change the id or store a derived status.
func (s *Store) Update(id string, fn func(*Comment) error) (Comment, error) {
	var c Comment
	err := s.modify(func(f *File) error {
		i, err := find(f.Comments, id)
		if err != nil {
			return err
		}
		c = f.Comments[i]
		c.Replies = slices.Clone(c.Replies)
		if err := fn(&c); err != nil {
			return err
		}
		if c.ID != f.Comments[i].ID {
			return errors.New("a comment's id can't change")
		}
		if !c.Status.Stored() {
			return fmt.Errorf("status %q can't be stored", c.Status)
		}
		f.Comments[i] = c
		return nil
	})
	return c, err
}

// Edit replaces the body of a comment.
func (s *Store) Edit(id, body string) (Comment, error) {
	if strings.TrimSpace(body) == "" {
		return Comment{}, errors.New("empty comment")
	}
	return s.Update(id, func(c *Comment) error {
		c.Body = body
		return nil
	})
}

// Resolve marks a comment resolved with a summary of what was done.
func (s *Store) Resolve(id, summary string) (Comment, error) {
	return s.close(id, StatusResolved, summary)
}

// Dismiss closes a comment without a change, with an optional reason.
func (s *Store) Dismiss(id, reason string) (Comment, error) {
	return s.close(id, StatusDismissed, reason)
}

func (s *Store) close(id string, status Status, summary string) (Comment, error) {
	return s.Update(id, func(c *Comment) error {
		at := s.now().UTC()
		c.Status = status
		c.Resolution = Resolution{Summary: summary, At: &at}
		return nil
	})
}

// Reopen makes a resolved or dismissed comment open again.
func (s *Store) Reopen(id string) (Comment, error) {
	return s.Update(id, func(c *Comment) error {
		c.Status = StatusOpen
		c.Resolution = Resolution{}
		return nil
	})
}

// Reply adds a reply to a comment. An empty author is AuthorUser.
func (s *Store) Reply(id, author, body string) (Comment, error) {
	if strings.TrimSpace(body) == "" {
		return Comment{}, errors.New("empty reply")
	}
	if author == "" {
		author = AuthorUser
	}
	return s.Update(id, func(c *Comment) error {
		c.Replies = append(c.Replies, Reply{Author: author, Body: body, CreatedAt: s.now().UTC()})
		return nil
	})
}

// Delete removes a comment.
func (s *Store) Delete(id string) error {
	return s.modify(func(f *File) error {
		i, err := find(f.Comments, id)
		if err != nil {
			return err
		}
		f.Comments = slices.Delete(f.Comments, i, i+1)
		return nil
	})
}

// Clear removes every comment with one of the given statuses and returns how many it removed.
func (s *Store) Clear(statuses ...Status) (int, error) {
	n := 0
	err := s.modify(func(f *File) error {
		before := len(f.Comments)
		f.Comments = slices.DeleteFunc(f.Comments, func(c Comment) bool {
			return slices.Contains(statuses, c.Status)
		})
		n = before - len(f.Comments)
		return nil
	})
	return n, err
}

// modify runs fn on the current file under the lock and writes the result when fn succeeds.
func (s *Store) modify(fn func(*File) error) error {
	unlock, err := lock(s.Path)
	if err != nil {
		return err
	}
	defer unlock()
	f, err := s.Load()
	if err != nil {
		return err
	}
	if err := fn(&f); err != nil {
		return err
	}
	return s.write(f)
}

// write replaces the file atomically, so a reader never sees half of it.
func (s *Store) write(f File) error {
	if f.Comments == nil {
		f.Comments = []Comment{}
	}
	for i := range f.Comments {
		if f.Comments[i].Replies == nil {
			f.Comments[i].Replies = []Reply{}
		}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".review-*.json")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows refuses to replace a file another process is reading for a moment; retry briefly.
	for i := 0; ; i++ {
		err = os.Rename(tmp.Name(), s.Path)
		if err == nil || i == 20 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (a Anchor) validate() error {
	switch a.Kind {
	case KindUnassigned:
	case KindAssigned:
		if a.Branch == "" {
			return errors.New("an assigned anchor needs a branch")
		}
	case KindCommit:
		if a.ChangeID == "" && a.CommitID == "" {
			return errors.New("a commit anchor needs a change or commit id")
		}
	default:
		return fmt.Errorf("unknown anchor kind %q", a.Kind)
	}
	if a.Path == "" {
		return errors.New("an anchor needs a path")
	}
	if a.Side != "" && a.Side != SideNew && a.Side != SideOld {
		return fmt.Errorf("unknown side %q", a.Side)
	}
	if a.Line < 1 || (a.EndLine != 0 && a.EndLine < a.Line) {
		return fmt.Errorf("bad line range %d-%d", a.Line, a.EndLine)
	}
	return nil
}

// find returns the index of the comment whose id is id, or the only one starting with it.
func find(cs []Comment, id string) (int, error) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" {
		return -1, ErrNotFound
	}
	if i := slices.IndexFunc(cs, func(c Comment) bool { return c.ID == id }); i >= 0 {
		return i, nil
	}
	match := -1
	for i, c := range cs {
		if strings.HasPrefix(c.ID, id) {
			if match >= 0 {
				return -1, fmt.Errorf("%w: %s", ErrAmbiguous, id)
			}
			match = i
		}
	}
	if match < 0 {
		return -1, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return match, nil
}

// newID returns a random 6-hex id not used by any comment.
func newID(cs []Comment) (string, error) {
	b := make([]byte, 3)
	for range 100 {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		id := hex.EncodeToString(b)
		if !slices.ContainsFunc(cs, func(c Comment) bool { return c.ID == id }) {
			return id, nil
		}
	}
	return "", errors.New("no free comment id")
}
