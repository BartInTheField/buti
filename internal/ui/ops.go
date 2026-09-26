package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
)

const opTimeout = 2 * time.Minute

// selectIntent says what to select once an operation's reload arrives.
type selectIntent int

const (
	keepSelection selectIntent = iota
	selectNew                  // the first commit or branch that did not exist before
	selectArea                 // the uncommitted area
)

type opDoneMsg struct {
	title  string
	output string
	err    error
	intent selectIntent
}

// runOp runs one mutation at a time in the background, then reloads.
func (m *Model) runOp(title string, intent selectIntent, fn func(context.Context, *but.Client) error) tea.Cmd {
	return m.runOpOut(title, intent, func(ctx context.Context, c *but.Client) (string, error) {
		return "", fn(ctx, c)
	})
}

// runOpOut is runOp for operations whose output is worth showing, like undo.
func (m *Model) runOpOut(title string, intent selectIntent, fn func(context.Context, *but.Client) (string, error)) tea.Cmd {
	if m.busy != "" {
		return m.notify(toastError, "Still working on: "+m.busy)
	}
	m.busy = title
	m.target = nil
	m.pendingBefore = m.knownKeys()
	client := m.client
	return tea.Batch(m.spinner.Tick, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		out, err := fn(ctx, client)
		return opDoneMsg{title: title, output: out, err: err, intent: intent}
	})
}

// execInteractive suspends the TUI to run cmd in the terminal (editors, shells).
func (m *Model) execInteractive(title string, intent selectIntent, cmd *exec.Cmd) tea.Cmd {
	if m.busy != "" {
		return m.notify(toastError, "Still working on: "+m.busy)
	}
	m.busy = title
	m.target = nil
	m.pendingBefore = m.knownKeys()
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return opDoneMsg{title: title, err: err, intent: intent}
	})
}

func (m *Model) handleOpDone(msg opDoneMsg) tea.Cmd {
	m.busy = ""
	m.pendingIntent = msg.intent
	cmds := []tea.Cmd{m.fetchStatus()}
	if msg.err != nil {
		m.pendingIntent = keepSelection
		cmds = append(cmds, m.notify(toastError, firstLines(msg.err.Error(), 6)))
	} else {
		m.marks = map[string]entity{}
		text := msg.title
		if out := firstLines(msg.output, 2); out != "" {
			text = out
		}
		cmds = append(cmds, m.notify(toastSuccess, text))
	}
	return tea.Batch(cmds...)
}

// knownKeys snapshots every entity key, so selectNew can find what an operation created.
func (m *Model) knownKeys() map[string]bool {
	keys := map[string]bool{}
	for _, l := range m.lanes {
		for _, e := range l.items {
			keys[e.key()] = true
		}
	}
	return keys
}

// startCommit asks for a message (unless empty) and commits ids at the placement.
func (m *Model) startCommit(ids []string, at but.Placement, empty bool, desc string) tea.Cmd {
	commit := func(msg string) tea.Cmd {
		return m.runOp(desc, selectNew, func(ctx context.Context, c *but.Client) error {
			return c.Commit(ctx, ids, msg, at)
		})
	}
	if empty {
		return commit("")
	}
	cm := newComposer(composeCommit, desc, "", commit)
	cm.onEditor = func() tea.Cmd {
		args := append(append([]string{"commit"}, ids...), at.Args()...)
		return m.execInteractive(desc, selectNew, m.client.Command(args...))
	}
	m.openModal(cm)
	return nil
}

func (m *Model) openComposer(kind composeKind, desc, initial string, onSubmit func(string) tea.Cmd) tea.Cmd {
	m.openModal(newComposer(kind, desc, initial, onSubmit))
	return nil
}

func editorCommand(path string) *exec.Cmd {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	return exec.Command("sh", "-c", editor+` "$1"`, "editor", path)
}

// openExternal opens a file with the desktop's default application, detached.
func openExternal(path string) error {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	cmd := exec.Command(opener, path)
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// shellCommand runs a line in the shell, pausing afterwards so the output can be read.
func shellCommand(dir, line string) *exec.Cmd {
	script := line + `; s=$?; printf '\n\033[2m[exit %d] press enter to return to buti\033[0m' "$s"; read -r _`
	cmd := exec.Command("sh", "-c", script)
	cmd.Dir = dir
	return cmd
}

// splitArgs splits a command line on whitespace, honouring single and double quotes.
func splitArgs(s string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inArg := false
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, inArg = r, true
		case r == ' ' || r == '\t':
			if inArg {
				args = append(args, cur.String())
				cur.Reset()
				inArg = false
			}
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated %c quote", quote)
	}
	if inArg {
		args = append(args, cur.String())
	}
	return args, nil
}

var placementNone = but.Placement{}

func placementBranch(name string) but.Placement { return but.Placement{Branch: name} }
