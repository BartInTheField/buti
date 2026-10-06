// Package testrepo builds a scratch repository in a known GitButler state, for
// tests that run against the real `but` CLI and for trying buti by hand
// (go run ./internal/testrepo/mkrepo <dir>).
//
// The workspace it leaves behind:
//
//	origin/main    one commit ahead of the local base (so the workspace is behind by 1)
//	stack 1        auth (2 commits, unpushed) stacked on api (2 commits, pushed)
//	stack 2        fix-typo (1 pushed commit + 1 local one)
//	stack 3        empty (no commits)
//	unapplied      old-experiment (1 commit)
//	uncommitted    README.md and src/server.go modified, src/util/strings.go added,
//	               docs/old.md deleted
//
// Conflict pulls origin/main on top of that, into a conflicted commit.
//
// Commits carry fixed dates, but but gives each commit a random change id, so
// commit and CLI ids differ between runs: find things by name, not by id.
package testrepo

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TempDir is t.TempDir for a test that runs `but`: its cleanup retries for a few seconds, because `but` can still
// be writing its settings under HOME when the test ends, and t.TempDir fails the test when removal loses that race.
func TempDir(t testing.TB) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "buti-"+strings.ReplaceAll(t.Name(), "/", "_"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(100 * time.Millisecond) {
			if err = os.RemoveAll(dir); err == nil || time.Now().After(deadline) {
				break
			}
		}
		if err != nil {
			t.Logf("leaving %s: %v", dir, err)
		}
	})
	return dir
}

// Repo is a fixture repository.
type Repo struct {
	Dir    string   // the working tree
	Origin string   // the bare repository behind the "origin" remote
	Home   string   // HOME and XDG dirs, so `but` keeps its state out of the real ones
	Env    []string // variables `but` and git need to run in Dir (see Setenv)
}

// Setenv applies Env with setenv, e.g. t.Setenv, so that commands inherit it.
func (r *Repo) Setenv(setenv func(key, value string)) {
	for _, kv := range r.Env {
		k, v, _ := strings.Cut(kv, "=")
		setenv(k, v)
	}
}

// Env is the environment for running git and but in a scratch repository: a
// fixed identity, and HOME/XDG dirs under home so `but` keeps its project
// registry and settings out of the real ones.
func Env(home string) []string {
	return []string{
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"XDG_DATA_HOME=" + filepath.Join(home, ".local", "share"),
		"XDG_CACHE_HOME=" + filepath.Join(home, ".cache"),
		"XDG_STATE_HOME=" + filepath.Join(home, ".local", "state"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Ada Lovelace",
		"GIT_AUTHOR_EMAIL=ada@example.com",
		"GIT_COMMITTER_NAME=Ada Lovelace",
		"GIT_COMMITTER_EMAIL=ada@example.com",
		"BUTI_NO_UPDATE_CHECK=1",
	}
}

// Create builds the fixture under root, which must be empty or missing.
func Create(root string) (*Repo, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &Repo{
		Dir:    filepath.Join(root, "repo"),
		Origin: filepath.Join(root, "origin.git"),
		Home:   filepath.Join(root, "home"),
	}
	r.Env = Env(r.Home)
	for _, d := range []string{r.Dir, r.Home} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}
	b := &builder{r: r}
	b.build()
	return r, b.err
}

type builder struct {
	r    *Repo
	err  error
	tick int // advances the fixed commit date
}

// run executes a command in the working tree (or in dir, when given as "@dir").
func (b *builder) run(args ...string) string {
	if b.err != nil {
		return ""
	}
	dir := b.r.Dir
	if strings.HasPrefix(args[0], "@") {
		dir, args = args[0][1:], args[1:]
	}
	b.tick++
	date := fmt.Sprintf("2026-01-01T12:%02d:00+00:00", b.tick%60)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), b.r.Env...)
	cmd.Env = append(cmd.Env, "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		b.err = fmt.Errorf("%s: %v\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}

func (b *builder) write(name, content string) {
	if b.err != nil {
		return
	}
	p := filepath.Join(b.r.Dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		b.err = err
		return
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		b.err = err
	}
}

// commit commits the named files to branch through but.
func (b *builder) commit(branch, msg string, files ...string) {
	b.run(append(append([]string{"but", "commit", "--message", msg}, files...), "--branch", branch)...)
}

