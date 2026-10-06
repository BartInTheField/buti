import { useMemo, useState } from "react"
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  closestCorners,
  pointerWithin,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
} from "@dnd-kit/core"
import { toast } from "sonner"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable"
import { ApiRequestError, type Workspace } from "@/api"
import { resolveDrop, type DragItem, type DropTarget } from "@/dnd"
import { useDiff, useWorkspaceOps } from "@/queries"
import {
  selectionDiffId,
  type Selection,
} from "@/selection"
import { CommitDialog } from "./CommitDialog"
import { DiffPane } from "./DiffPane"
import { StackLanes } from "./StackLanes"
import { UnstagedPanel } from "./UnstagedPanel"

/** Prefer the droppable under the pointer; fall back to nearest corners. */
const workspaceCollision: CollisionDetection = (args) => {
  const pointed = pointerWithin(args)
  if (pointed.length > 0) {
    return pointed
  }
  return closestCorners(args)
}

type Props = {
  workspace: Workspace
  apiUrl: string
  apiToken: string
  onRefresh: () => void
  refreshing?: boolean
}

type PendingCommit = {
  changes: string[]
  placement: { branch?: string; newBranch?: boolean }
  label: string
}

export function WorkspaceView({
  workspace,
  apiUrl,
  apiToken,
  onRefresh,
  refreshing,
}: Props) {
  const [selection, setSelection] = useState<Selection | null>({ kind: "unstaged" })
  const [activeDrag, setActiveDrag] = useState<DragItem | null>(null)
  const [pendingCommit, setPendingCommit] = useState<PendingCommit | null>(null)
  const ops = useWorkspaceOps(apiUrl, apiToken)
  const diffId = selectionDiffId(selection)
  const diff = useDiff(apiUrl, apiToken, diffId)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
  )

  const repoName = useMemo(() => {
    return workspace.repo.split(/[/\\]/).filter(Boolean).pop() ?? workspace.repo
  }, [workspace.repo])

  async function runDrop(source: DragItem, target: DropTarget) {
    const action = resolveDrop(source, target)
    if (!action) {
      toast.message(`Cannot drop ${source.kind} onto ${target.kind}`)
      return
    }
    try {
      switch (action.type) {
        case "commit":
          setPendingCommit({
            changes: [source.id],
            placement: action.placement,
            label: action.placement.newBranch
              ? "a new branch"
              : action.placement.branch ?? "branch",
          })
          return
        case "amend":
          await ops.amend.mutateAsync({
            target: action.target,
            changes: [source.id],
          })
          toast.success(`Amended into commit`)
          break
        case "move":
          await ops.move.mutateAsync({
            sources: [source.id],
            placement: action.placement,
          })
          toast.success(
            action.placement.newBranch
              ? "Moved onto a new branch"
              : `Moved onto ${action.placement.branch}`,
          )
          break
        case "uncommit":
          await ops.uncommit.mutateAsync({ sources: [source.id] })
          toast.success("Uncommitted")
          break
      }
    } catch (err) {
      toastError(err)
    }
  }

  function onDragStart(event: DragStartEvent) {
    const data = event.active.data.current as DragItem | undefined
    if (data?.kind && data.id) {
      setActiveDrag(data)
    }
  }

  function onDragEnd(event: DragEndEvent) {
    const source = event.active.data.current as DragItem | undefined
    const overData = event.over?.data.current as DropTarget | undefined
    setActiveDrag(null)
    if (!source?.kind || !overData?.kind || ops.busy) {
      return
    }
    void runDrop(source, overData)
  }

  async function confirmCommit(message: string) {
    if (!pendingCommit) return
    try {
      await ops.commit.mutateAsync({
        changes: pendingCommit.changes,
        message,
        placement: pendingCommit.placement,
      })
      toast.success(`Committed to ${pendingCommit.label}`)
      setPendingCommit(null)
    } catch (err) {
      toastError(err)
    }
  }

  return (
    <div className="flex h-svh flex-col">
      <header className="flex items-center justify-between gap-3 border-b px-4 py-2">
        <div className="min-w-0">
          <p className="text-xs text-muted-foreground">buti desktop</p>
          <h1 className="truncate font-heading text-base font-medium tracking-tight">
            {repoName}
          </h1>
        </div>
        <div className="flex items-center gap-2">
          {workspace.upstreamState?.behind ? (
            <span className="text-xs text-muted-foreground">
              upstream +{workspace.upstreamState.behind}
            </span>
          ) : null}
          <Button
            variant="outline"
            size="sm"
            onClick={onRefresh}
            disabled={refreshing || ops.busy}
          >
            Refresh
          </Button>
        </div>
      </header>

      {workspace.resolving ? (
        <Alert className="mx-4 mt-3">
          <AlertTitle>Resolving a conflicted commit</AlertTitle>
          <AlertDescription>
            {(workspace.resolving.conflicted_files?.length ?? 0)} conflicted,{" "}
            {(workspace.resolving.resolved_files?.length ?? 0)} resolved.
          </AlertDescription>
        </Alert>
      ) : null}

      <DndContext
        sensors={sensors}
        collisionDetection={workspaceCollision}
        onDragStart={onDragStart}
        onDragCancel={() => setActiveDrag(null)}
        onDragEnd={onDragEnd}
      >
        <ResizablePanelGroup orientation="vertical" className="min-h-0 flex-1">
          <ResizablePanel defaultSize="58" minSize="30">
            <ResizablePanelGroup orientation="horizontal" className="h-full">
              <ResizablePanel defaultSize={240} minSize={180} maxSize={420}>
                <UnstagedPanel
                  changes={workspace.uncommittedChanges}
                  selection={selection}
                  onSelect={setSelection}
                  activeDrag={activeDrag}
                  disabled={ops.busy}
                />
              </ResizablePanel>
              <ResizableHandle withHandle />
              <ResizablePanel defaultSize="78" minSize="40">
                <StackLanes
                  stacks={workspace.stacks}
                  selection={selection}
                  onSelect={setSelection}
                  activeDrag={activeDrag}
                  disabled={ops.busy}
                />
              </ResizablePanel>
            </ResizablePanelGroup>
          </ResizablePanel>
          <ResizableHandle withHandle />
          <ResizablePanel defaultSize="42" minSize="20">
            <DiffPane
              selection={selection}
              diff={diff.data}
              loading={diff.isPending || diff.isFetching}
              error={
                diff.isError
                  ? diff.error instanceof Error
                    ? diff.error.message
                    : "Could not load diff"
                  : null
              }
            />
          </ResizablePanel>
        </ResizablePanelGroup>
        <DragOverlay dropAnimation={null}>
          {activeDrag ? (
            <div className="rounded-md border bg-card px-3 py-1.5 text-xs shadow-md">
              {activeDrag.label}
            </div>
          ) : null}
        </DragOverlay>
      </DndContext>

      <CommitDialog
        open={pendingCommit !== null}
        title="Commit"
        description={
          pendingCommit
            ? `Commit onto ${pendingCommit.label}. The change goes through but, not git.`
            : ""
        }
        pending={ops.commit.isPending}
        onOpenChange={(open) => {
          if (!open) setPendingCommit(null)
        }}
        onConfirm={(msg) => void confirmCommit(msg)}
      />
    </div>
  )
}

function toastError(err: unknown) {
  if (err instanceof ApiRequestError) {
    toast.error(err.message)
    return
  }
  toast.error(err instanceof Error ? err.message : "Operation failed")
}
