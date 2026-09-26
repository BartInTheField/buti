// Package ui implements the buti Bubble Tea interface.
package ui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/bartinthefield/buti/internal/but"
)

const (
	refreshInterval = 3 * time.Second
	doubleClickTime = 400 * time.Millisecond
	minLaneWidth    = 32
	maxLaneWidth    = 46
)

type focusArea int

const (
	focusFiles focusArea = iota
	focusLanes
)

// Options configure a session.
type Options struct {
	Target      string // CLI id or branch name to select on start
	ShowDetails bool   // open the details pane on start
	StateFile   string // when set, the selection is saved here on quit and restored on start
	Version     string // running release; when set, a newer release is offered on start
	UpdateCache string // where the latest release version is cached between runs
}

type (
	statusMsg struct {
		status *but.Status
		err    error
	}
	tickMsg struct{}
)

type Model struct {
	client *but.Client
	opts   Options
	status *but.Status
	err    error
	loaded bool
	focus  focusArea

	// Unstaged file tree (sidebar). fileCursor -1 is the "Unstaged" header: all changes.
	files      []fileRow
	collapsed  map[string]bool
	fileCursor int
	fileOffset int

	// Stack lanes; the last one is always the "new branch" drop lane.
	lanes        []lane
	lane         int // focused lane
	laneCursor   int // selected item within the focused lane
	laneOffset   int // first visible lane
	showFiles    map[string]bool
	showAllFiles bool

	marks  map[string]entity // by entity key
	target *targetMode
	modal  modal
	det    details

	busy          string
	spinner       spinner.Model
	toasts        []toast
	toastSeq      int
	pendingIntent selectIntent
	pendingBefore map[string]bool
	pendingKey    string

	// Mouse.
	lastClickAt  time.Time
	lastClickKey string
	press        *pressState
	drag         *dragState

	width, height int
	quitting      bool
}

func New(client *but.Client, opts Options) Model {
	m := Model{
		client:     client,
		opts:       opts,
		collapsed:  map[string]bool{},
		showFiles:  map[string]bool{},
		marks:      map[string]entity{},
		det:        newDetails(),
		spinner:    spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(titleStyle)),
		fileCursor: -1,
	}
	m.det.visible = opts.ShowDetails
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchStatus(), tick(), m.checkUpdate())
}

func (m Model) fetchStatus() tea.Cmd {
	return func() tea.Msg {
		s, err := m.client.Status(context.Background())
		return statusMsg{status: s, err: err}
	}
}

func tick() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m *Model) openModal(md modal) { m.modal = md }
func (m *Model) closeModal()        { m.modal = nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	// Keep the details pane on whatever is selected now.
	return m, tea.Batch(cmd, m.syncDetails(false))
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampFiles()
		m.clampLanes()
		return nil

	case tickMsg:
		if m.busy != "" {
			return tick()
		}
		return tea.Batch(m.fetchStatus(), tick())

	case statusMsg:
		m.err = msg.err
		if msg.err == nil {
			m.applyStatus(msg.status)
		}
		return nil

	case detailsMsg:
		m.det.receive(msg)
		m.det.rerender(m.hunkMarks())
		return nil

	case opDoneMsg:
		return m.handleOpDone(msg)

	case commandMsg:
		body := msg.output
		if msg.err != nil {
			body = errorStyle.Render(msg.err.Error())
		}
		if strings.TrimSpace(body) == "" {
			body = mutedStyle.Render("(no output)")
		}
		m.openModal(newOutput(msg.title, body))
		return m.fetchStatus()

	case branchesMsg:
		return m.showApplyPicker(msg)

	case prURLMsg:
		return m.showPR(msg)

	case oplogMsg:
		return m.showOplog(msg)

	case updateAvailableMsg:
		return m.offerUpdate(msg.latest)

	case updateStartedMsg:
		return m.notify(toastInfo, "Updating to "+msg.version+"…")

	case updateDoneMsg:
		return m.handleUpdateDone(msg)

	case toastExpireMsg:
		m.expireToast(msg.id)
		return nil

	case spinner.TickMsg:
		if m.busy == "" {
			return nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return cmd

	case tea.MouseMsg:
		if m.modal != nil {
			return m.modal.update(m, msg)
		}
		return m.handleMouse(msg)

	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		if m.modal != nil {
			return m.modal.update(m, msg)
		}
		if m.det.focused || m.det.full {
			if cmd, handled := m.handleDetailsKey(msg); handled {
				return cmd
			}
		}
		if m.target != nil {
			return m.handleTargetKey(msg)
		}
		return m.handleKey(msg)

	default:
		if m.modal != nil {
			return m.modal.update(m, msg)
		}
	}
	return nil
}

