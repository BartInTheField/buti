// Package lsp is a language server that shows buti's review comments as diagnostics, so an editor marks the lines
// a reviewer commented on, and resolves or dismisses them with code actions.
//
// It speaks JSON-RPC over stdio with hand-rolled framing. The comments are re-anchored on the current workspace with
// `but`, then placed on the working copy of each file, which is what the editor shows.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/bartinthefield/buti/internal/editor"
	"github.com/bartinthefield/buti/internal/review"
)

// Commands the server executes, offered as code actions on its diagnostics.
const (
	cmdResolve = "buti.resolve"
	cmdDismiss = "buti.dismiss"
)

const source = "buti"

// Workspace is what the server reads comments from: the repository's top directory, which comment paths are
// relative to, its comment store and the GitButler workspace.
type Workspace struct {
	Dir    string
	Store  *review.Store
	Source review.Source
}

// Opener opens the workspace containing root. The server calls it once it knows the root from initialize, and
// again after a failure, so an editor started before `but setup` recovers.
type Opener func(root string) (*Workspace, error)

// Config configures a Server. Zero intervals take the defaults.
type Config struct {
	Root string // fallback when initialize names no root
	Open Opener
	In   io.Reader
	Out  io.Writer

	PollInterval    time.Duration // how often review.json is checked for changes (2s)
	RefreshInterval time.Duration // how often the workspace is re-read anyway, since it changes outside the editor (15s)
	SaveDelay       time.Duration // how long a save waits for more saves before refreshing (300ms)
}

// Server is one language server session.
type Server struct {
	cfg Config
	in  *bufio.Reader

	outMu sync.Mutex
	pubMu sync.Mutex // one publish at a time, so an older text never overwrites the diagnostics of a newer one

	wsMu sync.Mutex
	ws   *Workspace

	kick chan struct{} // a refresh is wanted; buffered 1 so requests coalesce

	mu        sync.Mutex
	root      string
	docs      map[string]string // open documents' text, by path
	located   []review.Located  // the comments shown, from the last refresh
	published map[string]string // the diagnostics last published per path, as JSON; absent when none
	logged    map[string]bool   // errors already shown to the user
	saveTimer *time.Timer
	started   bool
	shutdown  bool
}

// New returns a server for cfg.
func New(cfg Config) *Server {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 2 * time.Second
	}
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 15 * time.Second
	}
	if cfg.SaveDelay <= 0 {
		cfg.SaveDelay = 300 * time.Millisecond
	}
	return &Server{
		cfg:       cfg,
		in:        bufio.NewReader(cfg.In),
		kick:      make(chan struct{}, 1),
		root:      cfg.Root,
		docs:      map[string]string{},
		published: map[string]string{},
		logged:    map[string]bool{},
	}
}

// errExitWithoutShutdown is returned by Run when the client exits without shutting down first.
var errExitWithoutShutdown = errors.New("exit without shutdown")

// Run serves until the client sends exit or closes the input. It returns an error for exit without shutdown, which
// the spec says should end the process with code 1.
func (s *Server) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() {
		s.mu.Lock()
		if s.saveTimer != nil {
			s.saveTimer.Stop()
		}
		s.mu.Unlock()
	}()
	for {
		body, err := readMessage(s.in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var m message
		if err := json.Unmarshal(body, &m); err != nil {
			s.reply(nil, nil, &rpcError{Code: -32700, Message: "parse error"})
			continue
		}
		if m.Method == "exit" {
			s.mu.Lock()
			down := s.shutdown
			s.mu.Unlock()
			if !down {
				return errExitWithoutShutdown
			}
			return nil
		}
		if m.Method == "" {
			continue // a response; the server sends no requests that need one
		}
		result, rerr := s.handle(ctx, &m)
		if m.isRequest() {
			s.reply(m.ID, result, rerr)
		}
		if m.Method == "workspace/executeCommand" && rerr == nil {
			s.trigger() // after the reply, so the client hears the command succeeded first
		}
	}
}

