# Desktop

Tauri + React + shadcn/ui window on the Go core. `buti desktop` serves a localhost HTTP API and the webview calls it. The design is [ADR 0001](adr/0001-desktop-embedded-http.md) ([#52](https://github.com/BartInTheField/buti/issues/52)).

The workspace UI (#55) mirrors the TUI: an Unstaged tree, stack lanes with branch cards, and a details/diff pane. Drag a file onto a branch to commit, onto a commit to amend, or drag a commit onto Unstaged / another branch to uncommit or move. Every mutation goes through `internal/but`. The app uses the `but` already on `PATH`; it does not download or embed the GitButler CLI.

## Layout

- `cmd/buti` (`desktop` subcommand): flags, signals, then `internal/desktop`
- `internal/desktop`: loopback API and the code that starts the window
- `desktop/`: Vite, React, shadcn/ui, and the Tauri shell (`desktop/src-tauri`)

## Prerequisites

Both platforms:

- Go, the same toolchain as the rest of buti (`mise install`)
- Node.js 20 or newer, and npm
- Rust (stable) via [rustup](https://rustup.rs)
- GitButler's `but` on `PATH` if you want a real status. The window still opens without it and shows the install error.

### Linux

Tauri 2 needs WebKitGTK 4.1 and a few build libraries. On Debian or Ubuntu:

```sh
sudo apt update
sudo apt install libwebkit2gtk-4.1-dev build-essential curl wget file \
  libxdo-dev libssl-dev libayatana-appindicator3-dev librsvg2-dev patchelf
```

Fedora names differ (`webkit2gtk4.1-devel`, `gtk3-devel`, `libappindicator-gtk3-devel`, `librsvg2-devel`, `openssl-devel`). Use the [Tauri prerequisites](https://v2.tauri.app/start/prerequisites/) for the distro you are on.

A desktop session is required to see the window. The API itself does not need one: `buti desktop --serve` is enough to curl `/status`.

### macOS

Xcode Command Line Tools provide the macOS SDK and the linker:

```sh
xcode-select --install
```

Install Node and Rust the same way as on Linux (`node`, `rustup`). No extra GTK or WebKit package: Tauri uses the system WebKit.

## Install the frontend

From the repository root:

```sh
cd desktop
npm install
```

## Run

`-C` is the repository `but status` should read, same as the TUI. It goes before `desktop`; the other flags come after it.

Dev window (compiles the Rust shell on first run, then opens it against the embedded API):

```sh
go run ./cmd/buti -C . desktop --dev
# or: mise run desktop
```

API only, no window. The URL and bearer token are printed on stderr:

```sh
go run ./cmd/buti -C . desktop --serve
curl -sS -H "Authorization: Bearer <token>" http://127.0.0.1:<port>/status
```

Point the Vite dev server at that API from another terminal if you want the React screen in a browser:

```sh
cd desktop
BUTI_API_URL=http://127.0.0.1:<port> BUTI_API_TOKEN=<token> \
  VITE_BUTI_API_URL="$BUTI_API_URL" VITE_BUTI_API_TOKEN="$BUTI_API_TOKEN" \
  npm run dev
```

The webview reads `BUTI_API_URL` and `BUTI_API_TOKEN` from the environment through the `api_config` command. The browser fallback reads the `VITE_` pair. Do not put the token in the URL.

Built shell, after the build below:

```sh
go run ./cmd/buti -C . desktop --app desktop/src-tauri/target/release/buti-desktop
```

With no `--dev`, `--serve`, or `--app`, `buti desktop` looks for that binary (debug, then the macOS `.app` bundle) and starts it. `BUTI_DESKTOP_APP` overrides the path.

## Build

Frontend assets only (no Rust):

```sh
cd desktop
npm run build
```

Tauri shell. `beforeBuildCommand` runs `npm run build` first.

```sh
cd desktop
npm run tauri build
```

The binary is `desktop/src-tauri/target/release/buti-desktop`. macOS also produces `desktop/src-tauri/target/release/bundle/macos/buti.app`. Linux produces a binary and, where the packaging tools are installed, a bundle under `target/release/bundle/`. Installers, signing, and updates are out of scope.

`npm run tauri build` does not look for `but` and does not copy it into the bundle.

## What the screen does

The window loads `GET /workspace` with [TanStack Query](https://tanstack.com/query) and shows the workspace: Unstaged files on the left, one lane per stack with branch cards and commits, and a diff pane for the selection. Drag-and-drop runs the same verbs as the TUI (commit / amend / move / uncommit) through `POST /ops/...`. If `but` is not on `PATH`, the screen shows that error and a link to the [GitButler CLI install docs](https://docs.gitbutler.com/cli-guides/installation). Refresh syncs pull requests and refetches the workspace, like ctrl+r in the TUI. The workspace is polled every 5 seconds, except during a drag.

![Workspace: Unstaged, stack lanes, and diff](images/desktop/workspace.png)

Hovering a drop target never changes the layout: the hint is an overlay badge and the target only gets a ring. A file over a commit inside a branch card amends; the smallest target under the pointer that accepts the drag wins.

Keys, the command palette (cmd+k or ctrl+p), the help (`?`) and the right-click menus all come from one action registry in `desktop/src/actions/`, which mirrors `internal/ui/actions.go`. Space or cmd/ctrl-click marks files, commits or branches; actions and drags then act on all marks, and esc clears them.

### Branches and history

The TUI's Branch and History verbs (`desktop/src/actions/branches.ts`), with the same keys: `b` new branch (stacked on the selected branch, else a new lane), `B` new branch below, `enter` rename, `P` push (asks before a force push when the remote diverged), `N` create a pull request (title, description, draft), `o` open the branch's pull request, `a` apply a branch, `S` unapply its stack, `L` pull, `u` / `U` undo / redo (the toast names the operation), `H` operation history with restore, `t` go to a branch, ctrl+r reload and sync pull requests. Delete branch, land onto the target and clean up empty branches are in the palette and the menus.

Each branch card has a ⋯ menu with the same actions as right-click, a push-state badge (local, unpushed, diverged, pushed, integrated) and the PR number, both with tooltips. The header has undo, redo and history buttons, and `upstream +N` pulls when clicked. Clicking the dashed lane makes a new branch in a new lane. Destructive verbs confirm first and say how to undo. Inside the Tauri window links open through the opener plugin; in a browser, in a new tab.

### Commit verbs and the target picker

The commit verbs work as in the TUI ([usage](usage.md)). `c` commits the selection or the marks (on a branch or commit: everything uncommitted), `r` squashes, amends or uncommits, `m` moves commits or a branch, `p` cherry-picks. Each enters target mode: valid targets in the lanes get a ring, everything else dims, and a bar at the bottom says what the hovered target would do. Click a target to finish, press enter for the selected one, or open a searchable list with `/` (or the bar's **Targets** button). esc cancels. The bar also holds the options: above/below a commit (`a`), a new branch stacked above the target branch (`b`, commit and pick), an empty message (`e`, commit) and keeping the target's message (`u`, squash). A squash that would mix several messages opens the composer with all of them.

Drag-and-drop runs the same plans (the TUI's `dropVerb`): a file onto a branch commits, onto a commit amends; a commit onto a commit squashes, onto Unstaged uncommits, onto a branch moves; a branch onto a branch stacks, onto "new branch" unstacks; a committed file (listed with `f`) onto Unstaged uncommits it, onto another commit or a branch moves it there. `r` on a committed file does the same through the picker.

`enter` rewords a commit (the composer starts with the full message) or renames a branch, `n` inserts an empty commit, `A` absorbs after a confirm and shows where each change went, and `x` discards after a confirm; its toast has an Undo button. All of them are in the right-click menu and the palette too.

The plans live in `desktop/src/target.ts`, a port of `internal/ui/target.go`; `npm test` runs its unit tests.

### Details pane and review comments

The diff pane works like the TUI's details pane. Click a line for the line cursor (or tab into the pane and press `j`): `j`/`k` move it, `J`/`K` scroll, `[`/`]` step hunks, and dragging over lines, shift-click or `v` selects a range. Space marks the hunk under the cursor. Commit, amend and discard then act on the marked hunks, or on the cursor's hunk while the pane has focus. `d` hides the pane and `D` gives it the window, with a file tree you can fold (`T` toggles it). Both also work by dragging the divider. `f` lists a commit's files under it, `F` lists the files of every commit, `y`/`Y` copy, and `/` goes to any file, commit or branch. Syntax highlighting is Chroma on the Go side (`POST /highlight`), the TUI's highlighter. The diff shows plain text until the tokens arrive, and rows are virtualized, so long diffs scroll smoothly.

Review comments use the same store as the TUI and `buti review` (`internal/review`), located on the current workspace the same way. `C` comments on the cursor line or range (cmd/ctrl+S saves). With the cursor on a comment, `e` edits it, `d` deletes it after a confirm, and `x` resolves or reopens it. `z` hides resolved comments, and the palette's “Review comments…” jumps to an open one. The buttons on each thread and the diff's right-click menu do the same with the mouse.

### Conflicts and edit mode

As in the TUI ([Resolving conflicts](conflicts.md)): a conflicted commit has a red **✗ Conflicted** badge in its lane. Select it and press `e` (or right-click → *Resolve in edit mode*) to check it out in edit mode (`but resolve <commit>`). The lanes are then replaced by the commit, its files marked **Conflicted** or **Resolved**, and the selected file as it is on disk, with each side of a conflict coloured. The view refreshes as you remove markers.

| Key | Does |
| --- | --- |
| `o` | open every conflicted file in your text editor |
| `enter` / double-click | open the selected file |
| `j` / `k` | select a file |
| `e` | save and exit (`but resolve finish`); asks first if a file still has markers |
| `x` | cancel (`but resolve cancel --force`) after confirming; your edits are dropped |

Outside edit mode, `o` opens the selected file (uncommitted, in a commit, or the diff's hunk) from the working tree in your text editor and `O` opens it with its default app. A GUI has no terminal for `$EDITOR`, so "editor" is the OS's: `open -t` on macOS, `xdg-open` on Linux (the same as `O` there).

### API

All routes except `/health` need the bearer token. Mutations take a JSON body, run one `but` command at a time, and reply `{"ok":true}` (plus `"output"` when `but` printed something, minus the notice it prints for coding agents) or `{"ok":false,"error":{...}}`.

| Route | Does |
| --- | --- |
| `GET /workspace` (`?sync=1`) | `but status`; with sync, pull requests are synced from the forge first |
| `GET /diff?id=` | the diff of a file, commit or branch, or all uncommitted changes |
| `GET /oplog`, `GET /branches`, `GET /review-url?branch=` | operation history, applied and unapplied branches, a branch's PR URL |
| `POST /ops/commit`, `empty-commit`, `amend`, `absorb`, `squash`, `reword`, `discard` | commits (absorb replies with where each change went) |
| `POST /ops/move`, `uncommit`, `pick` | move, uncommit, cherry-pick |
| `POST /ops/branch-new`, `branch-delete`, `apply`, `unapply`, `push`, `pull`, `pr-new`, `land`, `clean` | branches |
| `POST /ops/undo`, `redo`, `oplog-restore` | history |
| `POST /ops/resolve-start`, `resolve-finish`, `resolve-cancel` | edit mode for a conflicted commit |
| `POST /exec` | `{"line":"branch list"}` runs `but branch list`, like the TUI's `:` prompt |
| `POST /open`, `GET /file?path=` | open repository files with the OS (`{"paths":[...],"editor":true}`), read one (edit mode shows conflicted files this way); paths must stay inside the repository |
| `GET /comments` | review comments, located on the current workspace |
| `POST /comments`, `/comments/edit`, `reply`, `resolve`, `reopen`, `delete` | write review comments; an anchor with only a branch goes on the branch's commit that has the lines |
| `POST /highlight` | Chroma token classes for source texts, by file name |

`desktop/src/api.ts` has a typed function for each (`ops.<name>`), and `useWorkspaceOps().run(name, args)` calls one and refetches what it changed.

`GET /status` still returns the spike summary for curl and older callers.

## Tests

The Go API is covered by `go test ./internal/desktop` and does not need Node, Rust, or `but`. `mise run test` includes it. The Tauri window is not in CI.

`npm run e2e` (in `desktop/`) runs Playwright against the real screen: it builds the test repository, starts `buti desktop --serve` on it and Vite, then checks that hovering branch cards and commits during a drag never moves them, plus the palette, help, context menu and marks. `e2e/branches.spec.ts` runs the branch and history flows and undoes what it changes; `e2e/commits.spec.ts` runs every commit verb and restores the repository from an oplog snapshot afterwards. `e2e/details.spec.ts` covers the line cursor, hunk marks feeding the target picker, committed files, full screen and review comments, and deletes the comments it wrote. `e2e/conflicts.spec.ts` builds its own conflicted repository (`mkrepo -conflict`), serves it on the next ports, and records opened files through `BUTI_DESKTOP_OPEN` (a command that replaces `open`/`xdg-open`) rather than opening them.

`BUTI_E2E_PORT` picks the Vite port (default 47310) and `BUTI_E2E_API_PORT` the API's (default: a free port), so checkouts can run side by side. The e2e needs `but` and Go on `PATH` (it skips without `but`) and a Chromium from `npx playwright install chromium`. Screenshots go to `$BUTI_SHOTS`, by default `/tmp/buti-desktop-shots`.
