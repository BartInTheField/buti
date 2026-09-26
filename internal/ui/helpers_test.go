package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/cursor"
	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

func testStatus() *but.Status {
	return &but.Status{
		UncommittedChanges: []but.Change{
			{CliID: "f1", FilePath: "cmd/buti/main.go", ChangeType: "added"},
			{CliID: "f2", FilePath: "go.mod", ChangeType: "modified"},
		},
		Stacks: []but.Stack{
			{CliID: "s1", Branches: []but.Branch{
				{CliID: "b4", Name: "branch-4", BranchStatus: "completelyUnpushed",
					Commits: []but.Commit{{CliID: "c1", ChangeID: "chg1", CommitID: "abcdef123", Message: "first",
						Changes: []but.Change{{CliID: "c1:x", FilePath: "x.go", ChangeType: "added"}}}}},
				{CliID: "b1", Name: "branch-1", BranchStatus: "nothingToPush"},
			}},
			{CliID: "s2", Branches: []but.Branch{{CliID: "b2", Name: "branch-2", BranchStatus: "completelyUnpushed",
				Commits: []but.Commit{{CliID: "c2", ChangeID: "chg2", CommitID: "123456789", Message: "second"}}}}},
		},
	}
}

// fakeBut installs a `but` stand-in that logs its arguments and serves status.
type fakeBut struct {
	t   *testing.T
	log string
}

func newFakeBut(t *testing.T, s *but.Status) (*but.Client, *fakeBut) {
	t.Helper()
	b, _ := json.Marshal(s)
	return newFakeButJSON(t, string(b))
}

// newFakeButJSON serves status as given, for states Status can't marshal to.
func newFakeButJSON(t *testing.T, status string) (*but.Client, *fakeBut) {
	t.Helper()
	dir := t.TempDir()
	statusFile := filepath.Join(dir, "status.json")
	if err := os.WriteFile(statusFile, []byte(status), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeBut{t: t, log: filepath.Join(dir, "log")}
	script := "#!/bin/sh\n" +
		"case \"$1\" in\n" +
		"  status) cat '" + statusFile + "' ;;\n" +
		"  diff) echo '{\"changes\":[]}' ;;\n" +
		"  *) echo \"$*\" >> '" + f.log + "' ;;\n" +
		"esac\n"
	bin := filepath.Join(dir, "but")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c := but.New(dir)
	c.Bin = bin
	return c, f
}

// calls returns the mutating commands run so far.
func (f *fakeBut) calls() []string {
	b, _ := os.ReadFile(f.log)
	s := strings.TrimSpace(string(b))
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func (f *fakeBut) expect(want ...string) {
	f.t.Helper()
	got := f.calls()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		f.t.Fatalf("but calls:\n got  %q\n want %q", got, want)
	}
}

// harness drives a Model through Update, running returned commands like the runtime.
type harness struct {
	t    *testing.T
	m    Model
	but  *fakeBut      // nil against a real repository
	wait time.Duration // how long a command may take before it counts as a timer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, testStatus())
}

// newHarnessWith runs against a fake serving s, a *but.Status or raw status JSON.
func newHarnessWith(t *testing.T, s any) *harness {
	t.Helper()
	var c *but.Client
	var f *fakeBut
	if raw, ok := s.(string); ok {
		c, f = newFakeButJSON(t, raw)
	} else {
		c, f = newFakeBut(t, s.(*but.Status))
	}
	h := &harness{t: t, m: New(c, Options{}), but: f, wait: 150 * time.Millisecond}
	h.send(tea.WindowSizeMsg{Width: 140, Height: 36})
	h.run(h.m.fetchStatus())
	return h
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	nm, cmd := h.m.Update(msg)
	h.m = nm.(Model)
	h.run(cmd)
}

// run executes cmd and feeds its messages back. Commands that block (ticks,
// toast timers) are dropped after h.wait; animation frames (spinner, cursor blink)
// are dropped so they don't loop.
func (h *harness) run(cmd tea.Cmd) {
	queue := []tea.Cmd{cmd}
	for steps := 0; len(queue) > 0 && steps < 200; steps++ {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		done := make(chan tea.Msg, 1)
		go func() { done <- c() }()
		var msg tea.Msg
		select {
		case msg = <-done:
		case <-time.After(h.wait):
			continue
		}
		switch msg := msg.(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case tickMsg, toastExpireMsg, spinner.TickMsg, cursor.BlinkMsg:
		default:
			nm, next := h.m.Update(msg)
			h.m = nm.(Model)
			queue = append(queue, next)
		}
	}
}

func (h *harness) keys(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		h.send(keyMsg(k))
	}
}

func (h *harness) typeText(s string) {
	for _, r := range s {
		h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func keyMsg(k string) tea.KeyPressMsg {
	special := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace}
	if c, ok := special[k]; ok {
		return tea.KeyPressMsg{Code: c}
	}
	if strings.HasPrefix(k, "ctrl+") {
		return tea.KeyPressMsg{Code: rune(k[5]), Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

// screen is the rendered view without styling.
func (h *harness) screen() string { return ansi.Strip(h.m.View().Content) }

// find returns the screen position of the first occurrence of text.
func (h *harness) find(text string) (x, y int) {
	h.t.Helper()
	for y, line := range strings.Split(h.screen(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return ansi.StringWidth(line[:i]), y
		}
	}
	h.t.Fatalf("%q not on screen:\n%s", text, h.screen())
	return 0, 0
}

func (h *harness) click(text string) {
	h.t.Helper()
	x, y := h.find(text)
	h.send(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	h.send(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
}

func (h *harness) dragTo(from, to string) {
	h.t.Helper()
	fx, fy := h.find(from)
	tx, ty := h.find(to)
	h.send(tea.MouseClickMsg{X: fx, Y: fy, Button: tea.MouseLeft})
	h.send(tea.MouseMotionMsg{X: tx, Y: ty, Button: tea.MouseLeft})
	h.send(tea.MouseReleaseMsg{X: tx, Y: ty, Button: tea.MouseLeft})
}

// selectText clicks an item and waits out the double-click window.
func (h *harness) selectText(text string) {
	h.t.Helper()
	h.click(text)
	h.m.lastClickKey = ""
}

// hover moves the cursor to an entity by key, as the arrow keys would.
func (h *harness) hover(key string) {
	h.t.Helper()
	if !h.m.selectKey(key) {
		h.t.Fatalf("no entity %q", key)
	}
}

func (h *harness) selectedKey() string { return h.m.selected().key() }
