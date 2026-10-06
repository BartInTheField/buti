package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/bartinthefield/buti/internal/but"
)

// opMu serialises mutations the way the TUI's busy flag does: one but write at a time.
var opMu sync.Mutex

// Placement is the JSON shape for where a commit or branch lands.
type Placement struct {
	Branch    string `json:"branch,omitempty"`
	Above     string `json:"above,omitempty"`
	Below     string `json:"below,omitempty"`
	NewBranch bool   `json:"newBranch,omitempty"`
}

func (p Placement) toBut() but.Placement {
	return but.Placement{
		Branch:    p.Branch,
		Above:     p.Above,
		Below:     p.Below,
		NewBranch: p.NewBranch,
	}
}

type commitReq struct {
	Changes   []string  `json:"changes"`
	Message   string    `json:"message"`
	Placement Placement `json:"placement"`
}

type emptyCommitReq struct {
	Message   string    `json:"message"`
	Placement Placement `json:"placement"`
}

type amendReq struct {
	Target  string   `json:"target"`
	Changes []string `json:"changes"`
}

type sourcesReq struct {
	Sources []string `json:"sources"`
}

type moveReq struct {
	Sources   []string  `json:"sources"`
	Placement Placement `json:"placement"`
}

type squashReq struct {
	Sources []string `json:"sources"`
	Target  string   `json:"target"`
	// Mode is "combine" (default), "target" or "source": whose message the result keeps.
	Mode    string `json:"mode"`
	Message string `json:"message"`
}

type rewordReq struct {
	Target  string `json:"target"`
	Message string `json:"message"`
}

type targetsReq struct {
	Targets []string `json:"targets"`
}

type branchNewReq struct {
	Name      string    `json:"name"`
	Placement Placement `json:"placement"`
}

type branchesReq struct {
	Branches []string `json:"branches"`
}

type branchReq struct {
	Branch string `json:"branch"`
}

type pushReq struct {
	Branch string `json:"branch"`
	Force  bool   `json:"force"`
}

type prNewReq struct {
	Branch  string `json:"branch"`
	Message string `json:"message"`
	Draft   bool   `json:"draft"`
}

type snapshotReq struct {
	Snapshot string `json:"snapshot"`
}

type commitTargetReq struct {
	Commit string `json:"commit"`
}

type forceReq struct {
	Force bool `json:"force"`
}

type execReq struct {
	// Line is a command line as typed at the TUI's ':' prompt; a leading "but " is dropped.
	Line string   `json:"line"`
	Args []string `json:"args"`
}

type noBody struct{}

type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }

func required(ok bool, msg string) error {
	if !ok {
		return badRequestError{msg: msg}
	}
	return nil
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if !bearerOK(r, s.Token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token", "")
		return false
	}
	return true
}

func writeButError(w http.ResponseWriter, err error) {
	status, code, msg, docs := classifyBut(err)
	writeError(w, status, code, stripAgentNotice(msg), docs)
}

// agentNoticeLines start the lines of the notice `but` prints when it runs under a coding
// agent ("AGENT ACTION REQUIRED: ... run: but skill install"). It is addressed to the agent,
// not to whoever reads the toast.
var agentNoticeLines = []string{
	"Run once: but skill",
	"Then reload/use the updated skill",
	"If this warning repeats",
	"To work effectively with but",
	"Then read the installed SKILL.md",
	"This notice repeats",
}

// stripAgentNotice drops that notice from but's output, so every op reply and error is
// clean in one place rather than in each toast.
func stripAgentNotice(out string) string {
	if !strings.Contains(out, "AGENT ACTION REQUIRED") {
		return out
	}
	var kept []string
	inNotice := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if strings.Contains(t, "AGENT ACTION REQUIRED") {
			inNotice = true
			continue
		}
		if inNotice && slices.ContainsFunc(agentNoticeLines, func(p string) bool { return strings.HasPrefix(t, p) }) {
			continue
		}
		inNotice = false
		kept = append(kept, line)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// workspace serves the status document. ?sync=1 syncs pull requests from the forge
// first, like ctrl+r in the TUI.
func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	status := s.client.Status
	if r.URL.Query().Get("sync") == "1" {
		status = s.client.SyncedStatus
	}
	st, err := status(r.Context())
	if err != nil {
		writeButError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK        bool      `json:"ok"`
		Workspace Workspace `json:"workspace"`
	}{OK: true, Workspace: workspaceFrom(s.client.Dir, st)})
}

