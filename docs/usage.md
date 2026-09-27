# Usage

```sh
buti [-C dir] [--diff] [--remember-selection] [--version] [target]
buti [-C dir] review <command> ...
```

| Flag | Does |
|---|---|
| `-C dir` | run as if started in `dir` |
| `--diff` | open the details pane on start |
| `--remember-selection` | restore the selection from the last session in this repository |
| `--version` | print the version and exit |
| `target` | a CLI id or branch name to select on start |

The `but` CLI has to be on `PATH`, and the repository has to be set up with GitButler (`but setup`). `buti review`
runs the [review commands](#review-comments-for-coding-agents) without starting the TUI; to select a branch called
`review` on start, run `buti -- review`.

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

## Review comments for coding agents

Review comments live in `<git-common-dir>/buti/review.json`, which is never committed. A coding agent reads and closes
them with `buti review`, which needs no terminal and respects `-C dir`:

```sh
buti review list [--status open|resolved|dismissed|outdated|orphaned|all] [--json]
buti review show <id> [--json]
buti review resolve <id> [--summary "<text>"]
buti review reply <id> --body "<text>" [--author <name>]
buti review dismiss <id> [--reason "<text>"]
buti review clear [--resolved] [--dismissed]
```

- An `<id>` can be shortened to any prefix that is unique.
- `list` shows the open comments by default. `open` includes the comments whose line is gone (`outdated`) or whose
  commit is gone (`orphaned`), because they still want an answer; `--status outdated` or `orphaned` lists only those.
- `list` and `show` run `but status` and `but diff` to find each comment's current place: a comment follows its line
  when lines are added above it, and an uncommitted comment follows its file when it is committed. The new places are
  saved. The other commands only write the comment file and don't need `but`.
- `reply` is signed `agent` unless you pass `--author`.

`list --json` prints one flat object per comment:

```json
[
  {
    "id": "a1b2c3",
    "status": "open",
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
| `line`, `end_line`, `side` | the commented lines as they are now, counted on the `new` or `old` side of the diff |
| `shortcode` | the GitButler cli id of what the comment is on: `zz` for unassigned changes, the stack's id for assigned ones, the commit's id for a commit; `null` when the commit is gone |
| `file_shortcode` | the file's cli id within `shortcode`, e.g. to `but amend` it; `null` when the file isn't there |
| `kind` | `unassigned`, `assigned` or `commit` |
| `branch` | the branch of an assigned change or a commit, `null` for unassigned changes |
| `commit` | for a commit comment, `{"title": ..., "sha": ...}`; otherwise `null` |
| `outdated` | `true` when the commented lines are no longer in the diff, or the commit is gone; `line` and `context` are then the stored ones |
| `context` | the commented lines, marked `>`, with two lines of the diff around them |

`show --json` adds `author`, `created_at`, `resolution` (`{"summary", "at"}` or `null`) and `replies`.

## Not (yet) ported from `but tui`

- **Reordering stacks:** `but tui` uses an internal API for this, and there is no CLI command.
- **Worktree mode:** `but tui` puts it behind a feature flag.
- **Single-branch-mode switching:** `but tui` puts it behind a feature flag.
