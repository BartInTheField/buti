package reviewcli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/editor"
	"github.com/bartinthefield/buti/internal/review"
)

const commentUsage = `buti review comment --file <path> --line <n> [--end-line <n>] [--side new|old] [--shortcode <id>] [--worktree] [--body "<text>"] [--author <name>]
  --file may be absolute. --worktree counts --line in the file on disk and finds it in the uncommitted changes or the
  commits that change it. Without --body, a terminal is asked for the comment.`

// comment adds a comment, by default signed "agent": how a reviewing agent (/buti-review) leaves its findings.
func comment(ctx context.Context, args []string, env Env) error {
	fs := flags("comment", commentUsage, env)
	path := fs.String("file", "", "the file, as `but status` shows it")
	line := fs.Int("line", 0, "the first line, numbered on --side")
	endLine := fs.Int("end-line", 0, "the last line of a range (default --line)")
	side := fs.String("side", string(review.SideNew), "new (the file as changed) or old (removed lines)")
	shortcode := fs.String("shortcode", "", "where the lines are: zz, a stack, a commit, a branch or a file's id (default: the uncommitted changes)")
	worktree := fs.Bool("worktree", false, "count --line in the file on disk (the new side); anchor it on the uncommitted changes or the commit that has the lines")
	body := fs.String("body", "", "the comment (default: ask on the terminal)")
	author := fs.String("author", "agent", "who is commenting")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	if *endLine == 0 {
		*endLine = *line
	}
	var problem string
	switch {
	case *path == "" && *shortcode == "":
		problem = "--file is required"
	case *line < 1:
		problem = "--line is required, counting from 1"
	case *endLine < *line:
		problem = "--end-line is before --line"
	case *side != string(review.SideNew) && *side != string(review.SideOld):
		problem = fmt.Sprintf("invalid --side %q: new or old", *side)
	case *worktree && *side != string(review.SideNew):
		problem = "--worktree counts lines in the file on disk, so it is the new side only"
	case *worktree && *shortcode != "":
		problem = "--worktree searches the whole workspace; drop --shortcode"
	case *worktree && *path == "":
		problem = "--worktree needs --file"
	case strings.TrimSpace(*body) == "" && !env.terminal():
		problem = "--body is required"
	case strings.TrimSpace(*author) == "":
		problem = "--author can't be empty"
	}
	if problem != "" {
		fmt.Fprintln(env.Stderr, problem)
		fs.Usage()
		return errUsage
	}
	// A copy that buti's diff in an editor shows (`Z`) counts its lines on the side and change it was copied from.
	var copyBranch string
	if *path != "" && filepath.IsAbs(*path) {
		if src, rel, sd, ok := editor.FromCopy(*path); ok {
			*path, *side, *worktree, *shortcode, copyBranch = rel, sd, false, src.Commit, src.Branch
		}
	}
	root := ""
	if *path != "" && filepath.IsAbs(*path) {
		var err error
		if root, err = repoRoot(ctx, env); err != nil {
			return err
		}
		if *path, err = relativeTo(root, *path); err != nil {
			return err
		}
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	st, err := env.But.Status(ctx)
	if err != nil {
		return err
	}
	// A file's own id, as `but status -f` prints it ("rl", "c3:m"), names both the file and where it is.
	if owner, p, ok := fileShortcode(st, *shortcode); ok {
		if *path != "" && *path != p {
			return fmt.Errorf("%s is %s, not %s", *shortcode, p, *path)
		}
		*shortcode, *path = owner, p
	}
	if *path == "" {
		fmt.Fprintln(env.Stderr, "--file is required unless --shortcode is a file's id")
		fs.Usage()
		return errUsage
	}
	var a review.Anchor
	var target string
	if *worktree {
		if root == "" {
			if root, err = repoRoot(ctx, env); err != nil {
				return err
			}
		}
		a, target, err = anchorInWorktree(ctx, st, env.But, root, *path, *line, *endLine)
	} else if copyBranch != "" {
		a, target, err = anchorOnBranch(ctx, st, env.But, copyBranch, *path, review.Side(*side), *line, *endLine)
	} else {
		a, target, err = anchorAt(ctx, st, env.But, *shortcode, *path, review.Side(*side), *line, *endLine)
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(*body) == "" {
		*body, err = askBody(env, *path, *line)
		if err != nil {
			return err
		}
		if *body == "" {
			fmt.Fprintln(env.Stderr, "cancelled")
			return nil
		}
	}
	c, err := s.Add(a, *author, *body)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "commented %s on %s %s\n", c.ID, target, item{Located: review.Located{Anchor: a}}.where())
	return nil
}

// candidate is a place the commented file may be in.
type candidate struct {
	anchor review.Anchor // Kind, Branch and for a commit ChangeID and CommitID
	target string
	fileID string
}

// anchorAt finds the lines in the diff of the place shortcode names and builds the anchor, with the lines' text so
// the comment can follow them. A branch stands for the commit on it that has the lines, preferring one that changes
// them; no shortcode means the uncommitted changes, unassigned or assigned.
func anchorAt(ctx context.Context, st *but.Status, d review.Differ, shortcode, path string, side review.Side, line, end int) (review.Anchor, string, error) {
	cands, err := candidates(st, shortcode, path)
	if err != nil {
		return review.Anchor{}, "", err
	}
	best, bestText, err := pickLines(ctx, d, cands, path, side, line, end)
	if err != nil {
		return review.Anchor{}, "", err
	}
	lines := fmt.Sprintf("line %d", line)
	if end > line {
		lines = fmt.Sprintf("lines %d-%d", line, end)
	}
	if best == nil {
		where := cands[0].target
		if len(cands) > 1 {
			where = "any of " + strings.Join(targets(cands), ", ")
		}
		return review.Anchor{}, "", fmt.Errorf("%s of %s on the %s side is not in the diff of %s; count lines as the file is %s",
			lines, path, side, where, map[review.Side]string{review.SideNew: "after the change", review.SideOld: "before the change"}[side])
	}
	a := best.anchor
	a.Path, a.Side, a.Line, a.EndLine, a.LineText = path, side, line, end, bestText
	return a, best.target, nil
}

// anchorInWorktree anchors lines counted in the working-copy file: on the uncommitted changes when they show those
// lines, else on the applied commit that has the text, preferring one that changes it, then the newest.
func anchorInWorktree(ctx context.Context, st *but.Status, d review.Differ, root, path string, line, end int) (review.Anchor, string, error) {
	data, err := os.ReadFile(filepath.Join(root, path))
	if err != nil {
		return review.Anchor{}, "", err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if end > len(lines) {
		return review.Anchor{}, "", fmt.Errorf("%s has %d lines, not %d", path, len(lines), end)
	}
	text := strings.Join(lines[line-1:end], "\n")

	if cands, err := candidates(st, "", path); err == nil {
		best, bestText, err := pickLines(ctx, d, cands, path, review.SideNew, line, end)
		if err != nil {
			return review.Anchor{}, "", err
		}
		if best != nil {
			a := best.anchor
			a.Path, a.Side, a.Line, a.EndLine, a.LineText = path, review.SideNew, line, end, bestText
			return a, best.target, nil
		}
	}

	want := review.Anchor{Side: review.SideNew, Line: line, EndLine: end, LineText: text}
	var best review.Anchor
	var target string
	var bestChanged, have bool
	// Stacks, their branches and a branch's commits are in `but status` order, newest first.
search:
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				id := changeID(c.Changes, path)
				if id == "" {
					continue
				}
				diff, err := d.Diff(ctx, id)
				if err != nil {
					return review.Anchor{}, "", fmt.Errorf("diff of %s: %w", path, err)
				}
				fd := fileDiff(diff, path)
				at, ok := review.FindLines(fd, want)
				// A line without words ("}", a blank line) is found nearly anywhere, so it only counts in place.
				if !ok || !hasWord(text) && at.Line != line {
					continue
				}
				t, changed, ok := review.LinesAt(fd, review.SideNew, at.Line, at.EndLine)
				if !ok || have && !changed {
					continue
				}
				best = review.Anchor{Kind: review.KindCommit, ChangeID: c.ChangeID, CommitID: c.CommitID, Branch: b.Name,
					Path: path, Side: review.SideNew, Line: at.Line, EndLine: at.EndLine, LineText: t}
				target, bestChanged, have = c.CliID, changed, true
				if bestChanged {
					break search
				}
			}
		}
	}
	if !have {
		return review.Anchor{}, "", fmt.Errorf("%s:%d is not part of any change in the workspace", path, line)
	}
	return best, target, nil
}

