import { cn } from "@/lib/utils"

/**
 * DropHint labels a drop target while it is hovered. It is absolutely positioned and
 * ignores the pointer, so showing it never changes the layout: a hint that pushed rows
 * down would move the droppables under the pointer and make `over` flicker.
 * The parent must be `relative`.
 */
export function DropHint({ text, className }: { text: string | null; className?: string }) {
  if (!text) return null
  return (
    <span
      data-testid="drop-hint"
      className={cn(
        "pointer-events-none absolute z-10 max-w-[calc(100%-1rem)] truncate rounded-full bg-primary px-2 py-0.5 text-[10px] leading-4 font-medium whitespace-nowrap text-primary-foreground shadow-sm",
        className,
      )}
    >
      {text}
    </span>
  )
}