// handle runs one request or notification and returns the result of a request.
func (s *Server) handle(ctx context.Context, m *message) (any, *rpcError) {
	s.mu.Lock()
	down := s.shutdown
	s.mu.Unlock()
	if down && m.isRequest() {
		return nil, &rpcError{Code: codeInvalidRequest, Message: "server is shut down"}
	}
	switch m.Method {
	case "initialize":
		var p initializeParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		s.initialize(p)
		return map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync":       map[string]any{"openClose": true, "change": 1, "save": true},
				"codeActionProvider":     true,
				"executeCommandProvider": map[string]any{"commands": []string{cmdResolve, cmdDismiss}},
			},
			"serverInfo": map[string]any{"name": "buti"},
		}, nil
	case "initialized":
		s.mu.Lock()
		start := !s.started
		s.started = true
		s.mu.Unlock()
		if start {
			go s.loop(ctx)
			s.trigger()
		}
	case "shutdown":
		s.mu.Lock()
		s.shutdown = true
		s.mu.Unlock()
	case "textDocument/didOpen":
		var p didOpenParams
		if unmarshal(m.Params, &p) == nil {
			s.setDoc(p.TextDocument.URI, p.TextDocument.Text, true)
		}
	case "textDocument/didChange":
		var p didChangeParams
		// With full sync the last change holds the whole text.
		if unmarshal(m.Params, &p) == nil && len(p.ContentChanges) > 0 {
			s.setDoc(p.TextDocument.URI, p.ContentChanges[len(p.ContentChanges)-1].Text, true)
		}
	case "textDocument/didClose":
		var p didCloseParams
		if unmarshal(m.Params, &p) == nil {
			s.setDoc(p.TextDocument.URI, "", false)
		}
	case "textDocument/didSave":
		s.saved()
	case "textDocument/codeAction":
		var p codeActionParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		return codeActions(p), nil
	case "workspace/executeCommand":
		var p executeCommandParams
		if err := unmarshal(m.Params, &p); err != nil {
			return nil, err
		}
		return nil, s.execute(p)
	default:
		if m.isRequest() {
			return nil, &rpcError{Code: codeMethodNotFound, Message: "method not found: " + m.Method}
		}
	}
	return nil, nil
}

func unmarshal(params json.RawMessage, v any) *rpcError {
	if len(params) == 0 {
		return nil
	}
	if err := json.Unmarshal(params, v); err != nil {
		return &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}
	return nil
}

// initialize takes the root from the client: rootUri, then rootPath, then the first workspace folder.
func (s *Server) initialize(p initializeParams) {
	root := uriToPath(p.RootURI)
	if root == "" && p.RootPath != "" {
		root = filepath.Clean(p.RootPath)
	}
	if root == "" && len(p.WorkspaceFolders) > 0 {
		root = uriToPath(p.WorkspaceFolders[0].URI)
	}
	if root != "" {
		s.mu.Lock()
		s.root = root
		s.mu.Unlock()
	}
}

// workspace opens the workspace on first use, and again after it failed.
func (s *Server) workspace() (*Workspace, error) {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if s.ws != nil {
		return s.ws, nil
	}
	s.mu.Lock()
	root := s.root
	s.mu.Unlock()
	ws, err := s.cfg.Open(root)
	if err != nil {
		return nil, err
	}
	s.ws = ws
	return ws, nil
}

// trigger asks for a refresh; one already asked for and not started yet covers it.
func (s *Server) trigger() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// saved refreshes after a save, waiting for a burst of saves (save all, format on save) to end.
func (s *Server) saved() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveTimer == nil {
		s.saveTimer = time.AfterFunc(s.cfg.SaveDelay, s.trigger)
		return
	}
	s.saveTimer.Reset(s.cfg.SaveDelay)
}

// loop runs every refresh, one at a time.
func (s *Server) loop(ctx context.Context) {
	poll := time.NewTicker(s.cfg.PollInterval)
	defer poll.Stop()
	every := time.NewTicker(s.cfg.RefreshInterval)
	defer every.Stop()
	var last fileStamp
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.kick:
		case <-every.C:
		case <-poll.C:
			path := s.storePath()
			if path == "" || stampOf(path) == last {
				continue
			}
		}
		last = s.refresh(ctx)
	}
}

// fileStamp is enough of a file's stat to see that it changed.
type fileStamp struct {
	mod  time.Time
	size int64
	ok   bool
}

func stampOf(path string) fileStamp {
	if path == "" {
		return fileStamp{}
	}
	fi, err := os.Stat(path)
	if err != nil {
		return fileStamp{}
	}
	return fileStamp{mod: fi.ModTime(), size: fi.Size(), ok: true}
}

func (s *Server) storePath() string {
	s.wsMu.Lock()
	defer s.wsMu.Unlock()
	if s.ws == nil {
		return ""
	}
	return s.ws.Store.Path
}