func (m *Model) quit() tea.Cmd {
	m.quitting = true
	if m.opts.StateFile != "" {
		if e := m.selected(); e.valid() {
			_ = os.MkdirAll(filepath.Dir(m.opts.StateFile), 0o755)
			_ = os.WriteFile(m.opts.StateFile, []byte(e.key()), 0o644)
		}
	}
	return tea.Quit
}

// applyStatus rebuilds the views from fresh status, keeping (or moving) the selection.
func (m *Model) applyStatus(s *but.Status) {
	prev := m.selected().key()
	m.status = s
	if m.target != nil {
		m.target.status = s
	}
	m.rebuild()
	// Drop marks whose entities are gone.
	live := m.knownKeys()
	for _, r := range m.files {
		live[r.entity().key()] = true
	}
	for k, e := range m.marks {
		if !live[k] && e.kind != entHunk {
			delete(m.marks, k)
		}
	}

	switch {
	case !m.loaded:
		m.loaded = true
		m.restoreInitialSelection()
	case m.pendingKey != "" && m.selectKey(m.pendingKey):
	case m.pendingIntent == selectArea:
		m.focus, m.fileCursor = focusFiles, -1
	case m.pendingIntent == selectNew && m.selectCreated():
	default:
		m.selectKey(prev)
	}
	m.pendingKey, m.pendingIntent, m.pendingBefore = "", keepSelection, nil
	m.clampFiles()
	m.clampLanes()
}

func (m *Model) rebuild() {
	if m.status == nil {
		return
	}
	prev := m.selected().key()
	m.files = buildFileRows(m.status.UncommittedChanges, m.collapsed)
	m.lanes = append(buildLanes(m.status, func(c *but.Commit) bool {
		return m.showAllFiles || m.showFiles[commitKey(c)]
	}), phantomLane())
	m.selectKey(prev)
}

func (m *Model) restoreInitialSelection() {
	if t := m.opts.Target; t != "" {
		for li, l := range m.lanes {
			for ii, e := range l.items {
				if e.id == t || (e.kind == entBranch && e.branch == t) {
					m.focus, m.lane, m.laneCursor = focusLanes, li, ii
					return
				}
			}
		}
		for i, r := range m.files {
			if r.change.CliID == t || r.path == t {
				m.focus, m.fileCursor = focusFiles, i
				return
			}
		}
	}
	if m.opts.StateFile != "" {
		if b, err := os.ReadFile(m.opts.StateFile); err == nil && m.selectKey(strings.TrimSpace(string(b))) {
			return
		}
	}
	if len(m.files) == 0 && len(m.lanes) > 1 {
		m.focus = focusLanes
	}
}

// selectKey selects the entity with the given key, reporting whether it exists.
func (m *Model) selectKey(key string) bool {
	if key == "" {
		return false
	}
	if key == (entity{kind: entArea}).key() {
		m.focus, m.fileCursor = focusFiles, -1
		return true
	}
	for i, r := range m.files {
		if r.entity().key() == key {
			m.focus, m.fileCursor = focusFiles, i
			return true
		}
	}
	for li, l := range m.lanes {
		for ii, e := range l.items {
			if e.key() == key {
				m.focus, m.lane, m.laneCursor = focusLanes, li, ii
				return true
			}
		}
	}
	return false
}

