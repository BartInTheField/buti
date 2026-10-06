import { useState } from "react"
import type { Workspace } from "./api"

export type Selection =
  | { kind: "unstaged" }
  /**
   * An uncommitted file, or with hunk set one hunk of it (id is then the hunk's cli id, as
   * entHunk in internal/ui): `but` takes hunk ids wherever it takes file ids.
   */
  | { kind: "file"; id: string; path: string; hunk?: string }
  /** A file inside a commit, listed under it with f / F (entCommittedFile). */
  | { kind: "cfile"; id: string; path: string; commit: string; branch: string }
  | { kind: "commit"; id: string; subject: string; branch: string }
  | { kind: "branch"; id: string; name: string }

export type SelectionKind = Selection["kind"]

/** selectionKey identifies an item across workspace refetches. */
export function selectionKey(sel: Selection): string {
  return sel.kind === "unstaged" ? "unstaged" : `${sel.kind}:${sel.id}`
}

export function sameSelection(a: Selection | null, b: Selection | null): boolean {
  return a !== null && b !== null && selectionKey(a) === selectionKey(b)
}

/**
 * markClass mirrors entityKind.markClass in internal/ui/entity.go: marks of different
 * classes can't be mixed. 0 means the kind can't be marked.
 */
export function markClass(kind: SelectionKind): number {
  switch (kind) {
    case "file":
      return 1
    case "commit":
      return 2
    case "cfile":
      return 3
    case "branch":
      return 4
    default:
      return 0
  }
}

export function selectionDiffId(sel: Selection | null): string | null {
  if (!sel) return null
  switch (sel.kind) {
    case "unstaged":
      return ""
    case "file":
    case "cfile":
    case "commit":
    case "branch":
      return sel.id
  }
}

export function selectionTitle(sel: Selection | null): string {
  if (!sel) return "Select a file, commit, or branch"
  switch (sel.kind) {
    case "unstaged":
      return "Unstaged changes"
    case "file":
    case "cfile":
      return sel.path
    case "commit":
      return sel.subject
    case "branch":
      return sel.name
  }
}

export function selectionLabel(sel: Selection): string {
  switch (sel.kind) {
    case "unstaged":
      return "all uncommitted changes"
    case "file":
      return sel.hunk ? `hunk ${sel.hunk} of ${sel.path}` : sel.path
    case "cfile":
      return `${sel.path} in its commit`
    case "commit":
      return `commit “${sel.subject}”`
    case "branch":
      return `branch ${sel.name}`
  }
}

/** describeSubjects is a short phrase for a set of subjects: "3 commits", "branch api". */
export function describeSubjects(subjects: Selection[]): string {
  if (subjects.length === 0) return "nothing"
  if (subjects.length === 1) return selectionLabel(subjects[0])
  const kind = subjects[0].kind
  return subjects.every((s) => s.kind === kind)
    ? `${subjects.length} ${kind}s`
    : `${subjects.length} items`
}

/** liveKeys is every selectable key in the workspace, to drop marks that disappeared. */
export function liveKeys(ws: Workspace): Set<string> {
  const keys = new Set<string>(["unstaged"])
  for (const c of ws.uncommittedChanges) keys.add(`file:${c.cliId}`)
  for (const st of ws.stacks) {
    for (const b of st.branches) {
      keys.add(`branch:${b.cliId}`)
      for (const c of b.commits) {
        keys.add(`commit:${c.cliId}`)
        for (const ch of c.changes ?? []) keys.add(`cfile:${ch.cliId}`)
      }
    }
  }
  return keys
}

/**
 * stillThere reports whether sel is still in the workspace. live is liveKeys(ws). The
 * workspace does not list hunks: a hunk stays while its file is uncommitted.
 */
export function stillThere(ws: Workspace, live: Set<string>, sel: Selection): boolean {
  if (live.has(selectionKey(sel))) return true
  if (sel.kind !== "file" || sel.hunk === undefined) return false
  return [...ws.uncommittedChanges, ...ws.stacks.flatMap((s) => s.assignedChanges ?? [])].some(
    (c) => c.filePath === sel.path,
  )
}

export type SelectionModel = {
  selection: Selection | null
  /** Marked items, in the order they were marked. */
  marks: Selection[]
  /**
   * subjects are what actions act on: the marks when there are any, else the detail (the
   * hunk under the diff pane's line cursor while it has focus), else the selection.
   */
  subjects: Selection[]
  select: (sel: Selection | null) => void
  detail: Selection | null
  setDetail: (sel: Selection | null) => void
  isMarked: (sel: Selection) => boolean
  /** toggleMark returns an error message when sel can't join the current marks. */
  toggleMark: (sel: Selection) => string | null
  setMarks: (marks: Selection[]) => void
  clearMarks: () => void
}

const unstaged: Selection = { kind: "unstaged" }

export function useSelectionModel(ws: Workspace): SelectionModel {
  const [selection, select] = useState<Selection | null>({ kind: "unstaged" })
  const [rawMarks, setMarks] = useState<Selection[]>([])
  const [detail, setDetail] = useState<Selection | null>(null)

  // Derived rather than pruned in an effect, so a refetch never flashes stale marks.
  const live = liveKeys(ws)
  const alive = (m: Selection) => stillThere(ws, live, m)
  const marks = rawMarks.filter(alive)
  const markedKeys = new Set(marks.map(selectionKey))
  // An item an op removed (discarded, absorbed, squashed away, a committed file whose commit
  // was rewritten) falls back to Unstaged, so the details pane never asks for a diff of
  // something that is gone.
  const current = selection && !alive(selection) ? unstaged : selection

  function toggleMark(sel: Selection): string | null {
    const key = selectionKey(sel)
    if (markedKeys.has(key)) {
      setMarks(marks.filter((m) => selectionKey(m) !== key))
      return null
    }
    if (markClass(sel.kind) === 0) {
      return `Can't mark ${selectionLabel(sel)}`
    }
    const other = marks.find((m) => markClass(m.kind) !== markClass(sel.kind))
    if (other) {
      return `Can't mix marked ${other.kind}s with ${sel.kind}s`
    }
    if (sel.kind === "cfile" && marks.some((m) => m.kind === "cfile" && m.commit !== sel.commit)) {
      return "Marked files must come from the same commit"
    }
    setMarks([...marks, sel])
    return null
  }

  return {
    selection: current,
    marks,
    subjects: marks.length > 0 ? marks : detail ? [detail] : current ? [current] : [],
    select,
    detail,
    setDetail,
    isMarked: (sel) => markedKeys.has(selectionKey(sel)),
    toggleMark,
    setMarks,
    clearMarks: () => setMarks([]),
  }
}
