# buti

A terminal UI for [GitButler](https://gitbutler.com). It covers what the built-in `but tui` can do, laid out like the
desktop app: an **Unstaged** file tree on the left, one lane per stack with branch cards, and a details pane with
syntax-highlighted diffs. Commit, squash, move and stack with a few keys or by dragging with the mouse. Everything runs
through the `but` CLI.

![buti: uncommitted files on the left, a lane per stack, and a diff; a file is being committed to a stacked branch](docs/images/screenshot.png)

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

buti needs the [GitButler CLI](https://docs.gitbutler.com/cli-guides/installation) (`but`) on `PATH`, in a repository
set up with `but setup`. It checks for a new release on start and offers to update itself; set
`BUTI_NO_UPDATE_CHECK=1` to turn that off.

## Usage

```sh
buti [-C dir] [--diff] [--remember-selection] [--version] [target]
```

Select something, press a verb (`c` commit, `r` squash/amend, `m` move, `p` cherry-pick), pick a target and press
`enter`. Or drag a file, commit or branch onto where it should go. `?` lists every key and `ctrl+p` opens the command
palette.

Leave review comments on a diff with `C` and have any coding agent fix them with the
[`/buti-resolve` skill](docs/usage.md#resolving-comments-with-a-coding-agent), or have one review your changes into
buti with [`/buti-review`](docs/usage.md#reviewing-with-a-coding-agent) (`buti skill install --agent claude`).

## Docs

- [Usage](docs/usage.md): flags, the select → verb → target flow, drag and drop, the mouse, review comments,
  `/buti-resolve` and `/buti-review`
- [Resolving conflicts](docs/conflicts.md): fixing a conflicted commit in edit mode
- [Keys](docs/keys.md): every key binding
- [Development](docs/development.md): building, the code layout, releases
- [Testing](docs/testing.md): unit tests, the test repository, end-to-end tests and screenshots, CI

Found a bug or have an idea? [Open an issue](https://github.com/BartInTheField/buti/issues/new/choose).

## License

[MIT](LICENSE)
