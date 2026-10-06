import { Badge } from "@/components/reui/badge"
import { cn } from "@/lib/utils"

/** Tones of a change type, as the TUI's changeTypeStyle colours them. */
function variant(status: string) {
  switch (status) {
    case "added":
      return "success-light"
    case "deleted":
    case "removed":
      return "destructive-light"
    case "renamed":
      return "info-light"
    default:
      return "warning-light"
  }
}

/** letter maps a change type to A, M, D or R; `but` says "removed", which must not read as renamed. */
function letter(status: string) {
  switch (status) {
    case "added":
      return "A"
    case "deleted":
    case "removed":
      return "D"
    case "renamed":
      return "R"
    default:
      return "M"
  }
}

/** StatusBadge is a file's change type as one letter: A, M, D or R. */
export function StatusBadge({ status, className }: { status: string; className?: string }) {
  return (
    <Badge
      size="xs"
      variant={variant(status)}
      title={status}
      className={cn("font-mono font-semibold uppercase", className)}
    >
      {letter(status)}
    </Badge>
  )
}

/** PathLabel shows the directory muted and the file name in full colour. */
export function PathLabel({ path, className }: { path: string; className?: string }) {
  const cut = path.lastIndexOf("/") + 1
  return (
    <span className={cn("min-w-0 truncate font-mono", className)}>
      {cut > 0 ? <span className="text-muted-foreground">{path.slice(0, cut)}</span> : null}
      {path.slice(cut)}
    </span>
  )
}
