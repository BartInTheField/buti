import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/components/ui/command"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Kbd } from "@/components/ui/kbd"
import { describeSubjects } from "@/selection"
import { formatKey } from "./keys"
import {
  actionGroups,
  allActions,
  availableActions,
  offers,
  type Action,
  type ActionContext,
  type PaletteMode,
} from "./registry"

/** Keys handled by the screen itself rather than an action (internal/ui navHelp). */
const navHelp: [string, string][] = [
  ["click", "select · cmd/ctrl-click: mark · right-click: actions"],
  ["drag", "drop onto a branch, commit, Unstaged or “new branch”"],
]

const titles: Record<PaletteMode, string> = {
  all: "Command palette",
  selection: "Actions",
  help: "Help · all commands",
}

type Props = {
  mode: PaletteMode | null
  onClose: () => void
  context: () => ActionContext
  run: (action: Action, ctx: ActionContext) => void
}

/**
 * CommandPalette lists actions from the registry: "all" (available, global included),
 * "selection" (the context menu's list, from the keyboard) and "help" (everything, with
 * the unavailable ones dimmed).
 */
export function CommandPalette({ mode, onClose, context, run }: Props) {
  const ctx = mode ? context() : null
  let actions: Action[] = []
  if (ctx && mode === "help") actions = allActions()
  else if (ctx) actions = availableActions(ctx, mode === "all")

  const title = mode ? titles[mode] : ""
  const heading =
    mode === "selection" && ctx ? `Actions · ${describeSubjects(ctx.subjects)}` : title

  return (
    <Dialog open={mode !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent
        className="top-1/3 translate-y-0 overflow-hidden p-0 sm:max-w-lg"
        showCloseButton={false}
      >
        <DialogHeader className="sr-only">
          <DialogTitle>{heading}</DialogTitle>
          <DialogDescription>Type to search, enter runs.</DialogDescription>
        </DialogHeader>
        {ctx ? (
          <Command>
            <CommandInput placeholder={`${heading} · type to search`} />
            <CommandList className="max-h-[min(28rem,60vh)]">
              <CommandEmpty>No matching commands.</CommandEmpty>
              {actionGroups.map((group) => {
                const inGroup = actions.filter((a) => a.group === group)
                if (inGroup.length === 0) return null
                return (
                  <CommandGroup key={group} heading={group}>
                    {inGroup.map((a) => (
                      <CommandItem
                        key={a.id}
                        value={`${a.group} ${a.title} ${a.id}`}
                        disabled={!offers(a, ctx)}
                        onSelect={() => {
                          onClose()
                          run(a, ctx)
                        }}
                      >
                        <span className="min-w-0 flex-1 truncate">{a.title}</span>
                        {a.keys?.[0] ? (
                          <CommandShortcut>
                            <Kbd>{formatKey(a.keys[0])}</Kbd>
                          </CommandShortcut>
                        ) : null}
                      </CommandItem>
                    ))}
                  </CommandGroup>
                )
              })}
              {mode === "help" ? (
                <CommandGroup heading="Navigate">
                  {navHelp.map(([key, what]) => (
                    <CommandItem key={key} value={`navigate ${key} ${what}`} disabled>
                      <span className="min-w-0 flex-1 truncate">{what}</span>
                      <CommandShortcut>
                        <Kbd>{key}</Kbd>
                      </CommandShortcut>
                    </CommandItem>
                  ))}
                </CommandGroup>
              ) : null}
            </CommandList>
          </Command>
        ) : null}
      </DialogContent>
    </Dialog>
  )
}
