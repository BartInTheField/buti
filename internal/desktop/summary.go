package desktop

import "github.com/bartinthefield/buti/internal/but"

// maxUncommittedPaths caps the paths returned with a summary. The count stays exact.
const maxUncommittedPaths = 20

// Summary is the workspace slice the desktop shell renders. It is derived from
// `but status` so the UI never parses GitButler JSON itself.
type Summary struct {
	Repo             string          `json:"repo"`
	Uncommitted      int             `json:"uncommitted"`
	UncommittedPaths []string        `json:"uncommittedPaths"`
	Stacks           []StackSummary  `json:"stacks"`
	UpstreamBehind   int             `json:"upstreamBehind"`
	Resolving        *ResolveSummary `json:"resolving,omitempty"`
}

// StackSummary is one applied stack, branches from base to tip.
type StackSummary struct {
	Branches []BranchSummary `json:"branches"`
}

// BranchSummary is one branch card's header: name, commit counts, and review state.
type BranchSummary struct {
	Name     string `json:"name"`
	Commits  int    `json:"commits"`
	Upstream int    `json:"upstream"`
	Status   string `json:"status,omitempty"`
	PR       string `json:"pr,omitempty"`
}

// ResolveSummary is set while `but` is in edit mode for a conflicted commit.
type ResolveSummary struct {
	Conflicted  int  `json:"conflicted"`
	Resolved    int  `json:"resolved"`
	AllResolved bool `json:"allResolved"`
}

func summarize(repo string, st *but.Status) Summary {
	out := Summary{
		Repo:             repo,
		UncommittedPaths: []string{},
		Stacks:           []StackSummary{},
	}
	if st == nil {
		return out
	}
	out.Uncommitted = len(st.UncommittedChanges)
	for i, ch := range st.UncommittedChanges {
		if i >= maxUncommittedPaths {
			break
		}
		out.UncommittedPaths = append(out.UncommittedPaths, ch.FilePath)
	}
	out.UpstreamBehind = st.UpstreamState.Behind
	for _, stack := range st.Stacks {
		ss := StackSummary{Branches: []BranchSummary{}}
		for _, br := range stack.Branches {
			ss.Branches = append(ss.Branches, BranchSummary{
				Name:     br.Name,
				Commits:  len(br.Commits),
				Upstream: len(br.UpstreamCommits),
				Status:   br.BranchStatus,
				PR:       br.PR(),
			})
		}
		out.Stacks = append(out.Stacks, ss)
	}
	if st.Resolving != nil {
		out.Resolving = &ResolveSummary{
			Conflicted:  len(st.Resolving.Conflicted),
			Resolved:    len(st.Resolving.Resolved),
			AllResolved: st.Resolving.AllResolved,
		}
	}
	return out
}
