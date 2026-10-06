import type { Diff, FileDiff } from "@/api"
import type { Anchor } from "./api"

// A port of the line matching in internal/review/locate.go, for showing a commit's
// comments on its branch's whole diff (internal/ui notesShown).

/**
 * fileOf is the diff of path with every hunk (but.Diff.File). An uncommitted diff lists one
 * entry per hunk, so the first entry for a path holds only its first hunk.
 */
export function fileOf(diff: Diff, path: string): FileDiff | undefined {
  const fs = diff.changes.filter((f) => f.path === path)
  if (fs.length <= 1) return fs[0]
  return { ...fs[0], diff: { ...fs[0].diff, hunks: fs.flatMap((f) => f.diff.hunks ?? []) } }
}

type SideLine = { text: string; changed: boolean }

/** sideLines returns the lines a file diff shows on side, by line number. */
export function sideLines(fd: FileDiff, side: "new" | "old"): Map<number, SideLine> {
  const lines = new Map<number, SideLine>()
  for (const h of fd.diff.hunks ?? []) {
    let body = h.diff.replace(/\n$/, "").split("\n")
    let oldN = h.oldStart
    let newN = h.newStart
    if (body[0]?.startsWith("@@")) {
      const m = /^@@ -(\d+)(?:,\d+)? \+(\d+)/.exec(body[0])
      if (m) {
        oldN = Number(m[1])
        newN = Number(m[2])
      }
      body = body.slice(1)
    }
    for (let ln of body) {
      if (ln === "") ln = " "
      const text = ln.slice(1)
      switch (ln[0]) {
        case " ":
          lines.set(side === "old" ? oldN : newN, { text, changed: false })
          oldN++
          newN++
          break
        case "+":
          if (side !== "old") lines.set(newN, { text, changed: true })
          newN++
          break
        case "-":
          if (side === "old") lines.set(oldN, { text, changed: true })
          oldN++
          break
      }
    }
  }
  return lines
}

const trimEOL = (s: string) => s.replace(/[ \t\r]+$/, "")

function anchorLines(a: Anchor): string[] {
  let ls = a.line_text.split("\n")
  const n = Math.max(a.end_line - a.line + 1, 1)
  if (ls.length > n && ls[ls.length - 1] === "") ls = ls.slice(0, -1)
  return ls.map(trimEOL)
}

/** findLines is the first line of the anchor's text: at its line, else the nearest, 0 when absent. */
function findLines(lines: Map<number, SideLine>, a: Anchor): number {
  const want = anchorLines(a)
  const at = (n: number) => want.every((w, i) => {
    const l = lines.get(n + i)
    return l !== undefined && trimEOL(l.text) === w
  })
  if (at(a.line)) return a.line
  let best = 0
  for (const n of lines.keys()) {
    if (!at(n)) continue
    const d = Math.abs(n - a.line)
    const bd = Math.abs(best - a.line)
    if (best === 0 || d < bd || (d === bd && n < best)) best = n
  }
  return best
}

/** moveToLines finds an anchor's text in a file diff (review.FindLines); null when it is not there. */
export function moveToLines(fd: FileDiff | undefined, a: Anchor): Anchor | null {
  if (!fd) return null
  const line = findLines(sideLines(fd, a.side), a)
  if (line === 0) return null
  return { ...a, end_line: line + (a.end_line - a.line), line }
}
