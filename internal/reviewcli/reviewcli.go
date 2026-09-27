// Package reviewcli implements `buti review`, the non-interactive commands a coding agent uses to read the review
// comments left in buti and to resolve, reply to or dismiss them.
package reviewcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
)

// Usage lists the subcommands, for `buti --help` and `buti review --help`.
const Usage = `buti review list [--status open|resolved|dismissed|outdated|orphaned|all] [--json]
buti review show <id> [--json]
buti review resolve <id> [--summary "<text>"]
buti review reply <id> --body "<text>" [--author <name>]
buti review dismiss <id> [--reason "<text>"]
buti review clear [--resolved] [--dismissed]`

// contextLines is how many lines around a comment its context shows.
const contextLines = 2

// Env is what the commands run against.
type Env struct {
	Stdout, Stderr io.Writer
	Store          func() (*review.Store, error) // the comment store, opened only by commands that need it
	But            review.Source                 // the workspace, for resolving shortcodes
}

// NeedsBut reports whether the subcommand in args reads the workspace through `but`.
func NeedsBut(args []string) bool {
	return len(args) > 0 && (args[0] == "list" || args[0] == "show")
}

// errUsage is a usage error that has already been reported.
var errUsage = errors.New("usage")

// Run runs `buti review <args>` and returns the exit code: 0 on success, 1 on failure and 2 on a usage error.
func Run(ctx context.Context, args []string, env Env) int {
	err := run(ctx, args, env)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	default:
		fmt.Fprintln(env.Stderr, "buti review:", err)
		return 1
	}
}

func run(ctx context.Context, args []string, env Env) error {
	if len(args) == 0 {
		fmt.Fprintln(env.Stderr, "usage:\n"+indent(Usage))
		return errUsage
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "-h", "-help", "--help", "help":
		fmt.Fprintln(env.Stdout, "usage:\n"+indent(Usage))
		return nil
	case "list":
		return list(ctx, args, env)
	case "show":
		return show(ctx, args, env)
	case "resolve", "dismiss":
		return closeComment(sub, args, env)
	case "reply":
		return reply(args, env)
	case "clear":
		return clearComments(args, env)
	}
	fmt.Fprintf(env.Stderr, "buti review: unknown command %q\nusage:\n%s\n", sub, indent(Usage))
	return errUsage
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}

// flags returns a flag set for a subcommand that reports errors on env.Stderr.
func flags(name, usage string, env Env) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	fs.Usage = func() {
		fmt.Fprintln(env.Stderr, "usage: "+usage)
		fs.PrintDefaults()
	}
	return fs
}

// parse parses args with flags allowed before and after the positional arguments, as in `resolve <id> --summary x`,
// and checks that there are exactly want of those.
func parse(fs *flag.FlagSet, args []string, want int) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return nil, err
			}
			return nil, errUsage
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		pos = append(pos, args[0])
		args = args[1:]
	}
	if len(pos) != want {
		if want == 0 {
			fmt.Fprintf(fs.Output(), "unexpected argument %q\n", pos[0])
		} else {
			fmt.Fprintln(fs.Output(), "expected one comment id")
		}
		fs.Usage()
		return nil, errUsage
	}
	return pos, nil
}

// statusFilter says which located comments a --status value selects.
func statusFilter(s string) (stored []review.Status, keep func(review.Located) bool, ok bool) {
	all := func(review.Located) bool { return true }
	switch review.Status(s) {
	case "all":
		return nil, all, true
	case review.StatusOpen, review.StatusResolved, review.StatusDismissed:
		// Open includes the open comments whose line is outdated or whose commit is gone: they still want work.
		return []review.Status{review.Status(s)}, all, true
	case review.StatusOutdated, review.StatusOrphaned:
		return []review.Status{review.StatusOpen}, func(l review.Located) bool { return l.Status == review.Status(s) }, true
	}
	return nil, nil, false
}

