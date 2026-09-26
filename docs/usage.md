# Usage

```sh
buti [-C dir] [--diff] [--remember-selection] [--version] [target]
```

| Flag | Does |
|---|---|
| `-C dir` | run as if started in `dir` |
| `--diff` | open the details pane on start |
| `--remember-selection` | restore the selection from the last session in this repository |
| `--version` | print the version and exit |
| `target` | a CLI id or branch name to select on start |

The `but` CLI has to be on `PATH`, and the repository has to be set up with GitButler (`but setup`).

## The screen

- **Unstaged** (left): uncommitted changes as a file tree. Select the header to act on all of them.
- **Lanes** (right): one per stack, with a card per branch, top of the stack first. Each card lists its commits.
- **Details** (bottom, `d`): the diff of whatever is selected, syntax highlighted. `D` makes it full screen.
- **Status bar**: the mode, the keys that apply now, and how far the workspace is behind upstream.

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

## Not (yet) ported from `but tui`

- **Reordering stacks:** `but tui` uses an internal API for this, and there is no CLI command.
- **Worktree mode:** `but tui` puts it behind a feature flag.
- **Single-branch-mode switching:** `but tui` puts it behind a feature flag.
