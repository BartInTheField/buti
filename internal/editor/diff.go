package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartinthefield/buti/internal/but"
)

// File is one file's changes: every hunk of its text diff.
type File struct {
	Path  string
	Hunks []but.Hunk
}

// Files lists the files of d that have a text diff, in order of appearance. An uncommitted diff has one entry per
// hunk, so entries of a path are merged; binary and other files without hunks are left out.
func Files(d *but.Diff) []File {
	var out []File
	seen := map[string]bool{}
	for _, c := range d.Changes {
		if seen[c.Path] {
			continue
		}
		seen[c.Path] = true
		fd := d.File(c.Path)
		if fd == nil || fd.Diff.Type != "patch" || len(fd.Diff.Hunks) == 0 {
			continue
		}
		out = append(out, File{Path: c.Path, Hunks: fd.Diff.Hunks})
	}
	return out
}

// OldContent undoes hunks on the file as it is now, giving the file before the change. It fails when the lines the
// hunks say are there are not, as when the file changed since the diff was made.
func OldContent(newContent string, hunks []but.Hunk) (string, error) {
	lines := splitLines(newContent)
	hunks = append([]but.Hunk(nil), hunks...)
	sortHunks(hunks)
	var out strings.Builder
	at := 0
	for _, h := range hunks {
		start := h.NewStart - 1
		if h.NewLines == 0 {
			start = h.NewStart // the line before the hunk, for a pure deletion
		}
		start = max(start, 0)
		if start < at || start > len(lines) {
			return "", fmt.Errorf("hunk at line %d does not fit the file: it has changed since the diff", h.NewStart)
		}
		for _, l := range lines[at:start] {
			out.WriteString(l)
		}
		at = start
		body := strings.Split(h.Diff, "\n")
		if len(body) > 0 && strings.HasPrefix(body[0], "@@") {
			body = body[1:]
		}
		for len(body) > 0 && body[len(body)-1] == "" {
			body = body[:len(body)-1]
		}
		consumed := 0
		for i := 0; i < len(body); i++ {
			sign, text := byte(' '), body[i]
			if text != "" {
				sign, text = text[0], text[1:]
			}
			if sign == '\\' {
				continue // a marker, handled with the line before it
			}
			noNL := i+1 < len(body) && strings.HasPrefix(body[i+1], `\`)
			switch sign {
			case ' ', '+':
				if at >= len(lines) || strings.TrimSuffix(lines[at], "\n") != text || strings.HasSuffix(lines[at], "\n") == noNL {
					return "", fmt.Errorf("line %d does not match the diff: the file has changed since", at+1)
				}
				if sign == ' ' {
					out.WriteString(lines[at])
				}
				at++
				consumed++
			case '-':
				out.WriteString(text)
				if !noNL {
					out.WriteString("\n")
				}
			default:
				return "", fmt.Errorf("unexpected diff line %q", body[i])
			}
		}
		if consumed != h.NewLines {
			return "", fmt.Errorf("hunk at line %d has %d new lines, expected %d", h.NewStart, consumed, h.NewLines)
		}
	}
	for _, l := range lines[at:] {
		out.WriteString(l)
	}
	return out.String(), nil
}

func sortHunks(hs []but.Hunk) {
	for i := 1; i < len(hs); i++ {
		for j := i; j > 0 && hs[j].NewStart < hs[j-1].NewStart; j-- {
			hs[j], hs[j-1] = hs[j-1], hs[j]
		}
	}
}

// splitLines splits after each newline, keeping it, so a missing final newline stays visible.
func splitLines(s string) []string {
	var out []string
	for s != "" {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			out = append(out, s)
			break
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	return out
}

// Root is where the diff files of the repository at dir are written: in its git dir, next to buti's review comments.
// Inside the repository the editor's language servers find its toolchain (a mise or asdf version) and its go.mod or
// package.json, so the copies load like the real files instead of failing outside any project; inside the git dir
// they are never committed and stay out of the way of `but`. Outside a repository, the system temp dir.
func Root(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return filepath.Join(os.TempDir(), "buti-diff")
	}
	return filepath.Join(strings.TrimSpace(string(out)), "buti", "diff")
}

// NewDir creates a fresh directory under Root(dir), after removing the ones older than a day.
func NewDir(dir string) (string, error) {
	root := Root(dir)
	Cleanup(root, 24*time.Hour)
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	return os.MkdirTemp(root, "d-")
}

// Cleanup removes the directories in root not modified for maxAge, ignoring failures.
func Cleanup(root string, maxAge time.Duration) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > maxAge {
			_ = os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// Worktree writes the old side of uncommitted files under tmp/old and returns the --diff pairs: each old file with
// the real file in the worktree, which stays editable. A deleted file is paired with an empty one.
func Worktree(tmp, dir string, files []File) ([][2]string, error) {
	var pairs [][2]string
	for _, f := range files {
		real := filepath.Join(dir, f.Path)
		cur, err := os.ReadFile(real)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		old, err := OldContent(string(cur), f.Hunks)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.Path, err)
		}
		oldPath := filepath.Join(tmp, "old", f.Path)
		if err := writeFile(oldPath, old); err != nil {
			return nil, err
		}
		if os.IsNotExist(statErr(real)) {
			real = filepath.Join(tmp, "empty", f.Path)
			if err := writeFile(real, ""); err != nil {
				return nil, err
			}
		}
		pairs = append(pairs, [2]string{oldPath, real})
	}
	return pairs, nil
}

func statErr(p string) error { _, err := os.Stat(p); return err }

// Commit writes both sides of files under tmp/<label>/old and tmp/<label>/new, the new side as read by show, and
// returns the two directories.
func Commit(tmp, label string, files []File, show func(path string) (string, error)) (oldDir, newDir string, err error) {
	oldDir, newDir = filepath.Join(tmp, label, "old"), filepath.Join(tmp, label, "new")
	for _, f := range files {
		cur, err := show(f.Path)
		if err != nil {
			return "", "", err
		}
		old, err := OldContent(cur, f.Hunks)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", f.Path, err)
		}
		if err := writeFile(filepath.Join(oldDir, f.Path), old); err != nil {
			return "", "", err
		}
		if err := writeFile(filepath.Join(newDir, f.Path), cur); err != nil {
			return "", "", err
		}
	}
	return oldDir, newDir, nil
}

// Source is where the copies in a diff directory come from, kept next to them so that a review comment left on a
// copy in the editor finds its way back to the change. Neither set means the uncommitted changes.
type Source struct {
	Commit string `json:"commit,omitempty"` // the diff of this commit (its sha)
	Branch string `json:"branch,omitempty"` // the diff of this branch, all its commits
	Repo   string `json:"repo,omitempty"`   // the repository the copies come from
}

const sourceFile = "buti-diff.json"

// WriteSource records src in the diff directory tmp.
func WriteSource(tmp string, src Source) error {
	data, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(tmp, sourceFile), string(data))
}

// FromCopy maps path, when it is a copy in a diff directory, back to where it comes from: the source, the file's
// repository-relative path and the side of the diff it shows, "old" or "new".
func FromCopy(path string) (src Source, rel, side string, ok bool) {
	dir, src, ok := sourceAbove(filepath.Dir(path))
	if !ok {
		return Source{}, "", "", false
	}
	r, err := filepath.Rel(dir, path)
	if err != nil {
		return Source{}, "", "", false
	}
	parts := strings.Split(filepath.ToSlash(r), "/")
	switch {
	case len(parts) > 1 && parts[0] == "old": // uncommitted: <tmp>/old/<path>
		return src, strings.Join(parts[1:], "/"), "old", true
	case len(parts) > 1 && parts[0] == "empty": // a deleted file's empty new side
		return src, strings.Join(parts[1:], "/"), "new", true
	case len(parts) > 2 && (parts[1] == "old" || parts[1] == "new"): // <tmp>/<label>/<side>/<path>
		return src, strings.Join(parts[2:], "/"), parts[1], true
	}
	return Source{}, "", "", false
}

// CopiedFrom returns the repository whose copies dir is in, for an editor opened on a diff directory.
func CopiedFrom(dir string) (string, bool) {
	_, src, ok := sourceAbove(dir)
	return src.Repo, ok && src.Repo != ""
}

// sourceAbove finds the diff directory at or above dir and reads its source.
func sourceAbove(dir string) (string, Source, bool) {
	for ; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, sourceFile))
		if err != nil {
			continue
		}
		var src Source
		return dir, src, json.Unmarshal(data, &src) == nil
	}
	return "", Source{}, false
}

// GitShow reads path as of commit in the repository at dir; a path the commit lacks reads as empty.
func GitShow(ctx context.Context, dir, commit, path string) (string, error) {
	spec := commit + ":" + path
	out, err := exec.CommandContext(ctx, "git", "-C", dir, "cat-file", "-p", spec).Output()
	if err == nil {
		return string(out), nil
	}
	if exec.CommandContext(ctx, "git", "-C", dir, "cat-file", "-e", spec).Run() != nil {
		return "", nil
	}
	return "", fmt.Errorf("git cat-file %s: %w", spec, err)
}

// ZedArgs turns pairs into the arguments of `zed --diff`.
func ZedArgs(pairs [][2]string) []string {
	var args []string
	for _, p := range pairs {
		args = append(args, "--diff", p[0], p[1])
	}
	return args
}
