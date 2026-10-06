// Package desktop is the localhost HTTP API the Tauri shell calls.
// The TUI and the desktop app share internal/but; this package only serves it.
package desktop

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bartinthefield/buti/internal/but"
)

// gitButlerCLIDocs is where the shell sends someone who does not have `but` installed.
const gitButlerCLIDocs = "https://docs.gitbutler.com/cli-guides/installation"

// Server is an in-process API bound to 127.0.0.1. Token is the bearer secret
// for this process; it is handed to the webview out of band, never in a URL.
type Server struct {
	URL   string
	Token string

	// client is swapped by POST /repo; read it with but().
	client atomic.Pointer[but.Client]
	http   *http.Server
	host   string // host:port the listener accepted, required on the Host header
	// chosen is whether the repository was asked for: -C, or POST /repo. Not just the working directory.
	chosen atomic.Bool
}

// Start listens on 127.0.0.1 and port (0 picks a free port) and serves until Shutdown.
func Start(client *but.Client, port int) (*Server, error) {
	if client == nil {
		return nil, errors.New("desktop: nil but client")
	}
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("desktop: port %d out of range", port)
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, fmt.Errorf("desktop: listen: %w", err)
	}
	host := ln.Addr().String()
	s := &Server{
		URL:   "http://" + host,
		Token: token,
		host:  host,
	}
	s.client.Store(client)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /status", s.status)
	mux.HandleFunc("GET /workspace", s.workspace)
	mux.HandleFunc("GET /repo", s.repo)
	mux.HandleFunc("POST /repo", s.openRepo)
	mux.HandleFunc("GET /diff", s.diff)
	mux.HandleFunc("POST /ops/commit", s.opCommit)
	mux.HandleFunc("POST /ops/amend", s.opAmend)
	mux.HandleFunc("POST /ops/move", s.opMove)
	mux.HandleFunc("GET /oplog", s.oplog)
	mux.HandleFunc("GET /branches", s.branches)
	mux.HandleFunc("GET /review-url", s.reviewURL)
	mux.HandleFunc("POST /ops/uncommit", s.opUncommit)
	mux.HandleFunc("POST /ops/empty-commit", s.opEmptyCommit)
	mux.HandleFunc("POST /ops/absorb", s.opAbsorb)
	mux.HandleFunc("POST /ops/squash", s.opSquash)
	mux.HandleFunc("POST /ops/reword", s.opReword)
	mux.HandleFunc("POST /ops/discard", s.opDiscard)
	mux.HandleFunc("POST /ops/branch-new", s.opBranchNew)
	mux.HandleFunc("POST /ops/branch-delete", s.opBranchDelete)
	mux.HandleFunc("POST /ops/pick", s.opPick)
	mux.HandleFunc("POST /ops/apply", s.opApply)
	mux.HandleFunc("POST /ops/unapply", s.opUnapply)
	mux.HandleFunc("POST /ops/push", s.opPush)
	mux.HandleFunc("POST /ops/pull", s.opPull)
	mux.HandleFunc("POST /ops/undo", s.opUndo)
	mux.HandleFunc("POST /ops/redo", s.opRedo)
	mux.HandleFunc("POST /ops/pr-new", s.opPRNew)
	mux.HandleFunc("POST /ops/oplog-restore", s.opOplogRestore)
	mux.HandleFunc("POST /ops/land", s.opLand)
	mux.HandleFunc("POST /ops/clean", s.opClean)
	mux.HandleFunc("POST /ops/resolve-start", s.opResolveStart)
	mux.HandleFunc("POST /ops/resolve-finish", s.opResolveFinish)
	mux.HandleFunc("POST /ops/resolve-cancel", s.opResolveCancel)
	mux.HandleFunc("POST /exec", s.exec)
	mux.HandleFunc("POST /open", s.open)
	mux.HandleFunc("GET /file", s.file)
	mux.HandleFunc("POST /highlight", s.highlight)
	mux.HandleFunc("GET /comments", s.listComments)
	mux.HandleFunc("POST /comments", s.addComment)
	mux.HandleFunc("POST /comments/edit", s.editComment)
	mux.HandleFunc("POST /comments/reply", s.replyComment)
	mux.HandleFunc("POST /comments/resolve", s.resolveComment)
	mux.HandleFunc("POST /comments/reopen", s.reopenComment)
	mux.HandleFunc("POST /comments/delete", s.deleteComment)
	s.http = &http.Server{
		Handler:           s.wrap(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := s.http.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			// The process is exiting or Shutdown was called; nothing else can surface this.
			_ = err
		}
	}()
	if err := waitReady(host); err != nil {
		_ = s.Shutdown(context.Background())
		return nil, err
	}
	return s, nil
}

