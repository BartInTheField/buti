import { createContext, useContext } from "react"
import { cn } from "@/lib/utils"
import type { Plan, TargetEntity, Verb } from "@/target"
import { useTargetHover } from "./store"

/** Deco is how one item looks while a verb waits for a target (deco in internal/ui/model.go). */
export type Deco = {
  entity: TargetEntity
  status: "valid" | "invalid" | "source"
  plan: Plan | null
  group: string
}

export type TargetMode = {
  /** Every item's decoration; empty when no verb is pending. */
  decos: Map<string, Deco>
  confirm: (entity: TargetEntity) => void
  openList: () => void
}

export const TargetModeContext = createContext<TargetMode>({
  decos: new Map(),
  confirm: () => {},
  openList: () => {},
})

export function useTargetMode(): TargetMode {
  return useContext(TargetModeContext)
}

export type ItemDeco = Deco & { hovered: boolean }

/** useTargetDeco is the decoration of the item with key, or null outside target mode. */
export function useTargetDeco(key: string): ItemDeco | null {
  const { decos } = useTargetMode()
  const hover = useTargetHover()
  const d = decos.get(key)
  return d ? { ...d, hovered: hover === key } : null
}

/**
 * targetClass styles a target with rings, outlines and opacity only, so entering target
 * mode or hovering a target never moves anything. inset is for rows inside a card.
 */
export function targetClass(d: ItemDeco | null, opts: { inset?: boolean } = {}): string | undefined {
  if (!d) return undefined
  switch (d.status) {
    case "valid":
      return cn(
        "cursor-pointer ring-1 ring-primary/50",
        opts.inset && "ring-inset",
        d.hovered && "bg-primary/10 ring-2 ring-primary",
      )
    case "source":
      return "outline-1 outline-dashed outline-primary/70 -outline-offset-2"
    case "invalid":
      return "cursor-not-allowed opacity-40"
  }
}

export const verbInfo: Record<Verb, { title: string; key: string; chip: string }> = {
  commit: { title: "Commit", key: "c", chip: "bg-emerald-400 text-stone-900" },
  squash: { title: "Squash", key: "r", chip: "bg-sky-400 text-stone-900" },
  move: { title: "Move", key: "m", chip: "bg-amber-400 text-stone-900" },
  pick: { title: "Pick", key: "p", chip: "bg-pink-400 text-stone-900" },
}