// selectCreated selects the first commit (or else branch) that an operation created.
func (m *Model) selectCreated() bool {
	for _, kind := range []entityKind{entCommit, entBranch} {
		for li, l := range m.lanes {
			for ii, e := range l.items {
				if e.kind == kind && !m.pendingBefore[e.key()] {
					m.focus, m.lane, m.laneCursor = focusLanes, li, ii
					return true
				}
			}
		}
	}
	return false
}

func (m *Model) selectEntity(e entity) tea.Cmd {
	if !m.selectKey(e.key()) {
		return m.notify(toastInfo, e.describe()+" is no longer there")
	}
	m.clampFiles()
	m.clampLanes()
	return nil
}

// selected is the entity under the cursor.
func (m *Model) selected() entity {
	if m.status == nil {
		return entity{}
	}
	if m.focus == focusFiles {
		if m.fileCursor < 0 || m.fileCursor >= len(m.files) {
			return entity{kind: entArea, id: "zz", label: "all changes"}
		}
		return m.files[m.fileCursor].entity()
	}
	if m.lane < len(m.lanes) && m.laneCursor < len(m.lanes[m.lane].items) {
		return m.lanes[m.lane].items[m.laneCursor]
	}
	return entity{}
}

// subjects are what an action applies to: the marks, the selected hunk in the
// details pane, or the selection.
func (m *Model) subjects() []entity {
	if len(m.marks) > 0 {
		keys := make([]string, 0, len(m.marks))
		for k := range m.marks {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out := make([]entity, len(keys))
		for i, k := range keys {
			out[i] = m.marks[k]
		}
		return out
	}
	if m.det.focused || m.det.full {
		if h, ok := m.det.hunkEntity(); ok {
			return []entity{h}
		}
	}
	if e := m.selected(); e.valid() {
		return []entity{e}
	}
	return nil
}

func (m *Model) hunkMarks() map[string]bool {
	out := map[string]bool{}
	for _, e := range m.marks {
		if e.kind == entHunk {
			out[e.id] = true
		}
	}
	return out
}

func (m *Model) toggleMark([]entity) tea.Cmd {
	e := m.selected()
	if m.det.focused || m.det.full {
		if h, ok := m.det.hunkEntity(); ok {
			e = h
		}
	}
	if e.kind == entArea {
		// Toggle every uncommitted file.
		all := true
		for _, r := range m.files {
			if !r.isDir && m.marks[r.entity().key()].kind == entNone {
				all = false
			}
		}
		for _, r := range m.files {
			if !r.isDir {
				if all {
					delete(m.marks, r.entity().key())
				} else if cmd := m.addMark(r.entity()); cmd != nil {
					return cmd
				}
			}
		}
		return nil
	}
	if e.kind.markClass() == 0 {
		return m.notify(toastInfo, "Can't mark "+e.describe())
	}
	if _, ok := m.marks[e.key()]; ok {
		delete(m.marks, e.key())
	} else if cmd := m.addMark(e); cmd != nil {
		return cmd
	}
	if m.det.focused || m.det.full {
		m.det.selectHunk(m.det.hunk+1, m.hunkMarks())
	} else {
		m.moveCursor(1)
	}
	return nil
}

func (m *Model) addMark(e entity) tea.Cmd {
	for _, o := range m.marks {
		if o.kind.markClass() != e.kind.markClass() {
			return m.notify(toastError, "Can't mix marked "+o.kind.String()+"s with "+e.kind.String()+"s")
		}
		if e.kind == entCommittedFile && o.commit != e.commit {
			return m.notify(toastError, "Marked files must come from the same commit")
		}
	}
	m.marks[e.key()] = e
	return nil
}

func (m *Model) toggleCommitFiles(sel []entity) tea.Cmd {
	c := sel[0].commit
	if c == nil || len(c.Changes) == 0 {
		return m.notify(toastInfo, "This commit has no files")
	}
	k := commitKey(c)
	m.showFiles[k] = !m.showFiles[k]
	if !m.showFiles[k] && sel[0].kind == entCommittedFile {
		m.pendingKey = "commit:" + k
	}
	m.rebuild()
	if m.pendingKey != "" {
		m.selectKey(m.pendingKey)
		m.pendingKey = ""
	}
	return nil
}

func (m *Model) syncDetails(force bool) tea.Cmd {
	if m.status == nil {
		return nil
	}
	return m.det.sync(m.client, m.selected(), force)
}

func (m *Model) setDetailsFull(on bool) tea.Cmd {
	m.det.full = on
	m.det.focused = on
	if on && m.det.hunk < 0 {
		m.det.selectHunk(0, m.hunkMarks())
	}
	return m.syncDetails(false)
}

// handleKey handles navigation, then actions bound to the key.
func (m *Model) handleKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	if m.handleNavKey(key) {
		return nil
	}
	switch key {
	case "q":
		return m.quit()
	case "esc":
		m.back()
		return nil
	case "enter":
		// Enter is contextual: fold directories, open file diffs, reword commits and branches.
		e := m.selected()
		switch e.kind {
		case entDir:
			m.toggleDir()
			return nil
		case entArea, entFile, entCommittedFile:
			return m.setDetailsFull(true)
		case entNewBranch:
			return m.promptNewBranch(but.Placement{}, "as a new lane")
		}
	}
	if a, ok := m.actionFor(key, m.subjects()); ok {
		return a.run(m, m.subjects())
	}
	return nil
}

