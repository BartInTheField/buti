import { EllipsisIcon } from "lucide-react"
import { useActions } from "@/actions/context"
import { formatKey } from "@/actions/keys"
import { actionGroups, availableActions } from "@/actions/registry"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import type { Branch } from "@/api"
import type { Selection } from "@/selection"

/**
 * BranchMenu is the ⋯ button on a branch card: the same actions as right-clicking the
 * branch, Branch group first. Opening it selects the branch, as right-click does.
 */
export function BranchMenu({ branch }: { branch: Branch }) {
  const actions = useActions()
  const item: Selection = { kind: "branch", id: branch.cliId, name: branch.name }

  return (
    <DropdownMenu
      onOpenChange={(o) => {
        if (!o) return
        const { sel } = actions.contextFor()
        if (!sel.isMarked(item)) {
          sel.clearMarks()
          sel.select(item)
        }
      }}
    >
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon-xs"
          aria-label={`Actions for ${branch.name}`}
          data-testid="branch-menu"
          className="shrink-0 text-muted-foreground"
        >
          <EllipsisIcon />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-56">
        <Items item={item} />
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function Items({ item }: { item: Selection }) {
  const actions = useActions()
  const { sel } = actions.contextFor()
  const ctx = sel.isMarked(item) ? actions.contextFor(sel.marks, sel.marks) : actions.contextFor([item], [])
  const available = availableActions(ctx, false)
  const order = ["Branch", ...actionGroups.filter((g) => g !== "Branch")]
  const groups = order.map((g) => available.filter((a) => a.group === g)).filter((as) => as.length > 0)

  return (
    <>
      <DropdownMenuLabel className="truncate">{item.kind === "branch" ? item.name : ""}</DropdownMenuLabel>
      {groups.map((as) => (
        <DropdownMenuGroup key={as[0].group}>
          <DropdownMenuSeparator />
          {as.map((a) => (
            <DropdownMenuItem key={a.id} onSelect={() => actions.run(a, ctx)}>
              {a.title}
              {a.keys?.[0] ? <DropdownMenuShortcut>{formatKey(a.keys[0])}</DropdownMenuShortcut> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuGroup>
      ))}
    </>
  )
}
