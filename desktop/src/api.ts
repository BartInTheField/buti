// The webview talks only to the loopback API `buti desktop` serves.
// Inside Tauri the URL and token come from the process environment via `api_config`.
// A browser session (Vite against `buti desktop --serve`) uses the VITE_ pair instead.

export type ApiConfig = {
  url: string
  token: string
}

export type Change = {
  cliId: string
  filePath: string
  changeType: string
}

export type Commit = {
  cliId: string
  changeId?: string
  commitId?: string
  message: string
  authorName?: string
  authorEmail?: string
  conflicted?: boolean | null
  changes?: Change[]
}

export type Branch = {
  cliId: string
  name: string
  commits: Commit[]
  upstreamCommits?: Commit[]
  branchStatus?: string
  reviewId?: string
}

export type Stack = {
  cliId: string
  assignedChanges?: Change[]
  branches: Branch[]
}

export type Workspace = {
  repo: string
  uncommittedChanges: Change[]
  stacks: Stack[]
  mergeBase?: Commit
  upstreamState?: { behind: number }
  resolving?: {
    conflicted_files?: string[]
    resolved_files?: string[]
    all_resolved?: boolean
  }
}

export type Hunk = {
  oldStart: number
  oldLines: number
  newStart: number
  newLines: number
  diff: string
}

export type FileDiff = {
  id: string
  path: string
  status: string
  diff: {
    type: string
    hunks: Hunk[]
  }
}

export type Diff = {
  changes: FileDiff[]
}

export type Placement = {
  branch?: string
  above?: string
  below?: string
  newBranch?: boolean
}

export type BranchSummary = {
  name: string
  commits: number
  upstream: number
  status?: string
  pr?: string
}

export type Summary = {
  repo: string
  uncommitted: number
  uncommittedPaths: string[]
  stacks: { branches: BranchSummary[] }[]
  upstreamBehind: number
  resolving?: {
    conflicted: number
    resolved: number
    allResolved: boolean
  }
}

export type ApiError = {
  code: string
  message: string
  docsUrl?: string
}

export type StatusError = ApiError

type OkSummary = { ok: true; summary: Summary }
type OkWorkspace = { ok: true; workspace: Workspace }
type OkDiff = { ok: true; diff: Diff }
type OkOp = { ok: true }
type ErrBody = { ok: false; error: ApiError }

// ApiRequestError is a failed API response. TanStack Query treats it as a query error.
export class ApiRequestError extends Error {
  readonly code: string
  readonly docsUrl?: string

  constructor(error: ApiError) {
    super(error.message)
    this.name = "ApiRequestError"
    this.code = error.code
    this.docsUrl = error.docsUrl
  }
}

/** @deprecated use ApiRequestError */
export const StatusRequestError = ApiRequestError

function baseUrl(cfg: ApiConfig): string {
  return cfg.url.replace(/\/$/, "")
}

function authHeaders(cfg: ApiConfig): HeadersInit {
  return { Authorization: `Bearer ${cfg.token}` }
}

async function parseBody<T extends { ok: boolean }>(res: Response): Promise<T> {
  let body: T
  try {
    body = (await res.json()) as T
  } catch {
    throw new ApiRequestError({
      code: "bad_response",
      message: `Unexpected response (${res.status}).`,
    })
  }
  if (!body || typeof body !== "object" || !("ok" in body)) {
    throw new ApiRequestError({
      code: "bad_response",
      message: `Unexpected response (${res.status}).`,
    })
  }
  if (!body.ok) {
    throw new ApiRequestError((body as unknown as ErrBody).error)
  }
  return body
}

export async function loadConfig(): Promise<ApiConfig> {
  try {
    const { invoke } = await import("@tauri-apps/api/core")
    const cfg = await invoke<ApiConfig>("api_config")
    if (cfg?.url) {
      return cfg
    }
  } catch {
    // Not the Tauri webview.
  }
  return {
    url: import.meta.env.VITE_BUTI_API_URL ?? "",
    token: import.meta.env.VITE_BUTI_API_TOKEN ?? "",
  }
}

