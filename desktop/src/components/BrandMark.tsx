import { cn } from "@/lib/utils"

/** The buti mark (brand/mark.svg): three lanes, the middle one carrying a commit. */
export function BrandMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 256 256" aria-hidden="true" className={cn("size-5 shrink-0", className)}>
      <rect width="256" height="256" rx="56" fill="#7c5cff" />
      <g fill="#ececec">
        <rect x="60" y="60" width="20" height="112" rx="10" />
        <rect x="118" y="60" width="20" height="136" rx="10" />
        <rect x="176" y="84" width="20" height="112" rx="10" />
      </g>
      <circle cx="128" cy="140" r="22" fill="#2dd4bf" stroke="#7c5cff" strokeWidth="8" />
    </svg>
  )
}

/** The lockup: the mark and the lowercase wordmark in JetBrains Mono ExtraBold. */
export function BrandLogo({ className }: { className?: string }) {
  return (
    <span className={cn("flex items-center gap-1.5", className)} data-testid="brand">
      <BrandMark />
      <span className="font-mono text-base leading-none font-extrabold tracking-[-0.04em]">buti</span>
    </span>
  )
}
