import { HistoryIcon, RotateCcwIcon } from "lucide-react"
import type { OplogEntry } from "@/api"
import { useActions } from "@/actions/context"
import { IconTile } from "@/components/reui/icon-tile"
import {
  Timeline,
  TimelineDate,
  TimelineIndicator,
  TimelineItem,
  TimelineSeparator,
  TimelineTitle,
} from "@/components/reui/timeline"
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
          <div className="flex items-center gap-3">
            <IconTile variant="soft" size="sm">
              <HistoryIcon />
            </IconTile>
            <div className="min-w-0">
              <SheetTitle>Operation history</SheetTitle>
              <SheetDescription>Restore the workspace to before any operation.</SheetDescription>
            </div>
          </div>
        </SheetHeader>
        <ScrollArea className="min-h-0 flex-1">
          {oplog.isPending ? (
            <ul className="flex flex-col p-2">
              {Array.from({ length: 8 }, (_, i) => (
                <li key={i} className="flex h-12 items-center gap-3 px-2">
                  <Skeleton className="size-2.5 rounded-full" />
                  <Skeleton className="h-4 w-2/3" />
                </li>
              ))}
            </ul>
          ) : oplog.isError ? (
            <p className="px-4 py-3 text-sm text-destructive">
              {oplog.error instanceof Error ? oplog.error.message : "Could not load the history"}
            </p>
          ) : oplog.data.length === 0 ? (
            <p className="px-4 py-3 text-sm text-muted-foreground">No operations recorded yet</p>
          ) : (
            // Step 1 is the newest snapshot, the workspace as it is now: only its dot is filled.
            <Timeline value={1} role="list" className="p-2">
              {oplog.data.map((e, i) => (
                <TimelineItem
                  key={e.id}
                  step={i + 1}
                  role="listitem"
                  data-testid="oplog-entry"
                  className="group flex-none flex-row items-center gap-3 rounded-md pr-2 hover:bg-muted/70 group-data-[orientation=vertical]/timeline:ms-6 group-data-[orientation=vertical]/timeline:not-last:pb-0 h-12"
                >
                  <TimelineSeparator className="bg-border group-data-[orientation=vertical]/timeline:-left-3 group-data-[orientation=vertical]/timeline:h-[calc(100%-10px)] group-data-[orientation=vertical]/timeline:translate-y-[29px]" />
                  <TimelineIndicator className="size-2.5 border-2 border-primary/50 bg-background group-data-completed/timeline-item:bg-primary group-data-[orientation=vertical]/timeline:top-[19px] group-data-[orientation=vertical]/timeline:-left-3" />
                  <div className="min-w-0 flex-1">
                    <TimelineTitle className="truncate font-normal">{entryTitle(e)}</TimelineTitle>
                    <TimelineDate className="mb-0 truncate font-mono text-[11px] font-normal">
                      {e.id.slice(0, 7)} · {relTime(e.createdAt)}
                    </TimelineDate>
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
                </TimelineItem>
              ))}
            </Timeline>
          )}
        </ScrollArea>
      </SheetContent>
    </Sheet>
  )
}
