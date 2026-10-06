export type Selection =
  | { kind: "unstaged" }
  | { kind: "file"; id: string; path: string }
  | { kind: "commit"; id: string; subject: string }
  | { kind: "branch"; id: string; name: string }

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
