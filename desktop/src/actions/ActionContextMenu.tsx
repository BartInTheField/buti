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
import { describeSubjects, type Selection } from "@/selection"
import { useActions } from "./context"
import { formatKey } from "./keys"
import { actionGroups, availableActions } from "./registry"

/**
 * ActionContextMenu is the right-click menu for a file, commit or branch, built from
 * the action registry. On a marked item it acts on all marks; otherwise it selects the
 * item and acts on it alone, as right-click does in the TUI.
 */
export function ActionContextMenu({ item, children }: { item: Selection; children: ReactNode }) {
  const actions = useActions()
  return (
    <ContextMenu
      onOpenChange={(open) => {
        const { sel } = actions.contextFor()
        if (open && !sel.isMarked(item)) {
          sel.clearMarks()
          sel.select(item)
        }
      }}
    >
      <ContextMenuTrigger asChild>{children}</ContextMenuTrigger>
      <ContextMenuContent className="min-w-56">
        <MenuItems item={item} />
      </ContextMenuContent>
    </ContextMenu>
  )
}

function MenuItems({ item }: { item: Selection }) {
  const actions = useActions()
  const { sel } = actions.contextFor()
  const marked = sel.isMarked(item)
  const ctx = marked
    ? actions.contextFor(sel.marks, sel.marks)
    : actions.contextFor([item], [])
  const available = availableActions(ctx, false)
  const groups = actionGroups
    .map((g) => available.filter((a) => a.group === g))
    .filter((as) => as.length > 0)

  return (
    <>
      <ContextMenuLabel className="truncate">{describeSubjects(ctx.subjects)}</ContextMenuLabel>
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
