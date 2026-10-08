package ui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/editor"
)

// startDetached starts a GUI program and leaves it running; tests replace it.
var startDetached = defaultStartDetached

func defaultStartDetached(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

// openInEditor opens paths in the user's editor, the first at line (0 for none). A GUI editor is started on the
// side, so the TUI keeps running; a terminal editor takes over the terminal until it exits.
func (m *Model) openInEditor(label string, line int, paths ...string) tea.Cmd {
	ed := editor.Resolve(os.Getenv)
	o := editor.OpenCommand(ed, line, paths...)
	if !o.Detached {
		return m.execInteractive("Edit "+label, keepSelection, o.Cmd)
	}
	o.Cmd.Dir = m.client.Dir
	if err := startDetached(o.Cmd); err != nil {
		return m.notify(toastError, err.Error())
	}
	return m.notify(toastInfo, "Opened "+label+" in "+editor.Name(ed))
}

// openAt is `o`: the entity's file, at the line cursor in the details pane, else at a hunk's first change.
func (m *Model) openAt(e entity) tea.Cmd {
	return m.openInEditor(e.label, m.openLine(e), m.pathOf(e))
}

// openLine is the new-side line to open e at, 0 for none.
func (m *Model) openLine(e entity) int {
	rel := e.label
	if e.kind == entHunk {
		rel, _, _ = strings.Cut(e.label, " ")
	}
	d := &m.det
	if rows := d.layout.rows; (d.focused || d.full) && d.cursor >= 0 && d.cursor < len(rows) {
		if r := rows[d.cursor]; r.isLine() && d.layout.hunks[r.hunk].path == rel {
			return newSideLine(rows, d.cursor, d.layout.hunks[r.hunk].text)
		}
	}
	if e.kind == entHunk {
		for _, h := range d.doc.hunks {
			if h.id == e.id {
				return firstChange(h.text)
			}
		}
	}
	return 0
}

// newSideLine is the line of the file that row i shows. A removed line has none: it gets the next line that is
// there, else the hunk's last one.
func newSideLine(rows []diffRow, i int, hunk string) int {
	r := rows[i]
	if r.sign != '-' && r.new > 0 {
		return r.new
	}
	for _, n := range rows[i+1:] {
		if n.hunk != r.hunk {
			break
		}
		if n.isLine() && n.new > 0 {
			return n.new
		}
	}
	for j := i - 1; j >= 0 && rows[j].hunk == r.hunk; j-- {
		if rows[j].isLine() && rows[j].new > 0 {
			return rows[j].new
		}
	}
	return hunkNewStart(hunk)
}

// hunkNewStart reads the new start line from a hunk's "@@ -a,b +c,d @@" header.
func hunkNewStart(hunk string) int {
	head, _, _ := strings.Cut(hunk, "\n")
	for _, f := range strings.Fields(head) {
		if strings.HasPrefix(f, "+") {
			n, _, _ := strings.Cut(f[1:], ",")
			v, _ := strconv.Atoi(n)
			return v
		}
	}
	return 0
}

// firstChange is the line of a hunk's first added line, or its start when it only removes.
func firstChange(hunk string) int {
	start := hunkNewStart(hunk)
	n := start
	_, body, _ := strings.Cut(hunk, "\n")
	for _, l := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(l, "+"):
			return n
		case strings.HasPrefix(l, " "):
			n++
		}
	}
	return start
}

// zedDiffable is what `Z` can open in Zed.
var zedDiffable = one(entFile, entHunk, entArea, entCommittedFile, entCommit, entBranch)

// fullDetails is `D`: full-screen details, except in Zed's terminal, where Zed's own diff view is a better full
// screen. With nothing to diff, or to leave full screen, it still toggles.
func (m *Model) fullDetails(sel []entity) tea.Cmd {
	if editor.InZed(os.Getenv) && !m.det.full && zedDiffable(m, sel) {
		return m.zedDiff(sel[0])
	}
	return m.setDetailsFull(!m.det.full)
}

