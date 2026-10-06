import { CornerDownLeftIcon } from "lucide-react"
import { Kbd } from "@/components/ui/kbd"

/** PaletteFooter is the key legend under the command palette and the pick lists. */
export function PaletteFooter({ action = "Run" }: { action?: string }) {
  return (
    <div className="flex h-9 shrink-0 items-center justify-end gap-3 border-t bg-muted/40 px-3 text-xs text-muted-foreground">
      <span className="flex items-center gap-1.5">
        <Kbd>↑</Kbd>
        <Kbd>↓</Kbd>
        Navigate
      </span>
      <span className="flex items-center gap-1.5">
        <Kbd>
          <CornerDownLeftIcon />
        </Kbd>
        {action}
      </span>
      <span className="flex items-center gap-1.5">
        <Kbd>esc</Kbd>
        Close
      </span>
    </div>
  )
}
