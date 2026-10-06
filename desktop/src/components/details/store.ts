import { useSyncExternalStore } from "react"
import type { LocatedComment } from "@/components/review/api"
import type { DiffLayout, HunkRef } from "./rows"

/**
 * DetailsView is the layout state of the details pane, shared by the pane, the lanes
 * (f / F) and the actions, as `details` and `showFiles` are in internal/ui.
 */
export type DetailsView = {
  /** The pane is shown under the lanes (d). */
  visible: boolean
  /** The pane fills the window (D). */
  full: boolean
  /** The file tree next to the diff in full screen (T). */
  tree: boolean
  /** Resolved and dismissed comments are left out of the diff (z). */
  hideResolved: boolean
  /** Commits whose files are listed under them (f), by commitKey. */
  showFiles: Record<string, boolean>
  /** Every commit's files are listed (F). */
  showAllFiles: boolean
}

let view: DetailsView = {
  visible: true,
  full: false,
  tree: true,
  hideResolved: false,
  showFiles: {},
  showAllFiles: false,
}
const listeners = new Set<() => void>()

export function getDetailsView(): DetailsView {
  return view
}

export function setDetailsView(patch: Partial<DetailsView> | ((v: DetailsView) => Partial<DetailsView>)) {
  view = { ...view, ...(typeof patch === "function" ? patch(view) : patch) }
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

export function useDetailsView(): DetailsView {
  return useSyncExternalStore(subscribe, getDetailsView)
}

/** useFilesShown reports whether a commit's files are listed under it in its lane. */
export function useFilesShown(commitKey: string): boolean {
  return useSyncExternalStore(subscribe, () => view.showAllFiles || Boolean(view.showFiles[commitKey]))
}

/** LineSel is the line cursor or range in the terms a review comment is anchored in (internal/ui lineSel). */
export type LineSel = {
  path: string
  side: "new" | "old"
  line: number
  endLine: number
  /** The lines on side, newline separated, without the diff signs. */
  text: string
  hunk: HunkRef
}

/**
 * DetailsPane is what the mounted diff pane exposes to actions: its cursor, and the
 * moves the keys and menus make. Null while no pane is mounted.
 */
export type DetailsPane = {
  layout: () => DiffLayout | null
  /** The pane has keyboard focus. */
  focused: () => boolean
  hasCursor: () => boolean
  ranged: () => boolean
  cursorHunk: () => HunkRef | null
  /** The comment under the line cursor. */
  cursorComment: () => LocatedComment | null
  lineSelection: () => LineSel | null
  stepHunk: (dir: number) => void
  toggleRange: () => void
  cancelRange: () => void
  focus: () => void
  /** Puts the cursor on a comment once the diff showing it is on screen. */
  jumpToComment: (id: string) => void
  /** Opens the comment composer at the cursor: a new comment, an edit or a reply. */
  compose: (c: Composer) => void
  scrollToFile: (path: string) => void
}

export type Composer =
  | { mode: "new"; title: string; submit: (body: string) => Promise<boolean> }
  | { mode: "edit" | "reply"; title: string; initial?: string; submit: (body: string) => Promise<boolean> }

let pane: { current: DetailsPane } | null = null

/** detailsPane holds the mounted pane's ref, which it updates every render. */
export const detailsPane = {
  get: (): DetailsPane | null => pane?.current ?? null,
  set: (ref: { current: DetailsPane } | null) => {
    pane = ref
  },
}

/**
 * paneActionIds are the actions the diff pane runs first for a key while it has focus,
 * ahead of workspace actions on the same key (the TUI lists comment actions first).
 */
export const paneActionIds = new Set<string>()

/** A comment to put the cursor on when the next diff arrives (internal/ui details.jump). */
let pendingJump: string | null = null

export function setPendingJump(id: string | null) {
  pendingJump = id
}

export function takePendingJump(): string | null {
  const id = pendingJump
  pendingJump = null
  return id
}

export function peekPendingJump(): string | null {
  return pendingJump
}
