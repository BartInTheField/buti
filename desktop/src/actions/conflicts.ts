import { useSyncExternalStore } from "react"
import { commitSubject, openFiles, type Commit, type Workspace } from "@/api"
import { one, registerActions, type ActionContext } from "./registry"

// Conflicts and edit mode, mirroring internal/ui/resolve.go: `e` checks a conflicted
// commit out in edit mode, then saves and exits; `x` cancels; `o` / enter open files.
// Also `o` / `O` for uncommitted files, which share the way files are opened.

/** EditedCommit is the commit being resolved; `but status` no longer names it in edit mode. */
export type EditedCommit = { cliId: string; subject: string; shortId: string; author?: string }

export type ConflictFile = { path: string; resolved: boolean }

type EditState = { editing: EditedCommit | null; selected: string | null }

const storageKey = "buti.editing"

function loadEditing(): EditedCommit | null {
  try {
    return JSON.parse(sessionStorage.getItem(storageKey) ?? "null") as EditedCommit | null
  } catch {
    return null
  }
}

// Kept outside React so actions (keys, palette) and the edit mode panel share it.
let state: EditState = { editing: loadEditing(), selected: null }
const listeners = new Set<() => void>()

function setState(next: Partial<EditState>) {
  state = { ...state, ...next }
  if ("editing" in next) {
    if (next.editing) sessionStorage.setItem(storageKey, JSON.stringify(next.editing))
    else sessionStorage.removeItem(storageKey)
  }
  listeners.forEach((l) => l())
}

export function useEditState(): EditState {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l)
      return () => listeners.delete(l)
    },
    () => state,
  )
}

export function selectConflictFile(path: string) {
  setState({ selected: path })
}

/** conflictFiles lists the files of the commit in edit mode, conflicted first (internal/ui conflictRows). */
export function conflictFiles(ws: Workspace): ConflictFile[] {
  const r = ws.resolving
  if (!r) return []
  const sorted = (ps?: string[]) => [...(ps ?? [])].sort()
  return [
    ...sorted(r.conflicted_files).map((path) => ({ path, resolved: false })),
    ...sorted(r.resolved_files).map((path) => ({ path, resolved: true })),
  ]
}

/** selectedConflictFile is the selected file, falling back to the first one while it is gone. */
export function selectedConflictFile(ws: Workspace, selected: string | null): ConflictFile | null {
  const files = conflictFiles(ws)
  return files.find((f) => f.path === selected) ?? files[0] ?? null
}

export function findCommit(ws: Workspace, cliId: string): Commit | undefined {
  for (const st of ws.stacks) {
    for (const b of st.branches) {
      const c = b.commits.find((c) => c.cliId === cliId)
      if (c) return c
    }
  }
  return undefined
}

function conflictedCommit(ctx: ActionContext): Commit | undefined {
  if (!one("commit")(ctx)) return undefined
  const c = findCommit(ctx.workspace, ctx.subjects[0].kind === "commit" ? ctx.subjects[0].id : "")
  return c?.conflicted ? c : undefined
}

const resolving = (ctx: ActionContext) => Boolean(ctx.workspace.resolving)

async function open(ctx: ActionContext, paths: string[], editor: boolean) {
  try {
    await openFiles(ctx.ops.cfg, paths, editor)
    ctx.notify(paths.length === 1 ? `Opened ${paths[0]}` : `Opened ${paths.length} files`)
  } catch (err) {
    ctx.notify(err instanceof Error ? err.message : "Could not open")
  }
}

/** openSelectedConflict opens one file of the commit in edit mode (enter / double-click). */
export function openConflictFile(ctx: ActionContext, path: string) {
  return open(ctx, [path], true)
}

/** saveAndExit commits the resolution, asking first when markers are left (internal/ui saveAndExit). */
export async function saveAndExit(ctx: ActionContext) {
  const left = ctx.workspace.resolving?.conflicted_files ?? []
  if (left.length > 0) {
    const ok = await ctx.dialogs.confirm({
      title: "Save with conflict markers left?",
      body: `These files still have markers, which would be committed as they are:\n\n${left.join("\n")}`,
      confirmLabel: "Save and exit",
      destructive: true,
    })
    if (!ok) return
  }
  if (await ctx.runOp("Saved and left edit mode · u to undo", () => ctx.ops.run("resolveFinish"))) {
    setState({ editing: null, selected: null })
  }
}

