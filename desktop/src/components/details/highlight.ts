import { useQuery } from "@tanstack/react-query"
import type { ApiConfig } from "@/api"
import type { HunkRef } from "./rows"

/** A token is [class, text]; the class is one of internal/desktop/highlight.go tokenClass. */
export type Token = [string, string]
/** Per hunk, the old and new side: per line, its tokens. */
export type Highlighted = [Token[][], Token[][]][]

// Highlighting is Chroma on the Go side, the TUI's highlighter: same lexers, no JS
// highlighter to ship, and the tokenising stays off the UI thread. Bigger diffs than
// this stay plain, as reading them highlighted would not be worth the wait.
const maxHighlightChars = 2_000_000

function hash(s: string): string {
  let h = 5381
  for (let i = 0; i < s.length; i++) h = ((h << 5) + h + s.charCodeAt(i)) | 0
  return (h >>> 0).toString(36)
}

/**
 * useHighlight tokenises every hunk's sources. The diff shows plain until the tokens
 * arrive; colours never change a line's size, so nothing moves when they do.
 */
export function useHighlight(cfg: ApiConfig, hunks: HunkRef[]) {
  let size = 0
  for (const h of hunks) size += h.sources[0].length + h.sources[1].length
  const key = hash(hunks.map((h) => `${h.path}\0${h.sources[0]}\0${h.sources[1]}`).join("\0"))
  return useQuery({
    queryKey: ["highlight", cfg.url, key],
    enabled: Boolean(cfg.url) && hunks.length > 0 && size <= maxHighlightChars,
    staleTime: Infinity,
    gcTime: 60_000,
    retry: false,
    queryFn: async (): Promise<Highlighted> => {
      const res = await fetch(`${cfg.url.replace(/\/$/, "")}/highlight`, {
        method: "POST",
        headers: { Authorization: `Bearer ${cfg.token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ files: hunks.map((h) => ({ path: h.path, texts: h.sources })) }),
      })
      const body = (await res.json()) as { ok: boolean; files?: { texts: Token[][][] }[] }
      if (!body.ok || !body.files) throw new Error("highlight failed")
      return body.files.map((f) => [f.texts[0] ?? [], f.texts[1] ?? []])
    },
  })
}
