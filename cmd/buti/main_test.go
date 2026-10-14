package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartinthefield/buti/internal/but"
	"github.com/bartinthefield/buti/internal/review"
	"github.com/bartinthefield/buti/internal/testrepo"
	"github.com/bartinthefield/buti/skills"
)

func runButi(args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = run(args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestHelpDocumentsReview(t *testing.T) {
	code, _, stderr := runButi("--help")
	if code != 0 || !strings.Contains(stderr, "buti [-C dir] review <command>") ||
		!strings.Contains(stderr, "buti review list [--status") || !strings.Contains(stderr, "buti review clear") ||
		!strings.Contains(stderr, "buti skill install") {
		t.Errorf("--help: exit %d\n%s", code, stderr)
	}
}

// TestReviewDispatch runs `buti review` in a plain git repository, before and without the TUI, and with -C.
func TestReviewDispatch(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	s, err := review.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.Add(review.Anchor{Kind: review.KindUnassigned, Path: "a.txt", Line: 1, LineText: "a"}, "", "Fix")
	if err != nil {
		t.Fatal(err)
	}
	// Writing needs no `but`, so this passes with only git on PATH.
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if code, out, stderr := runButi("-C", dir, "review", "resolve", c.ID, "--summary", "Fixed"); code != 0 ||
		out != "resolved "+c.ID+"\n" {
		t.Fatalf("resolve: exit %d, %q %q", code, out, stderr)
	}
	if got, _ := s.Get(c.ID); got.Status != review.StatusResolved || got.Resolution.Summary != "Fixed" {
		t.Errorf("stored %+v", got)
	}
	if code, _, stderr := runButi("-C", dir, "review", "list"); code != 1 || !strings.Contains(stderr, "`but`") {
		t.Errorf("list without but: exit %d, %q", code, stderr)
	}
	if code, _, stderr := runButi("-C", dir, "review"); code != 2 || !strings.Contains(stderr, "usage:") {
		t.Errorf("no subcommand: exit %d, %q", code, stderr)
	}
}

// TestIntegrationReviewList lists comments in the test repository with the real `but`. It is opt-in:
// BUTI_INTEGRATION=1 go test ./cmd/buti
func TestIntegrationReviewList(t *testing.T) {
	if os.Getenv("BUTI_INTEGRATION") == "" {
		t.Skip("set BUTI_INTEGRATION=1 to run against the real but CLI")
	}
	if _, err := exec.LookPath("but"); err != nil {
		t.Skip("but not on PATH")
	}
	r, err := testrepo.Create(testrepo.TempDir(t))
	if err != nil {
		t.Fatal(err)
	}
	r.Setenv(t.Setenv)
	ctx := context.Background()
	st, err := but.New(r.Dir).Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var token but.Commit
	var readmeID string
	for _, s := range st.Stacks {
		for _, b := range s.Branches {
			for _, cm := range b.Commits {
				if cm.Subject() == "Add token auth" {
					token = cm
				}
			}
		}
	}
	for _, ch := range st.UncommittedChanges {
		if ch.FilePath == "README.md" {
			readmeID = ch.CliID
		}
	}
	var tokenFile string
	for _, ch := range token.Changes {
		if ch.FilePath == "src/auth/token.go" {
			tokenFile = ch.CliID
		}
	}
	if token.CliID == "" || readmeID == "" || tokenFile == "" {
		t.Fatalf("fixture lacks the token commit or README.md: %+v", st)
	}

	s, err := review.Open(r.Dir)
	if err != nil {
		t.Fatal(err)
	}
	secret := `func Token() string { return "secret" }`
	onCommit, err := s.Add(review.Anchor{Kind: review.KindCommit, ChangeID: token.ChangeID, CommitID: token.CommitID,
		Branch: "auth", Path: "src/auth/token.go", Line: 3, LineText: secret}, "", "Don't hard-code the token")
	if err != nil {
		t.Fatal(err)
	}
	// Stored one line off, so list has to find it again.
	onReadme, err := s.Add(review.Anchor{Kind: review.KindUnassigned, Path: "README.md", Line: 4, LineText: "## Usage"},
		"", "Say more")
	if err != nil {
		t.Fatal(err)
	}

	// From another directory, through -C.
	t.Chdir(t.TempDir())
	code, out, stderr := runButi("-C", r.Dir, "review", "list", "--json")
	if code != 0 {
		t.Fatalf("list: exit %d\n%s", code, stderr)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if len(got) != 2 {
		t.Fatalf("got %d comments:\n%s", len(got), out)
	}
	c, u := got[0], got[1]
	commit, _ := c["commit"].(map[string]any)
	if c["id"] != onCommit.ID || c["shortcode"] != token.CliID || c["file_shortcode"] != tokenFile ||
		c["kind"] != "commit" || c["branch"] != "auth" || commit["title"] != "Add token auth" ||
		commit["sha"] != token.CommitID || c["line"] != 3.0 || c["outdated"] != false ||
		!strings.Contains(c["context"].(string), "> 3 | "+secret) {
		t.Errorf("commit comment:\n%s", out)
	}
	if u["id"] != onReadme.ID || u["shortcode"] != "zz" || u["file_shortcode"] != readmeID || u["kind"] != "unassigned" ||
		u["branch"] != nil || u["commit"] != nil || u["line"] != 5.0 || u["end_line"] != 5.0 ||
		!strings.Contains(u["context"].(string), "> 5 | ## Usage") {
		t.Errorf("uncommitted comment:\n%s", out)
	}

	code, out, stderr = runButi("-C", r.Dir, "review", "show", onCommit.ID[:4])
	if code != 0 || !strings.Contains(out, "on commit "+token.CliID+` "Add token auth"`) {
		t.Errorf("show: exit %d\n%s%s", code, out, stderr)
	}
	if code, _, stderr = runButi("-C", r.Dir, "review", "resolve", onReadme.ID, "--summary", "Documented"); code != 0 {
		t.Fatalf("resolve: exit %d\n%s", code, stderr)
	}
	if code, out, _ = runButi("-C", r.Dir, "review", "list"); code != 0 || strings.Contains(out, onReadme.ID) ||
		!strings.Contains(out, onCommit.ID) {
		t.Errorf("list after resolve:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(r.Dir, ".git", "buti", "review.json")); err != nil {
		t.Errorf("store not in the git dir: %v", err)
	}
}

func TestSkillCommands(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	want, err := skills.Read("buti-resolve")
	if err != nil {
		t.Fatal(err)
	}
	installed := func(dir string) bool {
		data, err := os.ReadFile(filepath.Join(dir, "buti-resolve", "SKILL.md"))
		return err == nil && bytes.Equal(data, want)
	}

	if code, out, _ := runButi("skill", "list"); code != 0 || !strings.Contains(out, "buti-resolve\n") ||
		!strings.Contains(out, "buti-review\n") {
		t.Errorf("list: exit %d, %q", code, out)
	}
	if code, out, _ := runButi("skill", "show", "buti-resolve"); code != 0 || out != string(want) {
		t.Errorf("show: exit %d", code)
	}
	if code, _, stderr := runButi("skill", "show", "nope"); code != 1 || !strings.Contains(stderr, `no skill "nope"`) {
		t.Errorf("show nope: exit %d, %q", code, stderr)
	}

	if code, out, stderr := runButi("skill", "install", "--agent", "claude"); code != 0 ||
		!installed(filepath.Join(home, ".claude", "skills")) || !strings.Contains(out, "installed buti-resolve to ") {
		t.Errorf("install --agent claude: exit %d, %q %q", code, out, stderr)
	}
	// Without names, install copies every skill: /buti-review too.
	review, _ := skills.Read("buti-review")
	if data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "buti-review", "SKILL.md")); err != nil ||
		!bytes.Equal(data, review) {
		t.Errorf("install --agent claude did not install buti-review (%v)", err)
	}
	if code, _, stderr := runButi("skill", "install", "buti-resolve", "--agent", "opencode"); code != 0 ||
		!installed(filepath.Join(home, ".config", "opencode", "skills")) {
		t.Errorf("install --agent opencode: exit %d, %q", code, stderr)
	}
	target := filepath.Join(t.TempDir(), "my-agent")
	if code, _, stderr := runButi("skill", "install", "--target", target); code != 0 || !installed(target) {
		t.Errorf("install --target: exit %d, %q", code, stderr)
	}
	if code, _, stderr := runButi("skill", "install", "--target", "~/prompts"); code != 0 ||
		!installed(filepath.Join(home, "prompts")) {
		t.Errorf("install --target ~/prompts: exit %d, %q", code, stderr)
	}

	// --project installs at the top of the repository, from a directory inside it.
	repo := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", repo).CombinedOutput(); err != nil {
		t.Skipf("git init: %v %s", err, out)
	}
	sub := filepath.Join(repo, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runButi("-C", sub, "skill", "install", "--agent", "cursor", "--project"); code != 0 ||
		!installed(filepath.Join(repo, ".cursor", "skills")) {
		t.Errorf("install --project: exit %d, %q", code, stderr)
	}

	for _, args := range [][]string{
		{"install"},
		{"install", "--agent", "nope"},
		{"install", "--agent", "claude", "--target", target},
		{"install", "--target", target, "--project"},
	} {
		if code, _, stderr := runButi(append([]string{"skill"}, args...)...); code != 2 || !strings.Contains(stderr, "usage:") {
			t.Errorf("%v: exit %d, %q", args, code, stderr)
		}
	}
	if code, _, stderr := runButi("skill", "install", "nope", "--target", target); code != 1 ||
		!strings.Contains(stderr, `no skill "nope"`) {
		t.Errorf("install nope: exit %d, %q", code, stderr)
	}
	if code, _, stderr := runButi("skill"); code != 2 || !strings.Contains(stderr, "buti skill install") {
		t.Errorf("no subcommand: exit %d, %q", code, stderr)
	}
}
