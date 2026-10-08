// Package editor resolves the user's editor, builds "open this file at a line" commands for it, and prepares the
// files for a side-by-side diff in Zed.
package editor

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Kind groups editors by how they take a file and a line, and whether they own the terminal.
type Kind int

const (
	Unknown  Kind = iota
	Zed           // zed, zeditor, zed-preview, zedit
	VSCode        // code, cursor, windsurf and other forks
	Sublime       // subl
	Terminal      // vi, nvim, nano and the like: they take over the terminal
	Helix         // hx: a terminal editor with its own line syntax
)

// Env looks up an environment variable, like os.Getenv.
type Env func(string) string

// Resolve returns the editor command line: $VISUAL, then $EDITOR, then Zed when running in its terminal, then vi.
func Resolve(env Env) string {
	for _, k := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(env(k)); v != "" {
			return v
		}
	}
	if InZed(env) {
		return "zed"
	}
	return "vi"
}

// InZed reports whether buti runs in Zed's terminal.
func InZed(env Env) bool { return env("TERM_PROGRAM") == "zed" }

// ZedApps are where Zed's command line tool is when it was never put on the PATH ("cli: install"), which Zed's own
// terminal doesn't do either. Tests replace it.
var ZedApps = func() []string {
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		var out []string
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			for _, app := range []string{"Zed.app", "Zed Preview.app"} {
				out = append(out, filepath.Join(dir, app, "Contents", "MacOS", "cli"))
			}
		}
		return out
	}
	return []string{filepath.Join(home, ".local", "bin", "zed"), filepath.Join(home, ".local", "zed.app", "bin", "zed"),
		filepath.Join(home, ".local", "zed-preview.app", "bin", "zed")}
}

// zedPath finds the Zed binary an editor command names: a path or a name on the PATH as given, else the CLI in an
// installed Zed.
func zedPath(word string) (string, bool) {
	if strings.ContainsRune(word, filepath.Separator) {
		return word, true
	}
	if _, err := exec.LookPath(word); err == nil {
		return word, true
	}
	for _, p := range ZedApps() {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	return "", false
}

// Classify reports the kind of an editor command line, from the base name of its first word.
func Classify(editor string) Kind {
	switch binaryName(editor) {
	case "zed", "zeditor", "zed-preview", "zedit":
		return Zed
	case "code", "code-insiders", "cursor", "windsurf", "codium":
		return VSCode
	case "subl":
		return Sublime
	case "vi", "vim", "nvim", "nano", "emacs", "micro", "kak":
		return Terminal
	case "hx":
		return Helix
	}
	return Unknown
}

// Detached reports whether the editor is a GUI that returns at once, so the TUI need not be suspended.
func (k Kind) Detached() bool { return k == Zed || k == VSCode || k == Sublime }

func binaryName(editor string) string {
	f := strings.Fields(editor)
	if len(f) == 0 {
		return ""
	}
	return filepath.Base(f[0])
}

// Name is the editor's binary name, for messages.
func Name(editor string) string { return binaryName(editor) }

// Open is a command that opens paths in an editor.
type Open struct {
	Cmd      *exec.Cmd
	Detached bool // start without suspending the TUI
}

// OpenCommand builds the command opening paths in editor, the first at line (0 for none).
func OpenCommand(editor string, line int, paths ...string) Open {
	kind := Classify(editor)
	editor = strings.TrimSpace(editor)
	if kind.Detached() {
		editor = stripWait(editor)
	}
	if f := strings.Fields(editor); kind == Zed {
		if p, ok := zedPath(f[0]); ok {
			editor = strings.Join(append([]string{shellQuote(p)}, f[1:]...), " ")
		}
	}
	var args []string
	for i, p := range paths {
		at := 0
		if i == 0 {
			at = line
		}
		args = append(args, pathArgs(kind, p, at)...)
	}
	script := editor + ` "$@"`
	return Open{
		Cmd:      exec.Command("sh", append([]string{"-c", script, "editor"}, args...)...),
		Detached: kind.Detached(),
	}
}

func pathArgs(kind Kind, path string, line int) []string {
	if line <= 0 {
		return []string{path}
	}
	at := path + ":" + strconv.Itoa(line)
	switch kind {
	case Zed, Sublime, Helix:
		return []string{at}
	case VSCode:
		return []string{"-g", at}
	case Terminal:
		return []string{"+" + strconv.Itoa(line), path}
	}
	return []string{path}
}

// stripWait drops -w and --wait: a GUI editor that waits would keep the process around for nothing.
func stripWait(editor string) string {
	var keep []string
	for _, w := range strings.Fields(editor) {
		if w != "-w" && w != "--wait" {
			keep = append(keep, w)
		}
	}
	return strings.Join(keep, " ")
}

// shellQuote quotes s for sh, for a path like "Zed Preview.app".
func shellQuote(s string) string {
	if !strings.ContainsAny(s, " '\"$`\\") {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ZedBinary finds Zed's command line tool: the editor's own when it is Zed, else `zed` on the PATH, else the one in
// an installed Zed.
func ZedBinary(env Env) (string, error) {
	word := "zed"
	if ed := Resolve(env); Classify(ed) == Zed {
		word = strings.Fields(ed)[0]
	}
	if p, ok := zedPath(word); ok {
		return p, nil
	}
	return "", errors.New(`Zed's command line tool was not found: install it from Zed with "cli: install"`)
}