func list(ctx context.Context, args []string, env Env) error {
	fs := flags("list", "buti review list [--status open|resolved|dismissed|outdated|orphaned|all] [--json]", env)
	status := fs.String("status", "open", "which comments to list: open, resolved, dismissed, outdated, orphaned or all")
	asJSON := fs.Bool("json", false, "print the comments as JSON")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	stored, keep, ok := statusFilter(*status)
	if !ok {
		fmt.Fprintf(env.Stderr, "invalid --status %q\n", *status)
		fs.Usage()
		return errUsage
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	cs, err := s.List(stored...)
	if err != nil {
		return err
	}
	items, err := locate(ctx, s, env.But, cs)
	if err != nil {
		return err
	}
	var out []item
	for _, it := range items {
		if keep(it.Located) {
			out = append(out, it)
		}
	}
	if *asJSON {
		js := make([]jsonComment, 0, len(out))
		for _, it := range out {
			js = append(js, it.json())
		}
		return writeJSON(env.Stdout, js)
	}
	if len(out) == 0 {
		if *status == "all" {
			fmt.Fprintln(env.Stdout, "No comments.")
		} else {
			fmt.Fprintf(env.Stdout, "No %s comments.\n", *status)
		}
		return nil
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, it := range out {
		first, _, _ := strings.Cut(it.Comment.Body, "\n")
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", it.Comment.ID, it.Status, dash(it.Target), it.where(), first)
	}
	return tw.Flush()
}

func show(ctx context.Context, args []string, env Env) error {
	fs := flags("show", "buti review show <id> [--json]", env)
	asJSON := fs.Bool("json", false, "print the comment as JSON")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	c, err := s.Get(pos[0])
	if err != nil {
		return err
	}
	items, err := locate(ctx, s, env.But, []review.Comment{c})
	if err != nil {
		return err
	}
	it := items[0]
	if *asJSON {
		return writeJSON(env.Stdout, it.detailJSON())
	}
	w := env.Stdout
	fmt.Fprintf(w, "%s  %s  by %s, %s\n", c.ID, it.Status, c.Author, c.CreatedAt.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(w, "%s (%s side)\n", it.where(), it.Anchor.Side)
	switch {
	case it.Target == "":
		fmt.Fprintln(w, "not in the workspace any more")
	case it.Anchor.Kind == review.KindCommit:
		fmt.Fprintf(w, "on commit %s %q (%s)", it.Target, it.commitTitle, short(it.Anchor.CommitID))
		if it.Anchor.Branch != "" {
			fmt.Fprintf(w, " on %s", it.Anchor.Branch)
		}
		fmt.Fprintf(w, ", file %s\n", dash(it.FileID))
	case it.Anchor.Kind == review.KindAssigned:
		fmt.Fprintf(w, "on %s (uncommitted, assigned to %s), file %s\n", it.Target, it.Anchor.Branch, dash(it.FileID))
	default:
		fmt.Fprintf(w, "on %s (uncommitted), file %s\n", it.Target, dash(it.FileID))
	}
	fmt.Fprintf(w, "\n%s\n", c.Body)
	if it.context != "" {
		fmt.Fprintf(w, "\n%s\n", it.context)
	}
	for _, r := range c.Replies {
		fmt.Fprintf(w, "\n%s, %s:\n%s\n", r.Author, r.CreatedAt.Local().Format("2006-01-02 15:04"), r.Body)
	}
	if c.Resolution.At != nil {
		fmt.Fprintf(w, "\n%s %s", c.Status, c.Resolution.At.Local().Format("2006-01-02 15:04"))
		if c.Resolution.Summary != "" {
			fmt.Fprintf(w, ": %s", c.Resolution.Summary)
		}
		fmt.Fprintln(w)
	}
	return nil
}

func closeComment(sub string, args []string, env Env) error {
	usage, name, what := `buti review resolve <id> [--summary "<text>"]`, "summary", "what was done"
	if sub == "dismiss" {
		usage, name, what = `buti review dismiss <id> [--reason "<text>"]`, "reason", "why nothing needs doing"
	}
	fs := flags(sub, usage, env)
	text := fs.String(name, "", what)
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	var c review.Comment
	if sub == "dismiss" {
		c, err = s.Dismiss(pos[0], *text)
	} else {
		c, err = s.Resolve(pos[0], *text)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "%s %s\n", c.Status, c.ID)
	return nil
}

func reply(args []string, env Env) error {
	fs := flags("reply", `buti review reply <id> --body "<text>" [--author <name>]`, env)
	body := fs.String("body", "", "the reply")
	author := fs.String("author", "agent", "who is replying")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*body) == "" {
		fmt.Fprintln(env.Stderr, "--body is required")
		fs.Usage()
		return errUsage
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	c, err := s.Reply(pos[0], *author, *body)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "replied to %s\n", c.ID)
	return nil
}

func clearComments(args []string, env Env) error {
	fs := flags("clear", "buti review clear [--resolved] [--dismissed]", env)
	resolved := fs.Bool("resolved", false, "delete the resolved comments")
	dismissed := fs.Bool("dismissed", false, "delete the dismissed comments")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	var statuses []review.Status
	if *resolved {
		statuses = append(statuses, review.StatusResolved)
	}
	if *dismissed {
		statuses = append(statuses, review.StatusDismissed)
	}
	if len(statuses) == 0 {
		fmt.Fprintln(env.Stderr, "say what to clear: --resolved, --dismissed or both")
		fs.Usage()
		return errUsage
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	n, err := s.Clear(statuses...)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "cleared %d %s\n", n, plural(n, "comment"))
	return nil
}

func plural(n int, s string) string {
	if n == 1 {
		return s
	}
	return s + "s"
}

// item is a located comment with what the output shows about it besides the anchor.
type item struct {
	review.Located
	commitTitle string
	context     string
}

// locate maps cs onto the workspace, saves the anchors that moved, and adds each comment's commit title and context.
// It runs one `but status` and one `but diff` per distinct file.
func locate(ctx context.Context, s *review.Store, src review.Source, cs []review.Comment) ([]item, error) {
	if len(cs) == 0 {
		return nil, nil
	}
	st, err := src.Status(ctx)
	if err != nil {
		return nil, err
	}
	d := &cachingDiffer{Differ: src, diffs: map[string]*but.Diff{}}
	ls, err := review.Locate(ctx, st, d, cs)
	if err != nil {
		return nil, err
	}
	if err := s.Save(ls); err != nil {
		return nil, err
	}
	items := make([]item, len(ls))
	for i, l := range ls {
		it := item{Located: l}
		if l.Anchor.Kind == review.KindCommit {
			it.commitTitle = commitTitle(st, l.Anchor.CommitID)
		}
		var fd *but.FileDiff
		if l.FileID != "" && l.Status != review.StatusOutdated {
			if diff, err := d.Diff(ctx, l.FileID); err == nil {
				fd = fileDiff(diff, l.Anchor.Path)
			}
		}
		it.context = review.Excerpt(fd, l.Anchor, contextLines)
		items[i] = it
	}
	return items, nil
}

func commitTitle(st *but.Status, commitID string) string {
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			for _, c := range b.Commits {
				if c.CommitID == commitID {
					return c.Subject()
				}
			}
		}
	}
	return ""
}

