package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func envOf(m map[string]string) Env { return func(k string) string { return m[k] } }

// noZed hides any Zed installed on the machine running the tests.
func noZed(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	apps := ZedApps
	ZedApps = func() []string { return nil }
	t.Cleanup(func() { ZedApps = apps })
}

func TestResolve(t *testing.T) {
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"visual wins", map[string]string{"VISUAL": "code -w", "EDITOR": "vim", "TERM_PROGRAM": "zed"}, "code -w"},
		{"editor", map[string]string{"EDITOR": "nvim"}, "nvim"},
		{"zed terminal", map[string]string{"TERM_PROGRAM": "zed"}, "zed"},
		{"other terminal", map[string]string{"TERM_PROGRAM": "iTerm.app"}, "vi"},
		{"blank is unset", map[string]string{"VISUAL": "  ", "EDITOR": "nano"}, "nano"},
		{"nothing", nil, "vi"},
	} {
		if got := Resolve(envOf(tc.env)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestClassify(t *testing.T) {
	for ed, want := range map[string]Kind{
		"zed": Zed, "/usr/local/bin/zed --wait": Zed, "zeditor": Zed, "zed-preview": Zed, "zedit": Zed,
		"code -w": VSCode, "cursor": VSCode, "windsurf": VSCode, "codium": VSCode, "code-insiders": VSCode,
		"subl -w": Sublime, "vim": Terminal, "nvim": Terminal, "emacs -nw": Terminal, "kak": Terminal,
		"hx": Helix, "mate": Unknown, "": Unknown,
	} {
		if got := Classify(ed); got != want {
			t.Errorf("Classify(%q) = %v, want %v", ed, got, want)
		}
	}
}

func TestOpenCommand(t *testing.T) {
	noZed(t)
	for _, tc := range []struct {
		editor   string
		line     int
		paths    []string
		script   string
		args     []string
		detached bool
	}{
		{"zed", 12, []string{"/r/a.go"}, `zed "$@"`, []string{"/r/a.go:12"}, true},
		{"zed --wait", 0, []string{"/r/a.go"}, `zed "$@"`, []string{"/r/a.go"}, true},
		{"code -w", 3, []string{"a", "b"}, `code "$@"`, []string{"-g", "a:3", "b"}, true},
		{"subl -w", 3, []string{"a"}, `subl "$@"`, []string{"a:3"}, true},
		{"vim -p", 7, []string{"a", "b"}, `vim -p "$@"`, []string{"+7", "a", "b"}, false},
		{"hx", 7, []string{"a"}, `hx "$@"`, []string{"a:7"}, false},
		{"mate -w", 7, []string{"a"}, `mate -w "$@"`, []string{"a"}, false},
	} {
		o := OpenCommand(tc.editor, tc.line, tc.paths...)
		want := append([]string{"sh", "-c", tc.script, "editor"}, tc.args...)
		if !reflect.DeepEqual(o.Cmd.Args, want) || o.Detached != tc.detached {
			t.Errorf("%q: got %q detached=%v, want %q detached=%v", tc.editor, o.Cmd.Args, o.Detached, want, tc.detached)
		}
	}
}

func TestZedBinary(t *testing.T) {
	if got, err := ZedBinary(envOf(map[string]string{"EDITOR": "/opt/zed --wait"})); err != nil || got != "/opt/zed" {
		t.Errorf("got %q, %v", got, err)
	}
	noZed(t)
	if _, err := ZedBinary(envOf(map[string]string{"EDITOR": "vim"})); err == nil {
		t.Error("want an error without zed")
	}
}

// Zed's terminal doesn't put zed on the PATH: the CLI inside the installed app stands in for it.
func TestZedFromTheApp(t *testing.T) {
	noZed(t)
	cli := filepath.Join(t.TempDir(), "Zed Preview.app", "Contents", "MacOS", "cli")
	if err := os.MkdirAll(filepath.Dir(cli), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cli, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	ZedApps = func() []string { return []string{"/nowhere/cli", cli} }

	env := envOf(map[string]string{"TERM_PROGRAM": "zed"})
	if got, err := ZedBinary(env); err != nil || got != cli {
		t.Errorf("ZedBinary: got %q, %v, want %q", got, err, cli)
	}
	o := OpenCommand(Resolve(env), 3, "/r/a.go")
	want := []string{"sh", "-c", "'" + cli + `' "$@"`, "editor", "/r/a.go:3"}
	if !reflect.DeepEqual(o.Cmd.Args, want) || !o.Detached {
		t.Errorf("OpenCommand: got %q detached=%v, want %q", o.Cmd.Args, o.Detached, want)
	}
}
