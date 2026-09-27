---
name: buti-review
description: Review a diff in a GitButler workspace and leave the findings as review comments in buti, the terminal UI for GitButler. Use when the user asks for a review of their uncommitted changes, a commit or a branch that they will read in buti, or runs /buti-review. Leaves each finding on its line with `buti review comment`, tagged [must-fix], [suggestion], [nit] or [question]; it changes no code.
---

# Review a diff into buti

Review a diff in a GitButler workspace like a careful colleague, and leave each finding as a comment on its line with
`buti review comment`. The user reads the comments inline in [buti](https://github.com/BartInTheField/buti), under the
lines they are about, and can then fix them or hand them to `/buti-resolve`.

This is a review only. Don't edit files, and don't change commits or branches. The repository is managed by
GitButler: never run git write commands (`git commit`, `git add`, `git checkout`, `git rebase`, `git stash`,
`git reset` and so on). Reading with git or `but` is fine. The `buti review` CLI is the only interface to the
comments: don't read or edit the comment file itself.

## 1. Check buti

Run `which buti`. If it's missing, stop and tell the user to install it:

```sh
curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh
```

If `buti review comment` reports an unknown command, their buti is too old: tell them to update it (`buti` offers to
on start) and stop. The command also needs `but` on `PATH`.

Run the commands from inside the repository, or pass `-C <dir>` before `review`.

## 2. Pick what to review

Run `but status -f` to see the workspace: the uncommitted changes (`zz`, and those assigned to each stack), and each
branch with its commits and their files. The first token on each line is its shortcode (cli id).

The user may have given an argument:

- **Nothing**, or **`zz`**: review the uncommitted changes, unassigned and assigned. `but diff` shows them all.
- **A commit** shortcode, such as `c3`: review that commit. `but diff <commit>` shows it.
- **A branch** name or shortcode: review each of its commits, oldest first, with `but diff <commit>` for each.
  `but diff <branch>` shows the whole branch at once, which helps to judge it as a whole, but comment on the commits.

If there is nothing to review (no uncommitted changes, or the commit or branch is not there), say so and stop.

Then run `buti review list --status all --json` to see the comments already there, so you don't repeat one that the
user or an earlier review left, or that was resolved or dismissed.

## 3. Review

Read each changed file in full around the hunks, not only the diff lines, and follow what the change calls or breaks.
Look for, in this order:

1. Bugs: wrong logic, missed cases, errors that are dropped, races, leaks, broken callers.
2. Security: secrets in code, injection, unchecked input, unsafe permissions.
3. Tests that are missing for the new behaviour, or that don't test what they say.
4. Design and naming: the change fits the code around it, no needless complexity or duplication.
5. Style that the project's own conventions (a CLAUDE.md, AGENTS.md, CONTRIBUTING.md, linters) ask for.

Report what you are confident about. Leave out what a formatter or linter already enforces, and don't comment on
lines the change doesn't touch unless the change breaks them.

## 4. Leave the comments

Leave one comment per finding, on the line it is about:

```sh
buti review comment --file <path> --line <n> [--end-line <n>] [--side new|old] [--shortcode <id>] --body "<tag> <text>"
```

- `--file` is the path as `but status` shows it, relative to the repository root.
- `--line` (and `--end-line` for a range) count lines as the file is after the change: the right-hand number column
  of `but diff`, which for uncommitted changes is the line number in the file on disk. On a line the change removed,
  pass `--side old` with the left-hand number, from before the change.
- `--shortcode` says where the lines are: `zz`, a stack, a commit or a branch shortcode (a branch means the commit on
  it that has the lines), or a file's own id from `but status -f` or `but diff` (such as `c3:m`), which also sets
  `--file`. Leave it out for uncommitted changes. Pass the commit's shortcode for a commit review.
- The lines have to be in that diff: added, removed or shown as context. If the command says they are not, check the
  numbers against the hunk header and try again; put a finding about the whole file on its first changed line.
- Comments are signed `agent`. `--author <name>` signs them with another name.

Start every body with one severity tag, then say what is wrong and what to do instead, in a sentence or two:

| Tag | For |
|---|---|
| `[must-fix]` | bugs, security problems, broken behaviour: should not be merged as is |
| `[suggestion]` | a clear improvement worth making: a missing test, a simpler way, a better name |
| `[nit]` | small and optional: wording, a typo, a tidier line |
| `[question]` | something you can't judge from the code: ask it |

For example:

```sh
buti review comment --file src/auth/token.go --line 3 --shortcode c3 \
  --body "[must-fix] The token is hard-coded. Read it from the environment and fail when it is unset."
```

The command prints the new comment's id. Quote shell metacharacters in the body (`$`, backticks, `"`): single quotes
are the safest.

## 5. Finish

End with a short summary: what you reviewed (`zz`, the commit with its title, or the branch), how many comments of
each tag you left, and the `[must-fix]` ones in one line each with their file and line. Tell the user the comments show
in buti under their lines (the palette's **Review comments…** lists them all), and that `/buti-resolve` can then work
through them. If you found nothing worth a comment, say so and leave none.