// back is esc: leave the innermost state.
func (m *Model) back() {
	switch {
	case m.target != nil:
		m.target = nil
	case len(m.marks) > 0:
		m.marks = map[string]entity{}
	case m.det.visible:
		m.det.visible = false
	}
}

// handleNavKey moves the selection; it reports whether the key was navigation.
func (m *Model) handleNavKey(key string) bool {
	switch key {
	case "j", "down":
		m.moveCursor(1)
	case "k", "up":
		m.moveCursor(-1)
	case "ctrl+d", "pgdown":
		m.moveCursor(10)
	case "ctrl+u", "pgup":
		m.moveCursor(-10)
	case "g", "home":
		m.moveCursor(-1 << 20)
	case "G", "end":
		m.moveCursor(1 << 20)
	case "J":
		m.jumpBranch(1)
	case "K":
		m.jumpBranch(-1)
	case "l", "right":
		if m.focus == focusFiles {
			m.focus = focusLanes
		} else {
			m.lane++
		}
	case "h", "left":
		if m.focus == focusLanes && m.lane == 0 {
			m.focus = focusFiles
		} else if m.focus == focusLanes {
			m.lane--
		}
	case "tab":
		switch {
		case m.focus == focusFiles:
			m.focus = focusLanes
		case m.det.visible && m.target == nil:
			m.det.focused = true
			if m.det.hunk < 0 {
				m.det.selectHunk(0, m.hunkMarks())
			}
		default:
			m.focus = focusFiles
		}
	case "shift+tab":
		if m.focus == focusLanes {
			m.focus = focusFiles
		} else {
			m.focus = focusLanes
		}
	case "+", "=":
		m.det.pct = clamp(m.det.pct+5, detailsMinPct, detailsMaxPct)
	case "-":
		m.det.pct = clamp(m.det.pct-5, detailsMinPct, detailsMaxPct)
	default:
		return false
	}
	m.clampFiles()
	m.clampLanes()
	return true
}

