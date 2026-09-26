package ui

import (
	"strings"
	"testing"
)

// conflictedHarness serves testStatus with the commit on branch-2 conflicted.
func conflictedHarness(t *testing.T) *harness {
	t.Helper()
	s := testStatus()
	yes := true
	c := &s.Stacks[1].Branches[0].Commits[0]
	c.Conflicted, c.Message = &yes, "Start a changelog"
	return newHarnessWith(t, s)
}

func TestResolveEntersEditMode(t *testing.T) {
	h := conflictedHarness(t)
	h.hover("commit:chg2")
	if f := h.m.footer(); !strings.Contains(f, "resolve") {
		t.Errorf("footer should hint resolving: %q", f)
	}
	h.keys("e")
	h.but.expect("resolve c2")
}

func TestResolveNotOfferedForCleanCommit(t *testing.T) {
	h := conflictedHarness(t)
	h.hover("commit:chg1")
	h.keys("e")
	h.but.expect()
}

const editModeStatus = `{"conflicted_files":["CHANGELOG.md"],"resolved_files":["src/a.go"],
	"conflicted_count":1,"resolved_count":1,"all_resolved":false}`

func TestEditMode(t *testing.T) {
	h := newHarnessWith(t, editModeStatus)
	h.wantOnScreen("You are editing a conflicted commit", "Commit files", "CHANGELOG.md", "Conflicted",
		"a.go src", "Resolved", "x Cancel", "o Open conflicted files", "e Save and exit", "edit mode")
	if got := h.selectedKey(); got != "conflicted file:CHANGELOG.md" {
		t.Errorf("selected %q, want the first file", got)
	}
	h.keys("l", "b", "a", "c")
	if h.m.focus != focusFiles || h.m.modal != nil || h.m.target != nil {
		t.Fatal("the workspace actions are out of reach in edit mode")
	}

	h.keys("e") // a file is still conflicted: confirm first
	if _, ok := h.m.modal.(*confirmModal); !ok {
		t.Fatalf("saving with markers left should ask:\n%s", h.screen())
	}
	h.wantOnScreen("• CHANGELOG.md")
	h.keys("y")
	h.keys("x", "y")
	h.but.expect("resolve finish", "resolve cancel --force")
}

func TestEditModeButtons(t *testing.T) {
	h := newHarnessWith(t, `{"conflicted_files":[],"resolved_files":["CHANGELOG.md"],"all_resolved":true}`)
	h.selectText("CHANGELOG.md")
	h.click("Save and exit")
	h.click("Cancel")
	h.keys("y")
	h.but.expect("resolve finish", "resolve cancel --force")
}