func (s *Server) diff(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	d, err := s.client.Diff(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		writeButError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK   bool      `json:"ok"`
		Diff *but.Diff `json:"diff"`
	}{OK: true, Diff: d})
}

func (s *Server) oplog(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	entries, err := s.client.Oplog(r.Context())
	if err != nil {
		writeButError(w, err)
		return
	}
	if entries == nil {
		entries = []but.OplogEntry{}
	}
	writeJSON(w, http.StatusOK, struct {
		OK      bool             `json:"ok"`
		Entries []but.OplogEntry `json:"entries"`
	}{OK: true, Entries: entries})
}

func (s *Server) branches(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	b, err := s.client.Branches(r.Context())
	if err != nil {
		writeButError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK       bool          `json:"ok"`
		Branches *but.Branches `json:"branches"`
	}{OK: true, Branches: b})
}

func (s *Server) reviewURL(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "branch is required", "")
		return
	}
	url, err := s.client.ReviewURL(r.Context(), branch)
	if err != nil {
		writeButError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "url": url})
}

func (s *Server) opCommit(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req commitReq) error {
		return s.client.Commit(ctx, req.Changes, req.Message, req.Placement.toBut())
	})
}

func (s *Server) opEmptyCommit(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req emptyCommitReq) error {
		return s.client.EmptyCommit(ctx, req.Message, req.Placement.toBut())
	})
}

func (s *Server) opAmend(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req amendReq) error {
		if err := required(req.Target != "", "target is required"); err != nil {
			return err
		}
		return s.client.Amend(ctx, req.Target, req.Changes)
	})
}

// opAbsorb absorbs each source in turn, or every uncommitted change when there are none.
// It replies with what `but` printed, which says where each change went.
func (s *Server) opAbsorb(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, req sourcesReq) (string, error) {
		if len(req.Sources) == 0 {
			return s.client.Exec(ctx, "absorb")
		}
		var outs []string
		for _, id := range req.Sources {
			out, err := s.client.Exec(ctx, "absorb", id)
			if err != nil {
				return strings.Join(outs, "\n"), err
			}
			if out != "" {
				outs = append(outs, out)
			}
		}
		return strings.Join(outs, "\n"), nil
	})
}

func (s *Server) opSquash(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req squashReq) error {
		if err := required(len(req.Sources) > 0, "sources are required"); err != nil {
			return err
		}
		var mode but.SquashMessage
		switch req.Mode {
		case "", "combine":
			mode = but.SquashCombineMessages
		case "target":
			mode = but.SquashUseTargetMessage
		case "source":
			mode = but.SquashUseSourceMessage
		default:
			return badRequestError{msg: `mode must be "combine", "target" or "source"`}
		}
		return s.client.Squash(ctx, req.Sources, req.Target, mode, req.Message)
	})
}

func (s *Server) opMove(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req moveReq) error {
		if err := required(len(req.Sources) > 0, "sources are required"); err != nil {
			return err
		}
		return s.client.Move(ctx, req.Sources, req.Placement.toBut())
	})
}

func (s *Server) opUncommit(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req sourcesReq) error {
		if err := required(len(req.Sources) > 0, "sources are required"); err != nil {
			return err
		}
		return s.client.Uncommit(ctx, req.Sources)
	})
}

func (s *Server) opReword(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req rewordReq) error {
		if err := required(req.Target != "", "target is required"); err != nil {
			return err
		}
		if err := required(strings.TrimSpace(req.Message) != "", "message is required"); err != nil {
			return err
		}
		return s.client.Reword(ctx, req.Target, req.Message)
	})
}

// opDiscard discards the targets; with none it discards every uncommitted change, as
// Discard on the TUI's Unstaged area does.
func (s *Server) opDiscard(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req targetsReq) error {
		return s.client.Discard(ctx, req.Targets)
	})
}

func (s *Server) opBranchNew(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req branchNewReq) error {
		name := strings.Join(strings.Fields(req.Name), "-")
		return s.client.BranchNew(ctx, name, req.Placement.toBut())
	})
}

func (s *Server) opBranchDelete(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req branchesReq) error {
		if err := required(len(req.Branches) > 0, "branches are required"); err != nil {
			return err
		}
		return s.client.BranchDelete(ctx, req.Branches)
	})
}

