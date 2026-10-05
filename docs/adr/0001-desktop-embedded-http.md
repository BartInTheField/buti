# ADR 0001: Desktop shell talks to the Go core over embedded localhost HTTP

Status: accepted (spike for [#53](https://github.com/BartInTheField/buti/issues/53), part of [#52](https://github.com/BartInTheField/buti/issues/52))

## Context

Buti desktop is a Tauri + React + shadcn/ui window on the Go core already in this repo (`internal/but` and the rest). Git writes stay `but` commands. The CLI is the one installed on the machine; the app does not ship a `but` binary ([#54](https://github.com/BartInTheField/buti/issues/54)).

The TUI and the window need one client. The window cannot import Go. Three glue options were in play:

| Glue | What it would mean |
| --- | --- |
| Sidecar | Tauri owns the process and spawns a Go binary beside it. |
| FFI | The webview calls into a cgo/shared library. |
| Embedded HTTP | `buti desktop` serves a loopback API in-process; the webview calls it. |

GitButler's desktop app uses the same split: the webview talks to a local HTTP API, and that API is the only thing that touches the workspace. Their server happens to live inside the Tauri process because the core is Rust. Ours lives in the `buti` process because the core is already Go.

## Decision

`buti desktop` is the entrypoint. It listens on `127.0.0.1` only, serves a small JSON API, and starts the Tauri window as a child. The React app calls that API. It does not shell out, and it does not link the Go module.

Rejected for v1:

- **Sidecar.** Ownership flips. Tauri would start Go, so the binary people already install (`buti`) would not be the parent, and a desktop release would be a second artifact with its own lifecycle. Shutdown, logs, and the working directory get harder to reason about before there is a workspace UI. A sidecar also invites bundling `but` next to it, which this epic does not do.
- **FFI.** A stable C ABI, cgo cross-compilation, and a second copy of the call surface. The TUI would keep calling `internal/but` directly while the window called through a different binding. Not worth it while the API is a handful of routes.

HTTP is the boundary [#55](https://github.com/BartInTheField/buti/issues/55) can grow: more routes, same window, same `internal/but` client.

## Lifecycle

1. `buti [-C dir] desktop` resolves the window mode before it listens (`--serve`, `--dev`, or a built `buti-desktop` binary). A missing shell exits without leaving a port open.
2. The process binds `127.0.0.1` (port `0` unless `--port` is set) and serves in-process.
3. It passes `BUTI_API_URL` and `BUTI_API_TOKEN` in the child environment. The Tauri command `api_config` reads them. They are not put in the window URL, the frontend bundle, or argv.
4. `--dev` runs `npm run tauri dev` in `desktop/` with that environment. Otherwise the built shell is executed (`--app`, `BUTI_DESKTOP_APP`, or `desktop/src-tauri/target/{release,debug}/buti-desktop`).
5. `--serve` does not open a window. It prints the URL and token on stderr and waits. That is the hook for tests and for a browser pointed at the Vite server.
6. When the child exits, or the process gets SIGINT or SIGTERM, the HTTP server shuts down with a short timeout. The API does not outlive `buti desktop`.

`but` is resolved when a route needs it, not before the window opens. A missing CLI is a `503` with code `but_missing` so the shell can show it. Version checks and the fuller first-run screen stay in #54.

## Routes

Implemented for the spike:

| Method | Path | Auth | Body |
| --- | --- | --- | --- |
| `GET` | `/health` | loopback only | `{"ok": true}` |
| `GET` | `/status` | loopback + bearer | `{"ok": true, "summary": ...}` or `{"ok": false, "error": {...}}` |

`/status` runs `but status --json -f` through `internal/but` and returns a summary (uncommitted paths, stacks and branches, upstream behind, edit-mode resolution). The raw GitButler document stays on the Go side.

Errors:

| HTTP | `error.code` | When |
| --- | --- | --- |
| 401 | `unauthorized` | missing or wrong bearer token |
| 403 | `forbidden` | non-loopback peer, or a `Host` that is not this listener |
| 503 | `but_missing` | `but` is not executable; `docsUrl` points at the GitButler CLI install docs |
| 502 | `but_failed` | `but` ran and failed; `message` is its stderr |

Not in this spike, sketched so later issues have a place to land:

| Method | Path | Later |
| --- | --- | --- |
| `GET` | `/diff?id=` | file, commit, or branch diff (#55) |
| `POST` | `/ops/...` | commit, amend, move, and the other `internal/but` mutations (#55) |
| `GET` | `/meta` | resolved `but` path and version (#54) |

## Loopback security

The server is a local API, not a public one. Two controls are required; either one alone is weak.

- **Bind `127.0.0.1` only.** Not `0.0.0.0`, not an unspecified address. Peers off the machine cannot connect.
- **Reject a `RemoteAddr` that is not loopback.** Defense in case the listener is ever wrapped or the check is the only thing a future bind change trips over.
- **Require `Host` to be the bound `host:port` (or `localhost` on that port).** A DNS-rebinding page can resolve to `127.0.0.1` after the browser connected; its `Host` is the attacker's name, not ours. That request is `403`.
- **Bearer token, one per process, 32 random bytes.** Compared in constant time. Loopback is shared by every local user and by every browser profile, so an open port is not an authenticator. The token is generated in the Go process and given to the webview through the environment. It is not a query parameter, so it does not land in access logs, history, or `Referer`.
- **CORS allowlist.** `http://127.0.0.1:<port>`, `http://localhost:<port>`, `http://tauri.localhost`, `https://tauri.localhost`, and `tauri://localhost`. Other origins are not reflected. The token is what stops a hostile page from making an authenticated call; CORS stops it from reading a response it did not authenticate.
- **`/health` is unauthenticated** and returns no repo data and no token. Everything that reads the workspace requires the bearer.

The token is printed on stderr for `--serve` because the person at that terminal is the one who started the process and has to configure a browser session. The window path does not print it; the child inherits it.

CSP in the Tauri config allows `connect-src` to `http://127.0.0.1:*` and `http://localhost:*` plus Tauri's `ipc:` endpoints. The webview still only learns the real URL from `api_config`.

## Consequences

- Desktop work adds routes and React screens. It does not add a second GitButler client.
- `internal/but` stays the CLI wrapper. This package does not shell out to git.
- The spike does not package, sign, or auto-update the window ([#57](https://github.com/BartInTheField/buti/issues/57)), and it does not bundle `but`.
- Build and run steps for macOS and Linux are in [docs/desktop.md](../desktop.md).
