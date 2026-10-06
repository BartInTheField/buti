import { useMemo, type CSSProperties, type MouseEvent } from "react"
import { useDraggable, useDroppable } from "@dnd-kit/core"
import { CircleCheckIcon, FolderGit2Icon } from "lucide-react"
import { Badge } from "@/components/reui/badge"
import { IconTile } from "@/components/reui/icon-tile"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import type { Change } from "@/api"
import { ActionContextMenu } from "@/actions/ActionContextMenu"
import { dropHint, type DragItem, type DropTarget } from "@/dnd"
import { sameSelection, type Selection, type SelectionModel } from "@/selection"
import { PathTree, TreeFileLabel, treeFileClass, treeRowHover, treeRowSelected } from "./details/FileTree"
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
  const files = useMemo(() => changes.map((change) => ({ path: change.filePath, change })), [changes])

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
            "flex h-10 shrink-0 items-center gap-2 border-b px-3 text-left text-sm font-medium transition-colors",
            sameSelection(sel.selection, area) ? "bg-sidebar-accent" : "hover:bg-sidebar-accent/60",
          )}
          onClick={(e) => onItemClick(area, e)}
        >
          <IconTile variant="outline" size="xs" aria-hidden="true">
            <FolderGit2Icon />
          </IconTile>
          <span className="flex-1">Unstaged</span>
          <Badge variant={changes.length ? "primary-light" : "secondary"} size="sm" radius="full" className="tabular-nums">
            {changes.length}
          </Badge>
        </button>
      </ActionContextMenu>
      <DropHint text={hint} className="bottom-3 left-1/2 -translate-x-1/2" />
      <TargetTag deco={deco} className="bottom-3 left-1/2 -translate-x-1/2" />
      <ScrollArea className="min-h-0 flex-1">
        {changes.length === 0 ? (
          <div className="flex flex-col items-center gap-2 px-4 py-8 text-center">
            <IconTile variant="soft" size="sm" className="text-success" aria-hidden="true">
              <CircleCheckIcon />
            </IconTile>
            <p className="text-xs text-muted-foreground">No uncommitted changes</p>
          </div>
        ) : (
          <PathTree
            files={files}
            className="p-1.5 select-none"
            renderFile={(f, name, pad) => (
              <FileRow
                change={f.change}
                name={name}
                pad={pad}
                sel={sel}
                onItemClick={onItemClick}
                disabled={disabled}
              />
            )}
          />
        )}
      </ScrollArea>
    </div>
  )
}

function FileRow({
  change,
  name,
  pad,
  sel,
  onItemClick,
  disabled,
}: {
  change: Change
  name: string
  pad: CSSProperties
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
    <ActionContextMenu item={item}>
      <button
        type="button"
        data-testid="file-row"
        data-target-key={`file:${change.cliId}`}
        ref={setNodeRef}
        {...listeners}
        {...attributes}
        className={cn(
          treeFileClass,
          "cursor-grab active:cursor-grabbing",
          selected ? treeRowSelected : treeRowHover,
          marked && "bg-primary/12 text-foreground ring-1 ring-inset ring-primary/40 dark:bg-primary/20",
          isDragging && "opacity-40",
          targetClass(deco, { inset: true }),
        )}
        style={pad}
        title={change.filePath}
        onClick={(e) => onItemClick(item, e)}
      >
        <TreeFileLabel name={name} status={change.changeType} />
      </button>
    </ActionContextMenu>
  )
}
