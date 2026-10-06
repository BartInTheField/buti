import type { MouseEvent } from "react"
import { useDraggable, useDroppable } from "@dnd-kit/core"
import { GitBranchIcon, PlusIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { branchPR, commitSubject, type Branch, type Commit, type Stack } from "@/api"
import { ActionContextMenu } from "@/actions/ActionContextMenu"
import { dropHint, type DragItem, type DropTarget } from "@/dnd"
import { sameSelection, type Selection, type SelectionModel } from "@/selection"
import { DropHint } from "./DropHint"

type Props = {
  stacks: Stack[]
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  activeDrag: DragItem | null
  disabled?: boolean
}

export function StackLanes({ stacks, sel, onItemClick, activeDrag, disabled }: Props) {
  return (
    <ScrollArea className="h-full min-h-0 flex-1">
      <div className="flex h-full min-h-[280px] gap-3 p-3">
        {stacks.length === 0 ? (
          <p className="self-center px-4 text-sm text-muted-foreground">
            No applied stacks.
          </p>
        ) : (
          stacks.map((stack) => (
            <div key={stack.cliId} className="flex w-64 shrink-0 flex-col gap-2" data-testid="lane">
              {[...stack.branches].reverse().map((branch) => (
                <BranchCard
                  key={branch.cliId}
                  branch={branch}
                  sel={sel}
                  onItemClick={onItemClick}
                  activeDrag={activeDrag}
                  disabled={disabled}
                />
              ))}
            </div>
          ))
        )}
        <NewBranchLane activeDrag={activeDrag} disabled={disabled} />
      </div>
    </ScrollArea>
  )
}

/** useDropTarget registers a droppable and returns the hint to show while it is hovered. */
function useDropTarget(target: DropTarget, activeDrag: DragItem | null, disabled?: boolean) {
  const { setNodeRef, isOver } = useDroppable({ id: `drop:${target.kind}:${target.id}`, data: target, disabled })
  return { dropRef: setNodeRef, hint: isOver ? dropHint(activeDrag, target) : null }
}

function BranchCard({
  branch,
  sel,
  onItemClick,
  activeDrag,
  disabled,
}: {
  branch: Branch
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  const target: DropTarget = { kind: "branch", id: branch.cliId, label: branch.name }
  const { dropRef, hint } = useDropTarget(target, activeDrag, disabled)
  const { setNodeRef: dragRef, listeners, attributes, isDragging } = useDraggable({
    id: `branch:${branch.cliId}`,
    data: { kind: "branch", id: branch.cliId, label: branch.name } satisfies DragItem,
    disabled,
  })
  const item: Selection = { kind: "branch", id: branch.cliId, name: branch.name }
  const selected = sameSelection(sel.selection, item)
  const marked = sel.isMarked(item)
  const pr = branchPR(branch.reviewId)

  return (
    <div
      ref={dropRef}
      data-testid="branch-card"
      data-branch={branch.name}
      className={cn(
        // Hover feedback is ring + background only: no border-width change, no inserted rows.
        "relative rounded-lg border bg-card text-card-foreground shadow-xs transition-[box-shadow,background-color]",
        hint && "bg-primary/5 ring-2 ring-primary/60",
        selected && !hint && "ring-1 ring-primary/40",
      )}
    >
      <DropHint text={hint} className="-top-2.5 right-2" />
      <ActionContextMenu item={item}>
        <button
          type="button"
          ref={dragRef}
          className={cn(
            "flex w-full cursor-grab items-center gap-2 rounded-t-lg border-b px-3 py-2 text-left active:cursor-grabbing",
            marked && "bg-primary/15",
            isDragging && "opacity-40",
          )}
          {...listeners}
          {...attributes}
          onClick={(e) => onItemClick(item, e)}
        >
          <GitBranchIcon className="size-3.5 shrink-0 opacity-70" />
          <span className="min-w-0 flex-1 truncate text-sm font-medium">{branch.name}</span>
          {pr ? (
            <Badge variant="outline" className="text-[10px]">
              {pr}
            </Badge>
          ) : null}
          {branch.branchStatus ? (
            <span className="truncate text-[10px] text-muted-foreground">
              {branch.branchStatus}
            </span>
          ) : null}
        </button>
      </ActionContextMenu>
      <ul className="flex flex-col gap-0.5 p-1.5">
        {branch.commits.length === 0 ? (
          <li className="px-2 py-2 text-xs text-muted-foreground">No commits</li>
        ) : (
          branch.commits.map((c) => (
            <CommitRow
              key={c.cliId}
              commit={c}
              branch={branch.name}
              sel={sel}
              onItemClick={onItemClick}
              activeDrag={activeDrag}
              disabled={disabled}
            />
          ))
        )}
      </ul>
    </div>
  )
}

function CommitRow({
  commit,
  branch,
  sel,
  onItemClick,
  activeDrag,
  disabled,
}: {
  commit: Commit
  branch: string
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  const subject = commitSubject(commit.message)
  const { dropRef, hint } = useDropTarget({ kind: "commit", id: commit.cliId, label: subject }, activeDrag, disabled)
  const { setNodeRef: dragRef, listeners, attributes, isDragging } = useDraggable({
    id: `commit:${commit.cliId}`,
    data: { kind: "commit", id: commit.cliId, label: subject } satisfies DragItem,
    disabled,
  })
  const item: Selection = { kind: "commit", id: commit.cliId, subject, branch }
  const selected = sameSelection(sel.selection, item)
  const marked = sel.isMarked(item)

  return (
    <li className="relative">
      <ActionContextMenu item={item}>
        <button
          type="button"
          data-testid="commit-row"
          ref={(node) => {
            dropRef(node)
            dragRef(node)
          }}
          {...listeners}
          {...attributes}
          className={cn(
            "flex h-7 w-full cursor-grab items-center rounded-md px-2 text-left active:cursor-grabbing",
            selected ? "bg-accent font-medium" : "hover:bg-muted/70",
            marked && "bg-primary/15",
            hint && "bg-primary/10 ring-2 ring-inset ring-primary/60",
            isDragging && "opacity-40",
            commit.conflicted && "text-destructive",
          )}
          onClick={(e) => onItemClick(item, e)}
        >
          <span className="truncate text-xs">{subject}</span>
        </button>
      </ActionContextMenu>
      <DropHint text={hint} className="top-1/2 right-1.5 -translate-y-1/2" />
    </li>
  )
}

function NewBranchLane({
  activeDrag,
  disabled,
}: {
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  const { dropRef, hint } = useDropTarget(
    { kind: "new-branch", id: "new-branch", label: "New branch" },
    activeDrag,
    disabled,
  )

  return (
    <div
      ref={dropRef}
      data-testid="new-branch-lane"
      className={cn(
        "flex w-44 shrink-0 flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-8 text-center text-muted-foreground",
        hint && "border-primary bg-primary/10 text-primary",
      )}
    >
      <PlusIcon className="size-5" />
      {/* Two fixed lines, whatever the text, so the lane never resizes. */}
      <p className="line-clamp-2 h-8 text-xs leading-4 font-medium">
        {hint ?? "Drop for a new branch"}
      </p>
    </div>
  )
}
