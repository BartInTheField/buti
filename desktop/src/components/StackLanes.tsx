import { useDraggable, useDroppable } from "@dnd-kit/core"
import { GitBranchIcon, PlusIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { branchPR, commitSubject, type Branch, type Commit, type Stack } from "@/api"
import { dropHint, type DragItem } from "@/dnd"
import type { Selection } from "@/selection"

type Props = {
  stacks: Stack[]
  selection: Selection | null
  onSelect: (sel: Selection) => void
  activeDrag: DragItem | null
  disabled?: boolean
}

export function StackLanes({
  stacks,
  selection,
  onSelect,
  activeDrag,
  disabled,
}: Props) {
  return (
    <ScrollArea className="h-full min-h-0 flex-1">
      <div className="flex h-full min-h-[280px] gap-3 p-3">
        {stacks.length === 0 ? (
          <p className="self-center px-4 text-sm text-muted-foreground">
            No applied stacks.
          </p>
        ) : (
          stacks.map((stack) => (
            <StackLane
              key={stack.cliId}
              stack={stack}
              selection={selection}
              onSelect={onSelect}
              activeDrag={activeDrag}
              disabled={disabled}
            />
          ))
        )}
        <NewBranchLane activeDrag={activeDrag} disabled={disabled} />
      </div>
    </ScrollArea>
  )
}

function StackLane({
  stack,
  selection,
  onSelect,
  activeDrag,
  disabled,
}: {
  stack: Stack
  selection: Selection | null
  onSelect: (sel: Selection) => void
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  return (
    <div className="flex w-64 shrink-0 flex-col gap-2">
      {[...stack.branches].reverse().map((branch) => (
        <BranchCard
          key={branch.cliId}
          branch={branch}
          selection={selection}
          onSelect={onSelect}
          activeDrag={activeDrag}
          disabled={disabled}
        />
      ))}
    </div>
  )
}

function BranchCard({
  branch,
  selection,
  onSelect,
  activeDrag,
  disabled,
}: {
  branch: Branch
  selection: Selection | null
  onSelect: (sel: Selection) => void
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  const drop = useDroppable({
    id: `drop:branch:${branch.cliId}`,
    data: {
      kind: "branch",
      id: branch.cliId,
      label: branch.name,
    },
    disabled,
  })
  const drag = useDraggable({
    id: `branch:${branch.cliId}`,
    data: {
      kind: "branch",
      id: branch.cliId,
      label: branch.name,
    } satisfies DragItem,
    disabled,
  })
  const target = { kind: "branch" as const, id: branch.cliId, label: branch.name }
  const hint = dropHint(activeDrag, target)
  const over = drop.isOver && hint
  const selected = selection?.kind === "branch" && selection.id === branch.cliId
  const pr = branchPR(branch.reviewId)

  return (
    <div
      ref={(node) => {
        drop.setNodeRef(node)
        drag.setNodeRef(node)
      }}
      className={cn(
        "rounded-lg border bg-card text-card-foreground shadow-xs",
        over && "ring-2 ring-primary/50",
        selected && "border-primary/40",
        drag.isDragging && "opacity-40",
      )}
    >
      <button
        type="button"
        className="flex w-full cursor-grab items-center gap-2 border-b px-3 py-2 text-left active:cursor-grabbing"
        {...drag.listeners}
        {...drag.attributes}
        onClick={() =>
          onSelect({ kind: "branch", id: branch.cliId, name: branch.name })
        }
      >
        <GitBranchIcon className="size-3.5 shrink-0 opacity-70" />
        <span className="min-w-0 flex-1 truncate text-sm font-medium">
          {branch.name}
        </span>
        {pr ? (
          <Badge variant="outline" className="text-[10px]">
            {pr}
          </Badge>
        ) : null}
        {branch.branchStatus ? (
          <span className="text-[10px] text-muted-foreground">
            {branch.branchStatus}
          </span>
        ) : null}
      </button>
      {over ? (
        <p className="bg-primary/10 px-3 py-1 text-xs text-primary">{hint}</p>
      ) : null}
      <ul className="flex flex-col gap-0.5 p-1.5">
        {branch.commits.length === 0 ? (
          <li className="px-2 py-2 text-xs text-muted-foreground">No commits</li>
        ) : (
          branch.commits.map((c) => (
            <CommitRow
              key={c.cliId}
              commit={c}
              selected={selection?.kind === "commit" && selection.id === c.cliId}
              onSelect={onSelect}
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
  selected,
  onSelect,
  activeDrag,
  disabled,
}: {
  commit: Commit
  selected: boolean
  onSelect: (sel: Selection) => void
  activeDrag: DragItem | null
  disabled?: boolean
}) {
  const subject = commitSubject(commit.message)
  const drop = useDroppable({
    id: `drop:commit:${commit.cliId}`,
    data: { kind: "commit", id: commit.cliId, label: subject },
    disabled,
  })
  const drag = useDraggable({
    id: `commit:${commit.cliId}`,
    data: {
      kind: "commit",
      id: commit.cliId,
      label: subject,
    } satisfies DragItem,
    disabled,
  })
  const hint = dropHint(activeDrag, {
    kind: "commit",
    id: commit.cliId,
    label: subject,
  })
  const over = drop.isOver && hint

  return (
    <li>
      <button
        type="button"
        ref={(node) => {
          drop.setNodeRef(node)
          drag.setNodeRef(node)
        }}
        {...drag.listeners}
        {...drag.attributes}
        className={cn(
          "flex w-full cursor-grab flex-col gap-0.5 rounded-md px-2 py-1.5 text-left active:cursor-grabbing",
          selected ? "bg-accent font-medium" : "hover:bg-muted/70",
          over && "ring-2 ring-inset ring-primary/40",
          drag.isDragging && "opacity-40",
          commit.conflicted && "text-destructive",
        )}
        onClick={() =>
          onSelect({ kind: "commit", id: commit.cliId, subject })
        }
      >
        <span className="truncate text-xs">{subject}</span>
        {over ? (
          <span className="text-[10px] text-primary">{hint}</span>
        ) : null}
      </button>
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
  const drop = useDroppable({
    id: "drop:new-branch",
    data: { kind: "new-branch", id: "new-branch", label: "New branch" },
    disabled,
  })
  const hint = dropHint(activeDrag, {
    kind: "new-branch",
    id: "new-branch",
    label: "New branch",
  })
  const over = drop.isOver && hint

  return (
    <div
      ref={drop.setNodeRef}
      className={cn(
        "flex w-44 shrink-0 flex-col items-center justify-center gap-2 rounded-lg border border-dashed px-3 py-8 text-center text-muted-foreground",
        over && "border-primary bg-primary/10 text-primary",
      )}
    >
      <PlusIcon className="size-5" />
      <p className="text-xs font-medium">
        {over ? hint : "Drop for a new branch"}
      </p>
    </div>
  )
}