func (m *Model) moveCursor(d int) {
	if m.focus == focusFiles {
		m.fileCursor = clamp(m.fileCursor+d, -1, len(m.files)-1)
		m.clampFiles()
		return
	}
	m.laneCursor += d
	if m.laneCursor < 0 && d < 0 && d > -10 && m.lane == 0 {
		m.laneCursor = 0
	}
	m.clampLanes()
}

// jumpBranch moves to the next/previous branch header, across lanes.
func (m *Model) jumpBranch(d int) {
	type pos struct{ lane, item int }
	var heads []pos
	for li, l := range m.lanes {
		for ii, e := range l.items {
			if e.kind == entBranch || e.kind == entNewBranch {
				heads = append(heads, pos{li, ii})
			}
		}
	}
	if len(heads) == 0 {
		return
	}
	cur := -1
	for i, h := range heads {
		if m.focus == focusLanes && (h.lane < m.lane || (h.lane == m.lane && h.item <= m.laneCursor)) {
			cur = i
		}
	}
	next := clamp(cur+d, 0, len(heads)-1)
	if cur == -1 && d < 0 {
		next = 0
	}
	m.focus, m.lane, m.laneCursor = focusLanes, heads[next].lane, heads[next].item
}

func (m *Model) toggleDir() {
	if m.fileCursor < 0 || m.fileCursor >= len(m.files) || !m.files[m.fileCursor].isDir {
		return
	}
	p := m.files[m.fileCursor].path
	m.collapsed[p] = !m.collapsed[p]
	m.rebuild()
}

// handleTargetKey handles keys while choosing a target.
func (m *Model) handleTargetKey(msg tea.KeyPressMsg) tea.Cmd {
	key := msg.String()
	t := m.target
	switch key {
	case "esc", "q":
		m.target = nil
		return nil
	case "enter":
		return m.confirmTarget(m.selected())
	case "a":
		if t.verb != verbSquash {
			t.side = -t.side
		}
		return nil
	case "u":
		if t.verb == verbSquash {
			t.useTarget = !t.useTarget
		}
		return nil
	case "e":
		if t.verb == verbCommit {
			t.emptyMsg = !t.emptyMsg
		}
		return nil
	case "b":
		if p, ok := t.newBranchPlan(m.selected()); ok {
			m.target = nil
			return p.run(m)
		}
		return m.notify(toastInfo, "Select a branch to put the new branch on")
	case "c", "r", "m", "p":
		v := map[string]verb{"c": verbCommit, "r": verbSquash, "m": verbMove, "p": verbPick}[key]
		if srcs, why := sourcesFor(v, t.sources); why == "" {
			t.verb, t.sources = v, srcs
		} else {
			return m.notify(toastInfo, why)
		}
		return nil
	case "/", "t":
		a, _ := m.actionFor(key, nil)
		return a.run(m, nil)
	}
	m.handleNavKey(key)
	return nil
}

func (m *Model) confirmTarget(e entity) tea.Cmd {
	t := m.target
	if t.isSource(e) && !t.selfTargetOK(e) {
		m.target = nil
		return nil
	}
	p, ok := t.plan(e)
	if !ok {
		return m.notify(toastInfo, "Can't "+t.verb.String()+" "+describeAll(t.sources)+" onto "+e.describe())
	}
	m.target = nil
	return p.run(m)
}