// refresh re-anchors the open comments on the workspace and publishes them. A failure keeps what is shown. It
// returns the stamp of the comment file it read, taken before reading so a change made meanwhile is polled again.
func (s *Server) refresh(ctx context.Context) fileStamp {
	ws, err := s.workspace()
	if err != nil {
		s.logError(err)
		return fileStamp{}
	}
	stamp := stampOf(ws.Store.Path)
	ls, err := ws.Store.Reanchor(ctx, ws.Source, review.StatusOpen)
	if err != nil {
		if ctx.Err() == nil {
			s.logError(err)
		}
		return stamp
	}
	shown := slices.DeleteFunc(ls, func(l review.Located) bool {
		return l.Status != review.StatusOpen && l.Status != review.StatusOutdated
	})
	s.mu.Lock()
	s.located = shown
	s.mu.Unlock()
	s.publishAll(ws.Dir)
	return stamp
}

// publishAll publishes every file with comments, and clears the files that had some and have none now.
func (s *Server) publishAll(dir string) {
	s.pubMu.Lock()
	defer s.pubMu.Unlock()
	byPath := map[string][]review.Located{}
	s.mu.Lock()
	for _, l := range s.located {
		p := filepath.Join(dir, filepath.FromSlash(l.Anchor.Path))
		byPath[p] = append(byPath[p], l)
	}
	for p := range s.docs {
		if ls, ok := s.onCopy(p); ok && len(ls) > 0 {
			byPath[p] = ls
		}
	}
	var stale []string
	for p := range s.published {
		if _, ok := byPath[p]; !ok {
			stale = append(stale, p)
		}
	}
	s.mu.Unlock()
	for p, ls := range byPath {
		s.publish(p, ls)
	}
	for _, p := range stale {
		s.publish(p, nil)
	}
}

// publishFile republishes one file from the last refresh, after its text changed in the editor.
func (s *Server) publishFile(path string) {
	s.wsMu.Lock()
	ws := s.ws
	s.wsMu.Unlock()
	if ws == nil {
		return
	}
	s.pubMu.Lock()
	defer s.pubMu.Unlock()
	s.mu.Lock()
	ls := s.commentsOn(path, ws.Dir)
	_, had := s.published[path]
	s.mu.Unlock()
	if len(ls) > 0 || had {
		s.publish(path, ls)
	}
}

// commentsOn returns the comments to show on the file at path, in the repository at dir. The caller holds s.mu.
func (s *Server) commentsOn(path, dir string) []review.Located {
	if ls, ok := s.onCopy(path); ok {
		return ls
	}
	var ls []review.Located
	for _, l := range s.located {
		if filepath.Join(dir, filepath.FromSlash(l.Anchor.Path)) == path {
			ls = append(ls, l)
		}
	}
	return ls
}

// onCopy returns the comments to show on path when it is a copy that buti's diff in the editor (`Z`) wrote: those on
// the change and the side of the diff it was copied from. They are placed on its text like on the working copy. The
// caller holds s.mu.
func (s *Server) onCopy(path string) ([]review.Located, bool) {
	src, rel, side, ok := editor.FromCopy(path)
	if !ok {
		return nil, false
	}
	var out []review.Located
	for _, l := range s.located {
		a := l.Anchor
		if a.Path != rel || string(a.Side) != side {
			continue
		}
		commit := a.Kind == review.KindCommit
		switch {
		case src.Commit != "" && commit && a.CommitID == src.Commit,
			src.Branch != "" && commit && a.Branch == src.Branch,
			src.Commit == "" && src.Branch == "" && !commit:
			out = append(out, l)
		}
	}
	return out, true
}

// publish sends the diagnostics of the comments ls on the file at path, placed on its open text or its text on
// disk. A file that is gone gets none.
func (s *Server) publish(path string, ls []review.Located) {
	diags := []diagnostic{}
	if len(ls) > 0 {
		if lines, ok := s.lines(path); ok {
			for _, l := range ls {
				diags = append(diags, toDiagnostic(l, lines))
			}
		}
	}
	// The periodic refresh mostly finds nothing new: only a change is sent, so the editor doesn't redraw for nothing.
	data, _ := json.Marshal(diags)
	s.mu.Lock()
	last, had := s.published[path]
	if len(diags) > 0 {
		s.published[path] = string(data)
	} else {
		delete(s.published, path)
	}
	s.mu.Unlock()
	if len(diags) == 0 && !had || had && last == string(data) {
		return
	}
	s.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: pathToURI(path), Diagnostics: diags})
}

// lines returns the text of the file at path, from the editor when it is open, split into lines without EOLs.
func (s *Server) lines(path string) ([]string, bool) {
	s.mu.Lock()
	text, open := s.docs[path]
	s.mu.Unlock()
	if !open {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, false
		}
		text = string(data)
	}
	return splitLines(text), true
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	ls := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	for i := range ls {
		ls[i] = strings.TrimSuffix(ls[i], "\r")
	}
	return ls
}

