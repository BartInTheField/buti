import { createContext, useContext } from "react"

export type ConfirmOptions = {
  title: string
  body?: string
  confirmLabel?: string
  /** Styles the confirm button as destructive (discard, force push). */
  destructive?: boolean
}

export type PromptOptions = {
  title: string
  description?: string
  initial?: string
  placeholder?: string
  /** A line under the field, e.g. "Spaces become dashes." */
  hint?: string
  /** A textarea instead of an input; cmd/ctrl+enter submits. */
  multiline?: boolean
  submitLabel?: string
  /** Returns why value can't be submitted, or null when it can. */
  validate?: (value: string) => string | null
}

export type PickItem<T> = {
  value: T
  label: string
  /** Right-aligned secondary text: a key, an id, a time. */
  detail?: string
  group?: string
  /** Extra words the filter matches on. */
  keywords?: string
  disabled?: boolean
}

export type PickOptions<T> = {
  title: string
  placeholder?: string
  items: PickItem<T>[]
  /** Shown when nothing matches the filter. */
  empty?: string
}

/**
 * Dialogs are promise-based so actions read top to bottom:
 *
 *   if (!(await dialogs.confirm({ title: "Discard?", destructive: true }))) return
 *   const name = await dialogs.prompt({ title: "New branch" }) // null when cancelled
 *   const target = await dialogs.pick({ title: "Move onto", items })
 */
export type Dialogs = {
  confirm: (opts: ConfirmOptions) => Promise<boolean>
  prompt: (opts: PromptOptions) => Promise<string | null>
  pick: <T>(opts: PickOptions<T>) => Promise<T | null>
}

export const DialogsContext = createContext<Dialogs | null>(null)

export function useDialogs(): Dialogs {
  const d = useContext(DialogsContext)
  if (!d) throw new Error("useDialogs outside DialogsProvider")
  return d
}
