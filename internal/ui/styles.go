package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

var (
	colorAccent  = lipgloss.Color("#7D56F4")
	colorBorder  = lipgloss.Color("#3F3B38")
	colorMuted   = lipgloss.Color("#8A827C")
	colorText    = lipgloss.Color("#E8E4E0")
	colorSurface = lipgloss.Color("#34302D")
	colorAdd     = lipgloss.Color("#4ADE80")
	colorMod     = lipgloss.Color("#FACC15")
	colorDel     = lipgloss.Color("#F87171")
	colorPushed  = lipgloss.Color("#2DD4BF")
	colorLocal   = lipgloss.Color("#7A716B")

	cardStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)
	dropStyle = lipgloss.NewStyle().
			Border(lipgloss.Border{
			Top: "╌", Bottom: "╌", Left: "╎", Right: "╎",
			TopLeft: "╭", TopRight: "╮", BottomLeft: "╰", BottomRight: "╯",
		}).
		BorderForeground(colorBorder).
		Foreground(colorMuted).
		Align(lipgloss.Center)

	buttonStyle      = lipgloss.NewStyle().Background(colorSurface).Foreground(colorText).Padding(0, 1)
	primaryBtnStyle  = lipgloss.NewStyle().Background(colorText).Foreground(lipgloss.Color("#1C1917")).Padding(0, 1)
	disabledBtnStyle = lipgloss.NewStyle().Foreground(colorMuted).Padding(0, 1)

	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(colorAccent)
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(colorText)
	countStyle    = lipgloss.NewStyle().Bold(true).Background(colorText).Foreground(lipgloss.Color("#1C1917")).Padding(0, 1)
	mutedStyle    = lipgloss.NewStyle().Foreground(colorMuted)
	dividerStyle  = lipgloss.NewStyle().Foreground(colorBorder)
	selectedStyle = lipgloss.NewStyle().Reverse(true)
	errorStyle    = lipgloss.NewStyle().Foreground(colorDel)
)

func changeTypeStyle(t string) (string, lipgloss.Style) {
	switch t {
	case "added":
		return "+", lipgloss.NewStyle().Foreground(colorAdd)
	case "deleted", "removed":
		return "-", lipgloss.NewStyle().Foreground(colorDel)
	case "renamed":
		return "→", lipgloss.NewStyle().Foreground(colorMod)
	default:
		return "~", lipgloss.NewStyle().Foreground(colorMod)
	}
}

// branchColor mirrors the desktop app's branch badge: teal once pushed, grey while local-only.
func branchColor(status string) color.Color {
	switch status {
	case "completelyUnpushed":
		return colorLocal
	case "unpushedCommits", "unpushedCommitsRequiringForce":
		return colorMod
	case "integrated":
		return colorAccent
	default:
		return colorPushed
	}
}
