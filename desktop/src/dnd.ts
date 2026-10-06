import type { Placement } from "./api"

export type DragKind = "file" | "commit" | "branch"

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

export type DropAction =
  | { type: "commit"; placement: Placement; needsMessage: true }
  | { type: "amend"; target: string }
  | { type: "move"; placement: Placement }
  | { type: "uncommit" }

/** Mirrors the TUI dropVerb matrix in internal/ui/target.go. */
export function resolveDrop(source: DragItem, target: DropTarget): DropAction | null {
  if (source.kind === "file") {
    if (target.kind === "branch") {
      return {
        type: "commit",
        placement: { branch: target.label },
        needsMessage: true,
      }
    }
    if (target.kind === "new-branch") {
      return { type: "commit", placement: { newBranch: true }, needsMessage: true }
    }
    if (target.kind === "commit") {
      return { type: "amend", target: target.id }
    }
    return null
  }
  if (source.kind === "commit") {
    if (target.kind === "unstaged") {
      return { type: "uncommit" }
    }
    if (target.kind === "branch") {
      return { type: "move", placement: { branch: target.label } }
    }
    if (target.kind === "new-branch") {
      return { type: "move", placement: { newBranch: true } }
    }
    if (target.kind === "commit") {
      // Squash is out of MVP scope for #55; treat as amend-adjacent no-op for now.
      return null
    }
    return null
  }
  if (source.kind === "branch") {
    if (target.kind === "branch") {
      return { type: "move", placement: { branch: target.label } }
    }
    if (target.kind === "new-branch") {
      return { type: "move", placement: { newBranch: true } }
    }
    if (target.kind === "unstaged") {
      return { type: "uncommit" }
    }
  }
  return null
}

export function dropHint(source: DragItem | null, target: DropTarget): string | null {
  if (!source) return null
  const action = resolveDrop(source, target)
  if (!action) return null
  switch (action.type) {
    case "commit":
      return action.placement.newBranch
        ? "Commit onto a new branch"
        : `Commit onto ${action.placement.branch}`
    case "amend":
      return "Amend into this commit"
    case "move":
      return action.placement.newBranch
        ? "Move onto a new branch"
        : `Move onto ${action.placement.branch}`
    case "uncommit":
      return "Uncommit to Unstaged"
  }
}
