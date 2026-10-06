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

async function postOp(cfg: ApiConfig, path: string, body: unknown): Promise<void> {
  const res = await fetch(`${baseUrl(cfg)}${path}`, {
    method: "POST",
    headers: {
      ...authHeaders(cfg),
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  })
  await parseBody<OkOp>(res)
}

export async function opCommit(
  cfg: ApiConfig,
  args: { changes?: string[]; message?: string; placement: Placement },
): Promise<void> {
  await postOp(cfg, "/ops/commit", {
    changes: args.changes ?? [],
    message: args.message ?? "",
    placement: args.placement,
  })
}

export async function opAmend(
  cfg: ApiConfig,
  args: { target: string; changes?: string[] },
): Promise<void> {
  await postOp(cfg, "/ops/amend", {
    target: args.target,
    changes: args.changes ?? [],
  })
}

export async function opMove(
  cfg: ApiConfig,
  args: { sources: string[]; placement: Placement },
): Promise<void> {
  await postOp(cfg, "/ops/move", args)
}

export async function opUncommit(
  cfg: ApiConfig,
  args: { sources: string[] },
): Promise<void> {
  await postOp(cfg, "/ops/uncommit", args)
}

export function commitSubject(message: string): string {
  const line = message.split("\n")[0]?.trim()
  return line || "(empty)"
}

export function branchPR(reviewId?: string): string {
  return (reviewId ?? "").replace(/[()]/g, "")
}
