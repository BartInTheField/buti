import type { MouseEvent } from "react"
import { useDraggable, useDroppable } from "@dnd-kit/core"
import { GitBranchIcon, PlusIcon } from "lucide-react"
import { Frame, FrameHeader, FramePanel } from "@/components/reui/frame"
import { IconTile } from "@/components/reui/icon-tile"
import { Timeline, TimelineIndicator, TimelineItem, TimelineSeparator } from "@/components/reui/timeline"
import { ScrollArea } from "@/components/ui/scroll-area"
import { cn } from "@/lib/utils"
import { commitSubject, type Branch, type Commit, type Stack } from "@/api"
import { ActionContextMenu } from "@/actions/ActionContextMenu"
import { useActions } from "@/actions/context"
import { allActions } from "@/actions/registry"
import { BranchBadges } from "./branches/BranchBadges"
import { BranchMenu } from "./branches/BranchMenu"
import { dropHint, type DragItem, type DropTarget } from "@/dnd"
import { sameSelection, type Selection, type SelectionModel } from "@/selection"
import { CommittedFiles } from "./details/CommittedFiles"
import { DropHint } from "./DropHint"
import { ConflictBadge } from "./conflicts/ConflictBadge"
import { targetClass, useTargetDeco } from "./target/context"
import { TargetTag } from "./target/TargetTag"

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
              {/* Top of the stack first, as `but status` lists it and the TUI shows it. */}
              {stack.branches.map((branch) => (
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
  const deco = useTargetDeco(`branch:${branch.cliId}`)

  return (
    <Frame
      ref={dropRef}
      spacing="xs"
      data-testid="branch-card"
      data-branch={branch.name}
      data-target-key={`branch:${branch.cliId}`}
      className={cn(
        // Hover feedback is ring + background only: no border-width change, no inserted rows.
        "gap-0.5 p-0.5 transition-[box-shadow,background-color] [--frame-radius:var(--radius-lg)]",
        hint && "bg-primary/10 ring-2 ring-primary/60",
        selected && !hint && "ring-1 ring-primary/50",
        targetClass(deco),
      )}
    >
      <DropHint text={hint} className="-top-2.5 right-2" />
      <TargetTag deco={deco} className="-top-2.5 right-2" />
      <FrameHeader className={cn("flex-row items-center rounded-md p-0 pr-1", marked && "bg-primary/15")}>
        <ActionContextMenu item={item}>
          <button
            type="button"
            ref={dragRef}
            className={cn(
              "flex h-8 min-w-0 flex-1 cursor-grab items-center gap-2 rounded-md pr-1 pl-2 text-left active:cursor-grabbing",
              isDragging && "opacity-40",
            )}
            {...listeners}
            {...attributes}
            onClick={(e) => onItemClick(item, e)}
          >
            <GitBranchIcon className="size-3.5 shrink-0 text-muted-foreground" />
            <span className="min-w-0 flex-1 truncate text-sm font-medium">{branch.name}</span>
            <BranchBadges branch={branch} />
          </button>
        </ActionContextMenu>
        <BranchMenu branch={branch} />
      </FrameHeader>
      <FramePanel className="p-1">
        {branch.commits.length === 0 ? (
          <p className="px-2 py-1.5 text-xs text-muted-foreground">No commits</p>
        ) : (
          // A timeline value of 0 completes no step: the dots are styled per commit instead.
          <Timeline value={0} role="list">
            {branch.commits.map((c) => (
              <CommitRow
                key={c.cliId}
                commit={c}
                branch={branch.name}
                sel={sel}
                onItemClick={onItemClick}
                activeDrag={activeDrag}
                disabled={disabled}
              />
            ))}
          </Timeline>
        )}
      </FramePanel>
    </Frame>
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
  const deco = useTargetDeco(`commit:${commit.cliId}`)

  return (
    <TimelineItem
      step={1}
      role="listitem"
      className="flex-none gap-0 group-data-[orientation=vertical]/timeline:ms-4 group-data-[orientation=vertical]/timeline:not-last:pb-0"
    >
      {/* The rail sits in the gutter left of the row: 10px dots centred on the 28px row. */}
      <TimelineSeparator className="bg-border group-data-[orientation=vertical]/timeline:-left-2 group-data-[orientation=vertical]/timeline:h-[calc(100%-14px)] group-data-[orientation=vertical]/timeline:translate-y-[21px]" />
      <TimelineIndicator
        className={cn(
          "size-2.5 border-2 bg-card group-data-[orientation=vertical]/timeline:top-[9px] group-data-[orientation=vertical]/timeline:-left-2",
          commit.conflicted
            ? "border-destructive bg-destructive"
            : selected
              ? "border-primary bg-primary"
              : "border-primary/60",
        )}
      />
      <ActionContextMenu item={item}>
        <button
          type="button"
          data-testid="commit-row"
          data-target-key={`commit:${commit.cliId}`}
          ref={(node) => {
            dropRef(node)
            dragRef(node)
          }}
          {...listeners}
          {...attributes}
          className={cn(
            "relative flex h-7 w-full cursor-grab items-center gap-2 rounded-md px-2 text-left active:cursor-grabbing",
            selected ? "bg-accent font-medium" : "hover:bg-muted/70",
            marked && "bg-primary/15",
            hint && "bg-primary/10 ring-2 ring-inset ring-primary/60",
            isDragging && "opacity-40",
            commit.conflicted && "text-destructive",
            targetClass(deco, { inset: true }),
          )}
          onClick={(e) => onItemClick(item, e)}
        >
          <span className="min-w-0 flex-1 truncate text-xs">{subject}</span>
          {commit.conflicted ? <ConflictBadge /> : null}
          {/* In the row, not over it: the subject ellipsizes before the hint. */}
          <DropHint text={hint} inline />
          <TargetTag deco={deco} inline />
        </button>
      </ActionContextMenu>
      <CommittedFiles commit={commit} branch={branch} sel={sel} onItemClick={onItemClick} disabled={disabled} />
    </TimelineItem>
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
  const actions = useActions()
  // A click makes a new lane too, so the lane is reachable without a drag (b in the TUI).
  function newLane() {
    const a = allActions().find((a) => a.id === "branch.new-lane")
    if (a) actions.run(a, actions.contextFor())
  }
  const deco = useTargetDeco("new-branch")

  return (
    <div
      ref={dropRef}
      data-testid="new-branch-lane"
      role="button"
      tabIndex={0}
      title="New branch in a new lane"
      onClick={newLane}
      onKeyDown={(e) => e.key === "Enter" && newLane()}
      data-target-key="new-branch"
      className={cn(
        "group/lane flex w-44 shrink-0 cursor-pointer flex-col items-center justify-center gap-2.5 rounded-lg border border-dashed bg-muted/30 px-3 py-7 text-center text-muted-foreground transition-colors hover:border-primary/40 hover:bg-primary/5",
        hint && "border-primary bg-primary/10 text-primary",
        targetClass(deco),
        deco?.hovered && deco.plan && "text-primary",
      )}
    >
      <IconTile variant="soft" size="sm">
        <PlusIcon />
      </IconTile>
      {/* Two fixed lines, whatever the text, so the lane never resizes. */}
      <p className="line-clamp-2 h-8 text-xs leading-4 font-medium">
        {hint ?? deco?.plan?.label ?? "Drop or click for a new branch"}
      </p>
    </div>
  )
}
