import type { QueryClient } from "@tanstack/react-query"
import type { OpResult, Workspace } from "@/api"
import type { Dialogs } from "@/dialogs/dialogs"
import type { WorkspaceOps } from "@/queries"
import type { Selection, SelectionKind, SelectionModel } from "@/selection"

/** Groups, in the order the palette and help list them (internal/ui/actions.go helpPalette). */
export const actionGroups = ["Commit", "Conflicts", "Branch", "History", "Review", "View"] as const
export type ActionGroup = (typeof actionGroups)[number]

/** UI hooks the workspace screen exposes to actions. */
export type ActionUI = {
  openPalette: (mode?: PaletteMode) => void
}

export type PaletteMode = "all" | "selection" | "help"

/**
 * ActionContext is what an action sees: the workspace, the subjects it acts on, and
 * the shared ops, dialogs and selection model. Built fresh for every keypress, palette
 * open and context menu, so `when` always reads current state.
 */
export type ActionContext = {
  workspace: Workspace
  /** The marks when there are any, else the selection (internal/ui Model.subjects). */
  subjects: Selection[]
  sel: SelectionModel
  ops: WorkspaceOps
  dialogs: Dialogs
  queryClient: QueryClient
  ui: ActionUI
  /** runOp runs a mutation with a toast for success, output and errors, like the TUI's runOp. */
  runOp: (title: string, fn: () => Promise<OpResult | void>) => Promise<boolean>
  notify: (message: string) => void
}

/**
 * Action mirrors `action` in internal/ui/actions.go: one entry serves the keyboard,
 * the command palette, the help and the context menu.
 */
export type Action = {
  /** Stable id, e.g. "commit", "branch.push". Registering the same id replaces it. */
  id: string
  title: string
  group: ActionGroup
  /**
   * Shortcuts in the TUI's notation: "c", "R" (shift is implied by the capital), "space",
   * "enter", "escape", "?", "ctrl+r". "mod+k" is cmd on macOS and ctrl elsewhere.
   * The first key is the one shown in menus.
   */
  keys?: string[]
  /** Not about the selection: left out of the context menu (TUI `global`). */
  global?: boolean
  /** Offered in edit mode (resolving a conflict); everything else is hidden there. */
  resolving?: boolean
  when: (ctx: ActionContext) => boolean
  run: (ctx: ActionContext) => void | Promise<void>
}

const registry = new Map<string, Action>()

/** registerActions adds actions; feature modules call it at import time. */
export function registerActions(...actions: Action[]) {
  for (const a of actions) registry.set(a.id, a)
}

export function allActions(): Action[] {
  return [...registry.values()]
}

/** offers reports whether a is available in the current mode (internal/ui Model.offers). */
export function offers(a: Action, ctx: ActionContext): boolean {
  return (a.resolving || !ctx.workspace.resolving) && a.when(ctx)
}

export function availableActions(ctx: ActionContext, includeGlobal: boolean): Action[] {
  return allActions().filter((a) => (includeGlobal || !a.global) && offers(a, ctx))
}

/** actionForKey is the first available action bound to key, as in internal/ui actionFor. */
export function actionForKey(key: string, ctx: ActionContext): Action | undefined {
  return allActions().find((a) => a.keys?.includes(key) && offers(a, ctx))
}

// Availability helpers, named after their counterparts in internal/ui/actions.go.

export const always = () => true

/** allOf reports whether every subject is one of kinds (and there is at least one). */
export function allOf(subjects: Selection[], ...kinds: SelectionKind[]): boolean {
  return subjects.length > 0 && subjects.every((s) => kinds.includes(s.kind))
}

/** one: exactly one subject, nothing marked, of one of kinds. */
export function one(...kinds: SelectionKind[]) {
  return (ctx: ActionContext) =>
    ctx.subjects.length === 1 && ctx.sel.marks.length === 0 && allOf(ctx.subjects, ...kinds)
}

/** anyOf: one or more subjects (marks included), all of kinds. */
export function anyOf(...kinds: SelectionKind[]) {
  return (ctx: ActionContext) => allOf(ctx.subjects, ...kinds)
}

export function hasUncommitted(ws: Workspace): boolean {
  return (
    ws.uncommittedChanges.length > 0 ||
    ws.stacks.some((s) => (s.assignedChanges?.length ?? 0) > 0)
  )
}

/** ids are the cliIds of the subjects; the Unstaged area has none. */
export function ids(subjects: Selection[]): string[] {
  return subjects.flatMap((s) => (s.kind === "unstaged" ? [] : [s.id]))
}