// setDoc records the editor's text of a document, or forgets it when closed, and replaces its diagnostics.
func (s *Server) setDoc(uri, text string, open bool) {
	path := uriToPath(uri)
	if path == "" {
		return
	}
	s.mu.Lock()
	if open {
		s.docs[path] = text
	} else {
		delete(s.docs, path)
	}
	s.mu.Unlock()
	s.publishFile(path)
}

// toDiagnostic places a comment on whole lines of the working copy.
func toDiagnostic(l review.Located, lines []string) diagnostic {
	start, end, exact := review.WorktreeLines(l, lines)
	endChar := 0
	if end >= 1 && end <= len(lines) {
		endChar = utf16Len(lines[end-1])
	}
	c := l.Comment
	severity := severityInformation
	switch {
	case l.Status == review.StatusOutdated || !exact:
		severity = severityHint
	case strings.HasPrefix(c.Body, "[must-fix]"):
		severity = severityWarning
	}
	return diagnostic{
		Range: lspRange{
			Start: position{Line: start - 1},
			End:   position{Line: end - 1, Character: endChar},
		},
		Severity: severity,
		Code:     c.ID,
		Source:   source,
		Message:  diagnosticMessage(l),
	}
}

// diagnosticMessage is the comment's body with its replies, one per line.
func diagnosticMessage(l review.Located) string {
	c := l.Comment
	var b strings.Builder
	b.WriteString("💬 ") // tells a review comment apart from the compiler's and linters' diagnostics
	if l.Status == review.StatusOutdated {
		b.WriteString("(outdated) ")
	}
	if c.Author != "" && c.Author != review.AuthorUser {
		b.WriteString(c.Author + ": ")
	}
	b.WriteString(c.Body)
	for _, r := range c.Replies {
		fmt.Fprintf(&b, "\n↳ %s: %s", r.Author, r.Body)
	}
	return b.String()
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// codeActions offers to resolve or dismiss each buti diagnostic.
func codeActions(p codeActionParams) []codeAction {
	actions := []codeAction{}
	for _, d := range p.Context.Diagnostics {
		id, ok := d.Code.(string)
		if d.Source != source || !ok || id == "" {
			continue
		}
		actions = append(actions,
			codeAction{Title: "Resolve buti comment", Kind: "quickfix",
				Command: command{Title: "Resolve buti comment", Command: cmdResolve, Arguments: []any{id}}},
			codeAction{Title: "Dismiss buti comment", Kind: "quickfix",
				Command: command{Title: "Dismiss buti comment", Command: cmdDismiss, Arguments: []any{id}}},
		)
	}
	return actions
}

// execute resolves or dismisses a comment. Run refreshes afterwards, so its diagnostic goes away.
func (s *Server) execute(p executeCommandParams) *rpcError {
	if p.Command != cmdResolve && p.Command != cmdDismiss {
		return &rpcError{Code: codeInvalidParams, Message: "unknown command " + p.Command}
	}
	var id string
	if len(p.Arguments) > 0 {
		id, _ = p.Arguments[0].(string)
	}
	if id == "" {
		return &rpcError{Code: codeInvalidParams, Message: p.Command + " needs a comment id"}
	}
	ws, err := s.workspace()
	if err != nil {
		return &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	if p.Command == cmdResolve {
		_, err = ws.Store.Resolve(id, "resolved in editor")
	} else {
		_, err = ws.Store.Dismiss(id, "dismissed in editor")
	}
	if err != nil {
		return &rpcError{Code: codeInternalError, Message: err.Error()}
	}
	return nil
}

// logError shows an error in the editor's log, once per distinct message, so a failing `but` does not flood it.
func (s *Server) logError(err error) {
	msg := "buti: " + err.Error()
	s.mu.Lock()
	seen := s.logged[msg]
	s.logged[msg] = true
	s.mu.Unlock()
	if !seen {
		s.notify("window/logMessage", logMessageParams{Type: messageError, Message: msg})
	}
}

func (s *Server) notify(method string, params any) {
	s.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (s *Server) reply(id json.RawMessage, result any, rerr *rpcError) {
	if id == nil {
		id = json.RawMessage("null")
	}
	if rerr != nil {
		s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": rerr})
		return
	}
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

// send writes one message; the read loop and the refresh loop both write, so writes are serialized.
func (s *Server) send(v any) {
	s.outMu.Lock()
	defer s.outMu.Unlock()
	_ = writeMessage(s.cfg.Out, v)
}