// Shutdown stops the listener. In-flight `but` calls are cancelled with the request context.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.http == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return s.http.Shutdown(ctx)
}

func waitReady(host string) error {
	deadline := time.Now().Add(2 * time.Second)
	var last error
	for {
		conn, err := net.DialTimeout("tcp", host, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		last = err
		if time.Now().After(deadline) {
			return fmt.Errorf("desktop: api did not accept connections: %w", last)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("desktop: token: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

func tokenOK(got, want string) bool {
	if len(got) != len(want) || want == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// wrap enforces loopback and the Host header before any handler runs.
// Binding to 127.0.0.1 is not enough on its own: a browser can be aimed at
// the port via DNS rebinding, and other local users can connect.
func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !fromLoopback(r) {
			writeError(w, http.StatusForbidden, "forbidden", "loopback only", "")
			return
		}
		if !s.hostOK(r) {
			writeError(w, http.StatusForbidden, "forbidden", "invalid host", "")
			return
		}
		origin := r.Header.Get("Origin")
		if origin != "" && allowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) hostOK(r *http.Request) bool {
	// r.Host is host:port. Accept the bound address and the localhost alias of it.
	if r.Host == s.host {
		return true
	}
	_, port, err := net.SplitHostPort(s.host)
	if err != nil {
		return false
	}
	return r.Host == "localhost:"+port
}

// allowedOrigin is the Tauri webview plus the Vite dev server. Anything else
// must not be able to read responses, even from a page the user has open.
func allowedOrigin(origin string) bool {
	switch origin {
	case "http://tauri.localhost", "https://tauri.localhost", "tauri://localhost":
		return true
	}
	u := strings.TrimPrefix(origin, "http://")
	if len(u) == len(origin) {
		return false
	}
	host, _, err := net.SplitHostPort(u)
	if err != nil {
		return false
	}
	return host == "localhost" || host == "127.0.0.1"
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	if !bearerOK(r, s.Token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid token", "")
		return
	}
	st, err := s.but().Status(r.Context())
	if err != nil {
		status, code, msg, docs := classifyBut(err)
		writeError(w, status, code, msg, docs)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK      bool    `json:"ok"`
		Summary Summary `json:"summary"`
	}{OK: true, Summary: summarize(s.but().Dir, st)})
}

func bearerOK(r *http.Request, token string) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return false
	}
	return tokenOK(strings.TrimSpace(h[len(prefix):]), token)
}

// classifyBut turns a failed `but` invocation into an API error the shell can show.
// A missing binary is distinct from a repository error so the window can explain install.
// LookPath reports exec.ErrNotFound; a Bin set to a path that is not there is os.ErrNotExist.
func classifyBut(err error) (status int, code, msg, docs string) {
	if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
		return http.StatusServiceUnavailable, "but_missing",
			"The GitButler CLI (`but`) is not on PATH. Install GitButler, then run `but setup` in this repository.",
			gitButlerCLIDocs
	}
	var be *but.Error
	if errors.As(err, &be) && be.Msg != "" {
		return http.StatusBadGateway, "but_failed", be.Msg, ""
	}
	return http.StatusBadGateway, "but_failed", err.Error(), ""
}

type apiError struct {
	OK    bool `json:"ok"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		DocsURL string `json:"docsUrl,omitempty"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg, docs string) {
	body := apiError{OK: false}
	body.Error.Code = code
	body.Error.Message = msg
	body.Error.DocsURL = docs
	writeJSON(w, status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
