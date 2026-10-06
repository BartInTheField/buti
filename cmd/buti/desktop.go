package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/bartinthefield/buti/internal/desktop"
)

// runDesktop starts the loopback API and the Tauri shell.
// `but` is not required to be on PATH here: a missing CLI is reported by GET /status
// so the window can show it.
func runDesktop(dir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("buti desktop", flag.ContinueOnError)
	fs.SetOutput(stderr)
	serve := fs.Bool("serve", false, "serve the local API and wait; do not open a window")
	dev := fs.Bool("dev", false, "open the Tauri dev window (npm run tauri dev)")
	app := fs.String("app", "", "path to the Buti desktop binary")
	port := fs.Int("port", 0, "loopback port; 0 chooses a free port")
	desktopDir := fs.String("desktop-dir", "", "path to the desktop/ frontend, for --dev")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: buti [-C dir] desktop [--serve | --dev] [--app path] [--port n]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "buti desktop: unexpected arguments")
		fs.Usage()
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := desktop.Run(ctx, desktop.Config{
		Dir:        dir,
		Port:       *port,
		ServeOnly:  *serve,
		Dev:        *dev,
		AppPath:    *app,
		DesktopDir: *desktopDir,
		Stdout:     stdout,
		Stderr:     stderr,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
