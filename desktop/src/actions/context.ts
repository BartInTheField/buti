import { createContext, useContext } from "react"
import type { Selection } from "@/selection"
import type { Action, ActionContext, PaletteMode } from "./registry"

export type Actions = {
  /**
   * contextFor builds an ActionContext. Subjects default to the selection model's;
   * a context menu passes the item it opened on (and the marks that apply to it).
   */
  contextFor: (subjects?: Selection[], marks?: Selection[]) => ActionContext
  /** run checks availability and the busy flag, then runs the action. */
  run: (action: Action, ctx: ActionContext) => void
  openPalette: (mode: PaletteMode) => void
}

export const ActionsContext = createContext<Actions | null>(null)

export function useActions(): Actions {
  const a = useContext(ActionsContext)
  if (!a) throw new Error("useActions outside ActionsProvider")
  return a
}
