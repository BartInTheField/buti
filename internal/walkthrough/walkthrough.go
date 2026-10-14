// Package walkthrough stores the walkthroughs pinned to branches, shared by the TUI and the `buti walkthrough` CLI.
//
// A walkthrough is a markdown description of a branch, with Mermaid diagrams kept as fenced code blocks, that a
// coding agent or the user writes for whoever reviews it. All of a repository's walkthroughs live in one JSON file
// that is never committed, keyed by branch name. Every change is a locked read-modify-write of that file, so the TUI
// and an agent can write at the same time. The file layout is an implementation detail: agents use the CLI.
package walkthrough

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/localstate"
)

// Version is the schema version written to the file.
const Version = 1

// AuthorUser is the default author; one an agent writes says so.
const (
	AuthorUser  = "user"
	AuthorAgent = "agent"
)

// Walkthrough is the walkthrough of one branch.
type Walkthrough struct {
	Branch    string    `json:"branch"`
	Title     string    `json:"title"`
	Body      string    `json:"body"` // markdown
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`

	// Head and ChangeID are the branch's tip commit when the walkthrough was written: Head tells whether the branch
	// has moved on since, and ChangeID, which survives amends and rebases, finds the branch again after a rename.
	Head     string `json:"head,omitempty"`
	ChangeID string `json:"change_id,omitempty"`
}

// File is the on-disk document.
type File struct {
	Version      int           `json:"version"`
	Walkthroughs []Walkthrough `json:"walkthroughs"`
}

var ErrNotFound = errors.New("no walkthrough")

// Store reads and writes the walkthrough file at Path.
type Store struct {
	Path string
	now  func() time.Time
}

// New returns a store for the file at path.
func New(path string) *Store {
	return &Store{Path: path, now: time.Now}
}

// Open returns the store of the repository containing dir, at <git-common-dir>/buti/walkthroughs.json; see
// localstate.Dir for why it lives there.
func Open(dir string) (*Store, error) {
	d, err := localstate.Dir(dir)
	if err != nil {
		return nil, err
	}
	return New(filepath.Join(d, "walkthroughs.json")), nil
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
	return f, nil
}

// List returns every walkthrough, by branch name.
func (s *Store) List() ([]Walkthrough, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(f.Walkthroughs, func(a, b Walkthrough) int { return strings.Compare(a.Branch, b.Branch) })
	return f.Walkthroughs, nil
}

// Get returns the walkthrough of branch.
func (s *Store) Get(branch string) (Walkthrough, error) {
	f, err := s.Load()
	if err != nil {
		return Walkthrough{}, err
	}
	i := index(f.Walkthroughs, branch)
	if i < 0 {
		return Walkthrough{}, fmt.Errorf("%w for branch %s", ErrNotFound, branch)
	}
	return f.Walkthroughs[i], nil
}

// Set stores w as the walkthrough of w.Branch, replacing any earlier one, and returns it as stored. An empty author
// is AuthorUser; UpdatedAt is now.
func (s *Store) Set(w Walkthrough) (Walkthrough, error) {
	w.Branch = strings.TrimSpace(w.Branch)
	if w.Branch == "" {
		return Walkthrough{}, errors.New("a walkthrough needs a branch")
	}
	if strings.TrimSpace(w.Body) == "" {
		return Walkthrough{}, errors.New("empty walkthrough")
	}
	if w.Author == "" {
		w.Author = AuthorUser
	}
	w.UpdatedAt = s.now().UTC()
	err := s.modify(func(f *File) error {
		if i := index(f.Walkthroughs, w.Branch); i >= 0 {
			f.Walkthroughs[i] = w
		} else {
			f.Walkthroughs = append(f.Walkthroughs, w)
		}
		return nil
	})
	return w, err
}

// Delete removes the walkthrough of branch.
func (s *Store) Delete(branch string) error {
	return s.modify(func(f *File) error {
		i := index(f.Walkthroughs, branch)
		if i < 0 {
			return fmt.Errorf("%w for branch %s", ErrNotFound, branch)
		}
		f.Walkthroughs = slices.Delete(f.Walkthroughs, i, i+1)
		return nil
	})
}

// Rename moves the walkthrough of from to the branch to, as when the branch was renamed. It fails when to already
// has one.
func (s *Store) Rename(from, to string) (Walkthrough, error) {
	var w Walkthrough
	err := s.modify(func(f *File) error {
		i := index(f.Walkthroughs, from)
		if i < 0 {
			return fmt.Errorf("%w for branch %s", ErrNotFound, from)
		}
		if index(f.Walkthroughs, to) >= 0 {
			return fmt.Errorf("branch %s already has a walkthrough", to)
		}
		f.Walkthroughs[i].Branch = to
		w = f.Walkthroughs[i]
		return nil
	})
	return w, err
}

// Follow re-keys the walkthroughs whose branch was renamed, as Locate finds them in st, so they can be read under
// the new name. One whose new name already has a walkthrough is left alone. It returns the renames, old name to new.
func (s *Store) Follow(st *but.Status) (map[string]string, error) {
	moved := map[string]string{}
	err := s.modify(func(f *File) error {
		for i, w := range f.Walkthroughs {
			loc := Locate(w, st)
			if !loc.Renamed || index(f.Walkthroughs, loc.Branch.Name) >= 0 {
				continue
			}
			f.Walkthroughs[i].Branch = loc.Branch.Name
			moved[w.Branch] = loc.Branch.Name
		}
		if len(moved) == 0 {
			return errUnchanged
		}
		return nil
	})
	if errors.Is(err, errUnchanged) {
		err = nil
	}
	return moved, err
}

// errUnchanged tells modify there is nothing to write.
var errUnchanged = errors.New("unchanged")

// modify runs fn on the current file under the lock and writes the result when fn succeeds.
func (s *Store) modify(fn func(*File) error) error {
	unlock, err := localstate.Lock(s.Path)
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
	if f.Walkthroughs == nil {
		f.Walkthroughs = []Walkthrough{}
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	return localstate.WriteFile(s.Path, append(data, '\n'))
}

func index(ws []Walkthrough, branch string) int {
	branch = strings.TrimSpace(branch)
	return slices.IndexFunc(ws, func(w Walkthrough) bool { return w.Branch == branch })
}

// Tip returns the head commit and its change id of a branch, as Set records them; both are empty for a branch
// without commits.
func Tip(b but.Branch) (head, changeID string) {
	if len(b.Commits) == 0 {
		return "", ""
	}
	return b.Commits[0].CommitID, b.Commits[0].ChangeID
}

// State is where a walkthrough's branch is in the workspace now.
type State struct {
	// Branch is the branch the walkthrough belongs to, nil when it is not in the workspace (deleted or unapplied).
	Branch *but.Branch
	// Renamed is true when the branch was found under another name, by the change id of its old tip.
	Renamed bool
	// Stale is true when the branch's tip is no longer the commit the walkthrough was written on, or when the branch
	// can't be found, so the walkthrough can't be checked.
	Stale bool
}

// Locate finds the branch of w in st: by name, or after a rename by the change id of the tip it was written on.
func Locate(w Walkthrough, st *but.Status) State {
	var byChange *but.Branch
	for si := range st.Stacks {
		for bi := range st.Stacks[si].Branches {
			b := &st.Stacks[si].Branches[bi]
			if b.Name == w.Branch {
				return State{Branch: b, Stale: stale(w, *b)}
			}
			if byChange == nil && w.ChangeID != "" &&
				slices.ContainsFunc(b.Commits, func(c but.Commit) bool { return c.ChangeID == w.ChangeID }) {
				byChange = b
			}
		}
	}
	if byChange != nil {
		return State{Branch: byChange, Renamed: true, Stale: stale(w, *byChange)}
	}
	return State{Stale: true}
}

// stale reports whether b has moved on from the tip w was written on. A walkthrough written without a tip, such as
// on a branch with no commits yet, goes stale once the branch has some.
func stale(w Walkthrough, b but.Branch) bool {
	head, _ := Tip(b)
	return head != w.Head
}
