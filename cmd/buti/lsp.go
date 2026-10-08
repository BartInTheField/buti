package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/editor"
	"github.com/bartinthefield/buti/internal/lsp"
	"github.com/bartinthefield/buti/internal/review"
)

// runLSP runs `buti lsp`, a language server on stdin and stdout that shows review comments as diagnostics. Errors
// reach the editor as log messages; stderr is for the rest, since stdout carries the protocol.
func runLSP(dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 {
		fmt.Fprintln(stderr, "usage: buti [-C dir] lsp")
		return 2
	}
	s := lsp.New(lsp.Config{Root: dir, Open: openLSPWorkspace, In: stdin, Out: stdout})
	if err := s.Run(context.Background()); err != nil {
		fmt.Fprintln(stderr, "buti lsp:", err)
		return 1
	}
	return 0
}

// openLSPWorkspace opens the repository containing root. Comment paths are relative to the top of the work tree,
// which an editor opened on a subdirectory does not have as its root.
func openLSPWorkspace(root string) (*lsp.Workspace, error) {
	if _, err := exec.LookPath("but"); err != nil {
		return nil, errors.New("the GitButler CLI (`but`) must be on PATH")
	}
	// An editor opened on the copies of a diff that buti wrote (`Z`) is about the repository they come from.
	if repo, ok := editor.CopiedFrom(root); ok {
		root = repo
	}
	top := root
	if out, err := exec.Command("git", "-C", root, "rev-parse", "--show-toplevel").Output(); err == nil {
		top = strings.TrimSpace(string(out))
	}
	// git resolves symlinks; keep the editor's spelling of the same directory, which its document URIs use.
	if same(root, top) {
		top = root
	}
	store, err := review.Open(top)
	if err != nil {
		return nil, err
	}
	return &lsp.Workspace{Dir: top, Store: store, Source: but.New(top)}, nil
}

// same reports whether a and b are the same directory once symlinks are resolved.
func same(a, b string) bool {
	ra, err := filepath.EvalSymlinks(a)
	if err != nil {
		return false
	}
	rb, err := filepath.EvalSymlinks(b)
	return err == nil && ra == rb
}