// anchorOnBranch anchors lines counted in the whole diff of branch on the branch's commit that has them, as the TUI
// does for a comment on a branch's diff.
func anchorOnBranch(ctx context.Context, st *but.Status, d review.Differ, branch, path string, side review.Side, line, end int) (review.Anchor, string, error) {
	var cliID string
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			if b.Name == branch {
				cliID = b.CliID
			}
		}
	}
	if cliID == "" {
		return review.Anchor{}, "", fmt.Errorf("branch %s is not applied in the workspace", branch)
	}
	diff, err := d.Diff(ctx, cliID)
	if err != nil {
		return review.Anchor{}, "", fmt.Errorf("diff of %s: %w", branch, err)
	}
	text, _, ok := review.LinesAt(fileDiff(diff, path), side, line, end)
	if !ok {
		return review.Anchor{}, "", fmt.Errorf("%s:%d on the %s side is not in the diff of %s", path, line, side, branch)
	}
	a, err := review.OnBranch(ctx, st, d, branch, review.Anchor{Path: path, Side: side, Line: line, EndLine: end, LineText: text})
	if err != nil {
		return review.Anchor{}, "", err
	}
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				if c.CommitID == a.CommitID {
					return a, c.CliID, nil
				}
			}
		}
	}
	return a, branch, nil
}

