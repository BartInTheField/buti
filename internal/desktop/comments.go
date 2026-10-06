package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

// Review comments live in internal/review's store, the same file the TUI and `buti review` use, so a comment
// left in any of them shows in the others. Nothing here runs a `but` write.

// LocatedComment is a stored comment plus where it is on the current workspace (review.Located).
type LocatedComment struct {
	review.Comment
	// State is the comment's status, or "outdated" / "orphaned" for an open comment whose line or commit is gone.
	State review.Status `json:"state"`
	// At is where the comment points now; Anchor is where it was stored.
	At     review.Anchor `json:"at"`
	Target string        `json:"target"`
	FileID string        `json:"fileId"`
}

func locatedJSON(ls []review.Located) []LocatedComment {
	out := make([]LocatedComment, 0, len(ls))
	for _, l := range ls {
		c := l.Comment
		if c.Replies == nil {
			c.Replies = []review.Reply{}
		}
		out = append(out, LocatedComment{Comment: c, State: l.Status, At: l.Anchor, Target: l.Target, FileID: l.FileID})
	}
	return out
}

// commentCache remembers the last located comments per repository, so the poll only locates again (a `but diff`
// per file) when the comments or the status changed, as the TUI's loadComments does.
type commentCache struct {
	mu      sync.Mutex
	store   *review.Store
	key     string
	located []LocatedComment
}

var (
	commentCachesMu sync.Mutex
	commentCaches   = map[string]*commentCache{}
)

func (s *Server) comments() (*commentCache, error) {
	commentCachesMu.Lock()
	defer commentCachesMu.Unlock()
	if c, ok := commentCaches[s.but().Dir]; ok {
		return c, nil
	}
	store, err := review.Open(s.but().Dir)
	if err != nil {
		return nil, err
	}
	c := &commentCache{store: store}
	commentCaches[s.but().Dir] = c
	return c, nil
}

func commentsFingerprint(cs []review.Comment, st *but.Status) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(cs)
	_ = json.NewEncoder(h).Encode(st)
	return hex.EncodeToString(h.Sum(nil))
}

// listComments serves every comment located on the current status. In edit mode the workspace is set aside, so
// there is nothing to locate on and the list is empty, as in the TUI.
func (s *Server) listComments(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	cc, err := s.comments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "review_failed", err.Error(), "")
		return
	}
	located, err := cc.locate(r.Context(), s.but())
	if err != nil {
		var be *but.Error
		if errors.As(err, &be) {
			writeButError(w, err)
			return
		}
		writeError(w, http.StatusInternalServerError, "review_failed", err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK       bool             `json:"ok"`
		Comments []LocatedComment `json:"comments"`
	}{OK: true, Comments: located})
}

func (cc *commentCache) locate(ctx context.Context, client *but.Client) ([]LocatedComment, error) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cs, err := cc.store.List()
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return []LocatedComment{}, nil
	}
	st, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	if st.Resolving != nil {
		return []LocatedComment{}, nil
	}
	key := commentsFingerprint(cs, st)
	if key == cc.key {
		return cc.located, nil
	}
	ls, err := review.Locate(ctx, st, client, cs)
	if err != nil {
		return nil, err
	}
	moved := false
	for _, l := range ls {
		moved = moved || l.Moved()
	}
	if moved {
		if err := cc.store.Save(ls); err != nil {
			return nil, err
		}
		// Saving changed the file; fingerprint it as saved so the next poll does not locate again.
		if cs, err := cc.store.List(); err == nil {
			key = commentsFingerprint(cs, st)
		}
	}
	cc.key, cc.located = key, locatedJSON(ls)
	return cc.located, nil
}

type addCommentReq struct {
	// Anchor is where the comment goes. An anchor with a branch and no kind is on a branch's whole diff: it is
	// moved onto the branch's commit that has the lines (review.OnBranch), as the TUI does.
	Anchor review.Anchor `json:"anchor"`
	Body   string        `json:"body"`
	Author string        `json:"author"`
}

type commentIDReq struct {
	ID string `json:"id"`
}

type commentBodyReq struct {
	ID   string `json:"id"`
	Body string `json:"body"`
}

type resolveCommentReq struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
}

func (s *Server) addComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(ctx context.Context, store *review.Store, req addCommentReq) (*review.Comment, error) {
		a := req.Anchor
		if a.Kind == "" && a.Branch != "" {
			st, err := s.but().Status(ctx)
			if err != nil {
				return nil, err
			}
			if a, err = review.OnBranch(ctx, st, s.but(), a.Branch, a); err != nil {
				return nil, err
			}
		}
		c, err := store.Add(a, req.Author, req.Body)
		return &c, err
	})
}

func (s *Server) editComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(_ context.Context, store *review.Store, req commentBodyReq) (*review.Comment, error) {
		c, err := store.Edit(req.ID, req.Body)
		return &c, err
	})
}

func (s *Server) replyComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(_ context.Context, store *review.Store, req commentBodyReq) (*review.Comment, error) {
		c, err := store.Reply(req.ID, review.AuthorUser, req.Body)
		return &c, err
	})
}

func (s *Server) resolveComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(_ context.Context, store *review.Store, req resolveCommentReq) (*review.Comment, error) {
		c, err := store.Resolve(req.ID, req.Summary)
		return &c, err
	})
}

func (s *Server) reopenComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(_ context.Context, store *review.Store, req commentIDReq) (*review.Comment, error) {
		c, err := store.Reopen(req.ID)
		return &c, err
	})
}

func (s *Server) deleteComment(w http.ResponseWriter, r *http.Request) {
	commentOp(s, w, r, func(_ context.Context, store *review.Store, req commentIDReq) (*review.Comment, error) {
		return nil, store.Delete(req.ID)
	})
}

// commentOp decodes T, runs fn on the repository's store and replies {"ok":true,"comment":...}. The store has
// its own file lock, so comment writes do not wait for a running `but` operation.
func commentOp[T any](s *Server, w http.ResponseWriter, r *http.Request, fn func(context.Context, *review.Store, T) (*review.Comment, error)) {
	if !s.requireAuth(w, r) {
		return
	}
	var req T
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body", "")
		return
	}
	cc, err := s.comments()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "review_failed", err.Error(), "")
		return
	}
	c, err := fn(r.Context(), cc.store, req)
	var be *but.Error
	switch {
	case err == nil:
	case errors.Is(err, review.ErrNotFound), errors.Is(err, review.ErrAmbiguous):
		writeError(w, http.StatusNotFound, "not_found", err.Error(), "")
		return
	case errors.As(err, &be):
		writeButError(w, err)
		return
	default:
		// Validation (an empty body, a bad anchor) or a branch with no commit holding the lines.
		writeError(w, http.StatusBadRequest, "bad_request", strings.TrimSpace(err.Error()), "")
		return
	}
	body := map[string]any{"ok": true}
	if c != nil {
		if c.Replies == nil {
			c.Replies = []review.Reply{}
		}
		body["comment"] = c
	}
	writeJSON(w, http.StatusOK, body)
}
