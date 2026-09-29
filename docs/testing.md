# Testing

There are three layers, from fast and fake to slow and real:

| Layer | Where | `but` | Runs with |
|---|---|---|---|
| UI unit tests | `internal/ui/*_test.go` | a fake shell script | `mise run test` |
| Client integration test | `internal/but/integration_test.go` | real | `mise run test:integration` |
| Re-anchoring integration test | `internal/review/locate_integration_test.go` | real, in the test repository | `mise run test:integration` |
| `buti review` CLI tests | `internal/reviewcli/*_test.go` | a fake shell script | `mise run test` |
| `buti review` integration test | `cmd/buti/main_test.go` | real, in the test repository | `mise run test:integration` |
| UI end-to-end tests | `internal/ui/e2e_test.go` | real, in the test repository | `mise run test:integration` |

The real-`but` tests are skipped unless `BUTI_INTEGRATION=1` is set and `but` is on `PATH`.

## UI unit tests

`newHarness` (in `internal/ui/helpers_test.go`) builds a `Model` against a fake `but` that serves a fixed status and
logs every other command. The harness sends keys, clicks and drags, runs the commands the model returns the way Bubble
Tea would, and `h.but.expect(...)` checks which `but` commands ran:

```go
h := newHarness(t)
h.dragTo("go.mod", "second")
h.but.expect("amend --target c2 f2")
```

`h.but.setDiff(d)` makes every `but diff` return `d`, for the details pane and for locating review comments; review
comments go to a scratch store in `t.TempDir()` (`h.m.review`), since the fake is no git repository.

Commands that don't return quickly (the refresh tick, toast timers) are dropped after `h.wait`, and animation frames
(spinner, cursor blink) are dropped so they don't loop.

## The test repository

`internal/testrepo` builds a scratch repository in a known GitButler state, through git and the real `but`:

| | |
|---|---|
| `origin/main` | one commit ahead of the local base, so the workspace is 1 behind |
| stack 1 | `auth` (2 unpushed commits) stacked on `api` (2 pushed commits) |
| stack 2 | `fix-typo`: 1 pushed commit and 1 local one |
| stack 3 | `empty`: no commits |
| unapplied | `old-experiment`, 1 commit |
| uncommitted | `README.md` and `src/server.go` modified, `src/util/strings.go` added, `docs/old.md` deleted |
| `Repo.Conflict()` | adds `changelog` (a commit adding `CHANGELOG.md`) and pulls, which leaves that commit conflicted |

`origin` is a local bare repository, so pushing works offline. `but` and git run with their own `HOME`, XDG dirs and a
fixed identity (`testrepo.Env`), so your GitButler project list and settings are left alone. Commit dates are fixed,
but `but` gives every commit a random change id, so commit and CLI ids differ between runs: tests find things by name.
Create the repository in `testrepo.TempDir(t)` rather than `t.TempDir()`: `but` can still be writing its settings
under that `HOME` when a test ends, and `t.TempDir` fails the test when its cleanup loses that race.

To look at it yourself:

```sh
mise run fixture   # prints the command to run buti against it
go run ./internal/testrepo/mkrepo -conflict "$(mktemp -d)"   # with the conflicted commit
```

`but` 0.22 has no command to assign a change to a stack, so the fixture has no assigned changes.

## End-to-end tests

`newRepoHarness` creates the test repository in a temp dir and drives the real UI against it. Assertions check both the
screen and the resulting workspace, read straight from `but`:

```go
h, _ := newRepoHarness(t)
h.selectText("README.md")
h.keys("c")
h.hover("branch:empty")
h.keys("enter")
h.typeText("Document usage")
h.keys("enter")
if got := h.commits("empty"); len(got) != 1 || got[0] != "Document usage" { ... }
```

### Screenshots

`h.snap("name")` saves the screen when `BUTI_SCREENSHOTS=<dir>` is set: `<dir>/<Test>-<name>.ansi` always, and a
`.png` when [freeze](https://github.com/charmbracelet/freeze) is on `PATH` (`mise install` provides it).

```sh
mise run screenshots   # -> screenshots/*.png (git-ignored)
```

This is the way to check a UI change by eye, including for agents that can read images. The README image is the
`Workspace-hero` snap, and [Resolving conflicts](conflicts.md) uses the `ResolveInEditMode` ones; `mise run
readme-screenshot` refreshes them all.

### The demo video

`TestE2EVideo` (`internal/ui/video_test.go`) walks the real UI through the README's flows and, with
`BUTI_VIDEO=<dir>`, saves every screen as a frame with how long it stays on, its caption and the mouse pointer's cell
while dragging. `tools/video` renders the frames with freeze and cuts them into a video with ffmpeg, with a title
card, an end card and captions:

```sh
mise run video   # -> video/demo.mp4 (1080p) and video/demo.gif (git-ignored)
```

Change the script in the test to change the video; the recorder's `frame`, `step`, `typeText` and `drag` helpers
take the time each screen stays on.

The marketing trailer (`trailer/`, a [Remotion](https://www.remotion.dev) project) is cut from the same frames: the
recorder also notes where texts worth zooming in on are (`videoAnchors`), and `trailer/src/shots.ts` says which
frame shows when, where the camera looks, and which label and key cap go with it, snapped to the music's bars.
`mise run trailer` renders `trailer/out/trailer.mp4` plus the X banner and avatar; the brand it uses is in
`brand/`.

## CI

`.github/workflows/test.yml` runs on every pull request and push to `main`:

- **unit**: `go vet` and `go test ./...`.
- **integration**: installs a pinned `but` release (URL and sha256 in the workflow; bump both together) and freeze,
  then runs every test with `BUTI_INTEGRATION=1`. The screenshots are uploaded as the `screenshots` artifact of the run.

Branch protection on `main` requires both jobs to pass before a PR can merge. If you rename a job, update the required
checks in the repository settings too.
