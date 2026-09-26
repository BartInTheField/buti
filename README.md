# buti

A terminal UI for [GitButler](https://gitbutler.com), built with
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea). It covers what the built-in `but tui` can do, laid out
like the desktop app: an **Unstaged** file tree on the left, one lane per stack with branch cards, and a details pane
with syntax-highlighted diffs ([Chroma](https://github.com/alecthomas/chroma)). Everything runs through the `but` CLI.

## Install

On Linux or macOS:

```sh
curl -fsSL https://raw.githubusercontent.com/BartInTheField/buti/main/install.sh | sh
```

You can also download a binary from the [latest release](https://github.com/BartInTheField/buti/releases/latest), or
build from source:

```sh
go install github.com/bartinthefield/buti/cmd/buti@latest
```

Every merge to `main` is released automatically, versioned with [CalVer](https://calver.org) as
`YYYY.MM.DD.N`, where `N` counts the releases of that day (`2026.09.26.1`, `2026.09.26.2`, ...).

## Setup

```sh
mise install     # pinned Go toolchain + golangci-lint
mise run run     # run in the current repo
mise run build   # -> bin/buti
mise run test    # unit tests (a fake `but`)
BUTI_INTEGRATION=1 go test ./internal/but   # every operation against the real `but` CLI
```

```
buti [-C dir] [--diff] [--remember-selection] [--version] [target]
```

The `but` CLI has to be on `PATH`, and the repo has to be set up with GitButler (`but setup`). `target` is a CLI id or
branch name to select on start, `--diff` opens the details pane, and `--remember-selection` restores the last selection.

## How it works

Most operations follow the same steps:

1. **Select** a source, or mark several with `space`.
2. **Press a verb:** `c` commit, `r` squash/amend, `m` move, `p` cherry-pick.
3. **Pick a target.** Each row shows what would happen there ("amend", "move above", "stack onto"), invalid targets are
   dimmed, and the status bar says the full action.
4. **Confirm** with `enter` or a click.

You can also **drag** a file, commit or branch onto a branch, a commit, the **Unstaged** header or the "new branch"
lane. The drop does the natural thing:

| Drag | Onto | Does |
|---|---|---|
| file / hunk / folder | branch | commit it to the branch |
| file / hunk / folder | commit | amend it into the commit |
| commit | commit | squash the commits |
| commit | branch | move the commit to the branch |
| commit / branch / committed file | Unstaged | uncommit it |
| branch | branch | stack it onto the branch |
| branch | new-branch lane | unstack it |

Every command is also in the palette (`ctrl+p`), the searchable help (`?`) and the right-click menu, which only lists
what applies to the selection.

## Keys

| Key | Action |
|---|---|
| `c` | commit the selection, marks, or everything (in the target picker: `a` above/below, `b` new branch here, `e` empty message) |
| `r` / `R` | squash, amend or uncommit into a target (`u` keeps the target message) / amend all changes into the selection |
| `m` / `p` | move / cherry-pick commits (`a` above/below); move a branch to stack or unstack it |
| `enter` | reword a commit or rename a branch; open a file's diff; fold a folder |
| `M` | reword in `$EDITOR` |
| `n` | insert an empty commit |
| `A` | absorb changes into the commits they belong to |
| `x` | discard, after confirming (undo with `u`) |
| `b` / `B` | new branch: stacked on the selected branch, or a new lane / below the selected branch |
| `P` / `N` | push the branch / open a pull request |
| `a` / `S` | apply a branch (picker) / unapply the stack |
| `L` | pull upstream changes |
| `u` / `U` / `H` | undo / redo / operation history (restore any snapshot) |
| `space` | mark (on **Unstaged**, marks every file) |
| `f` / `F` | files in the commit / in every commit |
| `d` / `D` / `+` `-` | details pane / full screen / resize |
| `tab` | cycle focus: sidebar → lanes → details. In details: `j`/`k` hunks, `space` marks a hunk, then `c` / `r` / `x` on hunks |
| `y` / `Y` | copy (branch name, change id, path, hunk) / pick what to copy |
| `o` / `O` | open in `$EDITOR` / with the default app |
| `/` / `t` | go to anything (fuzzy) / go to a branch |
| `:` / `!` | run a `but` command (the output is shown) / a shell command |
| `j` `k` `h` `l` · `J` `K` · `g` `G` | move · next/previous branch · top/bottom |
| `esc` | back: leave the mode, clear marks, close details |
| `ctrl+r` · `q` | reload (it also polls every 3 seconds) · quit |

**Mouse:**
- click to select; double-click for the full diff; right-click for actions;
- the card buttons (**Start a commit…**, **Push**, `⋯`) and the "new branch" lane are clickable;
- the wheel scrolls whatever is under the pointer.

## Layout

- `cmd/buti`: entrypoint and flags
- `internal/but`: the `but` CLI client
  - reads JSON: status, diff, branch list, oplog
  - runs mutations; each one has a function
- `internal/ui`:
  - `model.go`: state, keys, layout
  - `actions.go`: the command registry, which drives the keys, palette, help and context menu
  - `target.go`: the source × target matrix
  - `mouse.go`: clicks and drag and drop
  - `lanes.go`, `sidebar.go`: lanes and the Unstaged sidebar
  - `details.go`, `diff.go`, `highlight.go`: the details pane and diffs
  - `compose.go`, `modal.go`, `toast.go`: the message composer, dialogs, notifications

## Not (yet) ported from `but tui`

- **Reordering stacks:** `but tui` uses an internal API for this, and there is no CLI command.
- **Worktree mode:** `but tui` puts it behind a feature flag.
- **Single-branch-mode switching:** `but tui` puts it behind a feature flag.
