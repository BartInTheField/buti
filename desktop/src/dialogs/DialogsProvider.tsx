import { useRef, useState, type ReactNode } from "react"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Button } from "@/components/ui/button"
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Textarea } from "@/components/ui/textarea"
import {
  DialogsContext,
  type ConfirmOptions,
  type Dialogs,
  type PickItem,
  type PickOptions,
  type PromptOptions,
} from "./dialogs"

type Request =
  | { kind: "confirm"; opts: ConfirmOptions; resolve: (ok: boolean) => void }
  | { kind: "prompt"; opts: PromptOptions; resolve: (value: string | null) => void }
  | { kind: "pick"; opts: PickOptions<unknown>; resolve: (value: unknown) => void }

/**
 * DialogsProvider renders the one shared confirm / prompt / pick dialog. Opening a
 * new one settles the previous request as cancelled.
 */
export function DialogsProvider({ children }: { children: ReactNode }) {
  const [req, setReq] = useState<Request | null>(null)
  const current = useRef<Request | null>(null)

  function open(next: Request) {
    cancel(current.current)
    current.current = next
    setReq(next)
  }

  function settle(r: Request, value: unknown) {
    if (current.current !== r) return
    current.current = null
    setReq(null)
    ;(r.resolve as (v: unknown) => void)(value)
  }

  function cancel(r: Request | null) {
    if (r) settle(r, r.kind === "confirm" ? false : null)
  }

  const dialogs: Dialogs = {
    confirm: (opts) => new Promise((resolve) => open({ kind: "confirm", opts, resolve })),
    prompt: (opts) => new Promise((resolve) => open({ kind: "prompt", opts, resolve })),
    pick: <T,>(opts: PickOptions<T>) =>
      new Promise<T | null>((resolve) =>
        open({
          kind: "pick",
          opts: opts as PickOptions<unknown>,
          resolve: resolve as (v: unknown) => void,
        }),
      ),
  }

  return (
    <DialogsContext.Provider value={dialogs}>
      {children}
      {req?.kind === "confirm" ? (
        <ConfirmDialog
          opts={req.opts}
          onDone={(ok) => settle(req, ok)}
        />
      ) : null}
      {req?.kind === "prompt" ? (
        <PromptDialog opts={req.opts} onDone={(v) => settle(req, v)} />
      ) : null}
      {req?.kind === "pick" ? (
        <PickDialog opts={req.opts} onDone={(v) => settle(req, v)} />
      ) : null}
    </DialogsContext.Provider>
  )
}

function ConfirmDialog({
  opts,
  onDone,
}: {
  opts: ConfirmOptions
  onDone: (ok: boolean) => void
}) {
  return (
    <AlertDialog open onOpenChange={(o) => !o && onDone(false)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>{opts.title}</AlertDialogTitle>
          {opts.body ? (
            <AlertDialogDescription className="whitespace-pre-line">
              {opts.body}
            </AlertDialogDescription>
          ) : null}
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel onClick={() => onDone(false)}>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant={opts.destructive ? "destructive" : "default"}
            onClick={() => onDone(true)}
          >
            {opts.confirmLabel ?? "Continue"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

function PromptDialog({
  opts,
  onDone,
}: {
  opts: PromptOptions
  onDone: (value: string | null) => void
}) {
  const [value, setValue] = useState(opts.initial ?? "")
  const error = opts.validate?.(value) ?? null

  function submit() {
    if (error) return
    onDone(value)
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onDone(null)}>
      <DialogContent className="sm:max-w-md">
        <form
          className="contents"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>{opts.title}</DialogTitle>
            {opts.description ? (
              <DialogDescription>{opts.description}</DialogDescription>
            ) : null}
          </DialogHeader>
          {opts.multiline ? (
            <Textarea
              autoFocus
              rows={5}
              placeholder={opts.placeholder}
              value={value}
              onChange={(e) => setValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  submit()
                }
              }}
            />
          ) : (
            <Input
              autoFocus
              placeholder={opts.placeholder}
              value={value}
              onChange={(e) => setValue(e.target.value)}
            />
          )}
          {/* Fixed height: a validation message must not move the buttons. */}
          <p className="h-4 text-xs text-muted-foreground">
            {error && value !== (opts.initial ?? "") ? error : opts.hint ?? ""}
          </p>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={() => onDone(null)}>
              Cancel
            </Button>
            <Button type="submit" disabled={Boolean(error)}>
              {opts.submitLabel ?? "OK"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

function PickDialog({
  opts,
  onDone,
}: {
  opts: PickOptions<unknown>
  onDone: (value: unknown) => void
}) {
  const groups = new Map<string, { item: PickItem<unknown>; index: number }[]>()
  opts.items.forEach((item, index) => {
    const g = item.group ?? ""
    groups.set(g, [...(groups.get(g) ?? []), { item, index }])
  })
  return (
    <Dialog open onOpenChange={(o) => !o && onDone(null)}>
      <DialogContent className="top-1/3 translate-y-0 overflow-hidden p-0 sm:max-w-lg" showCloseButton={false}>
        <DialogHeader className="sr-only">
          <DialogTitle>{opts.title}</DialogTitle>
          <DialogDescription>{opts.placeholder ?? opts.title}</DialogDescription>
        </DialogHeader>
        <Command>
          <CommandInput placeholder={opts.placeholder ?? opts.title} />
          <CommandList className="max-h-96">
            <CommandEmpty>{opts.empty ?? "Nothing matches."}</CommandEmpty>
            {[...groups].map(([group, entries]) => (
              <CommandGroup key={group} heading={group || opts.title}>
                {entries.map(({ item, index }) => (
                  <CommandItem
                    key={index}
                    value={`${index} ${item.label} ${item.keywords ?? ""}`}
                    disabled={item.disabled}
                    onSelect={() => onDone(item.value)}
                  >
                    <span className="min-w-0 flex-1 truncate">{item.label}</span>
                    {item.detail ? <CommandShortcut>{item.detail}</CommandShortcut> : null}
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  )
}
