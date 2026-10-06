import type { MouseEvent } from "react"
import { useDraggable, useDroppable } from "@dnd-kit/core"
import { FileIcon, FolderOpenIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import type { Change } from "@/api"
import { ActionContextMenu } from "@/actions/ActionContextMenu"
import { dropHint, type DragItem, type DropTarget } from "@/dnd"
import { sameSelection, type Selection, type SelectionModel } from "@/selection"
import { DropHint } from "./DropHint"
import { targetClass, useTargetDeco } from "./target/context"
import { TargetTag } from "./target/TargetTag"

type Props = {
  changes: Change[]
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  activeDrag: DragItem | null
  disabled?: boolean
}

const unstagedTarget: DropTarget = { kind: "unstaged", id: "unstaged", label: "Unstaged" }

export function UnstagedPanel({ changes, sel, onItemClick, activeDrag, disabled }: Props) {
  const { setNodeRef, isOver } = useDroppable({ id: "drop:unstaged", data: unstagedTarget, disabled })
  const hint = isOver ? dropHint(activeDrag, unstagedTarget) : null
  const area: Selection = { kind: "unstaged" }
  // Only a valid Unstaged gets a ring: dimming the panel would dim the files in it too.
  const deco = useTargetDeco("unstaged")

  return (
    <div
      ref={setNodeRef}
      data-testid="unstaged-panel"
      data-target-key="unstaged"
      className={cn(
        "relative flex h-full min-h-0 min-w-[180px] flex-col border-r bg-sidebar text-sidebar-foreground",
        hint && "bg-primary/10 ring-2 ring-inset ring-primary/40",
        deco?.status === "valid" && targetClass(deco, { inset: true }),
      )}
    >
      <ActionContextMenu item={area}>
        <button
          type="button"
          className={cn(
            "flex items-center gap-2 border-b px-3 py-2 text-left text-sm font-medium hover:bg-sidebar-accent",
            sameSelection(sel.selection, area) && "bg-sidebar-accent",
          )}
          onClick={(e) => onItemClick(area, e)}
        >
          <FolderOpenIcon className="size-4 shrink-0 opacity-70" />
          <span className="flex-1">Unstaged</span>
          <Badge variant="secondary" className="tabular-nums">
            {changes.length}
          </Badge>
        </button>
      </ActionContextMenu>
      <DropHint text={hint} className="bottom-3 left-1/2 -translate-x-1/2" />
      <TargetTag deco={deco} className="bottom-3 left-1/2 -translate-x-1/2" />
      <ScrollArea className="min-h-0 flex-1">
        <ul className="flex flex-col gap-0.5 p-2">
          {changes.length === 0 ? (
            <li className="px-2 py-3 text-xs text-muted-foreground">No uncommitted changes</li>
          ) : (
            changes.map((ch) => (
              <FileRow
                key={ch.cliId}
                change={ch}
                sel={sel}
                onItemClick={onItemClick}
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
  sel,
  onItemClick,
  disabled,
}: {
  change: Change
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  disabled?: boolean
}) {
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: `file:${change.cliId}`,
    data: { kind: "file", id: change.cliId, label: change.filePath } satisfies DragItem,
    disabled,
  })
  const item: Selection = { kind: "file", id: change.cliId, path: change.filePath }
  const selected = sameSelection(sel.selection, item)
  const marked = sel.isMarked(item)
  const deco = useTargetDeco(`file:${change.cliId}`)
  return (
    <li>
      <ActionContextMenu item={item}>
        <button
          type="button"
          data-testid="file-row"
          data-target-key={`file:${change.cliId}`}
          ref={setNodeRef}
          {...listeners}
          {...attributes}
          className={cn(
            "flex w-full cursor-grab items-center gap-2 rounded-md px-2 py-1.5 text-left text-xs active:cursor-grabbing",
            selected ? "bg-sidebar-accent font-medium" : "hover:bg-sidebar-accent/70",
            marked && "bg-primary/15 ring-1 ring-inset ring-primary/40",
            isDragging && "opacity-40",
            targetClass(deco, { inset: true }),
          )}
          onClick={(e) => onItemClick(item, e)}
        >
          <FileIcon className="size-3.5 shrink-0 opacity-60" />
          <span className="min-w-0 flex-1 truncate font-mono">{change.filePath}</span>
          <span className="shrink-0 text-[10px] text-muted-foreground uppercase">
            {change.changeType.slice(0, 1)}
          </span>
        </button>
      </ActionContextMenu>
    </li>
  )
}
