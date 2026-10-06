import { useDraggable, useDroppable } from "@dnd-kit/core"
import { FileIcon, FolderOpenIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import type { Change } from "@/api"
import { dropHint, type DragItem } from "@/dnd"
import type { Selection } from "@/selection"

type Props = {
  changes: Change[]
  selection: Selection | null
  onSelect: (sel: Selection) => void
  activeDrag: DragItem | null
  disabled?: boolean
}

export function UnstagedPanel({
  changes,
  selection,
  onSelect,
  activeDrag,
  disabled,
}: Props) {
  const drop = useDroppable({
    id: "drop:unstaged",
    data: { kind: "unstaged", id: "unstaged", label: "Unstaged" },
    disabled,
  })
  const hint = dropHint(activeDrag, {
    kind: "unstaged",
    id: "unstaged",
    label: "Unstaged",
  })
  const over = drop.isOver && hint

  return (
    <div
      ref={drop.setNodeRef}
      className={cn(
        "flex h-full min-h-0 min-w-[180px] flex-col border-r bg-sidebar text-sidebar-foreground",
        over && "bg-primary/10 ring-2 ring-inset ring-primary/40",
      )}
    >
      <button
        type="button"
        className={cn(
          "flex items-center gap-2 border-b px-3 py-2 text-left text-sm font-medium hover:bg-sidebar-accent",
          selection?.kind === "unstaged" && "bg-sidebar-accent",
        )}
        onClick={() => onSelect({ kind: "unstaged" })}
      >
        <FolderOpenIcon className="size-4 shrink-0 opacity-70" />
        <span className="flex-1">Unstaged</span>
        <Badge variant="secondary" className="tabular-nums">
          {changes.length}
        </Badge>
      </button>
      {over ? (
        <p className="border-b bg-primary/15 px-3 py-1.5 text-xs text-primary">
          {hint}
        </p>
      ) : null}
      <ScrollArea className="min-h-0 flex-1">
        <ul className="flex flex-col gap-0.5 p-2">
          {changes.length === 0 ? (
            <li className="px-2 py-3 text-xs text-muted-foreground">
              No uncommitted changes
            </li>
          ) : (
            changes.map((ch) => (
              <FileRow
                key={ch.cliId}
                change={ch}
                selected={selection?.kind === "file" && selection.id === ch.cliId}
                onSelect={onSelect}
                disabled={disabled}
              />
            ))
          )}
        </ul>
      </ScrollArea>
    </div>
  )
}

function FileRow({
  change,
  selected,
  onSelect,
  disabled,
}: {
  change: Change
  selected: boolean
  onSelect: (sel: Selection) => void
  disabled?: boolean
}) {
  const drag = useDraggable({
    id: `file:${change.cliId}`,
    data: {
      kind: "file",
      id: change.cliId,
      label: change.filePath,
    } satisfies DragItem,
    disabled,
  })
  return (
    <li>
      <button
        type="button"
        ref={drag.setNodeRef}
        {...drag.listeners}
        {...drag.attributes}
        className={cn(
          "flex w-full cursor-grab items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs active:cursor-grabbing",
          selected ? "bg-sidebar-accent font-medium" : "hover:bg-sidebar-accent/70",
          drag.isDragging && "opacity-40",
        )}
        onClick={() =>
          onSelect({ kind: "file", id: change.cliId, path: change.filePath })
        }
      >
        <FileIcon className="size-3.5 shrink-0 opacity-60" />
        <span className="min-w-0 flex-1 truncate font-mono">{change.filePath}</span>
        <span className="shrink-0 text-[10px] uppercase text-muted-foreground">
          {change.changeType.slice(0, 1)}
        </span>
      </button>
    </li>
  )
}
