package lsp

import (
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
)

// The subset of the LSP types the server reads and writes.

type position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspRange struct {
	Start position `json:"start"`
	End   position `json:"end"`
}

// Diagnostic severities.
const (
	severityWarning     = 2
	severityInformation = 3
	severityHint        = 4
)

type diagnostic struct {
	Range    lspRange `json:"range"`
	Severity int      `json:"severity"`
	Code     string   `json:"code,omitempty"`
	Source   string   `json:"source"`
	Message  string   `json:"message"`
}

type publishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Version     *int         `json:"version,omitempty"`
	Diagnostics []diagnostic `json:"diagnostics"`
}

type workspaceFolder struct {
	URI string `json:"uri"`
}

type initializeParams struct {
	RootURI          string            `json:"rootUri"`
	RootPath         string            `json:"rootPath"`
	WorkspaceFolders []workspaceFolder `json:"workspaceFolders"`
}

type textDocumentItem struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
	Text    string `json:"text"`
}

type textDocumentIdentifier struct {
	URI string `json:"uri"`
}

type didOpenParams struct {
	TextDocument textDocumentItem `json:"textDocument"`
}

type didChangeParams struct {
	TextDocument   textDocumentItem `json:"textDocument"`
	ContentChanges []struct {
		Range *lspRange `json:"range"`
		Text  string    `json:"text"`
	} `json:"contentChanges"`
}

type didCloseParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
}

// incomingDiagnostic is a diagnostic as a client sends it back; code may be a number for other servers.
type incomingDiagnostic struct {
	Source string `json:"source"`
	Code   any    `json:"code"`
}

type codeActionParams struct {
	TextDocument textDocumentIdentifier `json:"textDocument"`
	Context      struct {
		Diagnostics []incomingDiagnostic `json:"diagnostics"`
	} `json:"context"`
}

type command struct {
	Title     string `json:"title"`
	Command   string `json:"command"`
	Arguments []any  `json:"arguments"`
}

type codeAction struct {
	Title       string       `json:"title"`
	Kind        string       `json:"kind"`
	Diagnostics []diagnostic `json:"diagnostics,omitempty"`
	Command     command      `json:"command"`
}

type executeCommandParams struct {
	Command   string `json:"command"`
	Arguments []any  `json:"arguments"`
}

// Message types of window/logMessage.
const messageError = 1

type logMessageParams struct {
	Type    int    `json:"type"`
	Message string `json:"message"`
}

// uriToPath turns a file:// URI into a local path, undoing percent-encoding. It returns "" for other schemes.
func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "file" {
		return ""
	}
	p := u.Path
	// file:///C:/x has the path /C:/x.
	if runtime.GOOS == "windows" && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.Clean(filepath.FromSlash(p))
}

// pathToURI turns an absolute local path into a file:// URI, percent-encoding what needs it.
func pathToURI(path string) string {
	p := filepath.ToSlash(path)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}
