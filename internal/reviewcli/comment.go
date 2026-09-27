package reviewcli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

const commentUsage = `buti review comment --file <path> --line <n> [--end-line <n>] [--side new|old] [--shortcode <id>] --body "<text>" [--author <name>]`

// comment adds a comment, by default signed "agent": how a reviewing agent (/buti-review) leaves its findings.
func comment(ctx context.Context, args []string, env Env) error {
	fs := flags("comment", commentUsage, env)
	path := fs.String("file", "", "the file, as `but status` shows it")
	line := fs.Int("line", 0, "the first line, numbered on --side")
	endLine := fs.Int("end-line", 0, "the last line of a range (default --line)")
	side := fs.String("side", string(review.SideNew), "new (the file as changed) or old (removed lines)")
	shortcode := fs.String("shortcode", "", "where the lines are: zz, a stack, a commit, a branch or a file's id (default: the uncommitted changes)")
	body := fs.String("body", "", "the comment")
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
	case strings.TrimSpace(*body) == "":
		problem = "--body is required"
	case strings.TrimSpace(*author) == "":
		problem = "--author can't be empty"
	}
	if problem != "" {
		fmt.Fprintln(env.Stderr, problem)
		fs.Usage()
		return errUsage
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
	a, target, err := anchorAt(ctx, st, env.But, *shortcode, *path, review.Side(*side), *line, *endLine)
	if err != nil {
		return err
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
	var best *candidate
	var bestText string
	for i := range cands {
		c := &cands[i]
		diff, err := d.Diff(ctx, c.fileID)
		if err != nil {
			return review.Anchor{}, "", fmt.Errorf("diff of %s: %w", path, err)
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
