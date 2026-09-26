package but

import (
	"context"
	"encoding/json"
)

// Resolution is the state of edit mode, entered with ResolveStart: the
// conflicted commit is checked out and its files hold conflict markers.
type Resolution struct {
	Conflicted  []string `json:"conflicted_files"`
	Resolved    []string `json:"resolved_files"`
	AllResolved bool     `json:"all_resolved"`
}

// decodeStatus decodes `but status --json`, which prints the edit mode state
// instead of the workspace while a commit is being resolved.
func decodeStatus(out []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, err
	}
	var r Resolution
	if err := json.Unmarshal(out, &r); err == nil && (r.Conflicted != nil || r.Resolved != nil) {
		s.Resolving = &r
	}
	return &s, nil
}

// ResolveStart enters edit mode for a conflicted commit, checking it out with
// conflict markers in its files.
func (c *Client) ResolveStart(ctx context.Context, commit string) error {
	return c.mutate(ctx, "resolve", commit)
}

// ResolveFinish commits the resolved files and returns to the workspace.
func (c *Client) ResolveFinish(ctx context.Context) error {
	return c.mutate(ctx, "resolve", "finish")
}

// ResolveCancel leaves edit mode without changing the commit. `but`
// refuses while files were edited, unless force drops the edits.
func (c *Client) ResolveCancel(ctx context.Context, force bool) error {
	if force {
		return c.mutate(ctx, "resolve", "cancel", "--force")
	}
	return c.mutate(ctx, "resolve", "cancel")
}
