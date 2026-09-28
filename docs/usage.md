# Usage

```sh
buti [-C dir] [--diff] [--remember-selection] [--version] [target]
buti [-C dir] review <command> ...
buti [-C dir] skill <command> ...
```

| Flag | Does |
|---|---|
| `-C dir` | run as if started in `dir` |
| `--diff` | open the details pane on start |
| `--remember-selection` | restore the selection from the last session in this repository |
| `--version` | print the version and exit |
| `target` | a CLI id or branch name to select on start |

The `but` CLI has to be on `PATH`, and the repository has to be set up with GitButler (`but setup`). `buti review`
runs the [review commands](#review-comments-for-coding-agents) and `buti skill` installs the
[agent skills](#resolving-comments-with-a-coding-agent), both without starting the TUI; to select a branch called
`review` or `skill` on start, run `buti -- review`.

## The screen

- **Unstaged** (left): uncommitted changes as a file tree. Select the header to act on all of them.
- **Lanes** (right): one per stack, with a card per branch, top of the stack first. Each card lists its commits.
- **Details** (bottom, `d`): the diff of whatever is selected, syntax highlighted. `D` makes it full screen. With the
  pane focused (`tab`, or a click), a line cursor (`▶` in the gutter) moves line by line with `j`/`k` or a click, and
  `[`/`]` jump between hunks; the hunk around the cursor is the one `space`, `c`, `r` and `x` act on. `v` (or
  shift-click) selects a range of lines within the hunk, marked `┃`; moving the cursor extends it and `esc` cancels it.
- **Status bar**: the mode, the keys that apply now, and how far the workspace is behind upstream.

buti reloads every 3 seconds. A branch's pull request and its checks come from GitButler's cache, which the reload
doesn't sync with the forge, because that is slow. buti syncs it after you push or create a pull request, and when you
press `ctrl+r`. Press `ctrl+r` to pick up a pull request opened elsewhere (with `gh`, say) or checks that finished.

## Select, verb, target

Most operations follow the same steps:

1. **Select** a source, or mark several with `space`.
2. **Press a verb:** `c` commit, `r` squash/amend, `m` move, `p` cherry-pick.
3. **Pick a target.** Each row shows what would happen there ("amend", "move above", "stack onto"), invalid targets are
   dimmed, and the status bar says the full action.
4. **Confirm** with `enter` or a click.

Every command is also in the palette (`ctrl+p`), the searchable help (`?`) and the right-click menu, which only lists
what applies to the selection. See [Keys](keys.md) for all of them.

## Drag and drop

You can **drag** a file, commit or branch onto a branch, a commit, the **Unstaged** header or the "new branch" lane.
The drop does the natural thing:

| Drag | Onto | Does |
|---|---|---|
| file / hunk / folder | branch | commit it to the branch |
| file / hunk / folder | commit | amend it into the commit |
| commit | commit | squash the commits |
| commit | branch | move the commit to the branch |
| commit / branch / committed file | Unstaged | uncommit it |
| branch | branch | stack it onto the branch |
| branch | new-branch lane | unstack it |

## Conflicts

A conflicted commit (`✗`) is resolved in edit mode: select it and press `e`, then fix its files in your editor. See
[Resolving conflicts](conflicts.md).

## Mouse

- Click to select; double-click for the full diff; right-click for actions.
- The card buttons (**Start a commit…**, **Push**, `⋯`) and the "new branch" lane are clickable.
- The wheel scrolls whatever is under the pointer.

## Updates

Releases are versioned with [CalVer](https://calver.org) as `YYYY.MM.DD.N`. buti checks for a newer release on start
(at most once a day) and offers to update itself in place. Set `BUTI_NO_UPDATE_CHECK=1` to turn that off. **Update buti**
in the help (`?`) or the command palette checks right away, and **Version** shows the version you're running. Builds
without a release version (`go run`, `go install`) never check.

## Review comments

Leave comments on a diff for a coding agent to pick up with [`/buti-resolve`](#resolving-comments-with-a-coding-agent),
or read the ones an agent left with [`/buti-review`](#reviewing-with-a-coding-agent):

1. Put the line cursor on a line in the details pane (or select a range with `v`) and press `C`. Write the comment
   (`enter` starts a new line) and save it with `ctrl+s`. Comments go on uncommitted changes, unassigned or assigned,
   and on commits. In a branch's diff, a comment goes on the branch's commit that has the lines (the newest that
   changes them), which the composer names; it shows in the branch's diff and in the commit's.
2. The comment shows in a box under its last line, headed by who wrote it: `you`, or `agent` (in teal) for one a
   coding agent left. A leading severity tag (`[must-fix]`, `[suggestion]`, `[nit]`, `[question]`) is coloured, and
   replies show as `↳ author: text`. Files in **Unstaged** and in the lanes, and commits, show how many
   open comments they have (`✎2`).
3. `j`/`k` also stop on a comment. On one, `e` edits it, `d` deletes it after confirming, and `x` resolves or reopens it.
4. Resolved and dismissed comments are collapsed to one muted line with the resolution; `z` hides or shows them.
5. **Review comments…** in the command palette (`ctrl+p`) lists every open comment; `enter` opens its diff with the
   cursor on it.

buti reads the comments again whenever the workspace or the comment file changes (it checks every 3 seconds), so what
an agent resolves or replies shows up on its own. A comment follows its line when lines move, an uncommitted
comment follows its file into the commit it lands in, and a comment on a commit you uncommit or squash follows the
lines the commit added or removed into the uncommitted changes or the other commit. One whose line is gone is marked
outdated and drawn at the end of its file, with the text it was on.

## Review comments for coding agents

Review comments live in `<git-common-dir>/buti/review.json`, which is never committed. A coding agent reads and closes
them with `buti review`, which needs no terminal and respects `-C dir`:

```sh
buti review list [--status open|resolved|dismissed|outdated|orphaned|all] [--author <name>] [--json]
buti review show <id> [--json]
buti review comment --file <path> --line <n> [--end-line <n>] [--side new|old] [--shortcode <id>] --body "<text>" [--author <name>]
buti review resolve <id> [--summary "<text>"]
buti review reply <id> --body "<text>" [--author <name>]
buti review dismiss <id> [--reason "<text>"]
buti review clear [--resolved] [--dismissed]
```

- An `<id>` can be shortened to any prefix that is unique.
- `list` shows the open comments by default. `open` includes the comments whose line is gone (`outdated`) or whose
  commit is gone (`orphaned`), because they still want an answer; `--status outdated` or `orphaned` lists only those.
- `list` and `show` run `but status` and `but diff` to find each comment's current place: a comment follows its line
  when lines are added above it, an uncommitted comment follows its file when it is committed, and a comment on a
  commit that was uncommitted or squashed follows the lines the commit added or removed into the uncommitted changes
  or the other commit. The new places are saved. `comment` runs them to find the lines. The other commands only
  write the comment file and don't need `but`.
- `list --author user` (or `agent`) lists only one author's comments.
- `comment` leaves a comment, as a reviewing agent does. `--line` and `--end-line` count lines on `--side`: `new` (the
  default) numbers the file after the change, which for uncommitted work is the file on disk; `old` numbers it before,
  for lines the change removed. `--shortcode` says where the lines are: `zz`, a stack, a commit, a branch (the commit on
  it that has the lines, preferring the one that changed them) or a file's own id (`rl`, `c3:m`, which also sets
  `--file`); without it, the uncommitted changes. The lines have to be in that diff, and the comment stores their text
  so it follows them like one left in buti.
- `comment` and `reply` are signed `agent` unless you pass `--author`.

`list --json` prints one flat object per comment:

```json
[
  {
    "id": "a1b2c3",
    "status": "open",
    "author": "user",
    "body": "Rename this to hunkIndex",
    "file": "internal/ui/diff.go",
    "line": 42,
    "end_line": 42,
    "side": "new",
    "shortcode": "zz",
    "file_shortcode": "k4",
    "kind": "unassigned",
    "branch": null,
    "commit": null,
    "outdated": false,
    "context": "  40 | ...\n  41 | ...\n> 42 | ...\n  43 | ...\n  44 | ..."
  }
]
```

| Field | Is |
|---|---|
| `status` | `open`, `resolved` or `dismissed` |
| `author` | `user` for a comment left in buti, `agent` (or the `--author` name) for one left with `comment` |
| `line`, `end_line`, `side` | the commented lines as they are now, counted on the `new` or `old` side of the diff |
| `shortcode` | the GitButler cli id of what the comment is on: `zz` for unassigned changes, the stack's id for assigned ones, the commit's id for a commit; `null` when the commit is gone |
| `file_shortcode` | the file's cli id within `shortcode`, e.g. to `but amend` it; `null` when the file isn't there |
| `kind` | `unassigned`, `assigned` or `commit` |
| `branch` | the branch of an assigned change or a commit, `null` for unassigned changes |
| `commit` | for a commit comment, `{"title": ..., "sha": ...}`; otherwise `null` |
| `outdated` | `true` when the commented lines are no longer in the diff, or the commit is gone; `line` and `context` are then the stored ones |
| `context` | the commented lines, marked `>`, with two lines of the diff around them |

`show --json` adds `created_at`, `resolution` (`{"summary", "at"}` or `null`) and `replies`.

## Resolving comments with a coding agent

`/buti-resolve` is a skill that has a coding agent fix the open comments: it reads them with
`buti review list --json`, makes each change, and marks the comment resolved with a one-line summary, which buti then
shows under the comment. A question gets its answer in the summary, an unclear comment gets a reply and stays open, and
an outdated one is left for you. Comments a reviewing agent left with [`/buti-review`](#reviewing-with-a-coding-agent)
are handled too, by their severity tag: it fixes a `[must-fix]` (or replies when it disagrees), makes a `[suggestion]`
or `[nit]` when it is right for the code and dismisses it with the reason when it isn't, and answers a `[question]`.
`/buti-resolve` then works on both unless you ask for only yours or only the agent's. For a comment on a commit, the agent fixes the working tree and then asks whether to
amend the fix into that commit with `but amend`. It never runs git write commands. It ends with a summary per
shortcode (`zz`, each stack, each commit).

Each skill is one agent-agnostic file, [`skills/buti-resolve/SKILL.md`](../skills/buti-resolve/SKILL.md) and
[`skills/buti-review/SKILL.md`](../skills/buti-review/SKILL.md), which any agent that can follow a markdown skill and run
shell commands can use. buti carries a copy of both, so they match the `buti review` commands of the version you run:

```sh
buti skill list                          # the skills buti carries
buti skill show buti-resolve             # print a SKILL.md
buti skill install --agent <agent>       # into the agent's skills directory in your home directory
buti skill install --agent <agent> --project   # into the repository instead, for everyone who works on it
buti skill install --target <dir>        # into <dir>/buti-resolve and <dir>/buti-review, for any other agent
```

`install` copies every skill unless you name some (`buti skill install --agent claude buti-resolve`), and replaces an earlier install, so run it again after updating
buti. The [skills CLI](https://github.com/vercel-labs/skills) can also install them from the repository:
`npx skills add BartInTheField/buti`. The agents `--agent` knows:

| `--agent` | Directory (home) | With `--project` | Invoke |
|---|---|---|---|
| `claude` (Claude Code) | `~/.claude/skills` | `.claude/skills` | `/buti-resolve`, or ask to resolve the buti comments |
| `cursor` (Cursor) | `~/.cursor/skills` | `.cursor/skills` | `/buti-resolve` in the agent chat |
| `codex` (OpenAI Codex) | `~/.agents/skills` | `.agents/skills` | `$buti-resolve`, or pick it from `/skills` |
| `opencode` (OpenCode) | `~/.config/opencode/skills` | `.opencode/skills` | ask to resolve the buti comments; the agent loads the skill |
| `copilot` (GitHub Copilot) | `~/.copilot/skills` | `.github/skills` | ask to resolve the buti comments |
| `agents` (shared Agent Skills) | `~/.agents/skills` | `.agents/skills` | as the agent does |

For an agent that isn't listed, point `--target` at its skills directory. One without skills can take the file as a
prompt: save it with `buti skill show buti-resolve > buti-resolve.md` and add it to the chat, for example with
`/read-only buti-resolve.md` in Aider, then ask it to resolve the buti review comments. You can also give an argument:
a comment id to fix only that one, or a shortcode (`zz`, a commit id) to fix only the comments there, as in
`/buti-resolve c3`. `/buti-review` is invoked the same way (`/buti-review`, `$buti-review`, or ask for a review to read
in buti).

## Reviewing with a coding agent

`/buti-review` is the other direction: a coding agent reviews a diff and leaves its findings as comments for you to
read in buti. It changes no code and never runs git write commands.

1. Run `/buti-review` in your agent: with no argument it reviews the uncommitted changes (`zz`, unassigned and
   assigned); give it a commit (`/buti-review c3`) or a branch (`/buti-review auth`, each of its commits) to review
   that instead.
2. The agent reads the diff with `but diff`, checks the comments already there so it doesn't repeat one, and leaves each
   finding on its line with `buti review comment`. Every comment starts with a severity tag: `[must-fix]` (bugs,
   security, broken behaviour), `[suggestion]` (a clear improvement), `[nit]` (small and optional) or `[question]`.
   It ends with a count per tag and the `[must-fix]` ones.
3. buti shows them within a few seconds, signed `agent`, under their lines and in **Review comments…**. Fix some
   yourself, delete (`d`) or resolve (`x`) the ones you don't want acted on, and run `/buti-resolve` to have an agent
   work through the rest.

It installs with `buti skill install` alongside `/buti-resolve`, as [above](#resolving-comments-with-a-coding-agent).
An agent without skills can take it as a prompt: `buti skill show buti-review > buti-review.md`.

## Not (yet) ported from `but tui`

- **Reordering stacks:** `but tui` uses an internal API for this, and there is no CLI command.
- **Worktree mode:** `but tui` puts it behind a feature flag.
- **Single-branch-mode switching:** `but tui` puts it behind a feature flag.
