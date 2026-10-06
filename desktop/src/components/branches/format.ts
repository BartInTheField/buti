// Text helpers shared by the branch and history UI, ported from internal/ui/actions.go.

/** relTime is "just now", "5m ago", "3h ago", "4d ago" or a date, like the TUI's relTime. */
export function relTime(millis: number, now = Date.now()): string {
  const d = now - millis
  const min = 60_000
  const hour = 60 * min
  if (d < min) return "just now"
  if (d < hour) return `${Math.floor(d / min)}m ago`
  if (d < 48 * hour) return `${Math.floor(d / hour)}h ago`
  if (d < 60 * 24 * hour) return `${Math.floor(d / (24 * hour))}d ago`
  return new Date(millis).toLocaleDateString(undefined, {
    day: "numeric",
    month: "short",
    year: "numeric",
  })
}

/** humanizeOp turns "CreateCommit" into "Create commit". */
export function humanizeOp(s: string): string {
  return s.replace(/(?!^)([A-Z])/g, (c) => ` ${c.toLowerCase()}`)
}

/** pushState describes a branch's push status for its badge; its colors follow internal/ui branchColor. */
export function pushState(status?: string): { label: string; tip: string; tone: "local" | "ahead" | "force" | "pushed" | "integrated" } | null {
  switch (status) {
    case "completelyUnpushed":
      return { label: "local", tip: "Not pushed yet", tone: "local" }
    case "unpushedCommits":
      return { label: "unpushed", tip: "Has commits to push", tone: "ahead" }
    case "unpushedCommitsRequiringForce":
      return { label: "diverged", tip: "The remote branch has diverged; pushing needs a force push", tone: "force" }
    case "nothingToPush":
      return { label: "pushed", tip: "Up to date with the remote", tone: "pushed" }
    case "integrated":
      return { label: "integrated", tip: "Merged into the target branch", tone: "integrated" }
    default:
      return null
  }
}

/** canPush mirrors canPush in internal/ui/lanes.go. */
export function canPush(status?: string): boolean {
  return status !== "nothingToPush" && status !== "integrated"
}
