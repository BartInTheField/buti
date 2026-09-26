package ui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/bartinthefield/buti/internal/update"
)

type (
	updateAvailableMsg struct{ latest string }
	updateNoticeMsg    struct {
		kind toastKind
		text string
	}
	updateStartedMsg struct{ version string }
	updateDoneMsg    struct {
		version string
		err     error
	}
)

// checkUpdate looks for a newer release in the background; failures (offline, rate limits) are silent.
func (m Model) checkUpdate() tea.Cmd {
	if !m.opts.UpdateCheck || !update.IsRelease(m.opts.Version) {
		return nil
	}
	current, cache := m.opts.Version, m.opts.UpdateCache
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		latest, err := update.Latest(ctx, cache)
		if err != nil || !update.Newer(latest, current) {
			return nil
		}
		return updateAvailableMsg{latest: latest}
	}
}

// checkUpdateNow asks GitHub for the latest release, skipping the cache, and reports the outcome either way.
// It runs from the help picker, which holds a stale *Model, so it reports back through messages only.
func (m *Model) checkUpdateNow([]entity) tea.Cmd {
	current := m.opts.Version
	notice := func(kind toastKind, text string) tea.Msg { return updateNoticeMsg{kind, text} }
	if !update.IsRelease(current) {
		return func() tea.Msg { return notice(toastInfo, "Development builds can't update themselves") }
	}
	return tea.Batch(func() tea.Msg { return notice(toastInfo, "Checking for updates…") }, func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		latest, err := update.Latest(ctx, "")
		switch {
		case err != nil:
			return notice(toastError, "Couldn't check for updates: "+firstLines(err.Error(), 3))
		case update.Newer(latest, current):
			return updateAvailableMsg{latest: latest}
		}
		return notice(toastSuccess, "buti "+current+" is the latest version")
	})
}

// showVersion reports the running version, through a message for the same reason as checkUpdateNow.
func (m *Model) showVersion([]entity) tea.Cmd {
	text := "buti " + m.opts.Version
	if !update.IsRelease(m.opts.Version) {
		text = "buti development build"
		if m.opts.Version != "" {
			text += " (" + m.opts.Version + ")"
		}
	}
	return func() tea.Msg { return updateNoticeMsg{toastInfo, text} }
}

func (m *Model) offerUpdate(latest string) tea.Cmd {
	text := "buti " + latest + " is available (you have " + m.opts.Version + ")."
	if m.modal != nil {
		// Don't interrupt a dialog the user already opened.
		return m.notify(toastInfo, text)
	}
	m.openModal(&confirmModal{
		title:    "Update available",
		body:     text + " Update now?",
		yesLabel: "update",
		// onYes runs after this Update returns, so it reports back through messages rather than m.
		onYes: func() tea.Cmd {
			return tea.Batch(func() tea.Msg { return updateStartedMsg{latest} }, installUpdate(latest))
		},
	})
	return nil
}

func installUpdate(version string) tea.Cmd {
	return func() tea.Msg {
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			err = update.Install(ctx, version, exe)
		}
		return updateDoneMsg{version: version, err: err}
	}
}

func (m *Model) handleUpdateDone(msg updateDoneMsg) tea.Cmd {
	if msg.err != nil {
		return m.notify(toastError, "Update failed: "+firstLines(msg.err.Error(), 3))
	}
	return m.notify(toastSuccess, "Updated to "+msg.version+". Restart buti to use it.")
}
