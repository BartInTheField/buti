import type { Diff, FileDiff } from "@/api"
import { isOpen, type LocatedComment } from "@/components/review/api"
import type { Selection } from "@/selection"

// The diff laid out as rows, as internal/ui/diff.go layoutDiff does: file headers, hunk
// headers, lines, and the review comments under the lines they are on. The pane renders
// the rows virtualized; the cursor and range are kept by row key across refetches.

export type LineRow = {
  type: "line"
  key: string
  hunk: number
  sign: "+" | "-" | " " | "\\"
  old: number
  new: number
  /** The line as in the file, without the sign; tabs are not expanded. */
  code: string
  /** Index into the hunk's old or new side source, for the highlighted tokens. */
  src: { side: 0 | 1; i: number } | null
}

export type Row =
  | { type: "file"; key: string; path: string; status: string }
  | { type: "hunk"; key: string; hunk: number; header: string }
  | LineRow
  | { type: "note"; key: string; hunk: number; comment: LocatedComment; outdated: boolean; after: string }
  | { type: "info"; key: string; text: string }
  | { type: "gap"; key: string }

export type HunkRef = {
  index: number
  path: string
  /** The hunk's cli id for uncommitted hunks, empty otherwise (internal/ui hunkRef.id). */
  id: string
  header: string
  /** The raw unified hunk, for copying. */
  text: string
  /** Row of the @@ header, and one past the hunk's last row. */
  first: number
  end: number
  /** The old and new side source (tabs expanded), highlighted separately like parseHunk. */
  sources: [string, string]
}

export type DiffLayout = {
  rows: Row[]
  hunks: HunkRef[]
  files: { path: string; row: number; hunk: number }[]
  /** Width of the longest line in characters, tabs expanded. */
  maxLen: number
  /** Width of a line number. */
  numW: number
}

export const tabWidth = 4
export const expandTabs = (s: string) => s.replaceAll("\t", " ".repeat(tabWidth))

/**
 * layoutDiff lays a diff out. ids: whether hunk ids address hunks (only for uncommitted
 * diffs, and only for single-hunk entries, which is how `but diff` lists uncommitted changes).
 */
export function layoutDiff(diff: Diff, ids: boolean): DiffLayout {
  const out: DiffLayout = { rows: [], hunks: [], files: [], maxLen: 0, numW: 1 }
  if (diff.changes.length === 0) {
    out.rows.push({ type: "info", key: "info:none", text: "No changes" })
    return out
  }
  for (const f of diff.changes) {
    for (const h of f.diff.hunks ?? []) {
      out.numW = Math.max(out.numW, String(h.oldStart + h.oldLines).length, String(h.newStart + h.newLines).length)
    }
  }
  let prevPath = ""
  for (const f of diff.changes) {
    // Uncommitted diffs list one entry per hunk; group them under one file header.
    if (f.path !== prevPath) {
      if (prevPath !== "") out.rows.push({ type: "gap", key: `gap:${f.path}` })
      out.files.push({ path: f.path, row: out.rows.length, hunk: -1 })
      out.rows.push({ type: "file", key: `file:${f.path}`, path: f.path, status: f.status })
    }
    prevPath = f.path
    if (f.diff.type !== "patch") {
      out.rows.push({ type: "info", key: `info:${f.path}`, text: `(${f.diff.type}, no text diff)` })
      continue
    }
    const hunks = f.diff.hunks ?? []
    hunks.forEach((h, hi) => {
      const index = out.hunks.length
      const file = out.files[out.files.length - 1]
      if (file.hunk < 0) file.hunk = index
      const raw = h.diff.replace(/\n$/, "").split("\n")
      const header = raw[0] ?? ""
      const ref: HunkRef = {
        index,
        path: f.path,
        id: ids && hunks.length === 1 ? f.id : "",
        header,
        text: h.diff,
        first: out.rows.length,
        end: 0,
        sources: ["", ""],
      }
      out.rows.push({ type: "hunk", key: `hunk:${f.path}:${f.id}:${hi}:${header}`, hunk: index, header })
      const oldSrc: string[] = []
      const newSrc: string[] = []
      let oi = 0
      let ni = 0
      for (let r of raw.slice(1)) {
        if (r === "") r = " "
        const sign = r[0] as LineRow["sign"]
        const code = r.slice(1)
        const shown = expandTabs(code)
        out.maxLen = Math.max(out.maxLen, shown.length)
        let row: LineRow
        switch (sign) {
          case "-":
            row = { type: "line", key: "", hunk: index, sign, old: h.oldStart + oi, new: 0, code, src: { side: 0, i: oldSrc.length } }
            oldSrc.push(shown)
            oi++
            break
          case "+":
            row = { type: "line", key: "", hunk: index, sign, old: 0, new: h.newStart + ni, code, src: { side: 1, i: newSrc.length } }
            newSrc.push(shown)
            ni++
            break
          case " ":
            row = { type: "line", key: "", hunk: index, sign, old: h.oldStart + oi, new: h.newStart + ni, code, src: { side: 1, i: newSrc.length } }
            oldSrc.push(shown)
            newSrc.push(shown)
            oi++
            ni++
            break
          default:
            row = { type: "line", key: "", hunk: index, sign: "\\", old: 0, new: 0, code: r, src: null }
        }
        row.key = `line:${f.path}:${row.sign}:${row.old}:${row.new}:${out.rows.length}`
        if (row.sign !== "\\") row.key = `line:${f.path}:${row.sign}:${row.old}:${row.new}`
        out.rows.push(row)
      }
      ref.sources = [oldSrc.join("\n"), newSrc.join("\n")]
      ref.end = out.rows.length
      out.hunks.push(ref)
    })
  }
  return out
}

