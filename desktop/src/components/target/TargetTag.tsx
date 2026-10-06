import { cn } from "@/lib/utils"
import { hintClass } from "../hint"
import type { ItemDeco } from "./context"

/**
 * TargetTag labels the hovered target with what clicking it does, and for commits marks
 * where the result lands. Positioned like DropHint (inline too); the insert line needs a
 * relative parent.
 */
export function TargetTag({
  deco,
  className,
  inline,
}: {
  deco: ItemDeco | null
  className?: string
  inline?: boolean
}) {
  if (!deco?.hovered || !deco.plan) return null
  const insert = deco.plan.insert
  return (
    <>
      <span data-testid="target-tag" className={hintClass(inline, className)}>
        {deco.plan.label}
      </span>
      {insert ? (
        <span
          data-testid="target-insert"
          className={cn(
            "pointer-events-none absolute inset-x-1 z-10 h-0.5 rounded-full bg-primary",
            insert === "above" ? "-top-px" : "-bottom-px",
          )}
        />
      ) : null}
    </>
  )
}
