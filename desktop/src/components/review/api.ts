import { useEffect } from "react"
import { keepPreviousData, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query"
import { ApiRequestError, type ApiConfig, type ApiError, type Workspace } from "@/api"

// Review comments, served by internal/desktop/comments.go from internal/review's store: the
// same comments the TUI and `buti review` show.

export type CommentKind = "unassigned" | "assigned" | "commit"
export type CommentStatus = "open" | "resolved" | "dismissed"
/** A stored status, or outdated / orphaned for an open comment whose line or commit is gone. */
export type CommentState = CommentStatus | "outdated" | "orphaned"

export type Anchor = {
  kind?: CommentKind
  change_id?: string
  commit_id?: string
  branch?: string
  path: string
  side: "new" | "old"
  line: number
  end_line: number
  line_text: string
}

export type Reply = { author: string; body: string; created_at: string }

export type Comment = {
  id: string
  status: CommentStatus
  author: string
  body: string
  created_at: string
  anchor: Anchor
  resolution: { summary: string; at: string | null }
  replies: Reply[]
}

export type LocatedComment = Comment & {
  state: CommentState
  /** Where the comment is now. */
  at: Anchor
  target: string
  fileId: string
}

/** isOpen: the comment still wants an answer, even when its line or commit is gone. */
export function isOpen(c: LocatedComment): boolean {
  return c.status === "open"
}

/** where describes an anchor's lines: "a.go line 3", "a.go lines 3–5 (old)" (internal/ui where). */
export function where(a: Anchor): string {
  let s = a.end_line > a.line ? `${a.path} lines ${a.line}–${a.end_line}` : `${a.path} line ${a.line}`
  if (a.side === "old") s += " (old)"
  return s
}

/** authorLabel shows the user as "you" and anyone else (an agent) by name. */
export function authorLabel(author: string): string {
  return !author || author === "user" ? "you" : author
}

function base(cfg: ApiConfig) {
  return cfg.url.replace(/\/$/, "")
}

async function call<T>(cfg: ApiConfig, path: string, body?: unknown): Promise<T> {
  const res = await fetch(`${base(cfg)}${path}`, {
    method: body === undefined ? "GET" : "POST",
    headers: {
      Authorization: `Bearer ${cfg.token}`,
      ...(body === undefined ? {} : { "Content-Type": "application/json" }),
    },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  let data: { ok: boolean; error?: ApiError } & T
  try {
    data = await res.json()
  } catch {
    throw new ApiRequestError({ code: "bad_response", message: `Unexpected response (${res.status}).` })
  }
  if (!data?.ok) {
    throw new ApiRequestError(data?.error ?? { code: "bad_response", message: `Unexpected response (${res.status}).` })
  }
  return data
}

export const commentsApi = {
  list: async (cfg: ApiConfig) => (await call<{ comments: LocatedComment[] }>(cfg, "/comments")).comments,
  /** An anchor with a branch and no kind goes on the branch's commit that has the lines. */
  add: async (cfg: ApiConfig, anchor: Anchor, body: string) =>
    (await call<{ comment: Comment }>(cfg, "/comments", { anchor, body })).comment,
  edit: (cfg: ApiConfig, id: string, body: string) => call(cfg, "/comments/edit", { id, body }),
  reply: (cfg: ApiConfig, id: string, body: string) => call(cfg, "/comments/reply", { id, body }),
  resolve: (cfg: ApiConfig, id: string) => call(cfg, "/comments/resolve", { id }),
  reopen: (cfg: ApiConfig, id: string) => call(cfg, "/comments/reopen", { id }),
  remove: (cfg: ApiConfig, id: string) => call(cfg, "/comments/delete", { id }),
}

export const commentsKey = (url: string) => ["comments", url] as const

const commentsRefetchMs = 5_000

/**
 * useComments polls the located comments, picking up an agent's comments as the TUI's
 * refresh tick does, and locates again whenever the workspace changed.
 */
export function useComments(cfg: ApiConfig, workspace: Workspace | undefined) {
  const qc = useQueryClient()
  const q = useQuery({
    queryKey: commentsKey(cfg.url),
    queryFn: () => commentsApi.list(cfg),
    enabled: Boolean(cfg.url),
    refetchInterval: commentsRefetchMs,
    placeholderData: keepPreviousData,
    retry: false,
  })
  useEffect(() => {
    if (workspace) void qc.invalidateQueries({ queryKey: commentsKey(cfg.url) })
  }, [workspace, qc, cfg.url])
  return q
}

export function invalidateComments(qc: QueryClient, url: string) {
  return qc.invalidateQueries({ queryKey: commentsKey(url) })
}

/** loadComments refetches the located comments, for a picker that must not miss an agent's latest. */
export async function loadComments(qc: QueryClient, cfg: ApiConfig): Promise<LocatedComment[]> {
  await qc.invalidateQueries({ queryKey: commentsKey(cfg.url) })
  return qc.fetchQuery({ queryKey: commentsKey(cfg.url), queryFn: () => commentsApi.list(cfg) })
}
