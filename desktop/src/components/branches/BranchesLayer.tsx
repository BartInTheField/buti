import type { ReactNode } from "react"
import { ArrowDownToLineIcon, HistoryIcon, Redo2Icon, Undo2Icon } from "lucide-react"
import { useActions } from "@/actions/context"
import { formatKey } from "@/actions/keys"
import { allActions } from "@/actions/registry"
import { Button } from "@/components/ui/button"
import { Spinner } from "@/components/ui/spinner"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { OplogSheet } from "./OplogSheet"
import { PullRequestDialog } from "./PullRequestDialog"
import { setOplogOpen, settlePullRequest, useBranchesLayer } from "./store"

/** BranchesLayer renders the operation history sheet and the pull request dialog when open. */
export function BranchesLayer() {
  const { oplog, pr } = useBranchesLayer()
  return (
    <>
      <OplogSheet open={oplog} onOpenChange={setOplogOpen} />
      {pr ? <PullRequestDialog key={pr.branch} branch={pr.branch} onDone={settlePullRequest} /> : null}
    </>
  )
}

/** useRunAction runs a registered action by id against the current selection. */
function useRunAction() {
  const actions = useActions()
  return (id: string) => {
    const a = allActions().find((a) => a.id === id)
    if (a) actions.run(a, actions.contextFor())
  }
}

function ToolButton({ id, label, children }: { id: string; label: string; children: ReactNode }) {
  const run = useRunAction()
  const key = allActions().find((a) => a.id === id)?.keys?.[0]
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label={label} onClick={() => run(id)}>
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {key ? ` · ${formatKey(key)}` : ""}
      </TooltipContent>
    </Tooltip>
  )
}

/** HistoryButtons are undo, redo and the operation history, always in the header. */
export function HistoryButtons() {
  return (
    <div className="flex items-center">
      <ToolButton id="history.undo" label="Undo">
        <Undo2Icon />
      </ToolButton>
      <ToolButton id="history.redo" label="Redo">
        <Redo2Icon />
      </ToolButton>
      <ToolButton id="history.oplog" label="Operation history">
        <HistoryIcon />
      </ToolButton>
    </div>
  )
}

/** UpstreamButton shows how far the target is ahead and pulls it on click (L); it spins while pulling. */
export function UpstreamButton({ behind, pulling }: { behind: number; pulling?: boolean }) {
  const run = useRunAction()
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="text-xs text-muted-foreground"
          // aria-disabled, not disabled: a disabled button gets no hover, so the tooltip would vanish.
          aria-disabled={pulling}
          aria-busy={pulling}
          data-testid="upstream-button"
          onClick={() => !pulling && run("branch.pull")}
        >
          {pulling ? <Spinner aria-hidden="true" /> : <ArrowDownToLineIcon />}
          {pulling ? "Pulling…" : `upstream +${behind}`}
        </Button>
      </TooltipTrigger>
      <TooltipContent>Pull (update from upstream) · {formatKey("L")}</TooltipContent>
    </Tooltip>
  )
}
