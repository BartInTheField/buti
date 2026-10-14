// Package walkthroughcli implements `buti walkthrough`, the non-interactive commands a coding agent uses to pin a
// walkthrough to a branch, and to read, check and clear them, without the TUI.
package walkthroughcli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/walkthrough"
)

// Usage lists the subcommands, for `buti --help` and `buti walkthrough --help`.
const Usage = `buti walkthrough list [--json]
buti walkthrough show <branch> [--json]
buti walkthrough set <branch> [--body-file <path> | --body "<markdown>"] [--title "<text>"] [--author <name>]
  (without --body or --body-file, the body is read from stdin)
buti walkthrough status <branch> [--json]
buti walkthrough clear <branch>`

// Status is the workspace, which `set` and `status` read to find the branch and its tip.
type Status interface {
	Status(ctx context.Context) (*but.Status, error)
}

// Env is what the commands run against.
type Env struct {
	Stdout, Stderr io.Writer
	Stdin          io.Reader                          // where `set` reads a body from; nil is os.Stdin
	Store          func() (*walkthrough.Store, error) // opened only by commands that need it
	But            Status
}

// NeedsBut reports whether the subcommand in args reads the workspace through `but`.
func NeedsBut(args []string) bool {
	return len(args) > 0 && (args[0] == "set" || args[0] == "status")
}

// errUsage is a usage error that has already been reported.
var errUsage = errors.New("usage")

// Run runs `buti walkthrough <args>` and returns the exit code: 0 on success, 1 on failure and 2 on a usage error.
func Run(ctx context.Context, args []string, env Env) int {
	err := run(ctx, args, env)
	switch {
	case err == nil, errors.Is(err, flag.ErrHelp):
		return 0
	case errors.Is(err, errUsage):
		return 2
	default:
		fmt.Fprintln(env.Stderr, "buti walkthrough:", err)
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
		return list(args, env)
	case "show":
		return show(args, env)
	case "set":
		return set(ctx, args, env)
	case "status":
		return status(ctx, args, env)
	case "clear":
		return clearWalkthrough(args, env)
	}
	fmt.Fprintf(env.Stderr, "buti walkthrough: unknown command %q\nusage:\n%s\n", sub, indent(Usage))
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

// parse parses args with flags allowed before and after the positional arguments, as in `set auth --title x`, and
// checks that there are exactly want of those.
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
			fmt.Fprintln(fs.Output(), "expected one branch")
		}
		fs.Usage()
		return nil, errUsage
	}
	return pos, nil
}

// jsonWalkthrough is a walkthrough as `--json` prints it; the field names are part of the CLI's interface.
type jsonWalkthrough struct {
	Branch    string    `json:"branch"`
	Title     string    `json:"title"`
	Author    string    `json:"author"`
	UpdatedAt time.Time `json:"updated_at"`
	Head      *string   `json:"head"`
	Body      *string   `json:"body,omitempty"`
}

