package but

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Worktree is a linked worktree of the project (GitButler 0.23): a stack checked out in its own directory, next to
// the workspace rather than applied to it. Branches is that one stack, top first. Its changes, branches and commits
// have CLI ids that work from the main checkout. Its own CliID is its top branch's.
type Worktree struct {
	CliID              string       `json:"cliId"`
	Name               string       `json:"name"`
	Reference          string       `json:"reference"`
	Base               WorktreeBase `json:"base"`
	UncommittedChanges []Change     `json:"uncommittedChanges"`
	Branches           []Branch     `json:"branches"`

	Path string `json:"-"` // from `but worktree list`; status leaves it out
}

// WorktreeBase is the commit a worktree's branch starts from.
type WorktreeBase struct {
	CommitID    string `json:"commitId"`
	InWorkspace bool   `json:"inWorkspace"` // the base is a commit of an applied branch
	Behind      int    `json:"behind"`      // commits the target has gained since
}

// Worktree finds a worktree by name.
func (s *Status) Worktree(name string) *Worktree {
	for i := range s.Worktrees {
		if s.Worktrees[i].Name == name {
			return &s.Worktrees[i]
		}
	}
	return nil
}

// worktreePaths fills in where each worktree is checked out. Without paths buti can still show the worktrees, so a
// failure is left at that.
func (c *Client) worktreePaths(ctx context.Context, wts []Worktree) {
	var list struct {
		Active []struct {
			Name    string `json:"name"`
			RefName string `json:"refName"`
			Path    string `json:"path"`
		} `json:"active"`
	}
	if c.decode(ctx, &list, "worktree", "list", "--json") != nil {
		return
	}
	for i := range wts {
		for _, a := range list.Active {
			if a.RefName == wts[i].Reference || a.Name == wts[i].Name {
				wts[i].Path = a.Path
			}
		}
	}
}

// WorktreeNew checks out a branch in a new linked worktree: name, or a generated one when empty, starting above
// base (a branch or commit) or at the workspace base when base is empty.
func (c *Client) WorktreeNew(ctx context.Context, name, base string) error {
	args := []string{"worktree", "new"}
	if base != "" {
		args = append(args, "--above", base)
	}
	if name != "" {
		args = append(args, name)
	}
	return c.mutate(ctx, args...)
}

// WorktreeArchive hides a worktree from the workspace until but runs inside it again.
func (c *Client) WorktreeArchive(ctx context.Context, worktree string) error {
	return c.mutate(ctx, "worktree", "archive", worktree)
}

// WorktreeRemove deletes a worktree's directory and keeps its branch; force drops its uncommitted changes.
func (c *Client) WorktreeRemove(ctx context.Context, worktree string, force bool) error {
	args := []string{"worktree", "remove", worktree}
	if force {
		args = append(args, "--force")
	}
	return c.mutate(ctx, args...)
}

// LinkedWorktree reports whether dir is in a linked worktree, and returns the worktree's top directory and the main
// working tree's, where GitButler keeps the workspace. It reads the worktree's .git file instead of running git:
// "gitdir: <main>/.git/worktrees/<x>", whose commondir file leads back to <main>/.git. A submodule's .git file has no
// commondir.
func LinkedWorktree(dir string) (top, main string, ok bool) {
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

// SamePath reports whether a and b are the same existing file or directory, through symlinks.
func SamePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}
