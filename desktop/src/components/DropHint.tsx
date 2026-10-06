import { hintClass } from "./hint"

/**
 * DropHint labels a drop target while it is hovered. It is absolutely positioned and
 * ignores the pointer, so showing it never changes the layout: a hint that pushed rows
 * down would move the droppables under the pointer and make `over` flicker.
 * The parent must be `relative`.
 *
 * inline puts the hint in a fixed-height flex row instead, after a `min-w-0 truncate` title:
 * the title then ellipsizes before the hint rather than being covered by it.
 */
export function DropHint({ text, className, inline }: { text: string | null; className?: string; inline?: boolean }) {
  if (!text) return null
  return (
    <span data-testid="drop-hint" className={hintClass(inline, className)}>
      {text}
    </span>
  )
}