func toJSON(w walkthrough.Walkthrough, withBody bool) jsonWalkthrough {
	j := jsonWalkthrough{Branch: w.Branch, Title: w.Title, Author: w.Author, UpdatedAt: w.UpdatedAt}
	if w.Head != "" {
		j.Head = &w.Head
	}
	if withBody {
		j.Body = &w.Body
	}
	return j
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

func list(args []string, env Env) error {
	fs := flags("list", "buti walkthrough list [--json]", env)
	asJSON := fs.Bool("json", false, "print the walkthroughs, without their bodies, as JSON")
	if _, err := parse(fs, args, 0); err != nil {
		return err
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	ws, err := s.List()
	if err != nil {
		return err
	}
	if *asJSON {
		out := make([]jsonWalkthrough, 0, len(ws))
		for _, w := range ws {
			out = append(out, toJSON(w, false))
		}
		return writeJSON(env.Stdout, out)
	}
	if len(ws) == 0 {
		fmt.Fprintln(env.Stdout, "no walkthroughs")
		return nil
	}
	tw := tabwriter.NewWriter(env.Stdout, 0, 0, 2, ' ', 0)
	for _, w := range ws {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", w.Branch, dash(w.Title), w.Author, w.UpdatedAt.Local().Format("2006-01-02 15:04"))
	}
	return tw.Flush()
}

func show(args []string, env Env) error {
	fs := flags("show", "buti walkthrough show <branch> [--json]", env)
	asJSON := fs.Bool("json", false, "print the walkthrough as JSON, with its body")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	w, err := s.Get(pos[0])
	if err != nil {
		return err
	}
	if *asJSON {
		return writeJSON(env.Stdout, toJSON(w, true))
	}
	body := w.Body
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	_, err = io.WriteString(env.Stdout, body)
	return err
}

func set(ctx context.Context, args []string, env Env) error {
	usage := `buti walkthrough set <branch> [--body-file <path> | --body "<markdown>"] [--title "<text>"] [--author <name>]`
	fs := flags("set", usage, env)
	body := fs.String("body", "", "the walkthrough, in markdown")
	bodyFile := fs.String("body-file", "", "read the walkthrough from `path`; - is stdin")
	title := fs.String("title", "", "a one-line title")
	author := fs.String("author", walkthrough.AuthorAgent, "who wrote it: agent, user or a name")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	bodySet := false
	fs.Visit(func(f *flag.Flag) { bodySet = bodySet || f.Name == "body" })
	if bodySet && *bodyFile != "" {
		fmt.Fprintln(env.Stderr, "pass --body or --body-file, not both")
		fs.Usage()
		return errUsage
	}
	text := *body
	if !bodySet {
		if text, err = readBody(*bodyFile, env); err != nil {
			return err
		}
	}
	if strings.TrimSpace(text) == "" {
		return errors.New("empty walkthrough")
	}

	s, st, _, err := followed(ctx, env)
	if err != nil {
		return err
	}
	b, err := findBranch(st, pos[0])
	if err != nil {
		return err
	}
	head, changeID := walkthrough.Tip(*b)
	w, err := s.Set(walkthrough.Walkthrough{
		Branch: b.Name, Title: strings.TrimSpace(*title), Body: text, Author: *author, Head: head, ChangeID: changeID,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "set the walkthrough of %s\n", w.Branch)
	return nil
}

// readBody reads the body from path, or from stdin when path is empty or "-".
func readBody(path string, env Env) (string, error) {
	if path != "" && path != "-" {
		data, err := os.ReadFile(path)
		return string(data), err
	}
	in := env.Stdin
	if in == nil {
		if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice != 0 && path == "" {
			return "", errors.New("no body: pass --body, --body-file or pipe the markdown to stdin")
		}
		in = os.Stdin
	}
	data, err := io.ReadAll(in)
	return string(data), err
}

// followed opens the store and reads the workspace, re-keying the walkthroughs of renamed branches first so they are
// found under their new names. It returns the renames, old name to new.
func followed(ctx context.Context, env Env) (*walkthrough.Store, *but.Status, map[string]string, error) {
	s, err := env.Store()
	if err != nil {
		return nil, nil, nil, err
	}
	st, err := env.But.Status(ctx)
	if err != nil {
		return nil, nil, nil, err
	}
	moved, err := s.Follow(st)
	return s, st, moved, err
}

// findBranch finds the applied branch called name, or whose cli id is name.
func findBranch(st *but.Status, name string) (*but.Branch, error) {
	var byID *but.Branch
	for si := range st.Stacks {
		for bi := range st.Stacks[si].Branches {
			b := &st.Stacks[si].Branches[bi]
			if b.Name == name {
				return b, nil
			}
			if b.CliID == name {
				byID = b
			}
		}
	}
	if byID != nil {
		return byID, nil
	}
	return nil, fmt.Errorf("no branch %s in the workspace", name)
}

// jsonStatus is what `status --json` prints.
type jsonStatus struct {
	Branch    string     `json:"branch"`
	Exists    bool       `json:"exists"`
	Title     *string    `json:"title"`
	Author    *string    `json:"author"`
	UpdatedAt *time.Time `json:"updated_at"`
	// InWorkspace is false when the branch is deleted or unapplied; RenamedTo is its new name after a rename.
	InWorkspace bool    `json:"in_workspace"`
	RenamedTo   *string `json:"renamed_to"`
	Stale       bool    `json:"stale"`
}

func status(ctx context.Context, args []string, env Env) error {
	fs := flags("status", "buti walkthrough status <branch> [--json]", env)
	asJSON := fs.Bool("json", false, "print the status as JSON")
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, st, moved, err := followed(ctx, env)
	if err != nil {
		return err
	}
	out := jsonStatus{Branch: pos[0]}
	name := pos[0]
	if to, ok := moved[name]; ok {
		name = to
	}
	w, err := s.Get(name)
	switch {
	case errors.Is(err, walkthrough.ErrNotFound):
	case err != nil:
		return err
	default:
		loc := walkthrough.Locate(w, st)
		out.Exists, out.Title, out.Author, out.UpdatedAt = true, &w.Title, &w.Author, &w.UpdatedAt
		out.InWorkspace, out.Stale = loc.Branch != nil, loc.Stale
		if name != pos[0] {
			out.RenamedTo = &name
		} else if loc.Renamed {
			out.RenamedTo = &loc.Branch.Name
		}
	}
	if *asJSON {
		return writeJSON(env.Stdout, out)
	}
	switch {
	case !out.Exists:
		fmt.Fprintf(env.Stdout, "%s has no walkthrough\n", out.Branch)
		return nil
	case !out.InWorkspace:
		fmt.Fprintf(env.Stdout, "%s: walkthrough from %s; the branch is not in the workspace\n", out.Branch, when(w))
	case out.RenamedTo != nil:
		fmt.Fprintf(env.Stdout, "%s: walkthrough from %s; the branch is now %s", out.Branch, when(w), *out.RenamedTo)
		if out.Stale {
			fmt.Fprint(env.Stdout, " and has new commits")
		}
		fmt.Fprintln(env.Stdout)
	case out.Stale:
		fmt.Fprintf(env.Stdout, "%s: walkthrough from %s; outdated, the branch has changed since\n", out.Branch, when(w))
	default:
		fmt.Fprintf(env.Stdout, "%s: walkthrough from %s; up to date\n", out.Branch, when(w))
	}
	return nil
}

func when(w walkthrough.Walkthrough) string {
	return w.UpdatedAt.Local().Format("2006-01-02 15:04") + " by " + w.Author
}

func clearWalkthrough(args []string, env Env) error {
	fs := flags("clear", "buti walkthrough clear <branch>", env)
	pos, err := parse(fs, args, 1)
	if err != nil {
		return err
	}
	s, err := env.Store()
	if err != nil {
		return err
	}
	if err := s.Delete(pos[0]); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "cleared the walkthrough of %s\n", pos[0])
	return nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