// handleDetailsKey handles keys while the details pane has focus. Keys it does not
// handle fall through to the normal bindings.
func (m *Model) handleDetailsKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	d := &m.det
	marks := m.hunkMarks()
	switch msg.String() {
	case "j", "down":
		if len(d.doc.hunks) > 0 {
			d.selectHunk(d.hunk+1, marks)
		} else {
			d.vp.ScrollDown(1)
		}
	case "k", "up":
		if len(d.doc.hunks) > 0 {
			d.selectHunk(d.hunk-1, marks)
		} else {
			d.vp.ScrollUp(1)
		}
	case "J":
		d.vp.ScrollDown(1)
	case "K":
		d.vp.ScrollUp(1)
	case "ctrl+d", "pgdown", "space":
		if msg.String() == "space" && d.ent.kind.uncommitted() {
			return m.toggleMark(nil), true
		}
		d.vp.HalfPageDown()
	case "ctrl+u", "pgup":
		d.vp.HalfPageUp()
	case "g", "home":
		d.selectHunk(0, marks)
		d.vp.GotoTop()
	case "G", "end":
		d.selectHunk(len(d.doc.hunks)-1, marks)
		d.vp.GotoBottom()
	case "esc", "h", "left", "tab":
		if d.full && msg.String() != "esc" && msg.String() != "tab" {
			return nil, true
		}
		d.focused, d.full = false, false
		d.rerender(marks)
	case "d":
		if d.full {
			d.full, d.focused, d.visible = false, false, true
		} else {
			d.visible, d.focused = false, false
		}
		d.rerender(marks)
	case "D":
		return m.setDetailsFull(!d.full), true
	case "q":
		if d.full {
			d.full, d.focused = false, false
			return nil, true
		}
		return nil, false
	default:
		return nil, false
	}
	return nil, true
}

// Layout.

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

func (m *Model) bodyHeight() int   { return max(m.height-1, 1) } // minus footer
func (m *Model) sidebarWidth() int { return clamp(m.width/4, 24, 40) }
func (m *Model) rightWidth() int   { return max(m.width-m.sidebarWidth()-1, minLaneWidth) }

// split returns the lanes' height and the details pane's height (0 when hidden).
func (m *Model) split() (lanesH, detH int) {
	h := m.bodyHeight()
	if !m.det.visible || m.det.full {
		return h, 0
	}
	detH = clamp(h*m.det.pct/100, 4, h-4)
	return h - detH - 1, detH
}

// laneLayout returns the lane width and how many lanes fit on screen.
func (m *Model) laneLayout() (width, visible int) {
	region := m.rightWidth()
	n := max(len(m.lanes), 1)
	width = clamp(region/n-1, minLaneWidth, maxLaneWidth)
	return width, max(region/(width+1), 1)
}

func (m *Model) clampFiles() {
	m.fileCursor = clamp(m.fileCursor, -1, len(m.files)-1)
	h := m.bodyHeight() - 2 // sidebar header + rule
	if m.fileCursor >= 0 {
		if m.fileCursor < m.fileOffset {
			m.fileOffset = m.fileCursor
		}
		if m.fileCursor >= m.fileOffset+h {
			m.fileOffset = m.fileCursor - h + 1
		}
	}
	m.fileOffset = clamp(m.fileOffset, 0, max(len(m.files)-h, 0))
}

func (m *Model) clampLanes() {
	m.lane = clamp(m.lane, 0, len(m.lanes)-1)
	if m.lane < len(m.lanes) {
		m.laneCursor = clamp(m.laneCursor, 0, len(m.lanes[m.lane].items)-1)
	}
	_, visible := m.laneLayout()
	if m.lane < m.laneOffset {
		m.laneOffset = m.lane
	}
	if m.lane >= m.laneOffset+visible {
		m.laneOffset = m.lane - visible + 1
	}
	m.laneOffset = max(m.laneOffset, 0)
}

// laneSel is the selected item index to draw in lane i, or -1.
func (m *Model) laneSel(i int) int {
	if m.focus == focusLanes && i == m.lane {
		return m.laneCursor
	}
	return -1
}

// scrollOffset returns the first of n lines to show in a window of h lines so that
// line sel stays visible.
func scrollOffset(n, sel, h int) int {
	off := 0
	if sel >= h-1 {
		off = min(sel-h+2, n-h)
	}
	return max(off, 0)
}

