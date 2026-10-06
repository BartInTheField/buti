package desktop

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/bartinthefield/buti/internal/but"
)

// but is the client for the repository the window shows. POST /repo replaces it.
func (s *Server) but() *but.Client {
	return s.client.Load()
}

type repoReq struct {
	// Dir is an absolute path to the repository. It is kept as given, so the window's recent list
	// and the workspace's repo field name a repository the same way.
	Dir string `json:"dir"`
}

// repo reports the repository the API serves, so the window can name it before the workspace loads.
// chosen is false when `buti desktop` ran without -C and nothing was opened since: the window then
// reopens the last repository it showed instead of the working directory.
func (s *Server) repo(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dir": s.but().Dir, "chosen": s.chosen.Load()})
}

// openRepo points the API at another repository: the desktop app is not started from a folder
// the way the TUI is. The folder must be a GitButler workspace (`but status` works there); otherwise
// the API keeps the current one and replies with what `but` said.
func (s *Server) openRepo(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(ctx context.Context, req repoReq) (string, error) {
		if !filepath.IsAbs(req.Dir) {
			return "", badRequestError{msg: "dir must be an absolute path"}
		}
		dir := filepath.Clean(req.Dir)
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return "", badRequestError{msg: dir + " is not a folder"}
		}
		cur := s.but()
		next := &but.Client{Dir: dir, Bin: cur.Bin}
		if _, err := next.Status(ctx); err != nil {
			return "", err
		}
		s.client.Store(next)
		s.chosen.Store(true)
		return "", nil
	})
}