func (b *builder) build() {
	r := b.r
	b.run("@"+filepath.Dir(r.Dir), "git", "init", "-q", "--bare", "-b", "main", r.Origin)
	b.run("git", "init", "-q", "-b", "main")
	b.write("README.md", "# demo\n\nA small service used to exercise buti.\n")
	b.write("go.mod", "module example.com/demo\n\ngo 1.22\n")
	b.write("src/server.go", serverGo)
	b.write("docs/old.md", "Old notes.\n")
	b.run("git", "add", ".")
	b.run("git", "commit", "-qm", "Initial commit")
	b.write("src/handlers.go", "package demo\n\nfunc Health() string { return \"ok\" }\n")
	b.run("git", "add", ".")
	b.run("git", "commit", "-qm", "Add health handler")
	b.run("git", "remote", "add", "origin", r.Origin)
	b.run("git", "push", "-q", "origin", "main")

	// Someone else lands a commit on origin/main that we have not integrated.
	b.run("git", "checkout", "-q", "-b", "upstream")
	b.write("CHANGELOG.md", "# Changelog\n\n- Health endpoint\n")
	b.run("git", "add", ".")
	b.run("git", "commit", "-qm", "Add changelog")
	b.run("git", "push", "-q", "origin", "upstream:main")
	b.run("git", "checkout", "-q", "main")
	b.run("git", "branch", "-q", "-D", "upstream")

	// A branch that exists but is not applied to the workspace.
	b.run("git", "checkout", "-q", "-b", "old-experiment")
	b.write("experiment.txt", "trying something\n")
	b.run("git", "add", ".")
	b.run("git", "commit", "-qm", "Try an experiment")
	b.run("git", "checkout", "-q", "main")

	b.run("but", "setup")

	// Stack 1: api (pushed) with auth stacked on top (unpushed).
	b.run("but", "branch", "new", "api")
	b.write("src/api/routes.go", "package api\n\nvar Routes = []string{\"/health\"}\n")
	b.commit("api", "Add API routes", "src/api/routes.go")
	b.write("src/api/routes.go", "package api\n\nvar Routes = []string{\"/health\", \"/users\"}\n")
	b.write("src/api/users.go", "package api\n\ntype User struct{ Name string }\n")
	b.commit("api", "Add users endpoint", "src/api/routes.go", "src/api/users.go")
	b.run("but", "push", "api")
	b.run("but", "branch", "new", "--above", "api", "auth")
	b.write("src/auth/token.go", "package auth\n\nfunc Token() string { return \"secret\" }\n")
	b.commit("auth", "Add token auth", "src/auth/token.go")
	b.write("src/auth/token_test.go", "package auth\n\nimport \"testing\"\n\nfunc TestToken(t *testing.T) {}\n")
	b.commit("auth", "Test token auth", "src/auth/token_test.go")

	// Stack 2: fix-typo, pushed, then one more local commit.
	b.run("but", "branch", "new", "fix-typo")
	b.write("src/handlers.go", "package demo\n\n// Health reports liveness.\nfunc Health() string { return \"ok\" }\n")
	b.commit("fix-typo", "Document the health handler", "src/handlers.go")
	b.run("but", "push", "fix-typo")
	b.write("go.mod", "module example.com/demo\n\ngo 1.23\n")
	b.commit("fix-typo", "Bump Go version", "go.mod")

	// Stack 3: an empty branch.
	b.run("but", "branch", "new", "empty")

	// Uncommitted changes.
	b.write("README.md", "# demo\n\nA small service used to exercise buti.\n\n## Usage\n\n    go run ./src\n")
	b.write("src/util/strings.go", "package util\n\nfunc Reverse(s string) string {\n\tr := []rune(s)\n\tfor i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {\n\t\tr[i], r[j] = r[j], r[i]\n\t}\n\treturn string(r)\n}\n")
	if b.err == nil {
		b.err = os.Remove(filepath.Join(r.Dir, "docs/old.md"))
	}
	b.write("src/server.go", strings.Replace(serverGo, ":8080", ":9090", 1))
}

// Conflict adds a branch changelog whose commit adds its own CHANGELOG.md,
// then pulls, which rebases that commit onto the upstream one into a conflict.
// It also sets the identity in the repository's config: resolving the commit writes a
// reflog entry, which needs a committer even when buti runs without testrepo.Env
// (the mkrepo command line, the desktop e2e tests).
func (r *Repo) Conflict() error {
	b := &builder{r: r}
	b.run("git", "config", "user.name", "Ada Lovelace")
	b.run("git", "config", "user.email", "ada@example.com")
	b.run("but", "branch", "new", "changelog")
	b.write("CHANGELOG.md", "# Changelog\n\n- Token auth\n")
	b.commit("changelog", "Start a changelog", "CHANGELOG.md")
	b.run("but", "pull")
	return b.err
}

const serverGo = `package demo

import "net/http"

func Serve() error {
	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(Health()))
	})
	return http.ListenAndServe(":8080", nil)
}
`