func (s *Server) opPick(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req moveReq) error {
		if err := required(len(req.Sources) > 0, "sources are required"); err != nil {
			return err
		}
		return s.client.Pick(ctx, req.Sources, req.Placement.toBut())
	})
}

func (s *Server) opApply(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req branchReq) error {
		if err := required(req.Branch != "", "branch is required"); err != nil {
			return err
		}
		return s.client.Apply(ctx, req.Branch)
	})
}

func (s *Server) opUnapply(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req branchReq) error {
		if err := required(req.Branch != "", "branch is required"); err != nil {
			return err
		}
		return s.client.Unapply(ctx, req.Branch)
	})
}

func (s *Server) opPush(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req pushReq) error {
		if err := required(req.Branch != "", "branch is required"); err != nil {
			return err
		}
		return s.syncPRs(ctx, s.client.Push(ctx, req.Branch, req.Force))
	})
}

func (s *Server) opPull(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, _ noBody) error { return s.client.Pull(ctx) })
}

func (s *Server) opUndo(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, _ noBody) (string, error) {
		return s.client.Exec(ctx, "undo")
	})
}

func (s *Server) opRedo(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, _ noBody) (string, error) {
		return s.client.Exec(ctx, "redo")
	})
}

func (s *Server) opPRNew(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req prNewReq) error {
		if err := required(req.Branch != "", "branch is required"); err != nil {
			return err
		}
		return s.syncPRs(ctx, s.client.PRNew(ctx, req.Branch, req.Message, req.Draft))
	})
}

func (s *Server) opOplogRestore(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req snapshotReq) error {
		if err := required(req.Snapshot != "", "snapshot is required"); err != nil {
			return err
		}
		return s.client.OplogRestore(ctx, req.Snapshot)
	})
}

// opLand merges a branch straight into the target and pushes it, as the TUI's "Land" does.
func (s *Server) opLand(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, req branchReq) (string, error) {
		if err := required(req.Branch != "", "branch is required"); err != nil {
			return "", err
		}
		return s.client.Exec(ctx, "land", req.Branch, "--yes")
	})
}

func (s *Server) opClean(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, _ noBody) (string, error) {
		return s.client.Exec(ctx, "clean")
	})
}

func (s *Server) opResolveStart(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req commitTargetReq) error {
		if err := required(req.Commit != "", "commit is required"); err != nil {
			return err
		}
		return s.client.ResolveStart(ctx, req.Commit)
	})
}

func (s *Server) opResolveFinish(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, _ noBody) error { return s.client.ResolveFinish(ctx) })
}

func (s *Server) opResolveCancel(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(ctx context.Context, req forceReq) error {
		return s.client.ResolveCancel(ctx, req.Force)
	})
}

// exec runs an arbitrary but command, like the TUI's ':' prompt, and returns its output.
func (s *Server) exec(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, req execReq) (string, error) {
		args := req.Args
		if len(args) == 0 {
			var err error
			args, err = but.SplitArgs(strings.TrimPrefix(strings.TrimSpace(req.Line), "but "))
			if err != nil {
				return "", badRequestError{msg: err.Error()}
			}
		}
		if err := required(len(args) > 0, "a command is required"); err != nil {
			return "", err
		}
		return s.client.Exec(ctx, args...)
	})
}

// syncPRs syncs pull requests from the forge after an operation that can change them, so
// the next workspace read shows them. A failed sync is not reported: the operation worked.
func (s *Server) syncPRs(ctx context.Context, err error) error {
	if err == nil {
		_, _ = s.client.SyncedStatus(ctx)
	}
	return err
}

func decodeAndRun[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(context.Context, T) error) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, req T) (string, error) { return "", fn(ctx, req) })
}

// decodeAndRunOut decodes the JSON body into T, runs fn under opMu and replies
// {"ok":true} plus "output" when fn printed something. An empty body decodes as T's zero value.
func decodeAndRunOut[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(context.Context, T) (string, error)) {
	if !s.requireAuth(w, r) {
		return
	}
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	opMu.Lock()
	defer opMu.Unlock()
	out, err := fn(r.Context(), req)
	if err != nil {
		if _, ok := err.(badRequestError); ok {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error(), "")
			return
		}
		writeButError(w, err)
		return
	}
	body := map[string]any{"ok": true}
	if out = stripAgentNotice(out); out != "" {
		body["output"] = out
	}
	writeJSON(w, http.StatusOK, body)
}
