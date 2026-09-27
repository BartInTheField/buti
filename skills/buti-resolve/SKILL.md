---
name: buti-resolve
description: Resolve the open review comments left in buti, the terminal UI for GitButler, by the user or by a reviewing agent (/buti-review). Use when the user asks to resolve, address or fix their buti review comments, or runs /buti-resolve. Reads the comments with `buti review list --json`, makes each change, and marks it resolved so buti shows the result.
---

# Resolve buti review comments

The user left review comments on lines of a diff in [buti](https://github.com/BartInTheField/buti): on uncommitted
changes or on a commit in a GitButler workspace. Some may come from a reviewing agent instead (`/buti-review`), which the
user has read in buti and left open. Work through the open ones: make each change, then mark the comment
resolved with a short summary. buti picks up the resolutions on its own.

The `buti review` CLI is the only interface to the comments. Don't read or edit the comment file itself. The repository
is managed by GitButler: use `but` for anything that changes commits or branches, and never run git write commands
(`git commit`, `git add`, `git checkout`, `git rebase`, `git stash`, `git reset` and so on). Reading with git is fine.

## 1. Check buti

Run `which buti`. If it's missing, stop and tell the user to install it:

```sh
curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh
```

If `buti review` reports an unknown command, their buti is too old: tell them to update it (`buti` offers to on
start) and stop. `buti review list` also needs `but` on `PATH`.

Run the commands from inside the repository, or pass `-C <dir>` before `review`.

## 2. Fetch the comments

```sh
buti review list --status open --json
```

The user may have given an argument:

- **A comment id** (or a prefix of one): only take that comment. `buti review show <id> --json` prints it, with its
  replies.
- **A shortcode**, such as `zz` or a commit id like `c3`: only take the comments whose `shortcode` is that value.
- **Only theirs, or only the review's:** add `--author user` or `--author agent` to the list command.

Each comment is one object:

| Field | Is |
|---|---|
| `id` | what `resolve`, `reply` and `dismiss` take; a unique prefix works too |
| `author` | `user` for the user's own comments; `agent` (or another name) for one a reviewing agent left |
| `body` | the comment: a request, or a question |
| `file`, `line`, `end_line`, `side` | the commented lines, now. `side` is `new` (the file as changed) or `old` (removed lines, which are no longer in the file) |
| `context` | the commented lines marked `>`, with two lines of the diff around them. Trust this over the line numbers |
| `kind` | `unassigned` (uncommitted, shortcode `zz`), `assigned` (uncommitted, assigned to a branch) or `commit` |
| `shortcode` | the GitButler cli id of where the comment lives: `zz`, the stack's id, or the commit's id. `null` when the commit is gone |
| `file_shortcode` | the file's cli id within `shortcode` |
| `branch`, `commit` | the branch, and for a commit comment `{"title", "sha"}` |
| `outdated` | `true` when the commented lines, or the commit, are no longer there |

Shortcodes are recomputed on every call and change when the workspace changes, so run `buti review list --json` again
after changing commits rather than reusing old ones.

## 3. Nothing open

If the list is empty (or nothing matches the argument), say there are no open review comments and stop.

## 4. Work through each comment

For each comment, oldest first:

1. Read the file around the line. Find the place by the text in `context`: earlier edits in the same file shift the
   line numbers. For an `old`-side comment, the commented lines were removed; the comment is about that removal.
2. Decide what the comment asks for:
   - **A change:** make it, keeping to what the comment asks, then run
     `buti review resolve <id> --summary "Fixed: <what you changed>"`.
   - **A question:** answer it in the summary instead of changing code:
     `buti review resolve <id> --summary "<the answer>"`.
   - **Unclear**, or it needs a decision from the user: ask with `buti review reply <id> --body "<question>"` and leave
     it open. Don't guess.
   - **Nothing to do** (already done, or no longer applies): `buti review dismiss <id> --reason "<why>"`.
3. **A comment by an agent** (`author` is not `user`) starts with a severity tag. The user left it open, so it wants
   handling like theirs, but it is a reviewer's opinion, not the user's request:
   - `[must-fix]`: fix it, or reply if you believe it is wrong, saying why, and leave it for the user.
   - `[suggestion]` and `[nit]`: make the change when it is right for the code; when it isn't, dismiss it with the
     reason.
   - `[question]`: answer it in the resolve summary, or reply when only the user can answer.
4. **Outdated** (`outdated: true`) or **orphaned** (`shortcode: null`, the commit is gone): don't guess where it
   belongs. Leave it open and tell the user about it in the summary, with its `body` and stored `context`.

Keep summaries to one line. They show in buti under the comment.

## 5. Comments on a commit

For a comment with `kind: commit`, make the fix in the working tree like any other. The change is then uncommitted.
When the comments on that commit are done, ask the user whether to amend the fix into that commit or leave it
uncommitted. To amend:

1. Run `but status --json` (or `but status`) and find the cli id of the uncommitted change to the file.
2. Run `but amend <that id> -t <commit shortcode>`, with the commit's current shortcode from a fresh
   `buti review list --json` (or the commit's `sha`).
3. If `but amend` reports a conflict or refuses, stop and tell the user.

`but absorb <that id>` amends a change into the commit it depends on, which is often the commented one; check that it
picked the right commit. Amend each file separately, and only the files you changed for these comments. Never use git
write commands for this.

## 6. Finish

End with a short summary grouped by shortcode: `zz`, each stack, and each commit (with its title). Under each, list
the comments resolved (with the summary), replied to (with the question), dismissed (saying which were the agent's), and left open because they are
outdated or orphaned, plus what is still uncommitted or was amended. Tell the user buti shows the resolutions and
replies under each comment.
