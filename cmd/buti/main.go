// Command buti is a terminal UI for GitButler.
package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
	"github.com/bartinthefield/buti/internal/reviewcli"
	"github.com/bartinthefield/buti/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// flagSet reports whether name was passed on the command line, rather than left at its default.
func flagSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

// run runs buti with args and returns the exit code.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("buti", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("C", ".", "run as if started in `dir`")
	diff := fs.Bool("diff", false, "show the details pane on start")
	remember := fs.Bool("remember-selection", false, "restore the selection from the last session")
	showVersion := fs.Bool("version", false, "print the version and exit")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: buti [-C dir] [--diff] [--remember-selection] [--version] [target]")
		fmt.Fprintln(stderr, "       buti [-C dir] desktop [--serve | --dev] [--app path]")
		fmt.Fprintln(stderr, "       buti [-C dir] review <command> ...")
		fmt.Fprintln(stderr, "       buti [-C dir] skill <command> ...")
		fs.PrintDefaults()
		fmt.Fprintln(stderr, "\nReview comments, for coding agents (no TUI):")
		fmt.Fprintln(stderr, indentLines(reviewcli.Usage))
		fmt.Fprintln(stderr, "\nAgent skills, /buti-resolve and /buti-review:")
		fmt.Fprintln(stderr, indentLines(skillUsage))
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	if *showVersion {
		fmt.Fprintln(stdout, "buti", version)
		return 0
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(stderr, "buti:", err)
		return 1
	}
	// A branch called review or skill can still be selected on start with `buti -- review`.
	rest := fs.Args()
	if len(rest) > 0 && !slices.Contains(args[:len(args)-len(rest)], "--") {
		switch rest[0] {
		case "desktop":
			return runDesktop(abs, flagSet(fs, "C"), rest[1:], stdout, stderr)
		case "review":
			return runReview(abs, rest[1:], stdout, stderr)
		case "skill":
			return runSkill(abs, rest[1:], stdout, stderr)
		}
	}
	if _, err := exec.LookPath("but"); err != nil {
		fmt.Fprintln(stderr, "buti: the GitButler CLI (`but`) must be on PATH")
		return 1
	}

	opts := ui.Options{Target: fs.Arg(0), ShowDetails: *diff, Version: version}
	// Builds without a release version (go run, go install) have nothing to update to.
	if version != "dev" {
		opts.UpdateCheck = os.Getenv("BUTI_NO_UPDATE_CHECK") == ""
		opts.UpdateCache = updateCache()
	}
	if *remember {
		opts.StateFile = stateFile(abs)
	}
	// Outside a git repository there is nowhere to keep review comments; buti works without them.
	if store, err := review.Open(abs); err == nil {
		opts.Review = store
	}
	p := tea.NewProgram(ui.New(but.New(abs), opts))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(stderr, "buti:", err)
		return 1
	}
	return 0
}

// runReview runs `buti review`, which needs no terminal, and `but` only for the commands that read the workspace.
func runReview(dir string, args []string, stdout, stderr io.Writer) int {
	if reviewcli.NeedsBut(args) {
		if _, err := exec.LookPath("but"); err != nil {
			fmt.Fprintln(stderr, "buti review: the GitButler CLI (`but`) must be on PATH")
			return 1
		}
	}
	return reviewcli.Run(context.Background(), args, reviewcli.Env{
		Stdout: stdout,
		Stderr: stderr,
		Store:  func() (*review.Store, error) { return review.Open(dir) },
		But:    but.New(dir),
	})
}

// updateCache is where the latest release version is remembered between runs.
func updateCache() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "buti", "latest-version")
}

// stateFile is where the selection of the repository at dir is remembered.
func stateFile(dir string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "state")
	}
	sum := sha1.Sum([]byte(dir))
	return filepath.Join(base, "buti", "selection-"+hex.EncodeToString(sum[:8]))
}
