// Command buti is a terminal UI for GitButler.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/ui"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	dir := flag.String("C", ".", "run as if started in `dir`")
	diff := flag.Bool("diff", false, "show the details pane on start")
	remember := flag.Bool("remember-selection", false, "restore the selection from the last session")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: buti [-C dir] [--diff] [--remember-selection] [--version] [target]")
		flag.PrintDefaults()
	}
	flag.Parse()

	if *showVersion {
		fmt.Println("buti", version)
		return
	}
	if _, err := exec.LookPath("but"); err != nil {
		fmt.Fprintln(os.Stderr, "buti: the GitButler CLI (`but`) must be on PATH")
		os.Exit(1)
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "buti:", err)
		os.Exit(1)
	}

	opts := ui.Options{Target: flag.Arg(0), ShowDetails: *diff}
	// Builds without a release version (go run, go install) have nothing to update to.
	if version != "dev" && os.Getenv("BUTI_NO_UPDATE_CHECK") == "" {
		opts.Version = version
		opts.UpdateCache = updateCache()
	}
	if *remember {
		opts.StateFile = stateFile(abs)
	}
	p := tea.NewProgram(ui.New(but.New(abs), opts))
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "buti:", err)
		os.Exit(1)
	}
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
