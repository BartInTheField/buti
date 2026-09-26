package ui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

// modal is a dialog drawn over the workspace. While open it receives all input;
// it closes itself through m.closeModal.
type modal interface {
	update(m *Model, msg tea.Msg) tea.Cmd
	view(width, height int) string
}

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorAccent).
	Padding(0, 1)

func modalWidth(screen, want int) int { return clamp(want, 20, max(screen-4, 20)) }

// overlay draws top centred over base.
func overlay(base, top string, width, height int) string {
	x := max((width-lipgloss.Width(top))/2, 0)
	y := max((height-lipgloss.Height(top))/3, 0)
	c := lipgloss.NewCanvas(width, height)
	c.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(top).X(x).Y(y).Z(1)))
	return c.Render()
}

func modalTitle(s string) string { return titleStyle.Render(s) }

func keyHints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, headerStyle.Render(pairs[i])+" "+mutedStyle.Render(pairs[i+1]))
	}
	return strings.Join(parts, mutedStyle.Render(" · "))
}

// confirmModal asks a yes/no question before a destructive operation.
type confirmModal struct {
	title, body string
	yesLabel    string
	onYes       func() tea.Cmd
}

func (c *confirmModal) update(m *Model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "y", "enter":
			m.closeModal()
			return c.onYes()
		case "n", "esc", "q":
			m.closeModal()
		}
	}
	return nil
}

func (c *confirmModal) view(width, _ int) string {
	w := modalWidth(width, 60)
	yes := c.yesLabel
	if yes == "" {
		yes = "confirm"
	}
	body := lipgloss.NewStyle().Width(w - 4).Render(c.body)
	return modalStyle.Width(w).Render(modalTitle(c.title) + "\n\n" + body + "\n\n" +
		keyHints("y/enter", yes, "n/esc", "cancel"))
}

// promptModal asks for a single line of text.
type promptModal struct {
	title    string
	hint     string
	input    textinput.Model
	onSubmit func(string) tea.Cmd
}

func newPrompt(title, value, placeholder, hint string, onSubmit func(string) tea.Cmd) *promptModal {
	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = placeholder
	in.SetValue(value)
	in.CursorEnd()
	in.Focus()
	return &promptModal{title: title, hint: hint, input: in, onSubmit: onSubmit}
}

func (p *promptModal) update(m *Model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "enter":
			m.closeModal()
			return p.onSubmit(p.input.Value())
		case "esc":
			m.closeModal()
			return nil
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return cmd
}

func (p *promptModal) view(width, _ int) string {
	w := modalWidth(width, 64)
	p.input.SetWidth(w - 6)
	hints := keyHints("enter", "save", "esc", "cancel")
	if p.hint != "" {
		hints = mutedStyle.Render(p.hint) + "\n" + hints
	}
	return modalStyle.Width(w).Render(modalTitle(p.title) + "\n\n" + p.input.View() + "\n\n" + hints)
}

// pickItem is one row of a pickerModal.
type pickItem struct {
	label  string
	detail string // right-aligned, e.g. a key binding
	value  any
	dim    bool // shown, but not applicable right now
}

// pickerModal is a fuzzy-filtered list: branch picker, command palette, oplog, context menu.
type pickerModal struct {
	title    string
	items    []pickItem
	filter   textinput.Model
	matches  []int // indexes into items, in display order
	cursor   int
	offset   int
	onPick   func(pickItem) tea.Cmd
	empty    string
	maxRows  int
	lastRows int
}

func newPicker(title string, items []pickItem, onPick func(pickItem) tea.Cmd) *pickerModal {
	f := textinput.New()
	f.Prompt = "/ "
	f.Placeholder = "type to filter"
	f.Focus()
	p := &pickerModal{title: title, items: items, filter: f, onPick: onPick, empty: "No matches", maxRows: 14}
	p.refilter()
	return p
}

type pickSource []pickItem

func (s pickSource) String(i int) string { return s[i].label + " " + s[i].detail }
func (s pickSource) Len() int            { return len(s) }

