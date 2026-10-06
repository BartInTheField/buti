package desktop

import "github.com/bartinthefield/buti/internal/but"

// Workspace is the full status document the desktop UI browses and mutates.
// Unlike Summary it keeps cliIds so the shell can select, diff, and run ops.
type Workspace struct {
	Repo               string            `json:"repo"`
	UncommittedChanges []but.Change      `json:"uncommittedChanges"`
	Stacks             []but.Stack       `json:"stacks"`
	MergeBase          but.Commit        `json:"mergeBase"`
	UpstreamState      but.UpstreamState `json:"upstreamState"`
	Resolving          *but.Resolution   `json:"resolving,omitempty"`
}

func workspaceFrom(repo string, st *but.Status) Workspace {
	out := Workspace{
		Repo:               repo,
		UncommittedChanges: []but.Change{},
		Stacks:             []but.Stack{},
	}
	if st == nil {
		return out
	}
	if st.UncommittedChanges != nil {
		out.UncommittedChanges = st.UncommittedChanges
	}
	if st.Stacks != nil {
		out.Stacks = st.Stacks
	}
	out.MergeBase = st.MergeBase
	out.UpstreamState = st.UpstreamState
	out.Resolving = st.Resolving
	return out
}
