import { cn } from "@/lib/utils"

/** hintClass is the pill style DropHint and TargetTag share; see DropHint for inline. */
export function hintClass(inline?: boolean, className?: string) {
  return cn(
    "pointer-events-none truncate rounded-full bg-primary px-2 py-0.5 text-[10px] leading-4 font-medium whitespace-nowrap text-primary-foreground shadow-sm",
    inline ? "max-w-[60%] shrink-0" : "absolute z-10 max-w-[calc(100%-1rem)]",
    className,
  )
}