export async function fetchStatus(cfg: ApiConfig): Promise<Summary> {
  const res = await fetch(`${baseUrl(cfg)}/status`, {
    headers: authHeaders(cfg),
  })
  const body = await parseBody<OkSummary>(res)
  return body.summary
}

export async function fetchWorkspace(cfg: ApiConfig): Promise<Workspace> {
  const res = await fetch(`${baseUrl(cfg)}/workspace`, {
    headers: authHeaders(cfg),
  })
  const body = await parseBody<OkWorkspace>(res)
  return body.workspace
}

export async function fetchDiff(cfg: ApiConfig, id: string): Promise<Diff> {
  const q = id ? `?id=${encodeURIComponent(id)}` : ""
  const res = await fetch(`${baseUrl(cfg)}${`/diff${q}`}`, {
    headers: authHeaders(cfg),
  })
  const body = await parseBody<OkDiff>(res)
  return body.diff
}

export type OplogEntry = {
  id: string
  createdAt: number // unix millis
  details: { operation: string; title: string; body: string }
}

export type BranchListing = {
  name: string
  hasLocal: boolean
  lastCommitAt: number
  commitsAhead?: number | null
  mergesCleanly?: boolean | null
  lastAuthor?: { name: string }
}

export type Branches = {
  appliedStacks: { heads: BranchListing[] }[] | null
  branches: BranchListing[] | null // not applied
}

/** What a mutation printed, when `but` said something worth showing (undo, clean, exec). */
export type OpResult = { output?: string }

async function getJSON<T extends { ok: boolean }>(cfg: ApiConfig, path: string): Promise<T> {
  const res = await fetch(`${baseUrl(cfg)}${path}`, { headers: authHeaders(cfg) })
  return parseBody<T>(res)
}

