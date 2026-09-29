package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// pressState is a left button held down on an entity; it becomes a drag once the
// pointer moves.
type pressState struct {
	ent  entity
	x, y int
}

type dragState struct {
	sources []entity
	x, y    int
	moved   bool

	// The operation a drop at x, y would do, computed once per move.
	target *targetMode
	hover  entity
}

type hitKind int

const (
	hitNone hitKind = iota
	hitEntity
	hitStartCommit // a lane's "Start a commit…" button
	hitPush        // a branch card's Push button
	hitMore        // a branch card's ⋯ menu
	hitDetails
	hitDetailsTree  // a row of the file tree next to the full-screen diff
	hitTreeSplit    // the divider between that tree and the diff
	hitSidebarSplit // the divider between the sidebar and the lanes
	hitDetailsClose
	hitButton // an edit mode button; key is its action's
)

type hit struct {
	kind hitKind
	ent  entity
	// Where the entity lives, to select it.
	files      bool
	row        int // file row, -1 for the header
	line       int // content line in the details pane, -1 on its title bar
	lane, item int
	key        string
}

// hitTest maps screen coordinates to what is drawn there.
func (m *Model) hitTest(x, y int) hit {
	if m.status == nil || y >= m.bodyHeight() {
		return hit{}
	}
	if m.resolving() {
		switch row, key := m.editView().hit(x, y); {
		case row >= 0:
			return hit{kind: hitEntity, ent: m.files[row].entity(), files: true, row: row}
		case key != "":
			return hit{kind: hitButton, key: key}
		}
		return hit{}
	}
	if m.det.full {
		switch {
		case y == 0 && x >= m.width-8:
			return hit{kind: hitDetailsClose}
		case y > 0 && m.det.treeShown() && x < m.det.treeWidth():
			return hit{kind: hitDetailsTree, row: m.det.tree.offset + y - 3} // below the title bar, header and rule
		case y > 0 && m.det.treeShown() && x == m.det.treeWidth():
			return hit{kind: hitTreeSplit}
		}
		return hit{kind: hitDetails, line: y - 1}
	}
	sw := m.sidebarWidth()
	if x < sw {
		switch y {
		case 0:
			return hit{kind: hitEntity, ent: entity{kind: entArea, id: "zz", label: "all changes"}, files: true, row: -1}
		case 1:
			return hit{}
		}
		row := m.fileOffset + y - 2
		if row >= len(m.files) {
			return hit{}
		}
		return hit{kind: hitEntity, ent: m.files[row].entity(), files: true, row: row}
	}
	if x == sw {
		return hit{kind: hitSidebarSplit}
	}
	lanesH, detH := m.split()
	if detH > 0 && y > lanesH {
		return hit{kind: hitDetails, line: y - lanesH - 2} // below the rule and the title bar
	}
	if y >= lanesH {
		return hit{}
	}

	lw, visible := m.laneLayout()
	rel := x - sw - 1
	col := rel / (lw + 1)
	if rel < 0 || rel%(lw+1) == lw || col >= visible {
		return hit{} // on a divider or past the last lane
	}
	li := m.laneOffset + col
	if li >= len(m.lanes) {
		return hit{}
	}
	l := m.lanes[li]
	lv := l.render(lw, m.laneSel(li), m.deco)
	line := scrollOffset(len(lv.lines), lv.selLine, lanesH) + y
	if line >= len(lv.items) {
		return hit{}
	}
	if l.phantom {
		return hit{kind: hitEntity, ent: l.items[0], lane: li, item: 0}
	}
	switch v := lv.items[line]; {
	case v >= 0:
		return hit{kind: hitEntity, ent: l.items[v], lane: li, item: v}
	case v == startCommitHit:
		return hit{kind: hitStartCommit, lane: li}
	case v <= pushHitBase:
		item := pushHitBase - v
		k := hitPush
		if rel%(lw+1) >= lw-4 {
			k = hitMore
		}
		return hit{kind: k, ent: l.items[item], lane: li, item: item}
	}
	return hit{}
}

// selectHit moves the cursor to a hit entity.
func (m *Model) selectHit(h hit) {
	if h.files {
		m.focus, m.fileCursor = focusFiles, h.row
		m.clampFiles()
		return
	}
	m.focus, m.lane, m.laneCursor = focusLanes, h.lane, h.item
	m.clampLanes()
}