// deco decides how an entity is drawn in the current mode.
func (m *Model) deco(e entity) rowDeco {
	d := rowDeco{marked: m.marks[e.key()].kind != entNone}
	t, hover := m.target, m.selected()
	if m.drag != nil {
		t, hover = m.dragTarget()
	}
	if t == nil {
		return d
	}
	d.source = t.isSource(e)
	p, ok := t.plan(e)
	switch {
	case e.key() == hover.key() && ok:
		d.tag, d.insert = p.label, p.insert
	case !ok && !d.source:
		d.dim = true
	}
	return d
}

func (m Model) View() tea.View {
	var content string
	switch {
	case m.quitting:
		content = ""
	case m.width == 0:
		content = "loading…"
	default:
		content = m.render()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	return v
}

func (m *Model) render() string {
	h := m.bodyHeight()
	var body string
	if m.det.full {
		body = m.det.view(m.width, h)
	} else {
		body = m.workspaceView()
	}
	out := lipgloss.JoinVertical(lipgloss.Left, body, m.footer())
	if m.drag != nil && m.drag.moved {
		out = m.overlayGhost(out)
	}
	if m.modal != nil {
		out = overlay(out, m.modal.view(m.width, m.height), m.width, m.height)
	}
	return overlayCorner(out, m.renderToasts(m.width), m.width, m.height)
}

func (m *Model) workspaceView() string {
	h := m.bodyHeight()
	sw := m.sidebarWidth()
	vdiv := dividerStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", h), "\n"))
	right := m.lanesView()
	if lanesH, detH := m.split(); detH > 0 {
		rw := m.rightWidth()
		rule := dividerStyle.Render(strings.Repeat("─", rw))
		if m.det.focused {
			rule = titleStyle.Render(strings.Repeat("━", rw))
		}
		right = lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.NewStyle().Height(lanesH).MaxHeight(lanesH).Render(right), rule, m.det.view(rw, detH))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, m.sidebarView(sw, h), vdiv, right)
}

func (m *Model) lanesView() string {
	lanesH, _ := m.split()
	switch {
	case m.err != nil:
		return " " + errorStyle.Width(m.rightWidth()-2).Render(m.err.Error())
	case m.status == nil:
		return mutedStyle.Render(" loading workspace…")
	}
	lw, visible := m.laneLayout()
	div := dividerStyle.Render(strings.TrimSuffix(strings.Repeat("│\n", lanesH), "\n"))
	var cols []string
	for i := m.laneOffset; i < min(m.laneOffset+visible, len(m.lanes)); i++ {
		lv := m.lanes[i].render(lw, m.laneSel(i), m.deco)
		off := scrollOffset(len(lv.lines), lv.selLine, lanesH)
		visibleLines := lv.lines[off:min(off+lanesH, len(lv.lines))]
		cols = append(cols, lipgloss.NewStyle().Width(lw).Height(lanesH).Render(strings.Join(visibleLines, "\n")), div)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, cols...)
}

func (m *Model) sidebarView(width, height int) string {
	count := 0
	if m.status != nil {
		count = len(m.status.UncommittedChanges)
	}
	area := entity{kind: entArea, id: "zz"}
	ad := m.deco(area)
	title := headerStyle.Render("Unstaged")
	if m.focus == focusFiles {
		title = titleStyle.Render("Unstaged")
	}
	header := " " + title + " " + countStyle.Render(itoa(count))
	header = withTag(header, width, ad)
	if m.focus == focusFiles && m.fileCursor < 0 {
		header = selectedStyle.Render(ansi.Strip(pad(header, width, false)))
	}
	lines := []string{dimmed(header, ad), dividerStyle.Render(strings.Repeat("─", width))}
	if len(m.files) == 0 {
		lines = append(lines, mutedStyle.Render(" No changes"))
	}
	end := min(m.fileOffset+height-len(lines), len(m.files))
	for i := m.fileOffset; i < end; i++ {
		r := m.files[i]
		lines = append(lines, " "+r.render(width-1, m.focus == focusFiles && i == m.fileCursor, m.deco(r.entity())))
	}
	return lipgloss.NewStyle().Width(width).Height(height).MaxHeight(height).Render(strings.Join(lines, "\n"))
}

