package ui

import (
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type composeKind int

const (
	composeCommit composeKind = iota
	composeSquash
	composeReword
	composeComment // a review comment: a body only
)

// composerModal edits a commit message (a subject line plus an optional body), or a review comment.
type composerModal struct {
	kind      composeKind
	context   string // what the message is for, e.g. "Commit 3 files to feat-x"
	subject   textinput.Model
	body      textarea.Model
	focusBody bool
	onSubmit  func(m *Model, msg string) tea.Cmd
	onEditor  func(*Model) tea.Cmd // optional: write the message in $EDITOR instead
}

func newComposer(kind composeKind, context, initial string, onSubmit func(*Model, string) tea.Cmd) *composerModal {
	subj, rest, _ := strings.Cut(strings.TrimSpace(initial), "\n")
	s := textinput.New()
	s.Prompt = ""
	s.Placeholder = "Commit subject"
	s.SetValue(subj)
	s.CursorEnd()
	s.Focus()

	b := textarea.New()
	b.Placeholder = "Description (optional)"
	b.ShowLineNumbers = false
	b.Prompt = ""
	b.SetValue(strings.TrimLeft(rest, "\n"))
	b.Blur()
	return &composerModal{kind: kind, context: context, subject: s, body: b, onSubmit: onSubmit}
}

// newCommentComposer edits a review comment; context says where it is.
func newCommentComposer(context, initial string, onSubmit func(*Model, string) tea.Cmd) *composerModal {
	c := newComposer(composeComment, context, "", onSubmit)
	c.body.Placeholder = "What should change here?"
	c.body.SetValue(initial)
	c.setFocusBody(true)
	return c
}

func (c *composerModal) message() string {
	if c.kind == composeComment {
		return strings.TrimSpace(c.body.Value())
	}
	subj := strings.TrimSpace(c.subject.Value())
	body := strings.TrimSpace(c.body.Value())
	if body == "" {
		return subj
	}
	return subj + "\n\n" + body
}

func (c *composerModal) setFocusBody(on bool) tea.Cmd {
	c.focusBody = on
	if on {
		c.subject.Blur()
		return c.body.Focus()
	}
	c.body.Blur()
	return c.subject.Focus()
}

func (c *composerModal) update(m *Model, msg tea.Msg) tea.Cmd {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "esc":
			m.closeModal()
			return nil
		case "ctrl+s", "ctrl+enter":
			m.closeModal()
			return c.onSubmit(m, c.message())
		}
		if c.kind == composeComment {
			var cmd tea.Cmd
			c.body, cmd = c.body.Update(msg) // enter is a new line
			return cmd
		}
		switch k.String() {
		case "ctrl+e":
			if c.onEditor != nil {
				m.closeModal()
				return c.onEditor(m)
			}
		case "tab":
			return c.setFocusBody(!c.focusBody)
		case "shift+tab":
			return c.setFocusBody(false)
		case "enter":
			if !c.focusBody {
				m.closeModal()
				return c.onSubmit(m, c.message())
			}
		case "down":
			if !c.focusBody {
				return c.setFocusBody(true)
			}
		case "up":
			if c.focusBody && c.body.Line() == 0 {
				return c.setFocusBody(false)
			}
		}
	}
	var cmd tea.Cmd
	if c.focusBody {
		c.body, cmd = c.body.Update(msg)
	} else {
		c.subject, cmd = c.subject.Update(msg)
	}
	return cmd
}

func (c *composerModal) view(width, height int) string {
	w := modalWidth(width, 76)
	inner := w - 4
	c.subject.SetWidth(inner - 2)
	c.body.SetWidth(inner)
	c.body.SetHeight(clamp(height-16, 3, 10))

	title := map[composeKind]string{composeCommit: "Commit", composeSquash: "Squash", composeReword: "Reword", composeComment: "Comment"}[c.kind]
	field := func(label string, focused bool, content string) string {
		st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorBorder).Width(inner).Padding(0, 1)
		if focused {
			st = st.BorderForeground(colorAccent)
		}
		return mutedStyle.Render(label) + "\n" + st.Render(content)
	}
	subjectLen := ansi.StringWidth(c.subject.Value())
	counter := mutedStyle.Render(itoa(subjectLen))
	if subjectLen > 72 {
		counter = errorStyle.Render(itoa(subjectLen))
	}

	var hints []string
	switch {
	case c.kind == composeComment:
		hints = []string{"ctrl+s", "save", "enter", "new line"}
	case c.focusBody:
		hints = []string{"ctrl+s", "save", "tab", "subject"}
	default:
		hints = []string{"enter", "save", "tab", "description"}
	}
	if c.onEditor != nil {
		hints = append(hints, "ctrl+e", "$EDITOR")
	}
	hints = append(hints, "esc", "cancel")

	head := modalTitle(title) + "  " + mutedStyle.Render(ansi.Truncate(c.context, inner-len(title)-2, "…"))
	parts := []string{head, "",
		field("Subject  "+counter, !c.focusBody, c.subject.View()),
		field("Description", c.focusBody, c.body.View()),
		"", keyHints(hints...),
	}
	if c.kind == composeComment {
		parts = []string{head, "", field("For the agent that picks it up with /buti-resolve", true, c.body.View()), "", keyHints(hints...)}
	}
	return modalStyle.Width(w).Render(strings.Join(parts, "\n"))
}