// askBody prompts for the comment on stderr and reads it from stdin up to an empty line; empty means cancel.
func askBody(env Env, path string, line int) (string, error) {
	fmt.Fprintf(env.Stderr, "Comment on %s:%d (end with an empty line; empty to cancel):\n", path, line)
	in := env.Stdin
	if in == nil {
		in = os.Stdin
	}
	var ls []string
	sc := bufio.NewScanner(in)
	sc.Buffer(nil, 1<<20) // a pasted line can be long
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			break
		}
		ls = append(ls, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return strings.Join(ls, "\n"), nil
}

// terminal reports whether stdin is a terminal a person types the comment on.
func (e Env) terminal() bool {
	if e.IsTerminal != nil {
		return e.IsTerminal()
	}
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// repoRoot is the repository's top directory, where `but status` paths start.
func repoRoot(ctx context.Context, env Env) (string, error) {
	c, ok := env.But.(*but.Client)
	if !ok {
		return "", errors.New("can't tell where the repository is")
	}
	dir := c.Dir
	if dir == "" {
		dir = "."
	}
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	if out, err := cmd.Output(); err == nil {
		dir = strings.TrimSpace(string(out))
	}
	return filepath.Abs(dir)
}

// relativeTo makes the absolute path relative to root, following symlinks like a temp dir under /var on macOS.
func relativeTo(root, path string) (string, error) {
	resolve := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}
		return p
	}
	rel, err := filepath.Rel(resolve(root), resolve(path))
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s is outside the repository %s", path, root)
	}
	return filepath.ToSlash(rel), nil
}

// pickLines finds the candidate whose diff has the lines, preferring one that changes them; nil when none does.
func pickLines(ctx context.Context, d review.Differ, cands []candidate, path string, side review.Side, line, end int) (*candidate, string, error) {
	var best *candidate
	var bestText string
	for i := range cands {
		c := &cands[i]
		diff, err := d.Diff(ctx, c.fileID)
		if err != nil {
			return nil, "", fmt.Errorf("diff of %s: %w", path, err)
		}
		text, changed, ok := review.LinesAt(fileDiff(diff, path), side, line, end)
		if !ok {
			continue
		}
		if best == nil {
			best, bestText = c, text
		}
		if changed {
			best, bestText = c, text
			break
		}
	}
	return best, bestText, nil
}

