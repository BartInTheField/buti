package desktop

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode/utf8"
)

// maxFileBytes caps GET /file: the edit mode view shows a file, not a blob.
const maxFileBytes = 1 << 20

type openReq struct {
	// Paths are relative to the repository.
	Paths []string `json:"paths"`
	// Editor opens text files in the default text editor rather than the app for the file type.
	Editor bool `json:"editor"`
}

// openFiles starts the desktop's opener on paths, detached, like the TUI's `O`.
// BUTI_DESKTOP_OPEN replaces the opener (tests use it to record what was opened).
var openFiles = func(editor bool, paths []string) error {
	opener, args := "xdg-open", []string(nil)
	if runtime.GOOS == "darwin" {
		opener = "open"
		if editor {
			args = []string{"-t"}
		}
	}
	if o := os.Getenv("BUTI_DESKTOP_OPEN"); o != "" {
		opener, args = o, nil
	}
	// xdg-open takes one path; open takes many, but one at a time works for both.
	for _, p := range paths {
		cmd := exec.Command(opener, append(args, p)...)
		if err := cmd.Start(); err != nil {
			return err
		}
		go func() { _ = cmd.Wait() }()
	}
	return nil
}

// repoPath resolves a repository-relative path, refusing anything outside the repository.
func (s *Server) repoPath(p string) (string, error) {
	if p == "" || filepath.IsAbs(p) {
		return "", badRequestError{msg: "path must be relative to the repository"}
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", badRequestError{msg: "path must stay inside the repository"}
	}
	root, err := filepath.Abs(s.client.Dir)
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, clean)
	// A symlink inside the repository must not lead out of it.
	if real, err := filepath.EvalSymlinks(full); err == nil {
		realRoot, _ := filepath.EvalSymlinks(root)
		if rel, err := filepath.Rel(realRoot, real); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", badRequestError{msg: "path must stay inside the repository"}
		}
	}
	return full, nil
}

// open opens repository files with the OS: the desktop counterpart of the TUI's `o` and `O`.
func (s *Server) open(w http.ResponseWriter, r *http.Request) {
	decodeAndRunOut(s, w, r, func(_ context.Context, req openReq) (string, error) {
		if err := required(len(req.Paths) > 0, "paths are required"); err != nil {
			return "", err
		}
		var full []string
		for _, p := range req.Paths {
			f, err := s.repoPath(p)
			if err != nil {
				return "", err
			}
			if _, err := os.Stat(f); err != nil {
				return "", badRequestError{msg: p + " does not exist"}
			}
			full = append(full, f)
		}
		return "", openFiles(req.Editor, full)
	})
}

// file returns a repository file's content: in edit mode `but diff` has nothing to
// show, so the conflicted file itself, markers and all, is what the view shows.
func (s *Server) file(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}
	p := r.URL.Query().Get("path")
	full, err := s.repoPath(p)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), "")
		return
	}
	b, err := readCapped(full)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "not_found", p+" does not exist", "")
		return
	case err != nil:
		writeError(w, http.StatusBadRequest, "bad_request", err.Error(), "")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		OK      bool   `json:"ok"`
		Path    string `json:"path"`
		Content string `json:"content"`
		Binary  bool   `json:"binary,omitempty"`
	}{OK: true, Path: p, Content: textOrEmpty(b), Binary: !utf8.Valid(b)})
}

func readCapped(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, errors.New("is a directory")
	}
	if info.Size() > maxFileBytes {
		return nil, errors.New("file is too large to show")
	}
	return os.ReadFile(path)
}

func textOrEmpty(b []byte) string {
	if !utf8.Valid(b) {
		return ""
	}
	return string(b)
}
