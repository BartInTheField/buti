package desktop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/bartinthefield/buti/internal/but"
)

// Config is how `buti desktop` starts the API and, unless ServeOnly, the Tauri window.
type Config struct {
	Dir        string
	Port       int
	ServeOnly  bool
	Dev        bool
	AppPath    string
	DesktopDir string
	Stdout     io.Writer
	Stderr     io.Writer
	// OnReady runs after the API is accepting connections, before the window blocks.
	OnReady func(url, token string)
}

// Run binds the loopback API, then either waits (ServeOnly), runs `tauri dev`, or
// execs the built shell. The API stops when the window exits or ctx is cancelled.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Stdout == nil {
		cfg.Stdout = io.Discard
	}
	if cfg.Stderr == nil {
		cfg.Stderr = io.Discard
	}
	mode, target, err := resolveLaunch(cfg)
	if err != nil {
		return err
	}
	client := but.New(cfg.Dir)
	srv, err := Start(client, cfg.Port)
	if err != nil {
		return err
	}
	defer func() {
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shut)
	}()

	fmt.Fprintf(cfg.Stderr, "buti desktop: %s\n", srv.URL)
	if mode == "serve" {
		// The token stays off stdout so a pipe of status JSON is not mixed with it.
		// The invoking terminal is the local user who started the process.
		fmt.Fprintf(cfg.Stderr, "buti desktop: token %s\n", srv.Token)
		fmt.Fprintf(cfg.Stderr, "buti desktop: serving until interrupted\n")
	}
	if cfg.OnReady != nil {
		cfg.OnReady(srv.URL, srv.Token)
	}

	switch mode {
	case "serve":
		<-ctx.Done()
		return nil
	case "dev":
		return runChild(ctx, cfg, target, srv, "npm", "run", "tauri", "dev")
	default:
		return runChild(ctx, cfg, "", srv, target)
	}
}

func runChild(ctx context.Context, cfg Config, dir string, srv *Server, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(),
		"BUTI_API_URL="+srv.URL,
		"BUTI_API_TOKEN="+srv.Token,
	)
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("buti desktop: %s: %w", name, err)
	}
	return nil
}

// resolveLaunch checks the window mode before listening, so a missing shell
// does not leave an API up with nobody to hand the token to.
func resolveLaunch(cfg Config) (mode, target string, err error) {
	if cfg.ServeOnly && cfg.Dev {
		return "", "", errors.New("buti desktop: pass only one of --serve and --dev")
	}
	if cfg.ServeOnly {
		return "serve", "", nil
	}
	if cfg.Dev {
		dir := cfg.DesktopDir
		if dir == "" {
			dir = findDesktopDir()
		}
		if _, err := os.Stat(filepath.Join(dir, "package.json")); err != nil {
			return "", "", fmt.Errorf("buti desktop: no desktop frontend at %s (see docs/desktop.md)", dir)
		}
		if _, err := exec.LookPath("npm"); err != nil {
			return "", "", errors.New("buti desktop: npm is not on PATH (see docs/desktop.md)")
		}
		return "dev", dir, nil
	}
	app := cfg.AppPath
	if app == "" {
		app = os.Getenv("BUTI_DESKTOP_APP")
	}
	if app == "" {
		app = findBuiltApp(findDesktopDir())
	}
	if app == "" {
		return "", "", errors.New("buti desktop: Tauri shell is not built; pass --dev, --serve, or --app (see docs/desktop.md)")
	}
	info, err := os.Stat(app)
	if err != nil {
		return "", "", fmt.Errorf("buti desktop: app %s: %w", app, err)
	}
	if info.IsDir() {
		return "", "", fmt.Errorf("buti desktop: app %s is a directory", app)
	}
	return "app", app, nil
}

func findDesktopDir() string {
	if dir := os.Getenv("BUTI_DESKTOP_DIR"); dir != "" {
		return dir
	}
	if cwd, err := os.Getwd(); err == nil {
		if d := walkForDesktop(cwd); d != "" {
			return d
		}
	}
	if exe, err := os.Executable(); err == nil {
		if d := walkForDesktop(filepath.Dir(exe)); d != "" {
			return d
		}
	}
	return "desktop"
}

func walkForDesktop(start string) string {
	dir := start
	for {
		cand := filepath.Join(dir, "desktop")
		info, err := os.Stat(filepath.Join(cand, "package.json"))
		if err == nil && !info.IsDir() {
			return cand
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func appBinaryName() string {
	name := "buti-desktop"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

// findBuiltApp looks for the cargo/tauri binary next to the frontend. The macOS
// bundle name follows Tauri's product name, which is not the cargo package name.
func findBuiltApp(desktopDir string) string {
	name := appBinaryName()
	candidates := []string{
		filepath.Join(desktopDir, "src-tauri", "target", "release", name),
		filepath.Join(desktopDir, "src-tauri", "target", "debug", name),
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates,
			filepath.Join(desktopDir, "src-tauri", "target", "release", "bundle", "macos", "Buti.app", "Contents", "MacOS", "buti-desktop"),
			filepath.Join(desktopDir, "src-tauri", "target", "release", "bundle", "macos", "Buti.app", "Contents", "MacOS", "Buti"),
		)
	}
	for _, c := range candidates {
		info, err := os.Stat(c)
		if err == nil && !info.IsDir() {
			return c
		}
	}
	return ""
}