// zedDiffMsg reports a diff handed to Zed, or why it could not be.
type zedDiffMsg struct {
	label string
	err   error
}

func (m *Model) receiveZedDiff(msg zedDiffMsg) tea.Cmd {
	if msg.err != nil {
		return m.notify(toastError, firstLines(msg.err.Error(), 4))
	}
	return m.notify(toastInfo, "Opened the diff of "+msg.label+" in Zed")
}

// zedDiff is `Z`: it writes the old and new side of the entity's files and starts Zed on them, off the UI thread.
func (m *Model) zedDiff(e entity) tea.Cmd {
	client := m.client
	var tip string // the commit a branch's new side is read at
	if e.kind == entBranch {
		for _, st := range m.status.Stacks {
			for _, b := range st.Branches {
				if b.Name == e.branch && len(b.Commits) > 0 {
					tip = b.Commits[0].CommitID // newest first, as `but status` lists them
				}
			}
		}
		if tip == "" {
			return m.notify(toastError, e.branch+" has no commits to diff")
		}
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
		defer cancel()
		return zedDiffMsg{label: e.label, err: runZedDiff(ctx, client, e, tip)}
	}
}

func runZedDiff(ctx context.Context, c *but.Client, e entity, tip string) error {
	zed, err := editor.ZedBinary(os.Getenv)
	if err != nil {
		return err
	}
	var args []string
	switch e.kind {
	case entFile, entHunk, entArea:
		d, err := c.Diff(ctx, "")
		if err != nil {
			return err
		}
		files := editor.Files(d)
		switch e.kind {
		case entFile:
			files = onlyPath(files, e.label)
		case entHunk:
			path, _, _ := strings.Cut(e.label, " ")
			files = onlyPath(files, path)
		}
		if len(files) == 0 {
			return errors.New("No text changes to show in Zed")
		}
		tmp, err := editor.NewDir(c.Dir)
		if err != nil {
			return err
		}
		if err := editor.WriteSource(tmp, editor.Source{Repo: c.Dir}); err != nil {
			return err
		}
		pairs, err := editor.Worktree(tmp, c.Dir, files)
		if err != nil {
			return err
		}
		args = editor.ZedArgs(pairs)
	case entCommit, entCommittedFile, entBranch:
		sha := tip
		label := strings.NewReplacer("/", "_", " ", "_").Replace(e.branch)
		if e.kind != entBranch {
			if e.commit == nil {
				return errors.New("No commit to diff")
			}
			sha, label = e.commit.CommitID, e.commit.ShortID()
		}
		d, err := c.Diff(ctx, e.id)
		if err != nil {
			return err
		}
		files := editor.Files(d)
		if e.kind == entCommittedFile {
			files = onlyPath(files, e.label)
		}
		if len(files) == 0 {
			return errors.New("No text changes to show in Zed")
		}
		tmp, err := editor.NewDir(c.Dir)
		if err != nil {
			return err
		}
		src := editor.Source{Commit: sha, Repo: c.Dir}
		if e.kind == entBranch {
			src = editor.Source{Branch: e.branch, Repo: c.Dir}
		}
		if err := editor.WriteSource(tmp, src); err != nil {
			return err
		}
		oldDir, newDir, err := editor.Commit(tmp, label, files, func(p string) (string, error) {
			return editor.GitShow(ctx, c.Dir, sha, p)
		})
		if err != nil {
			return err
		}
		pair := [2]string{oldDir, newDir}
		if e.kind == entCommittedFile {
			pair = [2]string{filepath.Join(oldDir, e.label), filepath.Join(newDir, e.label)}
		}
		args = editor.ZedArgs([][2]string{pair})
	default:
		return errors.New("Open a file, commit or branch in Zed")
	}
	cmd := exec.Command(zed, args...)
	cmd.Dir = c.Dir
	return startDetached(cmd)
}

func onlyPath(files []editor.File, path string) []editor.File {
	var out []editor.File
	for _, f := range files {
		if f.Path == path {
			out = append(out, f)
		}
	}
	return out
}
