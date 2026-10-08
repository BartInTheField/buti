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
	"sync/atomic"
	"testing"
	"time"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/editor"
	"github.com/bartinthefield/buti/internal/review"
)

// fakeBut serves a status with a.go and b.go added in the unassigned changes, and counts status calls.
type fakeBut struct {
	statuses atomic.Int32
}

func (f *fakeBut) Status(context.Context) (*but.Status, error) {
	f.statuses.Add(1)
	return &but.Status{UncommittedChanges: []but.Change{{CliID: "aa", FilePath: "a.go"}, {CliID: "bb", FilePath: "b.go"}}}, nil
}

func (f *fakeBut) Diff(_ context.Context, id string) (*but.Diff, error) {
	files := map[string]string{"aa": "a.go", "bb": "b.go"}
	path, ok := files[id]
	if !ok {
		return nil, fmt.Errorf("no diff for %s", id)
	}
	fd := but.FileDiff{Path: path}
	fd.Diff.Type = "patch"
	fd.Diff.Hunks = []but.Hunk{{Diff: added(fileText[path])}}
	return &but.Diff{Changes: []but.FileDiff{fd}}, nil
}

var fileText = map[string]string{
	"a.go": "package a\n\nfunc A() {\n\treturn\n}\n",
	"b.go": "package b\n",
}

// added is the diff of a new file with text.
func added(text string) string {
	lines := splitLines(text)
	out := fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines))
	for _, l := range lines {
		out += "+" + l + "\n"
	}
	return out
}

// client drives a server over pipes, sorting what it sends into responses and notifications.
type client struct {
	t       *testing.T
	w       io.Writer
	id      int
	resps   chan message
	notes   chan message
	pending []message
	done    chan error
}

func startServer(t *testing.T, open Opener, root string) *client {
	t.Helper()
	serverIn, clientOut := io.Pipe()
	clientIn, serverOut := io.Pipe()
	s := New(Config{
		Root: root, Open: open, In: serverIn, Out: serverOut,
		PollInterval: 10 * time.Millisecond, RefreshInterval: time.Hour, SaveDelay: 30 * time.Millisecond,
	})
	c := &client{t: t, w: clientOut, resps: make(chan message, 100), notes: make(chan message, 100), done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		c.done <- s.Run(ctx)
		_ = serverOut.Close()
	}()
	go func() {
		r := bufio.NewReader(clientIn)
		for {
			body, err := readMessage(r)
			if err != nil {
				return
			}
			var m message
			if err := json.Unmarshal(body, &m); err != nil {
				t.Errorf("bad message %s: %v", body, err)
				return
			}
			if m.Method != "" {
				c.notes <- m
			} else {
				c.resps <- m
			}
		}
	}()
	t.Cleanup(func() {
		cancel()
		_ = clientOut.Close()
	})
	return c
}

func (c *client) notify(method string, params any) {
	c.t.Helper()
	if err := writeMessage(c.w, map[string]any{"jsonrpc": "2.0", "method": method, "params": params}); err != nil {
		c.t.Fatal(err)
	}
}

// request sends a request and returns its response.
func (c *client) request(method string, params any) message {
	c.t.Helper()
	c.id++
	if err := writeMessage(c.w, map[string]any{"jsonrpc": "2.0", "id": c.id, "method": method, "params": params}); err != nil {
		c.t.Fatal(err)
	}
	select {
	case m := <-c.resps:
		if string(m.ID) != fmt.Sprint(c.id) {
			c.t.Fatalf("response %s to request %d", m.ID, c.id)
		}
		return m
	case <-time.After(5 * time.Second):
		c.t.Fatalf("no response to %s", method)
	}
	return message{}
}

// note waits for a notification of method that match accepts, keeping the others for later waits.
func (c *client) note(method string, match func(json.RawMessage) bool) json.RawMessage {
	c.t.Helper()
	ok := func(m message) bool { return m.Method == method && (match == nil || match(m.Params)) }
	for i, m := range c.pending {
		if ok(m) {
			c.pending = append(c.pending[:i], c.pending[i+1:]...)
			return m.Params
		}
	}
	timeout := time.After(5 * time.Second)
	for {
		select {
		case m := <-c.notes:
			if ok(m) {
				return m.Params
			}
			c.pending = append(c.pending, m)
		case <-timeout:
			c.t.Fatalf("no matching %s; got %d others", method, len(c.pending))
		}
	}
}

// diagnostics waits for a publish for path whose diagnostics want accepts (any without want).
func (c *client) diagnostics(path string, want func([]diagnostic) bool) []diagnostic {
	c.t.Helper()
	decode := func(raw json.RawMessage) (publishDiagnosticsParams, bool) {
		var p publishDiagnosticsParams
		return p, json.Unmarshal(raw, &p) == nil && p.URI == pathToURI(path)
	}
	raw := c.note("textDocument/publishDiagnostics", func(raw json.RawMessage) bool {
		p, ok := decode(raw)
		return ok && (want == nil || want(p.Diagnostics))
	})
	p, _ := decode(raw)
	return p.Diagnostics
}

