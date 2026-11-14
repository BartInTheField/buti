package but

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Worktree is a linked worktree (`but worktree new`): a branch checked out in its own directory beside the
// workspace. `but status` reports every worktree, wherever it runs.
type Worktree struct {
	CliID              string       `json:"cliId"`
	Name               string       `json:"name"`
	Reference          string       `json:"reference"`
	Base               WorktreeBase `json:"base"`
	UncommittedChanges []Change     `json:"uncommittedChanges"`
	Branches           []Branch     `json:"branches"`
}

// WorktreeBase is the commit a worktree's branch starts from.
type WorktreeBase struct {
	CommitID    string `json:"commitId"`
	InWorkspace bool   `json:"inWorkspace"` // a commit of an applied stack
	Behind      int    `json:"behind"`      // commits the target branch has gained since
}

// Worktree returns the worktree named name, nil when there is none.
func (s *Status) Worktree(name string) *Worktree {
	for i := range s.Worktrees {
		if s.Worktrees[i].Name == name {
			return &s.Worktrees[i]
		}
	}
	return nil
}

// InWorktree is s as seen from the worktree named name: its uncommitted changes, and its branches as the only
// stack. It is nil when there is no such worktree.
func (s *Status) InWorktree(name string) *Status {
	wt := s.Worktree(name)
	if wt == nil {
		return nil
	}
	return &Status{
		UncommittedChanges: wt.UncommittedChanges,
		Stacks:             []Stack{{CliID: wt.CliID, Branches: wt.Branches}},
		MergeBase:          Commit{CommitID: wt.Base.CommitID},
		UpstreamState:      UpstreamState{Behind: wt.Base.Behind, LatestCommit: s.UpstreamState.LatestCommit},
		Worktrees:          s.Worktrees,
	}
}

// WorktreeListing is a worktree as `but worktree list` shows it, with the directory it is checked out in.
type WorktreeListing struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	RefName     string `json:"refName"`
	Path        string `json:"path"`
	UpdatedAtMs int64  `json:"updatedAtMs"`
}

type WorktreeList struct {
	Active   []WorktreeListing `json:"active"`
	Archived []WorktreeListing `json:"archived"`
}

// Worktrees lists the worktrees, most recently updated first.
func (c *Client) Worktrees(ctx context.Context) (*WorktreeList, error) {
	var l WorktreeList
	return &l, c.decode(ctx, &l, "worktree", "list", "--json")
}

// NewWorktree checks out a new branch named name in a new worktree, and returns it.
func (c *Client) NewWorktree(ctx context.Context, name string) (*WorktreeListing, error) {
	var w WorktreeListing
	return &w, c.decode(ctx, &w, "worktree", "new", name, "--json")
}

// OpenWorktree finds the worktree dir is in, among those `but worktree list` knows, and returns a client for the
// workspace it was made from. `but status` run in a worktree reports the whole workspace, so buti has to open the
// worktree itself. It returns nil, nil when dir is in no such worktree.
func OpenWorktree(ctx context.Context, dir string) (*Client, *WorktreeListing, error) {
	top, workspace, ok := linkedWorktree(dir)
	if !ok {
		return nil, nil, nil
	}
	c := New(workspace)
	l, err := c.Worktrees(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, w := range append(l.Active, l.Archived...) {
		if samePath(w.Path, top) {
			return c, &w, nil
		}
	}
	return nil, nil, nil
}

// linkedWorktree reports whether dir is in a linked worktree, and returns the worktree's top directory and the
// main working tree's. It reads the worktree's .git file instead of running git: "gitdir: <main>/.git/worktrees/<x>",
// whose commondir file leads back to <main>/.git. A submodule's .git file has no commondir.
func linkedWorktree(dir string) (top, main string, ok bool) {
	for d := dir; ; d = filepath.Dir(d) {
		dotgit := filepath.Join(d, ".git")
		if info, err := os.Lstat(dotgit); err == nil {
			if info.IsDir() {
				return "", "", false
			}
			b, err := os.ReadFile(dotgit)
			gitdir, found := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
			if err != nil || !found {
				return "", "", false
			}
			if !filepath.IsAbs(gitdir) {
				gitdir = filepath.Join(d, gitdir)
			}
			c, err := os.ReadFile(filepath.Join(gitdir, "commondir"))
			if err != nil {
				return "", "", false
			}
			common := strings.TrimSpace(string(c))
			if !filepath.IsAbs(common) {
				common = filepath.Join(gitdir, common)
			}
			return d, filepath.Dir(filepath.Clean(common)), true
		}
		if filepath.Dir(d) == d {
			return "", "", false
		}
	}
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
