package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/bartinthefield/buti/skills"
)

// skillUsage lists the `buti skill` subcommands.
const skillUsage = `buti skill list
buti skill show <name>
buti skill install (--agent <agent> [--project] | --target <dir>) [name...]`

// agentDir is where an agent looks for skills, relative to the home directory (global) and to the repository.
type agentDir struct {
	name, global, project, note string
}

// agentDirs are the skills directories of the agents `buti skill install --agent` knows. Anything else takes
// --target.
var agentDirs = []agentDir{
	{"agents", ".agents/skills", ".agents/skills", "the shared Agent Skills location (Codex, Gemini CLI and others)"},
	{"claude", ".claude/skills", ".claude/skills", "Claude Code"},
	{"codex", ".agents/skills", ".agents/skills", "OpenAI Codex"},
	{"cursor", ".cursor/skills", ".cursor/skills", "Cursor"},
	{"opencode", ".config/opencode/skills", ".opencode/skills", "OpenCode"},
	{"copilot", ".copilot/skills", ".github/skills", "GitHub Copilot"},
}

// runSkill runs `buti skill`: the agent skills built into buti, to print or copy into an agent's skills directory.
func runSkill(dir string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage:\n"+indentLines(skillUsage))
		return 2
	}
	sub, args := args[0], args[1:]
	switch sub {
	case "-h", "-help", "--help", "help":
		fmt.Fprintln(stdout, "usage:\n"+indentLines(skillUsage))
		return 0
	case "list":
		for _, n := range skills.Names() {
			fmt.Fprintln(stdout, n)
		}
		return 0
	case "show":
		if len(args) != 1 {
			fmt.Fprintln(stderr, "usage: buti skill show <name>")
			return 2
		}
		data, err := skills.Read(args[0])
		if err != nil {
			fmt.Fprintln(stderr, "buti skill:", err)
			return 1
		}
		stdout.Write(data)
		return 0
	case "install":
		return installSkills(dir, args, stdout, stderr)
	}
	fmt.Fprintf(stderr, "buti skill: unknown command %q\nusage:\n%s\n", sub, indentLines(skillUsage))
	return 2
}

func installSkills(dir string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "install into this agent's skills directory")
	project := fs.Bool("project", false, "with --agent, install into the repository instead of the home directory")
	target := fs.String("target", "", "install into `dir`, the skills directory of any agent")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: buti skill install (--agent <agent> [--project] | --target <dir>) [name...]")
		fs.PrintDefaults()
		fmt.Fprintln(stderr, "\nAgents (global directory, then --project):")
		tw := tabwriter.NewWriter(stderr, 0, 0, 2, ' ', 0)
		for _, a := range agentDirs {
			fmt.Fprintf(tw, "  %s\t~/%s\t%s\t%s\n", a.name, a.global, a.project, a.note)
		}
		tw.Flush()
	}
	// Flags may come after the skill names, as in `install buti-resolve --agent claude`.
	var names []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return 0
			}
			return 2
		}
		if fs.NArg() == 0 {
			break
		}
		names = append(names, fs.Arg(0))
		args = fs.Args()[1:]
	}
	if len(names) == 0 {
		names = skills.Names()
	}

	dest, err := skillsDir(dir, *agent, *project, *target)
	if err != nil {
		fmt.Fprintln(stderr, "buti skill install:", err)
		fs.Usage()
		return 2
	}
	for _, n := range names {
		p, err := skills.Install(n, dest)
		if err != nil {
			fmt.Fprintln(stderr, "buti skill install:", err)
			return 1
		}
		fmt.Fprintf(stdout, "installed %s to %s\n", n, p)
	}
	return 0
}

// skillsDir works out the directory to install into from the flags.
func skillsDir(dir, agent string, project bool, target string) (string, error) {
	switch {
	case target != "" && agent != "":
		return "", errors.New("pass --agent or --target, not both")
	case target != "":
		if project {
			return "", errors.New("--project goes with --agent")
		}
		return filepath.Abs(expandHome(target))
	case agent == "":
		return "", errors.New("say where to install: --agent <agent>, or --target <dir> for any other agent")
	}
	for _, a := range agentDirs {
		if a.name != agent {
			continue
		}
		if project {
			return filepath.Join(repoRoot(dir), filepath.FromSlash(a.project)), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, filepath.FromSlash(a.global)), nil
	}
	return "", fmt.Errorf("unknown agent %q; use --target <dir> for it", agent)
}

// repoRoot is the top of the working tree containing dir, or dir outside a repository.
func repoRoot(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return dir
	}
	return strings.TrimSpace(string(out))
}

// expandHome expands a leading ~/, which a quoted --target keeps.
func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

func indentLines(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ")
}
