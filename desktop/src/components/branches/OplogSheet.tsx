import { HistoryIcon, RotateCcwIcon } from "lucide-react"
import type { OplogEntry } from "@/api"
import { useActions } from "@/actions/context"
import { Button } from "@/components/ui/button"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet"
import { Skeleton } from "@/components/ui/skeleton"
import { useOplog } from "@/queries"
import { humanizeOp, relTime } from "./format"

function entryTitle(e: OplogEntry): string {
  return humanizeOp(e.details.title || e.details.operation)
}

/**
 * OplogSheet is the operation history (H in the TUI): every snapshot `but` recorded,
 * newest first, each with a Restore button that goes back to before that operation.
 */
export function OplogSheet({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const actions = useActions()
  const { ops } = actions.contextFor()
  const oplog = useOplog(ops.cfg, open)

  async function restore(e: OplogEntry) {
    const ctx = actions.contextFor()
    const short = e.id.slice(0, 7)
    const ok = await ctx.dialogs.confirm({
      title: `Restore to before “${entryTitle(e)}”?`,
      body: `The workspace goes back to snapshot ${short}. This is itself recorded, so it can be undone.`,
      confirmLabel: "Restore",
    })
    if (ok) await ctx.runOp(`Restored snapshot ${short}`, () => ctx.ops.run("oplogRestore", { snapshot: e.id }))
  }

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="gap-0 sm:max-w-md" data-testid="oplog-sheet">
        <SheetHeader className="border-b">
          <SheetTitle className="flex items-center gap-2">
            <HistoryIcon className="size-4" />
            Operation history
          </SheetTitle>
          <SheetDescription>Restore the workspace to before any operation.</SheetDescription>
        </SheetHeader>
        <ScrollArea className="min-h-0 flex-1">
          <ul className="flex flex-col p-2">
            {oplog.isPending ? (
              Array.from({ length: 8 }, (_, i) => (
                <li key={i} className="flex h-12 items-center px-2">
                  <Skeleton className="h-4 w-2/3" />
                </li>
              ))
            ) : oplog.isError ? (
              <li className="px-2 py-3 text-sm text-destructive">
                {oplog.error instanceof Error ? oplog.error.message : "Could not load the history"}
              </li>
            ) : oplog.data.length === 0 ? (
              <li className="px-2 py-3 text-sm text-muted-foreground">No operations recorded yet</li>
            ) : (
              oplog.data.map((e) => (
                <li
                  key={e.id}
                  data-testid="oplog-entry"
                  className="group flex h-12 items-center gap-3 rounded-md px-2 hover:bg-muted/70"
                >
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-sm">{entryTitle(e)}</p>
                    <p className="truncate font-mono text-[11px] text-muted-foreground">
                      {e.id.slice(0, 7)} · {relTime(e.createdAt)}
                    </p>
                  </div>
                  {/* Always laid out; only its opacity follows hover, so rows never reflow. */}
                  <Button
                    variant="outline"
                    size="sm"
                    className="opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                    disabled={ops.busy}
                    onClick={() => void restore(e)}
                  >
                    <RotateCcwIcon />
                    Restore
                  </Button>
                </li>
              ))
            )}
          </ul>
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