func (m *Model) footer() string {
	chip := lipgloss.NewStyle().Background(colorSurface).Foreground(colorText).Padding(0, 1).Render("normal")
	var msg string
	switch {
	case m.drag != nil && m.drag.moved:
		t, hover := m.dragTarget()
		chip = lipgloss.NewStyle().Background(colorAccent).Foreground(colorText).Bold(true).Padding(0, 1).Render("drag")
		msg = "Drop to act · " + describeSel(m.drag.sources)
		if t != nil {
			chip = t.verb.color().Render(t.verb.String())
			if p, ok := t.plan(hover); ok {
				msg = p.desc
			}
		}
	case m.target != nil:
		t := m.target
		chip = t.verb.color().Render(t.verb.String())
		msg = mutedStyle.Render("choose a target for " + describeAll(t.sources))
		if p, ok := t.plan(m.selected()); ok {
			msg = headerStyle.Render(p.desc)
		}
		hints := []string{"enter", "confirm"}
		switch t.verb {
		case verbCommit:
			hints = append(hints, "a", "above/below", "b", "new branch here", "e", onOff("empty msg", t.emptyMsg))
		case verbSquash:
			hints = append(hints, "u", onOff("keep target msg", t.useTarget))
		case verbMove, verbPick:
			hints = append(hints, "a", "above/below")
		}
		hints = append(hints, "c/r/m/p", "switch", "esc", "cancel")
		msg += "   " + keyHints(hints...)
	case m.det.focused || m.det.full:
		chip = lipgloss.NewStyle().Background(lipgloss.Color("#FB923C")).Foreground(lipgloss.Color("#1C1917")).Bold(true).Padding(0, 1).Render("details")
		hints := []string{"j/k", "hunk", "J/K", "scroll"}
		if m.det.ent.kind.uncommitted() {
			hints = append(hints, "space", "mark", "c", "commit", "r", "amend", "x", "discard")
		}
		msg = keyHints(append(hints, "y", "copy", "D", "full", "esc", "back")...)
	default:
		var hints []string
		n := 0
		for _, a := range m.available(m.subjects(), false) {
			if a.key == "" || a.key == "." || len(a.key) > 5 || n >= 7 {
				continue
			}
			hints = append(hints, a.key, shortTitle(a.title))
			n++
		}
		msg = keyHints(append(hints, "?", "help")...)
	}

	var right []string
	if m.busy != "" {
		right = append(right, m.spinner.View()+" "+m.busy)
	}
	if n := len(m.marks); n > 0 {
		right = append(right, titleStyle.Render(itoa(n)+" marked"))
	}
	if m.status != nil && m.status.UpstreamState.Behind > 0 {
		right = append(right, errorStyle.Render("↓"+itoa(m.status.UpstreamState.Behind)+" upstream"))
	}
	if m.status != nil && m.status.MergeBase.CommitID != "" {
		right = append(right, mutedStyle.Render("base "+m.status.MergeBase.ShortID()))
	}
	r := strings.Join(right, mutedStyle.Render(" · ")) + " "
	left := chip + " "
	avail := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(r)-1, 0)
	msg = ansi.Truncate(msg, avail, "…")
	gap := max(m.width-ansi.StringWidth(left)-ansi.StringWidth(msg)-ansi.StringWidth(r), 1)
	return left + msg + strings.Repeat(" ", gap) + r
}

func onOff(label string, on bool) string {
	if on {
		return label + " ✓"
	}
	return label
}

// shortTitle turns "Squash / amend…" into "squash".
func shortTitle(t string) string {
	t = strings.TrimSuffix(t, "…")
	w, _, _ := strings.Cut(t, " ")
	return strings.ToLower(w)
}
