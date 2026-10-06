import { useEffect, useMemo, useRef, type ReactNode } from "react"
import { toast } from "sonner"
import type { Workspace } from "@/api"
import { runPlan } from "@/actions/commits"
import { useActions } from "@/actions/context"
import { typingIn } from "@/actions/keys"
import { toastError } from "@/actions/toast"
import { liveKeys, stillThere } from "@/selection"
import {
  allTargets,
  describe,
  describeAll,
  plan,
  targetKey,
  type TargetEntity,
  type TargetState,
  type Verb,
} from "@/target"
import { TargetModeContext, verbInfo, type Deco, type TargetMode } from "./context"
import { TargetHintBar } from "./TargetHintBar"
import {
  cancelTarget,
  setTargetHover,
  switchVerb,
  targetState,
  updateTarget,
  useTargetState,
} from "./store"

const verbKeys: Record<string, Verb> = { c: "commit", r: "squash", m: "move", p: "pick" }

/** decorate resolves every item against the pending verb, as the TUI's deco does per row. */
function decorate(ws: Workspace, t: TargetState | null): Map<string, Deco> {
  const decos = new Map<string, Deco>()
  if (!t) return decos
  const sources = new Set(t.sources.map(targetKey))
  for (const { entity, group } of allTargets(ws)) {
    const key = targetKey(entity)
    const p = plan(ws, t, entity)
    const status = p ? "valid" : sources.has(key) ? "source" : "invalid"
    decos.set(key, { entity, status, plan: p, group })
  }
  return decos
}

function entityLabel(e: TargetEntity): string {
  switch (e.kind) {
    case "unstaged":
      return "Unstaged"
    case "new-branch":
      return "New branch"
    case "file":
    case "cfile":
      return e.path
    case "commit":
      return e.subject
    case "branch":
      return e.name
  }
}

/**
 * TargetModeProvider runs the TUI's target mode in the window: while c, r, m or p waits
 * for a target, valid targets in the lanes get a ring, clicking one (or picking it from
 * the list) completes the verb, and the hint bar shows what it would do. Items opt in with
 * `data-target-key` and useTargetDeco; clicks on them are taken before they select anything.
 */
export function TargetModeProvider({ workspace, children }: { workspace: Workspace; children: ReactNode }) {
  const state = useTargetState()
  const actions = useActions()
  const decos = useMemo(() => decorate(workspace, state), [workspace, state])

  // A refetch that drops a source (another client committed it) ends the mode.
  useEffect(() => {
    if (!state) return
    const live = liveKeys(workspace)
    if (state.sources.some((s) => !stillThere(workspace, live, s))) cancelTarget()
  }, [workspace, state])

  function confirm(entity: TargetEntity) {
    const t = targetState()
    if (!t) return
    const d = decos.get(targetKey(entity))
    if (!d?.plan) {
      if (d?.status === "source") {
        cancelTarget() // like the TUI: confirming on a source cancels
        return
      }
      toast.message(`Can't ${t.verb} ${describeAll(t.sources)} onto ${describe(entity)}`)
      return
    }
    const ctx = actions.contextFor()
    if (ctx.ops.busy) {
      toast.message("Wait for the running operation to finish")
      return
    }
    cancelTarget()
    void runPlan(ctx, d.plan).catch(toastError)
  }

  async function openList() {
    const t = targetState()
    if (!t) return
    const items = [...decos.values()]
      .filter((d) => d.plan)
      .map((d) => ({
        value: d.entity,
        label: entityLabel(d.entity),
        detail: d.plan!.label,
        group: d.group,
        keywords: d.plan!.desc,
      }))
    const picked = await actions.contextFor().dialogs.pick<TargetEntity>({
      title: `${verbInfo[t.verb].title} ${describeAll(t.sources)} onto`,
      placeholder: `${verbInfo[t.verb].title} ${describeAll(t.sources)} onto…`,
      items,
      empty: "No target fits.",
    })
    if (picked) latest.current.confirm(picked)
  }

  // Window listeners are registered once and read the latest render through this ref.
  const latest = useRef({ confirm, openList, decos, actions })
  useEffect(() => {
    latest.current = { confirm, openList, decos, actions }
  })

  useEffect(() => {
    const keyOf = (e: Event) =>
      e.target instanceof Element
        ? e.target.closest<HTMLElement>("[data-target-key]")?.dataset.targetKey ?? null
        : null

    // Capture phase: runs before React's handlers and the global action keys, which
    // skip events already handled (defaultPrevented).
    function onKeyDown(e: KeyboardEvent) {
      const t = targetState()
      if (!t || e.defaultPrevented || e.isComposing || typingIn(e.target)) return
      if (e.metaKey || e.ctrlKey || e.altKey) return // the palette and reload still work
      e.preventDefault()
      const notify = (why: string | null) => why && toast.message(why)
      switch (e.key) {
        case "Escape":
        case "q":
          return cancelTarget()
        case "Enter": {
          const s = latest.current.actions.contextFor().sel.selection
          if (s) latest.current.confirm(s)
          return
        }
        case "a":
          if (t.verb !== "squash") updateTarget({ side: t.side === "above" ? "below" : "above" })
          return
        case "u":
          if (t.verb === "squash") updateTarget({ useTarget: !t.useTarget })
          return
        case "e":
          if (t.verb === "commit") updateTarget({ emptyMsg: !t.emptyMsg })
          return
        case "b":
          if (t.verb === "commit" || t.verb === "pick") updateTarget({ newBranchHere: !t.newBranchHere })
          return
        case "/":
        case "t":
          return void latest.current.openList()
      }
      if (verbKeys[e.key]) notify(switchVerb(t, verbKeys[e.key]))
      // Every other key is swallowed: the TUI ignores action keys while picking a target.
    }

    function onClick(e: MouseEvent) {
      if (!targetState() || e.button !== 0) return
      const key = keyOf(e)
      const d = key ? latest.current.decos.get(key) : undefined
      if (!d) return
      e.preventDefault()
      e.stopPropagation()
      latest.current.confirm(d.entity)
    }

    function onPointerOver(e: PointerEvent) {
      if (targetState()) setTargetHover(keyOf(e))
    }

    window.addEventListener("keydown", onKeyDown, true)
    window.addEventListener("click", onClick, true)
    window.addEventListener("pointerover", onPointerOver)
    return () => {
      window.removeEventListener("keydown", onKeyDown, true)
      window.removeEventListener("click", onClick, true)
      window.removeEventListener("pointerover", onPointerOver)
    }
  }, [])

  const value: TargetMode = { decos, confirm, openList: () => void openList() }

  return (
    <TargetModeContext.Provider value={value}>
      {children}
      {state ? <TargetHintBar state={state} /> : null}
    </TargetModeContext.Provider>
  )
}
