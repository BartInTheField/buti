// Package but wraps the GitButler CLI (`but`) and decodes its JSON output.
package but

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type Change struct {
	CliID      string `json:"cliId"`
	FilePath   string `json:"filePath"`
	ChangeType string `json:"changeType"`
}

type Commit struct {
	CliID       string    `json:"cliId"`
	ChangeID    string    `json:"changeId"`
	CommitID    string    `json:"commitId"`
	CreatedAt   time.Time `json:"createdAt"`
	Message     string    `json:"message"`
	AuthorName  string    `json:"authorName"`
	AuthorEmail string    `json:"authorEmail"`
	Conflicted  *bool     `json:"conflicted"`
	Changes     []Change  `json:"changes"`
}

// Subject returns the first line of the commit message.
func (c Commit) Subject() string {
	s, _, _ := strings.Cut(c.Message, "\n")
	return s
}

func (c Commit) ShortID() string {
	if len(c.CommitID) > 7 {
		return c.CommitID[:7]
	}
	return c.CommitID
}

type Branch struct {
	CliID           string   `json:"cliId"`
	Name            string   `json:"name"`
	Commits         []Commit `json:"commits"`
	UpstreamCommits []Commit `json:"upstreamCommits"`
	BranchStatus    string   `json:"branchStatus"`
	ReviewID        string   `json:"reviewId"` // e.g. "(#3)", empty without a pull request
	CI              *CI      `json:"ci"`
}

// PR returns the pull request label, e.g. "#3", or "" when the branch has none.
func (b Branch) PR() string {
	return strings.Trim(b.ReviewID, "()")
}

// CI is the state of the checks on a branch's pull request.
type CI struct {
	PendingCheckTitles []string `json:"pendingCheckTitles"`
	PassingCheckTitles []string `json:"passingCheckTitles"`
	FailingCheckTitles []string `json:"failingCheckTitles"`
	Status             string   `json:"status"`     // "queued", "inProgress" or "complete"
	Conclusion         string   `json:"conclusion"` // "success", "failure", ... or "unknown" without checks
}

type Stack struct {
	CliID           string   `json:"cliId"`
	AssignedChanges []Change `json:"assignedChanges"`
	Branches        []Branch `json:"branches"`
}

type UpstreamState struct {
	Behind       int    `json:"behind"`
	LatestCommit Commit `json:"latestCommit"`
}

type Status struct {
	UncommittedChanges []Change      `json:"uncommittedChanges"`
	Stacks             []Stack       `json:"stacks"`
	MergeBase          Commit        `json:"mergeBase"`
	UpstreamState      UpstreamState `json:"upstreamState"`

	Resolving *Resolution `json:"-"` // set in edit mode, when the rest is empty
}

// Client runs `but` commands in a repository directory.
type Client struct {
	Dir string
	Bin string
}

func New(dir string) *Client {
	return &Client{Dir: dir, Bin: "but"}
}

// Error is a failed `but` invocation. Msg is what `but` printed, suitable for showing to the user.
type Error struct {
	Args []string
	Msg  string
	Err  error
}

func (e *Error) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("but %s: %v", strings.Join(e.Args, " "), e.Err)
	}
	return e.Msg
}

func (e *Error) Unwrap() error { return e.Err }

func (c *Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, c.Bin, args...)
	cmd.Dir = c.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		msg = strings.TrimPrefix(msg, "Error: ")
		return nil, &Error{Args: args, Msg: msg, Err: err}
	}
	return stdout.Bytes(), nil
}

// decode runs a but command and decodes its JSON output into v.
func (c *Client) decode(ctx context.Context, v any, args ...string) error {
	out, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, v); err != nil {
		return fmt.Errorf("decode but %s: %w", args[0], err)
	}
	return nil
}

// Status returns the workspace state, including files changed per commit.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	out, err := c.run(ctx, "status", "--json", "-f")
	if err != nil {
		return nil, err
	}
	s, err := decodeStatus(out)
	if err != nil {
		return nil, fmt.Errorf("decode but status: %w", err)
	}
	return s, nil
}

type Hunk struct {
	OldStart int    `json:"oldStart"`
	OldLines int    `json:"oldLines"`
	NewStart int    `json:"newStart"`
	NewLines int    `json:"newLines"`
	Diff     string `json:"diff"` // unified diff, starting with the @@ header
}

type FileDiff struct {
	ID     string `json:"id"` // hunk id ("file:hunk") for uncommitted changes
	Path   string `json:"path"`
	Status string `json:"status"`
	Diff   struct {
		Type  string `json:"type"` // "patch" when Hunks is populated
		Hunks []Hunk `json:"hunks"`
	} `json:"diff"`
}

type Diff struct {
	Changes []FileDiff `json:"changes"`
}

// Diff returns the structured diff for a CLI id (file, commit or branch), or of
// all uncommitted changes when cliID is empty.
func (c *Client) Diff(ctx context.Context, cliID string) (*Diff, error) {
	args := []string{"diff", "--json"}
	if cliID != "" {
		args = append(args, cliID) // no id: every uncommitted change
	}
	var d Diff
	return &d, c.decode(ctx, &d, args...)
}

// Command builds a `but` invocation for running interactively (e.g. with $EDITOR).
func (c *Client) Command(args ...string) *exec.Cmd {
	cmd := exec.Command(c.Bin, args...)
	cmd.Dir = c.Dir
	return cmd
}