async function postOp(cfg: ApiConfig, path: string, body: unknown = {}): Promise<OpResult> {
  const res = await fetch(`${baseUrl(cfg)}${path}`, {
    method: "POST",
    headers: {
      ...authHeaders(cfg),
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  })
  const { output } = await parseBody<OkOp & OpResult>(res)
  return { output }
}

/** fetchSyncedWorkspace syncs pull requests from the forge first, like ctrl+r in the TUI. */
export async function fetchSyncedWorkspace(cfg: ApiConfig): Promise<Workspace> {
  return (await getJSON<OkWorkspace>(cfg, "/workspace?sync=1")).workspace
}

export async function fetchOplog(cfg: ApiConfig): Promise<OplogEntry[]> {
  return (await getJSON<{ ok: true; entries: OplogEntry[] }>(cfg, "/oplog")).entries
}

export async function fetchBranches(cfg: ApiConfig): Promise<Branches> {
  return (await getJSON<{ ok: true; branches: Branches }>(cfg, "/branches")).branches
}

/** fetchReviewURL is the web URL of a branch's pull request, or "" when it has none. */
export async function fetchReviewURL(cfg: ApiConfig, branch: string): Promise<string> {
  const q = `?branch=${encodeURIComponent(branch)}`
  return (await getJSON<{ ok: true; url: string }>(cfg, `/review-url${q}`)).url
}

export type RepoFile = { path: string; content: string; binary?: boolean }

/** fetchFile reads a repository file; edit mode shows conflicted files this way, as `but diff` is empty there. */
export async function fetchFile(cfg: ApiConfig, path: string): Promise<RepoFile> {
  const q = `?path=${encodeURIComponent(path)}`
  return getJSON<{ ok: true } & RepoFile>(cfg, `/file${q}`)
}

/** openFiles opens repository files with the OS; editor prefers the default text editor. */
export async function openFiles(cfg: ApiConfig, paths: string[], editor = false): Promise<void> {
  await postOp(cfg, "/open", { paths, editor })
}

export type SquashMode = "combine" | "target" | "source"

/**
 * Every mutation the API offers, keyed by name. Each takes the request body and
 * maps 1:1 onto a `POST /ops/...` handler in internal/desktop/ops.go.
 */
export const ops = {
  commit: (cfg: ApiConfig, a: { changes?: string[]; message?: string; placement: Placement }) =>
    postOp(cfg, "/ops/commit", { changes: a.changes ?? [], message: a.message ?? "", placement: a.placement }),
  emptyCommit: (cfg: ApiConfig, a: { message?: string; placement: Placement }) =>
    postOp(cfg, "/ops/empty-commit", a),
  amend: (cfg: ApiConfig, a: { target: string; changes?: string[] }) =>
    postOp(cfg, "/ops/amend", { target: a.target, changes: a.changes ?? [] }),
  /** No sources absorbs every uncommitted change. */
  absorb: (cfg: ApiConfig, a: { sources?: string[] }) => postOp(cfg, "/ops/absorb", a),
  squash: (
    cfg: ApiConfig,
    a: { sources: string[]; target?: string; mode?: SquashMode; message?: string },
  ) => postOp(cfg, "/ops/squash", a),
  move: (cfg: ApiConfig, a: { sources: string[]; placement: Placement }) => postOp(cfg, "/ops/move", a),
  uncommit: (cfg: ApiConfig, a: { sources: string[] }) => postOp(cfg, "/ops/uncommit", a),
  /** Rewords a commit, or renames a branch when target is a branch id. */
  reword: (cfg: ApiConfig, a: { target: string; message: string }) => postOp(cfg, "/ops/reword", a),
  /** No targets discards every uncommitted change. */
  discard: (cfg: ApiConfig, a: { targets?: string[] }) => postOp(cfg, "/ops/discard", a),
  branchNew: (cfg: ApiConfig, a: { name?: string; placement?: Placement }) =>
    postOp(cfg, "/ops/branch-new", a),
  branchDelete: (cfg: ApiConfig, a: { branches: string[] }) => postOp(cfg, "/ops/branch-delete", a),
  pick: (cfg: ApiConfig, a: { sources: string[]; placement: Placement }) => postOp(cfg, "/ops/pick", a),
  apply: (cfg: ApiConfig, a: { branch: string }) => postOp(cfg, "/ops/apply", a),
  unapply: (cfg: ApiConfig, a: { branch: string }) => postOp(cfg, "/ops/unapply", a),
  push: (cfg: ApiConfig, a: { branch: string; force?: boolean }) => postOp(cfg, "/ops/push", a),
  pull: (cfg: ApiConfig) => postOp(cfg, "/ops/pull"),
  undo: (cfg: ApiConfig) => postOp(cfg, "/ops/undo"),
  redo: (cfg: ApiConfig) => postOp(cfg, "/ops/redo"),
  /** An empty message uses the default title and description. */
  prNew: (cfg: ApiConfig, a: { branch: string; message?: string; draft?: boolean }) =>
    postOp(cfg, "/ops/pr-new", a),
  oplogRestore: (cfg: ApiConfig, a: { snapshot: string }) => postOp(cfg, "/ops/oplog-restore", a),
  land: (cfg: ApiConfig, a: { branch: string }) => postOp(cfg, "/ops/land", a),
  clean: (cfg: ApiConfig) => postOp(cfg, "/ops/clean"),
  resolveStart: (cfg: ApiConfig, a: { commit: string }) => postOp(cfg, "/ops/resolve-start", a),
  resolveFinish: (cfg: ApiConfig) => postOp(cfg, "/ops/resolve-finish"),
  resolveCancel: (cfg: ApiConfig, a: { force?: boolean }) => postOp(cfg, "/ops/resolve-cancel", a),
  /** Runs `but <line>` like the TUI's ':' prompt; a leading "but " is dropped. */
  exec: (cfg: ApiConfig, a: { line: string }) => postOp(cfg, "/exec", a),
} satisfies Record<string, (cfg: ApiConfig, args: never) => Promise<OpResult>>

export type OpName = keyof typeof ops

export function commitSubject(message: string): string {
  const line = message.split("\n")[0]?.trim()
  return line || "(empty)"
}

export function branchPR(reviewId?: string): string {
  return (reviewId ?? "").replace(/[()]/g, "")
}
