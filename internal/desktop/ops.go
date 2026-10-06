package desktop

import (
	"encoding/json"
	"net/http"
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

type amendReq struct {
	Target  string   `json:"target"`
	Changes []string `json:"changes"`
}

type moveReq struct {
	Sources   []string  `json:"sources"`
	Placement Placement `json:"placement"`
}

type uncommitReq struct {
	Sources []string `json:"sources"`
}

type badRequestError struct{ msg string }

func (e badRequestError) Error() string { return e.msg }

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	if !bearerOK(r, s.Token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token", "")
		return false
	}
	return true
}

func (s *Server) workspace(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	st, err := s.client.Status(r.Context())
	if err != nil {
		status, code, msg, docs := classifyBut(err)
		writeError(w, status, code, msg, docs)
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
	id := r.URL.Query().Get("id")
	d, err := s.client.Diff(r.Context(), id)
	if err != nil {
		status, code, msg, docs := classifyBut(err)
		writeError(w, status, code, msg, docs)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK   bool      `json:"ok"`
		Diff *but.Diff `json:"diff"`
	}{OK: true, Diff: d})
}

func (s *Server) opCommit(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(req commitReq) error {
		return s.client.Commit(r.Context(), req.Changes, req.Message, req.Placement.toBut())
	})
}

func (s *Server) opAmend(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(req amendReq) error {
		if req.Target == "" {
			return badRequestError{msg: "target is required"}
		}
		return s.client.Amend(r.Context(), req.Target, req.Changes)
	})
}

func (s *Server) opMove(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(req moveReq) error {
		if len(req.Sources) == 0 {
			return badRequestError{msg: "sources are required"}
		}
		return s.client.Move(r.Context(), req.Sources, req.Placement.toBut())
	})
}

func (s *Server) opUncommit(w http.ResponseWriter, r *http.Request) {
	decodeAndRun(s, w, r, func(req uncommitReq) error {
		if len(req.Sources) == 0 {
			return badRequestError{msg: "sources are required"}
		}
		return s.client.Uncommit(r.Context(), req.Sources)
	})
}

func decodeAndRun[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(T) error) {
	if !s.requireAuth(w, r) {
		return
	}
	var req T
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	opMu.Lock()
	defer opMu.Unlock()
	if err := fn(req); err != nil {
		if _, ok := err.(badRequestError); ok {
			writeError(w, http.StatusBadRequest, "bad_request", err.Error(), "")
			return
		}
		status, code, msg, docs := classifyBut(err)
		writeError(w, status, code, msg, docs)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
