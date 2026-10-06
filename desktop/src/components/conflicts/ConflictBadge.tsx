import { Badge } from "@/components/reui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

/** ConflictBadge marks a conflicted commit in its lane, like the TUI's ✗. Always rendered at its size, so rows never shift. */
export function ConflictBadge() {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge variant="destructive-light" size="xs" data-testid="conflict-badge">
          ✗ Conflicted
        </Badge>
      </TooltipTrigger>
      <TooltipContent side="top">Conflicted. Select it and press e to resolve it in edit mode.</TooltipContent>
    </Tooltip>
  )
}
