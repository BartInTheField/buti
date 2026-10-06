import { pointerWithin, type CollisionDetection } from "@dnd-kit/core"
import type { Workspace } from "./api"
import type { Selection } from "./selection"
import { dropPlan, entityFor, type Plan } from "./target"

export type DragKind = "file" | "cfile" | "commit" | "branch"

export type DragItem = {
  kind: DragKind
  id: string
  label: string
}

export type DropKind =
  | "unstaged"
  | "branch"
  | "commit"
  | "new-branch"

export type DropTarget = {
  kind: DropKind
  id: string
  label: string
}

// A drop runs the verb the TUI picks for it (dropVerb in internal/ui/target.go) through
// the same plans as the target picker. The drag session holds the workspace as it was
// when the drag started (refetches pause during a drag) and every source being dragged.
let session: { workspace: Workspace; sources: Selection[] } | null = null

/** beginDrag starts a drag of sources (the marks, when the dragged item is marked). */
export function beginDrag(workspace: Workspace, sources: Selection[]) {
  session = { workspace, sources }
}

export function endDrag() {
  session = null
}

/** resolveDrop is what dropping the current drag onto target does, or null when it can't. */
export function resolveDrop(source: DragItem, target: DropTarget): Plan | null {
  if (!session || target.id === source.id) return null
  const entity = entityFor(session.workspace, target.kind, target.id)
  return entity ? dropPlan(session.workspace, session.sources, entity) : null
}

export function dropHint(source: DragItem | null, target: DropTarget): string | null {
  return source ? (resolveDrop(source, target)?.label ?? null) : null
}

/**
 * innermostAcceptingDroppable is the workspace's collision detection. Of the droppables
 * under the pointer it keeps those that accept the dragged item (resolveDrop) and picks
 * the smallest, so a commit row inside a branch card always wins over the card. It never
 * falls back to the nearest droppable: empty space is no target, so `over` can't jump around.
 */
export const innermostAcceptingDroppable: CollisionDetection = (args) => {
  const source = args.active.data.current as DragItem | undefined
  if (!source) return []
  const pointed = pointerWithin(args)
  const accepting = pointed.flatMap((c) => {
    const container = args.droppableContainers.find((d) => d.id === c.id)
    const target = container?.data.current as DropTarget | undefined
    const rect = args.droppableRects.get(c.id)
    if (!target || !rect || !resolveDrop(source, target)) return []
    return [{ id: c.id, data: { ...c.data, value: rect.width * rect.height } }]
  })
  return accepting.sort((a, b) => a.data.value - b.data.value)
}