func fileDiff(d *but.Diff, path string) *but.FileDiff {
	for i := range d.Changes {
		if d.Changes[i].Path == path {
			return &d.Changes[i]
		}
	}
	return nil
}

// cachingDiffer remembers the diffs it fetched, so the context reuses the diffs re-anchoring already read.
type cachingDiffer struct {
	review.Differ
	diffs map[string]*but.Diff
}

func (c *cachingDiffer) Diff(ctx context.Context, id string) (*but.Diff, error) {
	if d, ok := c.diffs[id]; ok {
		return d, nil
	}
	d, err := c.Differ.Diff(ctx, id)
	if err == nil {
		c.diffs[id] = d
	}
	return d, err
}

// where is the comment's file and line, or line range.
func (it item) where() string {
	a := it.Anchor
	if a.EndLine > a.Line {
		return fmt.Sprintf("%s:%d-%d", a.Path, a.Line, a.EndLine)
	}
	return fmt.Sprintf("%s:%d", a.Path, a.Line)
}

// jsonComment is the flat shape of a comment in `list --json`, which agents read instead of the store's format.
type jsonComment struct {
	ID            string      `json:"id"`
	Status        string      `json:"status"`
	Body          string      `json:"body"`
	File          string      `json:"file"`
	Line          int         `json:"line"`
	EndLine       int         `json:"end_line"`
	Side          string      `json:"side"`
	Shortcode     *string     `json:"shortcode"`
	FileShortcode *string     `json:"file_shortcode"`
	Kind          string      `json:"kind"`
	Branch        *string     `json:"branch"`
	Commit        *jsonCommit `json:"commit"`
	Outdated      bool        `json:"outdated"`
	Context       string      `json:"context"`
}

type jsonCommit struct {
	Title string `json:"title"`
	SHA   string `json:"sha"`
}

// jsonDetail is `show --json`: the list fields plus the comment's history.
type jsonDetail struct {
	jsonComment
	Author     string          `json:"author"`
	CreatedAt  time.Time       `json:"created_at"`
	Resolution *jsonResolution `json:"resolution"`
	Replies    []jsonReply     `json:"replies"`
}

type jsonResolution struct {
	Summary string    `json:"summary"`
	At      time.Time `json:"at"`
}

type jsonReply struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func (it item) json() jsonComment {
	a := it.Anchor
	j := jsonComment{
		ID:            it.Comment.ID,
		Status:        string(it.Comment.Status),
		Body:          it.Comment.Body,
		File:          a.Path,
		Line:          a.Line,
		EndLine:       a.EndLine,
		Side:          string(a.Side),
		Shortcode:     orNull(it.Target),
		FileShortcode: orNull(it.FileID),
		Kind:          string(a.Kind),
		Outdated:      it.Status == review.StatusOutdated || it.Status == review.StatusOrphaned,
		Context:       it.context,
	}
	if a.Kind != review.KindUnassigned {
		j.Branch = orNull(a.Branch)
	}
	if a.Kind == review.KindCommit {
		j.Commit = &jsonCommit{Title: it.commitTitle, SHA: a.CommitID}
	}
	return j
}

func (it item) detailJSON() jsonDetail {
	c := it.Comment
	d := jsonDetail{jsonComment: it.json(), Author: c.Author, CreatedAt: c.CreatedAt, Replies: []jsonReply{}}
	if c.Resolution.At != nil {
		d.Resolution = &jsonResolution{Summary: c.Resolution.Summary, At: *c.Resolution.At}
	}
	for _, r := range c.Replies {
		d.Replies = append(d.Replies, jsonReply(r))
	}
	return d
}

func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
