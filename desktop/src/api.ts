// The webview talks only to the loopback API `buti desktop` serves.
// Inside Tauri the URL and token come from the process environment via `api_config`.
// A browser session (Vite against `buti desktop --serve`) uses the VITE_ pair instead.

export type ApiConfig = {
  url: string
  token: string
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

export type StatusError = {
  code: string
  message: string
  docsUrl?: string
}

export type StatusResult =
  | { ok: true; summary: Summary }
  | { ok: false; error: StatusError }

// StatusRequestError is a failed /status response. TanStack Query treats it as a query error.
export class StatusRequestError extends Error {
  readonly code: string
  readonly docsUrl?: string

  constructor(error: StatusError) {
    super(error.message)
    this.name = "StatusRequestError"
    this.code = error.code
    this.docsUrl = error.docsUrl
  }
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
  const res = await fetch(`${cfg.url.replace(/\/$/, "")}/status`, {
    headers: { Authorization: `Bearer ${cfg.token}` },
  })
  let body: StatusResult
  try {
    body = (await res.json()) as StatusResult
  } catch {
    throw new StatusRequestError({
      code: "bad_response",
      message: `Unexpected response (${res.status}).`,
    })
  }
  if (!body || typeof body !== "object" || !("ok" in body)) {
    throw new StatusRequestError({
      code: "bad_response",
      message: `Unexpected response (${res.status}).`,
    })
  }
  if (!body.ok) {
    throw new StatusRequestError(body.error)
  }
  return body.summary
}