func hasWord(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) >= 0
}

func targets(cs []candidate) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.target
	}
	return out
}

// candidates lists the places shortcode names that change path, in the order to try them.
func candidates(st *but.Status, shortcode, path string) ([]candidate, error) {
	var out []candidate
	unassigned := func() {
		if id := changeID(st.UncommittedChanges, path); id != "" {
			out = append(out, candidate{anchor: review.Anchor{Kind: review.KindUnassigned}, target: review.TargetUnassigned, fileID: id})
		}
	}
	assigned := func(s but.Stack) {
		if id := changeID(s.AssignedChanges, path); id != "" && len(s.Branches) > 0 {
			// Known by the stack's top branch, as the TUI does: `but status` has no stable stack id.
			out = append(out, candidate{anchor: review.Anchor{Kind: review.KindAssigned, Branch: s.Branches[0].Name},
				target: s.CliID, fileID: id})
		}
	}
	commit := func(c but.Commit, branch string) {
		if id := changeID(c.Changes, path); id != "" {
			out = append(out, candidate{anchor: review.Anchor{Kind: review.KindCommit, ChangeID: c.ChangeID,
				CommitID: c.CommitID, Branch: branch}, target: c.CliID, fileID: id})
		}
	}
	notHere := func(what string) error {
		return fmt.Errorf("%s does not change %s", what, path)
	}

	switch {
	case shortcode == "":
		unassigned()
		for _, s := range st.Stacks {
			assigned(s)
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("%s has no uncommitted changes; pass --shortcode with the commit or branch it is changed in", path)
		}
		return out, nil
	case shortcode == review.TargetUnassigned:
		if unassigned(); len(out) == 0 {
			return nil, notHere("zz (the unassigned changes)")
		}
		return out, nil
	}
	for _, s := range st.Stacks {
		if s.CliID == shortcode {
			if assigned(s); len(out) == 0 {
				return nil, notHere("the uncommitted changes of stack " + shortcode)
			}
			return out, nil
		}
	}
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				if c.CliID == shortcode || c.ChangeID == shortcode || len(shortcode) >= 7 && strings.HasPrefix(c.CommitID, shortcode) {
					if commit(c, b.Name); len(out) == 0 {
						return nil, notHere("commit " + shortcode)
					}
					return out, nil
				}
			}
		}
	}
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			if b.CliID == shortcode || b.Name == shortcode {
				for _, c := range b.Commits {
					commit(c, b.Name)
				}
				if len(out) == 0 {
					return nil, fmt.Errorf("no commit on branch %s changes %s", b.Name, path)
				}
				return out, nil
			}
		}
	}
	return nil, errors.New("no stack, commit or branch " + shortcode + " in the workspace; run `but status` for the shortcodes")
}

// fileShortcode finds the file whose cli id is id, and the shortcode of where it is: zz, its stack or its commit.
func fileShortcode(st *but.Status, id string) (owner, path string, ok bool) {
	if id == "" {
		return "", "", false
	}
	find := func(cs []but.Change) string {
		for _, c := range cs {
			if c.CliID == id {
				return c.FilePath
			}
		}
		return ""
	}
	if p := find(st.UncommittedChanges); p != "" {
		return review.TargetUnassigned, p, true
	}
	for _, s := range st.Stacks {
		if p := find(s.AssignedChanges); p != "" {
			return s.CliID, p, true
		}
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				if p := find(c.Changes); p != "" {
					return c.CliID, p, true
				}
			}
		}
	}
	return "", "", false
}

func changeID(cs []but.Change, path string) string {
	for _, c := range cs {
		if c.FilePath == path {
			return c.CliID
		}
	}
	return ""
}
