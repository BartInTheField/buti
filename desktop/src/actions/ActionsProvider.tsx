import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { toast } from "sonner"
import type { Workspace } from "@/api"
import { useDialogs } from "@/dialogs/dialogs"
import type { WorkspaceOps } from "@/queries"
import { useRepoSwitcher } from "@/repo"
import type { Selection, SelectionModel } from "@/selection"
import { ActionsContext, type Actions } from "./context"
import { eventKeys, typingIn } from "./keys"
import { toastError } from "./toast"
import { CommandPalette } from "./CommandPalette"
import {
  actionForKey,
  offers,
  type Action,
  type ActionContext,
  type PaletteMode,
} from "./registry"
import "./commits"
import "./builtin"
import "./conflicts"
import "./branches"
import "./review"
import "./details"

type Props = {
  workspace: Workspace
  sel: SelectionModel
  ops: WorkspaceOps
  children: ReactNode
}

/**
 * ActionsProvider owns the global keyboard handler and the command palette, and gives
 * the context menus a way to build an ActionContext. It reads the current state on every
 * event, so actions never see a stale selection.
 */
export function ActionsProvider({ workspace, sel, ops, children }: Props) {
  const dialogs = useDialogs()
  const repos = useRepoSwitcher()
  const queryClient = useQueryClient()
  const [palette, setPalette] = useState<PaletteMode | null>(null)

  function contextFor(subjects?: Selection[], marks?: Selection[]): ActionContext {
    const model = marks ? { ...sel, marks } : sel
    return {
      workspace,
      subjects: subjects ?? sel.subjects,
      sel: model,
      ops,
      dialogs,
      queryClient,
      ui: { openPalette: (mode = "all") => setPalette(mode), chooseRepo: repos.choose },
      runOp: async (title, fn) => {
        try {
          const res = await fn()
          toast.success(title, res?.output ? { description: res.output } : undefined)
          return true
        } catch (err) {
          toastError(err)
          return false
        }
      },
      notify: (message) => toast.message(message),
    }
  }

  function run(action: Action, ctx: ActionContext) {
    if (!offers(action, ctx)) {
      toast.message(`${action.title} is not available here`)
      return
    }
    if (ops.busy && !action.global) {
      toast.message("Wait for the running operation to finish")
      return
    }
    // Deferred so a closing menu or palette hands focus back before a dialog opens.
    setTimeout(() => {
      void Promise.resolve(action.run(ctx)).catch(toastError)
    }, 0)
  }

  // The window listener is registered once and reads the latest render's state through this ref.
  // A layout effect, so a key pressed right after a click already sees what the click selected.
  const latest = useRef({ contextFor, run, paletteOpen: false })
  useLayoutEffect(() => {
    latest.current = { contextFor, run, paletteOpen: palette !== null }
  })

  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const { contextFor, run, paletteOpen } = latest.current
      if (e.defaultPrevented || e.isComposing || paletteOpen || typingIn(e.target)) return
      const ctx = contextFor()
      for (const key of eventKeys(e)) {
        const action = actionForKey(key, ctx)
        if (action) {
          e.preventDefault()
          run(action, ctx)
          return
        }
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])

  const value: Actions = { contextFor, run, openPalette: setPalette }

  return (
    <ActionsContext.Provider value={value}>
      {children}
      <CommandPalette
        mode={palette}
        onClose={() => setPalette(null)}
        context={contextFor}
        run={run}
      />
    </ActionsContext.Provider>
  )
}
