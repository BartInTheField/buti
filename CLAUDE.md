# CLAUDE.md

Guidance for AI agents working on buti. Humans may find it useful too; the full docs are in `docs/`.

## What buti is

A terminal UI for [GitButler](https://gitbutler.com), written in Go with Bubble Tea v2, Lip Gloss v2 and Chroma. It
mirrors the GitButler desktop layout: an **Unstaged** file tree on the left, one lane per stack with branch cards, and a
details pane with diffs. buti does not touch git itself: it reads state with `but status --json` / `but diff --json`
and makes every change by running a `but` command.

## See your work: the test harness

**Don't call a UI change done until you have looked at it.** buti is a TUI, so passing unit tests don't show whether a
screen reads well, fits, or looks right. The harness renders the real UI against a real GitButler repository and saves
screenshots you can open with your image-reading tool.

```sh
mise run test               # unit tests, fake `but`, fast: run after every change
mise run test:integration   # plus the real `but` CLI (the client and the UI end to end)
mise run screenshots        # end-to-end tests; every screen saved to screenshots/<Test>-<name>.png
```

Then read the PNGs in `screenshots/` for the screens you changed. The `.ansi` files next to them hold the raw
rendering, which is handy for grepping.

The pieces, all described in [docs/testing.md](docs/testing.md):

- **Unit tests** (`internal/ui/ui_test.go`, harness in `helpers_test.go`): `newHarness(t)` runs the model against a fake
  `but` with a fixed status. Drive it with `h.keys(...)`, `h.selectText(...)`, `h.click(...)`, `h.dragTo(from, to)`,
  `h.typeText(...)`, and check the `but` commands it ran with `h.but.expect("commit --message ... --branch ...")`. Use
  these for which command an interaction runs.
- **The test repository** (`internal/testrepo`): stacks `auth` on `api` (api pushed), `fix-typo` (pushed plus local
  commits), `empty`, an unapplied `old-experiment`, four uncommitted files, and an upstream one commit ahead.
  `mise run fixture` creates one and prints how to run buti on it.
- **End-to-end tests** (`internal/ui/e2e_test.go`): `newRepoHarness(t)` runs the real UI on a fresh test repository.
  Check the screen with `h.wantOnScreen(...)`, the resulting state with `h.commits("branch")` / `h.status()`, and save
  screens with `h.snap("name")`. Use these for anything visual and for flows where the real `but` matters.

When you change the UI:

1. Add or update a unit test for the behavior.
2. Add an `h.snap` at the screen you changed (in an existing e2e test, or a new `TestE2E...`), run
   `mise run screenshots`, and look at the result. Check alignment, truncation, colors and that nothing overflows.
3. If the README screenshot is affected (`Workspace-hero`), refresh it with `mise run readme-screenshot`.

If the fixture lacks a state you need (a conflict, a long branch, many stacks), extend `internal/testrepo` rather than
building one inline, and update the table in `docs/testing.md`.

### Harness gotchas

- `but` gives commits random change ids, so ids differ between runs. Find things by name or text, never by id.
- `but` runs with an isolated `HOME` (`testrepo.Env`), so tests never touch the real GitButler config. Keep it that
  way in new tests.
- The harness drops commands that don't return within `h.wait` (the refresh tick, toast timers) and drops spinner and
  cursor-blink frames. If a new command seems to never deliver its message, that's the likely reason.
- `but` 0.22 can't assign changes to a stack from the CLI, so the fixture has none.
- End-to-end tests need `but` on `PATH` and `BUTI_INTEGRATION=1`; without them they skip, and they don't fail.

## Conventions

- Every mutation goes through `internal/but` and `runOp`. Never shell out to git for writes.
- New keys: add an `action` in `actions.go` and a row in `docs/keys.md`. Behavior users should know about goes in
  `docs/usage.md`. The README stays short: an introduction, installation and links.
- Match the surrounding code: short doc comments that say why, no decorative comments, `gofmt`.
- Run `go vet ./...` and `mise run test:integration` before you finish. CI (`.github/workflows/test.yml`) runs the
  `unit` and `integration` jobs on every PR, and both must pass to merge.

## Version control

The repository itself is managed with GitButler. Use `but` for commits, branches and PRs (`but commit -b <branch>`,
`but pr new <branch>`), not git write commands. Other branches may be applied in the workspace at the same time, so
commit only the files that belong to your change.
