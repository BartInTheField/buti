import { useRef, useState, type MouseEvent } from "react"
import {
  DndContext,
  DragOverlay,
  MeasuringStrategy,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  type Modifier,
} from "@dnd-kit/core"
import { getEventCoordinates } from "@dnd-kit/utilities"
import { CommandIcon, RefreshCwIcon } from "lucide-react"
import { toast } from "sonner"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { Kbd } from "@/components/ui/kbd"
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable"
import type { Workspace } from "@/api"
import { ActionsProvider } from "@/actions/ActionsProvider"
import { toastError } from "@/actions/toast"
import { runPlan } from "@/actions/commits"
import { useActions } from "@/actions/context"
import { formatKey } from "@/actions/keys"
import { ids } from "@/actions/registry"
import {
  beginDrag,
  endDrag as endDragSession,
  innermostAcceptingDroppable,
  resolveDrop,
  type DragItem,
  type DropTarget,
} from "@/dnd"
import { entityFor } from "@/target"
import { holdRefetch, useDiff, useRefreshing, useWorkspaceOps, type WorkspaceOps } from "@/queries"
import {
  describeSubjects,
  selectionDiffId,
  useSelectionModel,
  type Selection,
  type SelectionModel,
} from "@/selection"
import { EditModePanel } from "./conflicts/EditModePanel"
import { BranchesLayer, HistoryButtons, UpstreamButton } from "./branches/BranchesLayer"
import { DetailsLayout } from "./details/DetailsLayout"
import { BrandLogo } from "./BrandMark"
import { DiffPane } from "./DiffPane"
import { RepoMenu } from "./RepoMenu"
import { cancelTarget } from "./target/store"
import { TargetModeProvider } from "./target/TargetModeProvider"
import { StackLanes } from "./StackLanes"
import { UnstagedPanel } from "./UnstagedPanel"

/** besideCursor draws the drag overlay just below-right of the pointer, clear of drop hints. */
const besideCursor: Modifier = ({ activatorEvent, draggingNodeRect, transform }) => {
  const p = activatorEvent && getEventCoordinates(activatorEvent)
  if (!p || !draggingNodeRect) return transform
  return {
    ...transform,
    x: transform.x + p.x - draggingNodeRect.left + 14,
    y: transform.y + p.y - draggingNodeRect.top + 14,
  }
}

type Props = {
  workspace: Workspace
  apiUrl: string
  apiToken: string
  /** A background refetch failed; the last good workspace stays on screen. */
  error?: string | null
}

export function WorkspaceView({ workspace, apiUrl, apiToken, error }: Props) {
  const sel = useSelectionModel(workspace)
  const ops = useWorkspaceOps(apiUrl, apiToken)
  return (
    <ActionsProvider workspace={workspace} sel={sel} ops={ops}>
      <TargetModeProvider workspace={workspace}>
        <WorkspaceScreen workspace={workspace} sel={sel} ops={ops} error={error} />
      </TargetModeProvider>
      <BranchesLayer />
    </ActionsProvider>
  )
}