/** isCursorRow: the line cursor stops on diff lines and on the top of a comment. */
export function isCursorRow(r: Row): boolean {
  return (r.type === "line" && r.sign !== "\\") || r.type === "note"
}

/**
 * withComments puts each comment under the row of its last line; one whose line is not
 * in the diff (outdated) goes after the last line of its file, and one on a file the diff
 * does not show is left out (internal/ui diffLayout.withComments).
 */
export function withComments(l: DiffLayout, notes: LocatedComment[]): DiffLayout {
  if (notes.length === 0) return l
  const after = new Map<number, { c: LocatedComment; outdated: boolean }[]>()
  for (const c of notes) {
    const [row, outdated] = noteRow(l, c)
    if (row < 0) continue
    const list = after.get(row) ?? []
    list.push({ c, outdated })
    after.set(row, list)
  }
  if (after.size === 0) return l
  const rows: Row[] = []
  const hunks = l.hunks.map((h) => ({ ...h, first: -1, end: -1 }))
  const files = l.files.map((f) => ({ ...f }))
  let fi = 0
  l.rows.forEach((r, i) => {
    while (fi < files.length && l.files[fi].row === i) files[fi++].row = rows.length
    rows.push(r)
    for (const { c, outdated } of after.get(i) ?? []) {
      const hunk = r.type === "line" ? r.hunk : -1
      rows.push({ type: "note", key: `note:${c.id}`, hunk, comment: c, outdated, after: r.key })
    }
  })
  rows.forEach((r, i) => {
    if ((r.type === "hunk" || r.type === "line" || r.type === "note") && r.hunk >= 0) {
      const h = hunks[r.hunk]
      if (h.first < 0) h.first = i
      h.end = i + 1
    }
  })
  return { ...l, rows, hunks, files }
}

function noteRow(l: DiffLayout, c: LocatedComment): [number, boolean] {
  const a = c.at
  const end = Math.max(a.end_line, a.line)
  let last = -1
  for (const h of l.hunks) {
    if (h.path !== a.path) continue
    for (let i = h.first; i < h.end; i++) {
      const r = l.rows[i]
      if (r.type !== "line" || r.sign === "\\") continue
      last = i
      if (c.state === "outdated") continue
      if (a.side === "old" ? r.old === end && r.sign !== "+" : r.new === end && r.sign !== "-") return [i, false]
    }
  }
  return [last, true]
}

/** The subject the pane shows, for picking the comments that belong on it. */
export type Shown =
  | { kind: "uncommitted" }
  | { kind: "commit"; changeId?: string; commitId?: string; path?: string }
  | { kind: "branch"; name: string }
  | { kind: "none" }

function matchesCommit(c: LocatedComment, s: { changeId?: string; commitId?: string }): boolean {
  const a = c.at
  return Boolean((a.change_id && a.change_id === s.changeId) || (a.commit_id && a.commit_id === s.commitId))
}

/**
 * notesShown are the comments on what the pane shows: uncommitted comments on an
 * uncommitted diff, a commit's on its diff, and on a branch's diff those of its commits
 * moved to where the branch diff has their text (internal/ui details.notesShown).
 */
export function notesShown(
  all: LocatedComment[],
  shown: Shown,
  hideResolved: boolean,
  fileDiff: (path: string) => FileDiff | undefined,
  moveToLines: (fd: FileDiff | undefined, a: LocatedComment["at"]) => LocatedComment["at"] | null,
): LocatedComment[] {
  const out: LocatedComment[] = []
  for (const c of all) {
    const a = c.at
    if (c.state === "orphaned" || (hideResolved && !isOpen(c))) continue
    switch (shown.kind) {
      case "uncommitted":
        if (a.kind !== "commit") out.push(c)
        break
      case "commit":
        if (a.kind === "commit" && matchesCommit(c, shown) && (!shown.path || a.path === shown.path)) out.push(c)
        break
      case "branch":
        if (a.kind === "commit" && a.branch === shown.name) {
          if (c.state === "outdated") {
            out.push(c)
          } else {
            const moved = moveToLines(fileDiff(a.path), a)
            if (moved) out.push({ ...c, at: moved })
          }
        }
        break
    }
  }
  return out
}

/** hunkSubject is a hunk as an action subject: a file-kind selection with the hunk's id. */
export function hunkSubject(h: HunkRef): Selection {
  return { kind: "file", id: h.id, path: h.path, hunk: h.header }
}