func (m *Model) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		return m.handleClick(msg)
	case tea.MouseMotionMsg:
		if m.det.tree.dragW > 0 {
			m.det.tree.dragW = max(msg.X, 1)
			return nil
		}
		if m.sidebarDrag > 0 {
			m.sidebarDrag = max(msg.X, 1)
			return nil
		}
		if m.press != nil {
			if m.drag == nil {
				srcs := []entity{m.press.ent}
				if _, marked := m.marks[m.press.ent.key()]; marked {
					srcs = m.subjects()
				}
				m.drag = &dragState{sources: srcs}
			}
			m.drag.x, m.drag.y = msg.X, msg.Y
			m.drag.moved = m.drag.moved || msg.X != m.press.x || msg.Y != m.press.y
			m.drag.target, m.drag.hover = m.computeDragTarget()
		}
	case tea.MouseReleaseMsg:
		defer func() { m.press, m.drag = nil, nil }()
		if t := &m.det.tree; t.dragW > 0 {
			t.width, t.dragW = m.det.clampTreeWidth(t.dragW), 0
			return nil
		}
		if m.sidebarDrag > 0 {
			m.sidebarW, m.sidebarDrag = m.sidebarWidth(), 0
			m.clampFiles()
			m.clampLanes()
			return nil
		}
		if m.drag != nil && m.drag.moved {
			return m.drop(msg.X, msg.Y)
		}
	case tea.MouseWheelMsg:
		return m.handleWheel(msg)
	}
	return nil
}

func (m *Model) handleClick(msg tea.MouseClickMsg) tea.Cmd {
	h := m.hitTest(msg.X, msg.Y)
	switch msg.Button {
	case tea.MouseRight:
		if h.kind == hitEntity || h.kind == hitMore || h.kind == hitPush {
			m.selectHit(h)
			return m.contextMenu(m.subjects())
		}
		return nil
	case tea.MouseLeft:
	default:
		return nil
	}

	if m.target != nil {
		if h.kind == hitEntity {
			m.selectHit(h)
			return m.confirmTarget(h.ent)
		}
		return nil
	}

	switch h.kind {
	case hitDetailsClose:
		m.det.full, m.det.focused = false, false
		return nil
	case hitDetails:
		marks := m.hunkMarks()
		m.det.tree.focused = false
		if !m.det.focused {
			m.det.focus(marks)
		}
		if h.line >= 0 {
			m.det.clickLine(h.line, msg.Mod&tea.ModShift != 0, marks)
		}
		return nil
	case hitTreeSplit:
		m.det.tree.dragW = max(msg.X, 1)
		return nil
	case hitSidebarSplit:
		m.sidebarDrag = max(msg.X, 1)
		return nil
	case hitDetailsTree:
		t := &m.det.tree
		t.focused = true
		switch {
		case h.row < 0 || h.row >= len(t.rows):
		case t.marker(h.row): // a "more" marker: scroll a page that way
			if h.row == t.offset {
				t.scroll(-(t.height - 2))
			} else {
				t.scroll(t.height - 2)
			}
		default:
			t.cursor = h.row
			t.clamp()
			if path, ok := t.file(); ok {
				m.det.jumpToFile(path, m.hunkMarks())
			} else {
				t.toggle(m.det.data)
			}
		}
		return nil
	case hitButton:
		if a, ok := m.actionFor(h.key, m.subjects()); ok {
			return a.run(m, m.subjects())
		}
		return nil
	case hitStartCommit:
		return m.startCommitInLane(h.lane)
	case hitPush:
		m.selectHit(h)
		return m.push([]entity{h.ent})
	case hitMore:
		m.selectHit(h)
		return m.contextMenu([]entity{h.ent})
	case hitEntity:
		m.det.focused = false
		m.selectHit(h)
		m.press = &pressState{ent: h.ent, x: msg.X, y: msg.Y}
		double := m.registerClick(h.ent.key())
		switch {
		case h.ent.kind == entDir:
			m.toggleDir()
		case h.ent.kind == entNewBranch:
			return m.promptNewBranch(placementNone, "as a new lane")
		case double && h.ent.kind == entConflict:
			return m.execInteractive("Edit "+h.ent.label, keepSelection, editorCommand(m.pathOf(h.ent)))
		case double:
			return m.setDetailsFull(true)
		}
	}
	return nil
}

