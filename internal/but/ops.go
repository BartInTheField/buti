package but

import (
	"context"
	"strings"
)

// Placement says where new or moved commits and branches go. At most one field is set;
// the zero value means "let but decide" (usually a new unstacked branch).
type Placement struct {
	Branch    string // onto the tip of this branch (created if missing)
	Above     string // above this branch or commit
	Below     string // below this branch or commit
	NewBranch bool   // onto a new unstacked branch with a generated name
}

// Args renders the placement flags. They go last on the command line, because a
// bare --branch would otherwise swallow the next positional argument.
func (p Placement) Args() []string {
	switch {
	case p.NewBranch:
		return []string{"--branch"}
	case p.Above != "":
		return []string{"--above", p.Above}
	case p.Below != "":
		return []string{"--below", p.Below}
	case p.Branch != "":
		return []string{"--branch", p.Branch}
	}
	return nil
}

// Exec runs an arbitrary but command and returns its output, for the command prompt.
func (c *Client) Exec(ctx context.Context, args ...string) (string, error) {
	out, err := c.run(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func (c *Client) mutate(ctx context.Context, args ...string) error {
	_, err := c.run(ctx, args...)
	return err
}

func messageArgs(msg string) []string {
	if strings.TrimSpace(msg) == "" {
		return []string{"--no-message"}
	}
	return []string{"--message", msg}
}

// Commit commits changes (file or hunk ids; all uncommitted changes when empty).
func (c *Client) Commit(ctx context.Context, changes []string, msg string, at Placement) error {
	args := append([]string{"commit"}, messageArgs(msg)...)
	args = append(args, changes...)
	return c.mutate(ctx, append(args, at.Args()...)...)
}

// EmptyCommit inserts an empty commit relative to a branch or commit.
func (c *Client) EmptyCommit(ctx context.Context, msg string, at Placement) error {
	args := append([]string{"commit", "--empty"}, messageArgs(msg)...)
	if at.NewBranch {
		at = Placement{}
	}
	return c.mutate(ctx, append(args, at.Args()...)...)
}

// Amend folds uncommitted changes into a commit or branch.
func (c *Client) Amend(ctx context.Context, target string, changes []string) error {
	return c.mutate(ctx, append([]string{"amend", "--target", target}, changes...)...)
}

// Absorb amends a change (or everything uncommitted when empty) into the commits it belongs to.
func (c *Client) Absorb(ctx context.Context, source string) error {
	if source == "" {
		return c.mutate(ctx, "absorb")
	}
	return c.mutate(ctx, "absorb", source)
}

type SquashMessage int

const (
	SquashCombineMessages SquashMessage = iota
	SquashUseTargetMessage
	SquashUseSourceMessage
)

// Squash squashes sources into target (all commits of a single branch source when
// target is empty). msg is only used with SquashCombineMessages; when empty, both
// messages are kept by `but`.
func (c *Client) Squash(ctx context.Context, sources []string, target string, m SquashMessage, msg string) error {
	args := []string{"squash"}
	if target != "" {
		args = append(args, "--target", target)
	}
	switch m {
	case SquashUseTargetMessage:
		args = append(args, "--use-target-message")
	case SquashUseSourceMessage:
		args = append(args, "--use-source-message")
	default:
		args = append(args, messageArgs(msg)...)
	}
	return c.mutate(ctx, append(args, sources...)...)
}

// Move moves commits, committed files or a branch.
func (c *Client) Move(ctx context.Context, sources []string, at Placement) error {
	args := append([]string{"move"}, sources...)
	if at.NewBranch || at.Args() == nil {
		return c.mutate(ctx, append(args, "--unstack")...)
	}
	return c.mutate(ctx, append(args, at.Args()...)...)
}

// Uncommit moves commits, branches or committed files back to the uncommitted area.
func (c *Client) Uncommit(ctx context.Context, sources []string) error {
	return c.mutate(ctx, append([]string{"uncommit"}, sources...)...)
}

// Reword changes a commit message, or renames a branch.
func (c *Client) Reword(ctx context.Context, target, msg string) error {
	return c.mutate(ctx, "reword", target, "--message", msg)
}

// Discard throws away changes, commits or branches.
func (c *Client) Discard(ctx context.Context, targets []string) error {
	return c.mutate(ctx, append([]string{"discard"}, targets...)...)
}

func (c *Client) BranchNew(ctx context.Context, name string, at Placement) error {
	args := []string{"branch", "new"}
	if a := at.Args(); a != nil && at.Branch == "" && !at.NewBranch {
		args = append(args, a...)
	}
	if name != "" {
		args = append(args, name)
	}
	return c.mutate(ctx, args...)
}

func (c *Client) BranchDelete(ctx context.Context, branches []string) error {
	return c.mutate(ctx, append([]string{"branch", "delete"}, branches...)...)
}

// Pick cherry-picks commits into the workspace.
func (c *Client) Pick(ctx context.Context, sources []string, at Placement) error {
	args := append([]string{"pick"}, sources...)
	return c.mutate(ctx, append(args, at.Args()...)...)
}

func (c *Client) Apply(ctx context.Context, branch string) error {
	return c.mutate(ctx, "apply", branch)
}

func (c *Client) Unapply(ctx context.Context, branchOrStack string) error {
	return c.mutate(ctx, "unapply", branchOrStack)
}

func (c *Client) Push(ctx context.Context, branch string, force bool) error {
	args := []string{"push", branch}
	if force {
		args = append(args, "--with-force")
	}
	return c.mutate(ctx, args...)
}

func (c *Client) Pull(ctx context.Context) error { return c.mutate(ctx, "pull") }

func (c *Client) Undo(ctx context.Context) error { return c.mutate(ctx, "undo") }

func (c *Client) Redo(ctx context.Context) error { return c.mutate(ctx, "redo") }

// PRNew opens a review for a branch. An empty message uses the default title and description.
func (c *Client) PRNew(ctx context.Context, branch, msg string, draft bool) error {
	args := []string{"pr", "new", branch}
	if msg == "" {
		args = append(args, "--default")
	} else {
		args = append(args, "--message", msg)
	}
	if draft {
		args = append(args, "--draft")
	}
	return c.mutate(ctx, args...)
}

func (c *Client) OplogRestore(ctx context.Context, snapshot string) error {
	return c.mutate(ctx, "oplog", "restore", snapshot)
}

type OplogEntry struct {
	ID        string `json:"id"`
	CreatedAt int64  `json:"createdAt"` // unix millis
	Details   struct {
		Operation string `json:"operation"`
		Title     string `json:"title"`
		Body      string `json:"body"`
	} `json:"details"`
}

// Oplog returns the operation history, newest first.
func (c *Client) Oplog(ctx context.Context) ([]OplogEntry, error) {
	var entries []OplogEntry
	return entries, c.decode(ctx, &entries, "oplog", "list", "--json")
}

type BranchListing struct {
	Name          string `json:"name"`
	HasLocal      bool   `json:"hasLocal"`
	LastCommitAt  int64  `json:"lastCommitAt"`
	CommitsAhead  *int   `json:"commitsAhead"`
	MergesCleanly *bool  `json:"mergesCleanly"`
	LastAuthor    struct {
		Name string `json:"name"`
	} `json:"lastAuthor"`
}

type Branches struct {
	Applied []struct {
		Heads []BranchListing `json:"heads"`
	} `json:"appliedStacks"`
	Branches []BranchListing `json:"branches"` // not applied
}

// Branches lists applied and unapplied branches.
func (c *Client) Branches(ctx context.Context) (*Branches, error) {
	var b Branches
	return &b, c.decode(ctx, &b, "branch", "list", "--json", "--all", "--empty", "--no-check")
}
