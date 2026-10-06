import { useSyncExternalStore } from "react"
import type { Selection } from "@/selection"
import { newTargetState, sourcesFor, type TargetState, type Verb } from "@/target"

// The pending verb lives outside React so actions (which only get an ActionContext)
// can start it, and the provider, the hint bar and the drag handlers can all read it.

let state: TargetState | null = null
let hover: string | null = null
const listeners = new Set<() => void>()

function emit() {
  listeners.forEach((l) => l())
}

function subscribe(l: () => void) {
  listeners.add(l)
  return () => listeners.delete(l)
}

/** startTarget enters target mode for verb, or returns why the subjects can't be its sources. */
export function startTarget(verb: Verb, subjects: Selection[]): string | null {
  const s = sourcesFor(verb, subjects)
  if ("why" in s) return s.why
  state = newTargetState(verb, s.sources)
  hover = null
  emit()
  return null
}

export function updateTarget(patch: Partial<TargetState>) {
  if (!state) return
  state = { ...state, ...patch }
  emit()
}

export function cancelTarget() {
  if (!state) return
  state = null
  hover = null
  emit()
}

export function targetState(): TargetState | null {
  return state
}

/** setTargetHover records the target under the pointer, for the hint bar. */
export function setTargetHover(key: string | null) {
  if (hover === key) return
  hover = key
  emit()
}

export function useTargetState(): TargetState | null {
  return useSyncExternalStore(subscribe, () => state)
}

export function useTargetHover(): string | null {
  return useSyncExternalStore(subscribe, () => hover)
}

/** switchVerb is c/r/m/p in target mode: the same sources, another verb. */
export function switchVerb(state: TargetState, v: Verb): string | null {
  const s = sourcesFor(v, state.sources)
  if ("why" in s) return s.why
  updateTarget({
    verb: v,
    sources: s.sources,
    side: v === "move" ? "above" : state.side,
    newBranchHere: state.newBranchHere && (v === "commit" || v === "pick"),
  })
  return null
}
