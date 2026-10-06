import { useState } from "react"
import type { Workspace } from "./api"

export type Selection =
  | { kind: "unstaged" }
  | { kind: "file"; id: string; path: string }
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
      return sel.path
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
      for (const c of b.commits) keys.add(`commit:${c.cliId}`)
    }
  }
  return keys
}

export type SelectionModel = {
  selection: Selection | null
  /** Marked items, in the order they were marked. */
  marks: Selection[]
  /** subjects are what actions act on: the marks when there are any, else the selection. */
  subjects: Selection[]
  select: (sel: Selection | null) => void
  isMarked: (sel: Selection) => boolean
  /** toggleMark returns an error message when sel can't join the current marks. */
  toggleMark: (sel: Selection) => string | null
  setMarks: (marks: Selection[]) => void
  clearMarks: () => void
}

export function useSelectionModel(ws: Workspace): SelectionModel {
  const [selection, select] = useState<Selection | null>({ kind: "unstaged" })
  const [rawMarks, setMarks] = useState<Selection[]>([])

  // Derived rather than pruned in an effect, so a refetch never flashes stale marks.
  const live = liveKeys(ws)
  const marks = rawMarks.filter((m) => live.has(selectionKey(m)))
  const markedKeys = new Set(marks.map(selectionKey))

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
    setMarks([...marks, sel])
    return null
  }

  return {
    selection,
    marks,
    subjects: marks.length > 0 ? marks : selection ? [selection] : [],
    select,
    isMarked: (sel) => markedKeys.has(selectionKey(sel)),
    toggleMark,
    setMarks,
    clearMarks: () => setMarks([]),
  }
}
