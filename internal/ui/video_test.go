package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestE2EVideo records the frames of the demo video (mise run video). It drives the real UI through the flows the
// README talks about and saves every screen with the time it stays on and the caption under it. tools/video turns
// the frames into an MP4 and a GIF. It only runs with BUTI_VIDEO=<dir>, so the screenshot run stays small.
func TestE2EVideo(t *testing.T) {
	dir := os.Getenv("BUTI_VIDEO")
	if dir == "" {
		t.Skip("set BUTI_VIDEO=<dir> to record the demo video frames")
	}
	h, _ := newRepoHarness(t)
	v := &videoRecorder{h: h, dir: dir}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	// The workspace, and a diff.
	v.caption = "Unstaged changes on the left, a lane per stack, branches as cards"
	v.frame(3500 * time.Millisecond)
	v.caption = "Select anything to read its diff"
	v.step("j", 400*time.Millisecond)
	h.selectText("README.md")
	h.keys("d")
	h.wantOnScreen("## Usage")
	v.frame(1800 * time.Millisecond)
	h.selectText("Add users endpoint")
	h.wantOnScreen("2 files changed")
	v.frame(2200 * time.Millisecond)

	// Commit: select, verb, target, message.
	v.caption = "Commit: select a file, press c, pick a branch"
	h.selectText("strings.go")
	v.frame(900 * time.Millisecond)
	v.step("c", 900*time.Millisecond)
	for _, key := range []string{"branch:empty", "branch:fix-typo", "branch:auth"} {
		h.hover(key)
		v.frame(700 * time.Millisecond)
	}
	h.hover("branch:api")
	v.frame(1000 * time.Millisecond)
	v.caption = "Type a message, press enter"
	v.step("enter", 700*time.Millisecond)
	v.typeText("Add greeting helpers", 70*time.Millisecond)
	v.frame(700 * time.Millisecond)
	v.step("enter", 300*time.Millisecond)
	if got := h.commits("api"); len(got) != 3 || got[0] != "Add greeting helpers" {
		t.Fatalf("api has %q", got)
	}
	v.caption = "The commit lands where you pointed"
	v.frame(2200 * time.Millisecond)

	// Squash by dragging one commit onto another.
	h.m.toasts = nil
	v.caption = "Or use the mouse: drag a commit onto another to squash them"
	v.drag("Test token auth", "Add token auth", 6, 90*time.Millisecond)
	h.wantOnScreen("Squash into commit")
	v.caption = "Check the squashed message, press enter"
	v.frame(1600 * time.Millisecond)
	h.keys("enter")
	if got := h.commits("auth"); len(got) != 1 {
		t.Fatalf("auth has %q", got)
	}
	v.frame(2000 * time.Millisecond)
	h.m.toasts = nil

	// Uncommit by dragging onto Unstaged.
	v.caption = "Drag a commit onto Unstaged to uncommit it"
	v.drag("Bump Go version", "Unstaged", 10, 80*time.Millisecond)
	h.wantOnScreen("go.mod")
	v.frame(2200 * time.Millisecond)
	h.m.toasts = nil

	// Review comments.
	v.caption = "Leave review comments on a diff for your coding agent"
	h.selectText("README.md")
	if !h.m.det.visible {
		h.keys("d")
	}
	h.click("## Usage")
	v.frame(900 * time.Millisecond)
	v.step("C", 600*time.Millisecond)
	v.typeText("Document the flags here too", 55*time.Millisecond)
	v.frame(600 * time.Millisecond)
	v.step("ctrl+s", 200*time.Millisecond)
	h.wantOnScreen("Document the flags here too", "✎1")
	v.caption = "/buti-resolve has the agent fix each one and mark it resolved"
	v.frame(2200 * time.Millisecond)
	cs, err := h.m.review.List()
	if err != nil || len(cs) != 1 {
		t.Fatalf("comments %+v, %v", cs, err)
	}
	if _, err := h.m.review.Resolve(cs[0].ID, "Listed -C, --diff and --remember-selection"); err != nil {
		t.Fatal(err)
	}
	h.run(h.m.fetchStatus())
	h.m.toasts = nil
	h.wantOnScreen("✓ resolved")
	v.frame(2600 * time.Millisecond)

	// The palette and the help.
	v.caption = "Every command in the palette (ctrl+p) and the searchable help (?)"
	h.keys("esc")
	v.step("ctrl+p", 1000*time.Millisecond)
	v.typeText("squash", 80*time.Millisecond)
	v.frame(1400 * time.Millisecond)
	h.keys("esc")
	v.step("?", 2200*time.Millisecond)
	h.keys("esc")
	v.caption = ""
	v.frame(1500 * time.Millisecond)

	v.save()
}

