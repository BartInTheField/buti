import type { MouseEvent, ReactNode } from "react"
import { Button } from "@/components/ui/button"
import { Kbd } from "@/components/ui/kbd"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"

type Props = {
  label: string
  /** The key shown in the tooltip; the accessible name stays "label (key)", as the title was. */
  hint?: string
  /** Overrides the accessible name, for buttons whose name tests and screen readers already know. */
  ariaLabel?: string
  onClick: (e: MouseEvent) => void
  disabled?: boolean
  pressed?: boolean
  size?: "icon-sm" | "icon-xs" | "xs"
  className?: string
  children: ReactNode
}

/** HintButton is a ghost toolbar button with a tooltip that names its key (ReUI c-kbd-5). */
export function HintButton({ label, hint, ariaLabel, onClick, disabled, pressed, size = "icon-sm", className, children }: Props) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size={size}
          aria-label={ariaLabel ?? (hint ? `${label} (${hint})` : label)}
          aria-pressed={pressed}
          disabled={disabled}
          className={cn(
            "text-muted-foreground hover:text-foreground aria-pressed:bg-accent aria-pressed:text-foreground",
            className,
          )}
          onClick={onClick}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>
        {label}
        {hint ? <Kbd>{hint}</Kbd> : null}
      </TooltipContent>
    </Tooltip>
  )
}
