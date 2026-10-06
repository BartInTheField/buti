import type { ReactNode } from "react"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuGroup,
  ContextMenuItem,
  ContextMenuLabel,
  ContextMenuSeparator,
  ContextMenuShortcut,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import { useActions } from "@/actions/context"
import { formatKey } from "@/actions/keys"
import { actionGroups, availableActions } from "@/actions/registry"
import { describeSubjects, type Selection } from "@/selection"

/**
 * DiffContextMenu is the right-click menu of the diff: the actions for the hunk under the
 * line cursor (or what the pane shows, for a commit's diff), comments included. Unlike
 * ActionContextMenu it never changes the selection, which would replace the diff.
 */
export function DiffContextMenu({ subject, children }: { subject: () => Selection | null; children: ReactNode }) {
  return (
    <ContextMenu>
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="min-w-56" data-testid="diff-menu">
        <Items subject={subject} />
      </ContextMenuContent>
    </ContextMenu>
  )
}

function Items({ subject }: { subject: () => Selection | null }) {
  const actions = useActions()
  const { sel } = actions.contextFor()
  const s = subject()
  const marked = s !== null && sel.isMarked(s)
  const ctx = marked ? actions.contextFor(sel.marks, sel.marks) : actions.contextFor(s ? [s] : [], [])
  // On a hunk, "Mark / unmark" would mark the selected file rather than the hunk.
  const available = availableActions(ctx, false).filter((a) => !(a.id === "mark" && s?.kind === "file" && s.hunk))
  const groups = actionGroups
    .map((g) => available.filter((a) => a.group === g))
    .filter((as) => as.length > 0)
  return (
    <>
      <ContextMenuLabel className="max-w-80 truncate">{describeSubjects(ctx.subjects)}</ContextMenuLabel>
      {groups.length === 0 ? (
        <ContextMenuItem disabled>No actions</ContextMenuItem>
      ) : (
        groups.map((as) => (
          <ContextMenuGroup key={as[0].group}>
            <ContextMenuSeparator />
            {as.map((a) => (
              <ContextMenuItem key={a.id} onSelect={() => actions.run(a, ctx)}>
                {a.title}
                {a.keys?.[0] ? <ContextMenuShortcut>{formatKey(a.keys[0])}</ContextMenuShortcut> : null}
              </ContextMenuItem>
            ))}
          </ContextMenuGroup>
        ))
      )}
    </>
  )
}
