# Development

## Setup

[mise](https://mise.jdx.dev) pins the toolchain and defines the tasks:

```sh
mise install                # Go, golangci-lint, freeze
mise run run                # run buti in the current repo
mise run build              # -> bin/buti
mise run lint
mise run test               # unit tests (a fake `but`)
mise run test:integration   # plus the tests against the real `but` CLI
mise run screenshots        # end-to-end tests, saving every screen to screenshots/
mise run readme-screenshot  # refresh the screenshots in docs/images
mise run fixture            # create the test repository to try buti by hand
```

The integration tests and the fixture need the [GitButler CLI](https://docs.gitbutler.com/cli-guides/installation) on
`PATH`. See [Testing](testing.md) for how the tests are put together.

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
  - `update.go`: offering and installing a new release
- `internal/review`: the review comment store, `<git-common-dir>/buti/review.json`, shared by the TUI and the CLI.
  `Locate` / `Store.Reanchor` map the stored anchors onto the current `but status`, deriving each comment's shortcode,
  its moved line, and the outdated and orphaned statuses
- `internal/update`: finding the latest release and replacing the running binary
- `internal/testrepo`: the test repository (see [Testing](testing.md))

## CI and releases

- **Pull requests** (`.github/workflows/test.yml`): the `unit` and `integration` jobs must pass before a PR can merge
  into `main`. See [Testing](testing.md#ci).
- **Releases** (`.github/workflows/release.yml`): every hour, if `main` has new commits, the tests run again and a
  release is published for Linux, macOS and Windows. Versions are [CalVer](https://calver.org) `YYYY.MM.DD.N`, where
  `N` counts the releases of that (UTC) day: `2026.09.26.1`, `2026.09.26.2`, ... The version is baked in with
  `-ldflags "-X main.version=..."`; builds without it report `dev` and never check for updates.
- `install.sh` downloads the latest release for the current platform.