function WorkspaceScreen({
  workspace,
  sel,
  ops,
  error,
}: {
  workspace: Workspace
  sel: SelectionModel
  ops: WorkspaceOps
  error?: string | null
}) {
  const actions = useActions()
  const [activeDrag, setActiveDrag] = useState<DragItem | null>(null)
  const releaseRefetch = useRef<(() => void) | null>(null)
  const refreshing = useRefreshing(ops.cfg.url)
  const diff = useDiff(ops.cfg.url, ops.cfg.token, selectionDiffId(sel.selection))

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 6 } }))

  function onItemClick(item: Selection, e: MouseEvent) {
    if (e.metaKey || e.ctrlKey) {
      const err = sel.toggleMark(item)
      if (err) toast.message(err)
      return
    }
    sel.clearMarks()
    sel.select(item)
  }

  /** dragSources: dragging a marked item drags all marks, as acting on marks does in the TUI. */
  function dragSources(source: DragItem): string[] {
    const marked = sel.marks.some((m) => m.kind !== "unstaged" && m.kind === source.kind && m.id === source.id)
    return marked ? ids(sel.marks) : [source.id]
  }

  /** dragSelections: the marks when the dragged item is marked, else the item itself. */
  function dragSelections(source: DragItem): Selection[] {
    const marked = sel.marks.some((m) => m.kind !== "unstaged" && m.kind === source.kind && m.id === source.id)
    if (marked) return sel.marks
    const e = entityFor(workspace, source.kind, source.id)
    return e && e.kind !== "new-branch" ? [e] : []
  }

  function endDrag() {
    setActiveDrag(null)
    endDragSession()
    releaseRefetch.current?.()
    releaseRefetch.current = null
  }

  function onDragStart(event: DragStartEvent) {
    const data = event.active.data.current as DragItem | undefined
    if (!data?.kind || !data.id) return
    releaseRefetch.current?.()
    releaseRefetch.current = holdRefetch()
    cancelTarget()
    beginDrag(workspace, dragSelections(data))
    setActiveDrag(data)
  }

  function onDragEnd(event: DragEndEvent) {
    const source = event.active.data.current as DragItem | undefined
    const target = event.over?.data.current as DropTarget | undefined
    // Resolved before endDrag ends the session: a drop runs the same plan as the target picker.
    const plan = source?.kind && target?.kind ? resolveDrop(source, target) : null
    endDrag()
    if (!plan) return
    if (ops.busy) {
      toast.message("Wait for the running operation to finish")
      return
    }
    void runPlan(actions.contextFor(), plan).catch(toastError)
  }

  const marked = sel.marks.length

  return (
    <div className="flex h-svh flex-col">
      <header className="flex items-center justify-between gap-3 border-b px-4 py-2">
        <div className="flex min-w-0 items-center gap-3">
          <BrandLogo />
          <span className="h-4 w-px shrink-0 bg-border" aria-hidden="true" />
          <h1 className="flex min-w-0">
            <RepoMenu dir={workspace.repo} />
          </h1>
        </div>
        <div className="flex items-center gap-2">
          {marked > 0 ? (
            <span className="text-xs text-muted-foreground" data-testid="marks">
              {describeSubjects(sel.marks)} marked
            </span>
          ) : null}
          {workspace.upstreamState?.behind ? (
            <UpstreamButton behind={workspace.upstreamState.behind} />
          ) : null}
          <HistoryButtons />
          <Button variant="ghost" size="sm" onClick={() => actions.openPalette("all")}>
            <CommandIcon />
            Commands
            <Kbd>{formatKey("mod+k")}</Kbd>
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void ops.refresh({ sync: true }).catch(toastError)}
            disabled={refreshing}
          >
            <RefreshCwIcon className={refreshing ? "animate-spin" : undefined} />
            Refresh
          </Button>
        </div>
      </header>

      {error ? (
        <Alert variant="destructive" className="mx-4 mt-3 w-auto">
          <AlertTitle>Could not refresh the workspace</AlertTitle>
          <AlertDescription>{error}</AlertDescription>
        </Alert>
      ) : null}

      {workspace.resolving ? (
        <EditModePanel workspace={workspace} cfg={ops.cfg} />
      ) : (
      <DndContext
        sensors={sensors}
        collisionDetection={innermostAcceptingDroppable}
        // Measure droppables once per drag; hover feedback never changes layout, so the
        // rects stay valid and `over` can't oscillate.
        measuring={{ droppable: { strategy: MeasuringStrategy.BeforeDragging } }}
        onDragStart={onDragStart}
        onDragCancel={endDrag}
        onDragEnd={onDragEnd}
      >
        <DetailsLayout
          top={
            <ResizablePanelGroup orientation="horizontal" className="h-full">
              <ResizablePanel defaultSize={240} minSize={180} maxSize={420}>
                <UnstagedPanel
                  changes={workspace.uncommittedChanges}
                  sel={sel}
                  onItemClick={onItemClick}
                  activeDrag={activeDrag}
                  disabled={ops.busy}
                />
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize="78" minSize="40">
                <StackLanes
                  stacks={workspace.stacks}
                  sel={sel}
                  onItemClick={onItemClick}
                  activeDrag={activeDrag}
                  disabled={ops.busy}
                />
              </ResizablePanel>
            </ResizablePanelGroup>
          }
          details={
            <DiffPane
              selection={sel.selection}
              diff={diff.data}
              loading={diff.isPending && diff.fetchStatus !== "idle"}
              stale={diff.isPlaceholderData}
              error={diff.isError ? (diff.error instanceof Error ? diff.error.message : "Could not load diff") : null}
            />
          }
        />
        <DragOverlay dropAnimation={null} modifiers={[besideCursor]}>
          {activeDrag ? (
            <div className="w-fit max-w-64 truncate rounded-md border bg-card px-3 py-1.5 text-xs shadow-md">
              {dragSources(activeDrag).length > 1
                ? `${dragSources(activeDrag).length} ${activeDrag.kind === "cfile" ? "file" : activeDrag.kind}s`
                : activeDrag.label}
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>
      )}
    </div>
  )
}
