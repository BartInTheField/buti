// Package localstate keeps buti's per-repository files, such as review comments and walkthroughs, inside the git
// directory where they are never committed, and writes them safely while the TUI and a coding agent both do.
package localstate

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Dir returns <git-common-dir>/buti of the repository containing dir.
//
// The common dir, rather than --git-dir, is shared by all worktrees of a repository. GitButler keeps its own state
// there too, and commit change ids are repository-wide, so every worktree sees the same files. Living inside the git
// dir keeps them out of commits and out of the way of `but` operations.
func Dir(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && len(ee.Stderr) > 0 {
			return "", fmt.Errorf("finding the git dir: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return "", fmt.Errorf("finding the git dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(out))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(dir, gitDir)
	}
	return filepath.Join(gitDir, "buti"), nil
}

// WriteFile replaces the file at path atomically, so a reader never sees half of it.
func WriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ext := filepath.Ext(path)
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+strings.TrimSuffix(filepath.Base(path), ext)+"-*"+ext)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Windows refuses to replace a file another process is reading for a moment; retry briefly.
	for i := 0; ; i++ {
		err = os.Rename(tmp.Name(), path)
		if err == nil || i == 20 {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}