type videoFrame struct {
	File    string `json:"file"`
	Millis  int    `json:"ms"`
	Caption string `json:"caption"`
	// The mouse pointer's cell while dragging, for the renderer to draw; the UI draws no pointer itself.
	Pointer *[2]int `json:"pointer,omitempty"`
	// Where texts worth zooming in on are on this screen, for the trailer (trailer/).
	Anchors map[string][2]int `json:"anchors,omitempty"`
}

// videoAnchors are the texts whose cell the recorder notes on every frame that shows them.
var videoAnchors = map[string]string{
	"unstaged":  "Unstaged",
	"target":    "commit here",
	"composer":  "Subject",
	"squash":    "Squash into commit",
	"source":    "source",
	"dragGhost": "↳ commit",
	"landed":    "Add greeting helpers",
	"comment":   "Document the flags here too",
	"resolved":  "✓ resolved",
	"palette":   "type to search",
	"details":   "Details ·",
	"lanes":     "No staged changes",
	"toast":     "✓ ",
}

// videoRecorder saves each screen as a frame and the captions as they change.
type videoRecorder struct {
	h       *harness
	dir     string
	caption string
	pointer *[2]int
	frames  []videoFrame
}

func (v *videoRecorder) frame(d time.Duration) {
	v.h.t.Helper()
	name := fmt.Sprintf("%04d.ansi", len(v.frames))
	if err := os.WriteFile(filepath.Join(v.dir, name), []byte(v.h.m.View().Content), 0o644); err != nil {
		v.h.t.Fatal(err)
	}
	anchors := map[string][2]int{}
	lines := strings.Split(v.h.screen(), "\n")
	for key, text := range videoAnchors {
		for y, line := range lines {
			if i := strings.Index(line, text); i >= 0 {
				anchors[key] = [2]int{ansi.StringWidth(line[:i]), y}
				break
			}
		}
	}
	v.frames = append(v.frames, videoFrame{File: name, Millis: int(d / time.Millisecond), Caption: v.caption,
		Pointer: v.pointer, Anchors: anchors})
}

// step presses a key and holds the resulting screen.
func (v *videoRecorder) step(key string, d time.Duration) {
	v.h.keys(key)
	v.frame(d)
}

// typeText types a string one character per frame.
func (v *videoRecorder) typeText(s string, perChar time.Duration) {
	for _, r := range s {
		v.h.send(tea.KeyPressMsg{Code: r, Text: string(r)})
		v.frame(perChar)
	}
}

// drag moves an item to a target in steps, saving a frame at each, so the drag reads as motion.
func (v *videoRecorder) drag(from, to string, steps int, perStep time.Duration) {
	v.h.t.Helper()
	fx, fy := v.h.find(from)
	tx, ty := v.h.find(to)
	v.pointer = &[2]int{fx, fy}
	v.frame(600 * time.Millisecond)
	v.h.send(tea.MouseClickMsg{X: fx, Y: fy, Button: tea.MouseLeft})
	v.frame(perStep)
	for i := 1; i <= steps; i++ {
		x := fx + (tx-fx)*i/steps
		y := fy + (ty-fy)*i/steps
		v.h.send(tea.MouseMotionMsg{X: x, Y: y, Button: tea.MouseLeft})
		v.pointer = &[2]int{x, y}
		v.frame(perStep)
	}
	v.frame(700 * time.Millisecond)
	v.h.send(tea.MouseReleaseMsg{X: tx, Y: ty, Button: tea.MouseLeft})
	v.pointer = nil
}

func (v *videoRecorder) save() {
	data, err := json.MarshalIndent(v.frames, "", "  ")
	if err != nil {
		v.h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.dir, "frames.json"), data, 0o644); err != nil {
		v.h.t.Fatal(err)
	}
}