func (p *pickerModal) refilter() {
	q := p.filter.Value()
	p.matches = p.matches[:0]
	if q == "" {
		for i := range p.items {
			p.matches = append(p.matches, i)
		}
	} else {
		for _, r := range fuzzy.FindFrom(q, pickSource(p.items)) {
			p.matches = append(p.matches, r.Index)
		}
	}
	p.cursor, p.offset = 0, 0
}

func (p *pickerModal) update(m *Model, msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			m.closeModal()
			return nil
		case "enter":
			if p.cursor < len(p.matches) {
				m.closeModal()
				return p.onPick(p.items[p.matches[p.cursor]])
			}
			return nil
		case "up", "ctrl+p", "ctrl+k":
			p.move(-1)
			return nil
		case "down", "ctrl+n", "ctrl+j", "tab":
			p.move(1)
			return nil
		}
	case tea.MouseWheelMsg:
		switch msg.Button {
		case tea.MouseWheelUp:
			p.move(-1)
		case tea.MouseWheelDown:
			p.move(1)
		}
		return nil
	}
	prev := p.filter.Value()
	var cmd tea.Cmd
	p.filter, cmd = p.filter.Update(msg)
	if p.filter.Value() != prev {
		p.refilter()
	}
	return cmd
}

func (p *pickerModal) move(d int) {
	p.cursor = clamp(p.cursor+d, 0, len(p.matches)-1)
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	if p.lastRows > 0 && p.cursor >= p.offset+p.lastRows {
		p.offset = p.cursor - p.lastRows + 1
	}
}

func (p *pickerModal) view(width, height int) string {
	w := modalWidth(width, 72)
	inner := w - 4
	p.filter.SetWidth(inner - 3)
	rows := clamp(height-10, 3, p.maxRows)
	p.lastRows = rows

	lines := []string{modalTitle(p.title), p.filter.View(), dividerStyle.Render(strings.Repeat("─", inner))}
	if len(p.matches) == 0 {
		lines = append(lines, mutedStyle.Render(p.empty))
	}
	for i := p.offset; i < min(p.offset+rows, len(p.matches)); i++ {
		it := p.items[p.matches[i]]
		detail := mutedStyle.Render(it.detail)
		label := ansi.Truncate(it.label, max(inner-ansi.StringWidth(it.detail)-2, 1), "…")
		line := fileLine("", label, detail, inner, i == p.cursor)
		if it.dim && i != p.cursor {
			line = mutedStyle.Faint(true).Render(ansi.Strip(line))
		}
		lines = append(lines, line)
	}
	if n := len(p.matches); n > rows {
		lines = append(lines, mutedStyle.Render(ansi.Truncate(
			itoa(p.cursor+1)+"/"+itoa(n), inner, "")))
	}
	lines = append(lines, "", keyHints("↑↓", "move", "enter", "select", "esc", "close"))
	return modalStyle.Width(w).Render(strings.Join(lines, "\n"))
}

// outputModal shows the output of a command run from the prompt.
type outputModal struct {
	title string
	vp    viewport.Model
	body  string
}

func newOutput(title, body string) *outputModal {
	return &outputModal{title: title, vp: viewport.New(), body: body}
}

func (o *outputModal) update(m *Model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc", "q", "enter":
			m.closeModal()
			return nil
		}
	}
	var cmd tea.Cmd
	o.vp, cmd = o.vp.Update(msg)
	return cmd
}

func (o *outputModal) view(width, height int) string {
	w := modalWidth(width, width-8)
	h := clamp(lipgloss.Height(o.body), 1, max(height-8, 3))
	o.vp.SetWidth(w - 4)
	o.vp.SetHeight(h)
	o.vp.SetContent(o.body)
	return modalStyle.Width(w).Render(modalTitle(o.title) + "\n\n" + o.vp.View() + "\n\n" +
		keyHints("j/k", "scroll", "esc", "close"))
}