/** cancelEdit leaves edit mode without saving, after confirming (internal/ui cancelEdit). */
export async function cancelEdit(ctx: ActionContext) {
  const ok = await ctx.dialogs.confirm({
    title: "Leave edit mode without saving?",
    body: "The commit stays conflicted, and edits to its files are dropped.",
    confirmLabel: "Leave",
    destructive: true,
  })
  if (!ok) return
  if (await ctx.runOp("Left edit mode", () => ctx.ops.run("resolveCancel", { force: true }))) {
    setState({ editing: null, selected: null })
  }
}

function step(ctx: ActionContext, by: number) {
  const files = conflictFiles(ctx.workspace)
  if (files.length === 0) return
  const cur = selectedConflictFile(ctx.workspace, state.selected)
  const i = Math.max(files.findIndex((f) => f.path === cur?.path), 0)
  setState({ selected: files[Math.min(Math.max(i + by, 0), files.length - 1)].path })
}

registerActions(
  {
    id: "conflict.resolve",
    title: "Resolve in edit mode",
    group: "Conflicts",
    keys: ["e"],
    when: (ctx) => conflictedCommit(ctx) !== undefined,
    run: async (ctx) => {
      const c = conflictedCommit(ctx)
      if (!c) return
      const subject = commitSubject(c.message)
      setState({
        editing: { cliId: c.cliId, subject, shortId: c.commitId?.slice(0, 7) ?? c.cliId, author: c.authorName },
        selected: null,
      })
      if (!(await ctx.runOp(`Editing “${subject}”`, () => ctx.ops.run("resolveStart", { commit: c.cliId })))) {
        setState({ editing: null })
      }
    },
  },
  {
    id: "conflict.save",
    title: "Save and exit",
    group: "Conflicts",
    keys: ["e"],
    resolving: true,
    when: resolving,
    run: saveAndExit,
  },
  {
    id: "conflict.open-all",
    title: "Open conflicted files",
    group: "Conflicts",
    keys: ["o"],
    resolving: true,
    when: (ctx) => (ctx.workspace.resolving?.conflicted_files?.length ?? 0) > 0,
    run: (ctx) => open(ctx, ctx.workspace.resolving?.conflicted_files ?? [], true),
  },
  {
    id: "conflict.open-selected",
    title: "Open selected file",
    group: "Conflicts",
    keys: ["enter"],
    resolving: true,
    when: (ctx) => selectedConflictFile(ctx.workspace, state.selected) !== null,
    run: (ctx) => {
      const f = selectedConflictFile(ctx.workspace, state.selected)
      if (f) return openConflictFile(ctx, f.path)
    },
  },
  {
    id: "conflict.cancel",
    title: "Cancel editing…",
    group: "Conflicts",
    keys: ["x"],
    resolving: true,
    when: resolving,
    run: cancelEdit,
  },
  {
    id: "conflict.next-file",
    title: "Next file",
    group: "Conflicts",
    keys: ["j", "down"],
    resolving: true,
    when: (ctx) => conflictFiles(ctx.workspace).length > 1,
    run: (ctx) => step(ctx, 1),
  },
  {
    id: "conflict.prev-file",
    title: "Previous file",
    group: "Conflicts",
    keys: ["k", "up"],
    resolving: true,
    when: (ctx) => conflictFiles(ctx.workspace).length > 1,
    run: (ctx) => step(ctx, -1),
  },
  {
    id: "open-editor",
    title: "Open in editor",
    group: "View",
    keys: ["o"],
    // Committed files and hunks open the working-tree file, as in the TUI.
    when: one("file", "cfile"),
    run: (ctx) => {
      const s = ctx.subjects[0]
      if (s.kind === "file" || s.kind === "cfile") return open(ctx, [s.path], true)
    },
  },
  {
    id: "open-default",
    title: "Open with default app",
    group: "View",
    keys: ["O"],
    // Committed files and hunks open the working-tree file, as in the TUI.
    when: one("file", "cfile"),
    run: (ctx) => {
      const s = ctx.subjects[0]
      if (s.kind === "file" || s.kind === "cfile") return open(ctx, [s.path], false)
    },
  },
)
