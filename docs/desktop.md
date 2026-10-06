# Desktop spike

Tauri + React + shadcn/ui window on the Go core. `buti desktop` serves a localhost HTTP API and the webview calls it. The design is [ADR 0001](adr/0001-desktop-embedded-http.md) ([#53](https://github.com/BartInTheField/buti/issues/53), part of [#52](https://github.com/BartInTheField/buti/issues/52)).

This is the shell and one status screen, not the workspace UI. It uses the `but` already on `PATH`. Nothing here downloads or embeds the GitButler CLI.

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

`-C` is the repository `but status` should read, same as the TUI. The flags below come after `desktop`.

Dev window (compiles the Rust shell on first run, then opens it against the embedded API):

```sh
go run ./cmd/buti desktop --dev -C .
# or: mise run desktop
```

API only, no window. The URL and bearer token are printed on stderr:

```sh
go run ./cmd/buti desktop --serve -C .
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
go run ./cmd/buti desktop --app desktop/src-tauri/target/release/buti-desktop -C .
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

The binary is `desktop/src-tauri/target/release/buti-desktop`. macOS also produces `desktop/src-tauri/target/release/bundle/macos/Buti.app`. Linux produces a binary and, where the packaging tools are installed, a bundle under `target/release/bundle/`. Installers, signing, and updates are out of scope.

`npm run tauri build` does not look for `but` and does not copy it into the bundle.

## What the screen does

The window loads `GET /status` with [TanStack Query](https://tanstack.com/query) and shows a summary: repository path, uncommitted files, stacks and branches, how far upstream is ahead. If `but` is not on `PATH`, the same screen shows that error and a link to the [GitButler CLI install docs](https://docs.gitbutler.com/cli-guides/installation). Refresh refetches the query.

## Tests

The Go API is covered by `go test ./internal/desktop` and does not need Node, Rust, or `but`. `mise run test` includes it. The Tauri window is not in CI.
