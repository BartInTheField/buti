# buti

A terminal UI for [GitButler](https://gitbutler.com), built with
[Bubble Tea v2](https://github.com/charmbracelet/bubbletea). It drives the `but` CLI and reads its `--json` output.

## Setup

```sh
mise install     # installs the pinned Go toolchain + golangci-lint
mise run run     # run in the current repo
mise run build   # -> bin/buti
```

To point it at another repo, use `buti -C path/to/repo`. The `but` CLI has to be on `PATH`, and the repo has to be set up
with GitButler (`but setup`).

## Layout

- `cmd/buti`: entrypoint
- `internal/but`: thin wrapper around the `but` CLI (`status --json -f`, `diff --json`)
- `internal/ui`: Bubble Tea model, laid out like the GitButler desktop app
  - `sidebar.go`: "Unstaged" file tree
  - `lanes.go`: one lane per stack, with a commit box and a branch card per branch (stacked branches are connected)
  - `diff.go` / `highlight.go`: unified diff view, syntax highlighted with [Chroma](https://github.com/alecthomas/chroma)

## Keys

| key             | action                                      |
|-----------------|---------------------------------------------|
| `j`/`k`         | move selection                              |
| `h`/`l`         | move between the sidebar and lanes          |
| `tab`           | toggle focus between the sidebar and lanes  |
| `enter`         | open the diff (folds/unfolds a directory)   |
| `space`         | fold/unfold a directory                     |
| `esc`           | close the diff                              |
| `r`             | refresh (also polls every 3s)               |
| `q`             | quit                                        |
