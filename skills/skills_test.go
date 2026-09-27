package skills

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestSkillFiles checks that each skill has the front matter agents need to load it, named after its directory.
func TestSkillFiles(t *testing.T) {
	names := Names()
	for _, want := range []string{"buti-resolve", "buti-review"} {
		if !slices.Contains(names, want) {
			t.Fatalf("Names() = %v, want %s", names, want)
		}
	}
	for _, n := range names {
		data, err := Read(n)
		if err != nil {
			t.Fatal(err)
		}
		front, _, ok := strings.Cut(strings.TrimPrefix(string(data), "---\n"), "\n---\n")
		if !strings.HasPrefix(string(data), "---\n") || !ok {
			t.Fatalf("%s: no front matter", n)
		}
		if !strings.Contains(front, "\nname: "+n+"\n") && !strings.HasPrefix(front, "name: "+n+"\n") {
			t.Errorf("%s: front matter has no name: %s\n%s", n, n, front)
		}
		if !strings.Contains(front, "\ndescription: ") {
			t.Errorf("%s: front matter has no description", n)
		}
	}
}

// TestResolveSkillCommands keeps the skill in step with the commands it tells the agent to run.
func TestResolveSkillCommands(t *testing.T) {
	data, err := Read("buti-resolve")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"which buti",
		"install.sh",
		"buti review list --status open --json",
		"buti review resolve <id> --summary",
		"buti review reply <id> --body",
		"buti review dismiss <id> --reason",
		"but amend <that id> -t <commit shortcode>",
		"never run git write commands",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("SKILL.md does not mention %q", want)
		}
	}
}

// TestReviewSkillCommands keeps /buti-review in step with the commands and tags it relies on.
func TestReviewSkillCommands(t *testing.T) {
	data, err := Read("buti-review")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"which buti",
		"install.sh",
		"but status -f",
		"but diff <commit>",
		"buti review list --status all --json",
		"buti review comment --file <path> --line <n> [--end-line <n>] [--side new|old] [--shortcode <id>] --body",
		"[must-fix]", "[suggestion]", "[nit]", "[question]",
		"never run git write commands",
		"/buti-resolve",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("SKILL.md does not mention %q", want)
		}
	}
}

// TestResolveSkillAuthors checks that /buti-resolve handles comments a reviewing agent left.
func TestResolveSkillAuthors(t *testing.T) {
	data, err := Read("buti-resolve")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"`author`", "--author agent", "[must-fix]", "[question]"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("SKILL.md does not mention %q", want)
		}
	}
}

func TestInstall(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	got, err := Install("buti-resolve", dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "buti-resolve"); got != want {
		t.Errorf("Install = %q, want %q", got, want)
	}
	want, _ := Read("buti-resolve")
	// A second install replaces what the first wrote.
	if err := os.WriteFile(filepath.Join(got, "SKILL.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install("buti-resolve", dir); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(got, "SKILL.md")); err != nil || !bytes.Equal(data, want) {
		t.Errorf("installed SKILL.md differs (%v)", err)
	}
	if _, err := Install("nope", dir); err == nil {
		t.Error("Install of an unknown skill succeeded")
	}
}
