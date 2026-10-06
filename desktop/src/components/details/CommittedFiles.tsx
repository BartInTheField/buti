import type { MouseEvent } from "react"
import { useDraggable } from "@dnd-kit/core"
import { FileCodeIcon } from "lucide-react"
import { cn } from "@/lib/utils"
import type { Change, Commit } from "@/api"
import { ActionContextMenu } from "@/actions/ActionContextMenu"
import { commitKey } from "@/actions/details"
import type { DragItem } from "@/dnd"
import { sameSelection, type Selection, type SelectionModel } from "@/selection"
import { PathLabel, StatusBadge } from "./StatusBadge"
import { useFilesShown } from "./store"

type Props = {
  commit: Commit
  branch: string
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  disabled?: boolean
}

/** CommittedFiles lists a commit's files under it once f (or F for all) shows them. */
export function CommittedFiles({ commit, branch, sel, onItemClick, disabled }: Props) {
  const shown = useFilesShown(commitKey(commit))
  if (!shown || !commit.changes?.length) return null
  return (
    <ul className="ml-3.5 flex flex-col gap-px border-l pt-0.5 pb-1 pl-1.5" data-testid="commit-files">
      {commit.changes.map((ch) => (
        <CommittedFileRow
          key={ch.cliId}
          change={ch}
          item={{ kind: "cfile", id: ch.cliId, path: ch.filePath, commit: commit.cliId, branch }}
          sel={sel}
          onItemClick={onItemClick}
          disabled={disabled}
        />
      ))}
    </ul>
  )
}

/**
 * CommittedFileRow drags like the TUI's committed file: onto Unstaged to uncommit it,
 * onto another commit or a branch to move it there (target.go dropVerb).
 */
function CommittedFileRow({
  change,
  item,
  sel,
  onItemClick,
  disabled,
}: {
  change: Change
  item: Extract<Selection, { kind: "cfile" }>
  sel: SelectionModel
  onItemClick: (item: Selection, e: MouseEvent) => void
  disabled?: boolean
}) {
  const { setNodeRef, listeners, attributes, isDragging } = useDraggable({
    id: `cfile:${item.id}`,
    data: { kind: "cfile", id: item.id, label: item.path } satisfies DragItem,
    disabled,
  })
  return (
    <li>
      <ActionContextMenu item={item}>
        <button
          type="button"
          data-testid="commit-file"
          ref={setNodeRef}
          {...listeners}
          {...attributes}
          className={cn(
            "flex h-6 w-full cursor-grab items-center gap-1.5 rounded-md px-1.5 text-left text-xs transition-colors active:cursor-grabbing",
            sameSelection(sel.selection, item)
              ? "bg-primary/10 font-medium ring-1 ring-primary/40 ring-inset"
              : "hover:bg-muted/70",
            sel.isMarked(item) && "bg-primary/15",
            isDragging && "opacity-40",
          )}
          onClick={(e) => onItemClick(item, e)}
        >
          <FileCodeIcon className="size-3.5 shrink-0 text-muted-foreground" />
          <PathLabel path={change.filePath} className="flex-1" />
          <StatusBadge status={change.changeType} />
        </button>
      </ActionContextMenu>
    </li>
  )
}