func TestSession(t *testing.T) {
	root := t.TempDir()
	for name, text := range fileText {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	store := review.New(filepath.Join(t.TempDir(), "review.json"))
	mustFix, err := store.Add(review.Anchor{Kind: review.KindUnassigned, Path: "a.go", Line: 3, EndLine: 4,
		LineText: "func A() {\n\treturn"}, "agent", "[must-fix] handle errors")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Reply(mustFix.ID, "", "ok"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Add(review.Anchor{Kind: review.KindUnassigned, Path: "b.go", Line: 1, LineText: "package b"},
		"", "nice"); err != nil {
		t.Fatal(err)
	}
	src := &fakeBut{}
	var openedAt string
	open := func(dir string) (*Workspace, error) {
		openedAt = dir
		return &Workspace{Dir: dir, Store: store, Source: src}, nil
	}
	c := startServer(t, open, "/fallback")
	aGo, bGo := filepath.Join(root, "a.go"), filepath.Join(root, "b.go")

	init := c.request("initialize", map[string]any{"processId": nil, "rootUri": pathToURI(root), "capabilities": map[string]any{}})
	var initResult struct {
		Capabilities struct {
			CodeActionProvider     bool `json:"codeActionProvider"`
			ExecuteCommandProvider struct {
				Commands []string `json:"commands"`
			} `json:"executeCommandProvider"`
			TextDocumentSync struct {
				Change int `json:"change"`
			} `json:"textDocumentSync"`
		} `json:"capabilities"`
		ServerInfo struct {
			Name string `json:"name"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(init.Result, &initResult); err != nil || !initResult.Capabilities.CodeActionProvider ||
		initResult.Capabilities.TextDocumentSync.Change != 1 || initResult.ServerInfo.Name != "buti" ||
		len(initResult.Capabilities.ExecuteCommandProvider.Commands) != 2 {
		t.Fatalf("initialize: %s (%v)", init.Result, err)
	}
	c.notify("initialized", map[string]any{})

	got := c.diagnostics(aGo, nil)
	if openedAt != root {
		t.Fatalf("opened %q, want the root from initialize %q", openedAt, root)
	}
	want := diagnostic{
		Range:    lspRange{Start: position{Line: 2}, End: position{Line: 3, Character: 7}},
		Severity: severityWarning, Code: mustFix.ID, Source: "buti",
		Message: "💬 agent: [must-fix] handle errors\n↳ user: ok",
	}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("a.go: got %+v, want %+v", got, want)
	}
	if got := c.diagnostics(bGo, nil); len(got) != 1 || got[0].Severity != severityInformation || got[0].Message != "💬 nice" ||
		got[0].Range.End != (position{Line: 0, Character: 9}) {
		t.Fatalf("b.go: %+v", got)
	}

	// Editing moves the diagnostic with the text, without asking `but`.
	before := src.statuses.Load()
	c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
		"uri": pathToURI(aGo), "languageId": "go", "version": 1, "text": fileText["a.go"]}})
	c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": pathToURI(aGo), "version": 2},
		"contentChanges": []any{map[string]any{"text": "// héader 😀\n" + fileText["a.go"]}},
	})
	if got := c.diagnostics(aGo, func(d []diagnostic) bool { return len(d) == 1 && d[0].Range.Start.Line != 2 }); len(got) != 1 || got[0].Range.Start.Line != 3 || got[0].Range.End.Line != 4 {
		t.Fatalf("after change: %+v", got)
	}
	if n := src.statuses.Load(); n != before {
		t.Fatalf("didChange ran but status %d times", n-before)
	}

	// A burst of saves refreshes once.
	for range 3 {
		c.notify("textDocument/didSave", map[string]any{"textDocument": map[string]any{"uri": pathToURI(aGo)}})
	}
	// The refresh finds nothing new, so nothing is published: wait for it to ask `but` instead.
	for deadline := time.Now().Add(5 * time.Second); src.statuses.Load() == before && time.Now().Before(deadline); {
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(100 * time.Millisecond)
	if n := src.statuses.Load(); n != before+1 {
		t.Fatalf("three saves ran but status %d times", n-before)
	}

	// Code actions on a buti diagnostic, none on another server's.
	actions := c.request("textDocument/codeAction", map[string]any{
		"textDocument": map[string]any{"uri": pathToURI(aGo)},
		"range":        want.Range,
		"context": map[string]any{"diagnostics": []any{
			map[string]any{"range": want.Range, "source": "buti", "code": mustFix.ID, "message": "x"},
			map[string]any{"range": want.Range, "source": "gopls", "code": 12, "message": "y"},
		}},
	})
	var acts []codeAction
	if err := json.Unmarshal(actions.Result, &acts); err != nil {
		t.Fatal(err)
	}
	if len(acts) != 2 || acts[0].Command.Command != cmdResolve || acts[1].Command.Command != cmdDismiss ||
		acts[0].Kind != "quickfix" || acts[0].Title != "Resolve buti comment" || acts[1].Title != "Dismiss buti comment" ||
		acts[0].Command.Arguments[0] != mustFix.ID {
		t.Fatalf("code actions: %s", actions.Result)
	}

	// Resolving clears the file's diagnostics.
	res := c.request("workspace/executeCommand", map[string]any{"command": cmdResolve, "arguments": []any{mustFix.ID}})
	if res.Error != nil || string(res.Result) != "null" {
		t.Fatalf("executeCommand: %+v %s", res.Error, res.Result)
	}
	if got := c.diagnostics(aGo, func(d []diagnostic) bool { return len(d) == 0 }); len(got) != 0 {
		t.Fatalf("after resolve: %+v", got)
	}
	if cm, err := store.Get(mustFix.ID); err != nil || cm.Status != review.StatusResolved || cm.Resolution.Summary != "resolved in editor" {
		t.Fatalf("stored: %+v, %v", cm, err)
	}

	// A comment written elsewhere shows up without an editor event.
	if _, err := store.Add(review.Anchor{Kind: review.KindUnassigned, Path: "a.go", Line: 1, LineText: "package a"},
		"", "from the TUI"); err != nil {
		t.Fatal(err)
	}
	if got := c.diagnostics(aGo, func(d []diagnostic) bool { return len(d) > 0 }); len(got) != 1 || got[0].Message != "💬 from the TUI" || got[0].Range.Start.Line != 1 {
		t.Fatalf("after an outside change: %+v", got)
	}

	if m := c.request("textDocument/hover", map[string]any{}); m.Error == nil || m.Error.Code != codeMethodNotFound {
		t.Fatalf("unknown request: %+v", m)
	}
	c.notify("$/unknown", nil)
	if m := c.request("shutdown", nil); m.Error != nil || string(m.Result) != "null" {
		t.Fatalf("shutdown: %+v", m)
	}
	c.notify("exit", nil)
	select {
	case err := <-c.done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not exit")
	}
}

func TestOpenErrorIsLoggedOnce(t *testing.T) {
	var calls atomic.Int32
	open := func(string) (*Workspace, error) {
		calls.Add(1)
		return nil, errors.New("not a GitButler repository")
	}
	c := startServer(t, open, t.TempDir())
	c.request("initialize", map[string]any{"rootUri": nil})
	c.notify("initialized", map[string]any{})
	var p logMessageParams
	if err := json.Unmarshal(c.note("window/logMessage", nil), &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != messageError || p.Message != "buti: not a GitButler repository" {
		t.Fatalf("log: %+v", p)
	}
	// Saving retries; the same error is not shown again, and the server keeps answering.
	c.notify("textDocument/didSave", map[string]any{"textDocument": map[string]any{"uri": "file:///x"}})
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatal("the save did not retry opening")
	}
	if m := c.request("shutdown", nil); m.Error != nil {
		t.Fatalf("shutdown: %+v", m)
	}
	select {
	case m := <-c.notes:
		t.Fatalf("unexpected %s %s", m.Method, m.Params)
	default:
	}
	c.notify("exit", nil)
	if err := <-c.done; err != nil {
		t.Fatal(err)
	}
}

func TestExitWithoutShutdown(t *testing.T) {
	c := startServer(t, func(string) (*Workspace, error) { return nil, errors.New("unused") }, t.TempDir())
	c.notify("exit", nil)
	if err := <-c.done; !errors.Is(err, errExitWithoutShutdown) {
		t.Fatalf("got %v", err)
	}
}

// A copy that buti's diff in the editor wrote shows the comments on its change and side, not those of the file.
func TestCommentsOnDiffCopy(t *testing.T) {
	tmp := t.TempDir()
	if err := editor.WriteSource(tmp, editor.Source{Branch: "feat"}); err != nil {
		t.Fatal(err)
	}
	at := func(kind review.Kind, branch string, side review.Side) review.Located {
		return review.Located{Anchor: review.Anchor{Kind: kind, Branch: branch, Path: "a.go", Side: side, Line: 1}}
	}
	s := New(Config{})
	s.located = []review.Located{
		at(review.KindCommit, "feat", review.SideNew),  // shown on new
		at(review.KindCommit, "feat", review.SideOld),  // shown on old
		at(review.KindCommit, "other", review.SideNew), // another branch
		at(review.KindUnassigned, "", review.SideNew),  // uncommitted
	}
	for side, want := range map[string]int{"new": 0, "old": 1} {
		ls, ok := s.onCopy(filepath.Join(tmp, "feat", side, "a.go"))
		if !ok || len(ls) != 1 || ls[0].Anchor != s.located[want].Anchor {
			t.Errorf("%s: %+v, %v", side, ls, ok)
		}
	}
	if _, ok := s.onCopy(filepath.Join(t.TempDir(), "a.go")); ok {
		t.Error("a file that is not a copy")
	}
}