// startCommitInLane commits the lane's staged changes, or everything uncommitted,
// to the top branch of the lane.
func (m *Model) startCommitInLane(li int) tea.Cmd {
	l := m.lanes[li]
	if len(l.stack.Branches) == 0 {
		return nil
	}
	top := l.stack.Branches[0].Name
	var ids []string
	what := "all changes"
	for _, c := range l.stack.AssignedChanges {
		ids = append(ids, c.CliID)
	}
	if len(ids) > 0 {
		what = pluralize(len(ids), "staged file")
	} else if !hasUncommitted(m) {
		return m.notify(toastInfo, "Nothing to commit")
	}
	return m.startCommit(ids, placementBranch(top), false, "Commit "+what+" to branch "+top)
}

// registerClick records a click on key and reports whether it completes a double click.
func (m *Model) registerClick(key string) bool {
	now := time.Now()
	double := key == m.lastClickKey && now.Sub(m.lastClickAt) < doubleClickTime
	if double {
		m.lastClickKey = "" // a third click starts over
	} else {
		m.lastClickKey, m.lastClickAt = key, now
	}
	return double
}

// dragTarget is the operation a drop at the pointer would do.
func (m *Model) dragTarget() (*targetMode, entity) { return m.drag.target, m.drag.hover }

// computeDragTarget hit-tests without drag decorations, which themselves depend on
// the drag target.
func (m *Model) computeDragTarget() (*targetMode, entity) {
	d := m.drag
	m.drag = nil
	h := m.hitTest(d.x, d.y)
	m.drag = d
	if h.kind != hitEntity || len(m.drag.sources) == 0 {
		return nil, entity{}
	}
	v, ok := dropVerb(m.drag.sources[0], h.ent)
	if !ok {
		return nil, h.ent
	}
	srcs, why := sourcesFor(v, m.drag.sources)
	if why != "" {
		return nil, h.ent
	}
	return newTargetMode(v, srcs, m.status), h.ent
}

func (m *Model) drop(x, y int) tea.Cmd {
	m.drag.x, m.drag.y = x, y
	t, target := m.computeDragTarget()
	if t == nil {
		if target.valid() {
			return m.notify(toastInfo, "Can't drop "+describeSel(m.drag.sources)+" onto "+target.describe())
		}
		return nil
	}
	p, ok := t.plan(target)
	if !ok {
		return m.notify(toastInfo, "Can't drop "+describeSel(m.drag.sources)+" onto "+target.describe())
	}
	return p.run(m)
}

func (m *Model) overlayGhost(base string) string {
	label := "↳ " + describeSel(m.drag.sources)
	st := lipgloss.NewStyle().Background(colorAccent).Foreground(colorText).Bold(true).Padding(0, 1)
	if t, hover := m.dragTarget(); t != nil {
		if p, ok := t.plan(hover); ok {
			label += " → " + p.label
			st = t.verb.color()
		}
	}
	ghost := st.Render(label)
	x := clamp(m.drag.x+2, 0, max(m.width-lipgloss.Width(ghost), 0))
	c := lipgloss.NewCanvas(m.width, m.height)
	c.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(ghost).X(x).Y(min(m.drag.y+1, m.height-2)).Z(3)))
	return c.Render()
}

// handleWheel scrolls whatever is under the pointer.
func (m *Model) handleWheel(msg tea.MouseWheelMsg) tea.Cmd {
	delta := 0
	switch msg.Button {
	case tea.MouseWheelDown:
		delta = 1
	case tea.MouseWheelUp:
		delta = -1
	default:
		return nil
	}
	h := m.hitTest(msg.X, msg.Y)
	switch {
	case h.kind == hitDetailsTree:
		m.det.tree.scroll(3 * delta)
		return nil
	case h.kind == hitDetails:
		var cmd tea.Cmd
		m.det.vp, cmd = m.det.vp.Update(msg)
		return cmd
	case msg.X < m.sidebarWidth():
		m.fileOffset = clamp(m.fileOffset+3*delta, 0, max(len(m.files)-(m.bodyHeight()-2), 0))
		return nil
	}
	lw, visible := m.laneLayout()
	li := m.laneOffset + (msg.X-m.sidebarWidth()-1)/(lw+1)
	if li >= min(m.laneOffset+visible, len(m.lanes)) {
		return nil
	}
	if li != m.lane || m.focus != focusLanes {
		m.focus, m.lane, m.laneCursor = focusLanes, li, 0
	} else {
		m.laneCursor += delta
	}
	m.clampLanes()
	return nil
}
