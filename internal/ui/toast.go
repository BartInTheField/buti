package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const toastTTL = 4 * time.Second

type toastKind int

const (
	toastInfo toastKind = iota
	toastSuccess
	toastError
)

type toast struct {
	id    int
	kind  toastKind
	text  string
	until time.Time
}

type toastExpireMsg struct{ id int }

// notify shows a transient message; errors stay twice as long.
func (m *Model) notify(kind toastKind, text string) tea.Cmd {
	m.toastSeq++
	ttl := toastTTL
	if kind == toastError {
		ttl *= 2
	}
	t := toast{id: m.toastSeq, kind: kind, text: text, until: time.Now().Add(ttl)}
	m.toasts = append(m.toasts, t)
	if len(m.toasts) > 3 {
		m.toasts = m.toasts[len(m.toasts)-3:]
	}
	return tea.Tick(ttl, func(time.Time) tea.Msg { return toastExpireMsg{id: t.id} })
}

func (m *Model) expireToast(id int) {
	for i, t := range m.toasts {
		if t.id == id {
			m.toasts = append(m.toasts[:i], m.toasts[i+1:]...)
			return
		}
	}
}

func (m Model) renderToasts(width int) string {
	var out []string
	for _, t := range m.toasts {
		icon, c := "•", colorAccent
		switch t.kind {
		case toastSuccess:
			icon, c = "✓", colorAdd
		case toastError:
			icon, c = "✗", colorDel
		}
		w := clamp(lipgloss.Width(t.text)+6, 24, min(width-2, 64))
		text := lipgloss.NewStyle().Width(w - 6).Render(t.text)
		box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(c).Padding(0, 1).
			Render(lipgloss.NewStyle().Foreground(c).Render(icon) + " " + text)
		out = append(out, box)
	}
	return lipgloss.JoinVertical(lipgloss.Right, out...)
}

// overlayCorner draws top at the bottom-right corner of base, above the footer.
func overlayCorner(base, top string, width, height int) string {
	if top == "" {
		return base
	}
	x := max(width-lipgloss.Width(top)-1, 0)
	y := max(height-1-lipgloss.Height(top), 0)
	c := lipgloss.NewCanvas(width, height)
	c.Compose(lipgloss.NewCompositor(lipgloss.NewLayer(base), lipgloss.NewLayer(top).X(x).Y(y).Z(2)))
	return c.Render()
}

func itoa(n int) string { return strconv.Itoa(n) }

// firstLine keeps error toasts to a readable size.
func firstLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return ansi.Strip(strings.Join(lines, "\n"))
}
