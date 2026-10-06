import { CheckIcon, PencilIcon, ReplyIcon, RotateCcwIcon, Trash2Icon } from "lucide-react"
import { Button } from "@/components/ui/button"
import { cn } from "@/lib/utils"
import { authorLabel, isOpen, type LocatedComment } from "./api"

/** Colours of the tags /buti-review starts a comment with (internal/ui severityStyles). */
const severity: Record<string, string> = {
  "[must-fix]": "text-destructive font-semibold",
  "[suggestion]": "text-amber-600 dark:text-amber-400 font-semibold",
  "[nit]": "text-muted-foreground font-semibold",
  "[question]": "text-sky-600 dark:text-sky-400 font-semibold",
}

function Body({ text }: { text: string }) {
  const trimmed = text.trim()
  const tag = Object.keys(severity).find((t) => trimmed.startsWith(t))
  if (!tag) return <>{trimmed}</>
  return (
    <>
      <span className={severity[tag]}>{tag}</span>
      {trimmed.slice(tag.length)}
    </>
  )
}

function Author({ name }: { name: string }) {
  const label = authorLabel(name)
  return (
    <span className={cn("font-medium", label === "you" ? "text-foreground" : "text-sky-700 dark:text-sky-400")}>
      {label}
    </span>
  )
}

function at(c: LocatedComment, outdated: boolean): string {
  const a = c.at
  let s = a.end_line > a.line ? `lines ${a.line}–${a.end_line}` : `line ${a.line}`
  if (a.side === "old") s += " (old)"
  return outdated ? `outdated · was ${s}` : s
}

type Props = {
  comment: LocatedComment
  outdated: boolean
  /** The line cursor is on the comment. */
  selected: boolean
  onEdit: () => void
  onReply: () => void
  onToggleResolved: () => void
  onDelete: () => void
}

/**
 * CommentThread is a review comment drawn under its line (internal/ui noteRows): open
 * ones as a box with the body and replies, resolved and dismissed ones as one line.
 */
export function CommentThread({ comment: c, outdated, selected, onEdit, onReply, onToggleResolved, onDelete }: Props) {
  const stop = (fn: () => void) => (e: React.MouseEvent) => {
    e.stopPropagation()
    fn()
  }
  if (!isOpen(c)) {
    const label = c.status === "dismissed" ? "✕ dismissed" : "✓ resolved"
    return (
      <div
        data-testid="comment-thread"
        data-comment={c.id}
        data-state={c.status}
        className={cn(
          "flex h-7 items-center gap-2 rounded-md border border-transparent px-2 text-xs text-muted-foreground",
          selected && "border-ring/60 bg-accent/40",
        )}
      >
        <span className="min-w-0 flex-1 truncate">
          {label} · {c.body.split("\n")[0]}
          {c.resolution.summary ? ` — ${c.resolution.summary.split("\n")[0]}` : ""}
        </span>
        <Button variant="ghost" size="xs" onClick={stop(onToggleResolved)} title="Reopen (x)">
          <RotateCcwIcon />
          Reopen
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={stop(onDelete)} title="Delete (d)" aria-label="Delete comment">
          <Trash2Icon />
        </Button>
      </div>
    )
  }
  const was = outdated ? c.at.line_text.replace(/\n$/, "").split("\n").slice(0, 3) : []
  return (
    <div
      data-testid="comment-thread"
      data-comment={c.id}
      data-state={c.state}
      className={cn(
        "rounded-md border bg-card text-xs shadow-xs",
        outdated ? "border-border" : "border-amber-500/50",
        selected && "ring-2 ring-ring/60",
      )}
    >
      <div className="flex h-7 items-center gap-1.5 border-b px-2">
        <span className="min-w-0 flex-1 truncate">
          <Author name={c.author} />
          <span className="text-muted-foreground"> · {at(c, outdated)}</span>
        </span>
        <span className="font-mono text-[10px] text-muted-foreground">{c.id}</span>
        <Button variant="ghost" size="icon-xs" onClick={stop(onEdit)} title="Edit (e)" aria-label="Edit comment">
          <PencilIcon />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={stop(onReply)} title="Reply" aria-label="Reply">
          <ReplyIcon />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={stop(onToggleResolved)} title="Resolve (x)" aria-label="Resolve comment">
          <CheckIcon />
        </Button>
        <Button variant="ghost" size="icon-xs" onClick={stop(onDelete)} title="Delete (d)" aria-label="Delete comment">
          <Trash2Icon />
        </Button>
      </div>
      <div className="flex flex-col gap-1 px-2.5 py-1.5 leading-relaxed">
        {was.map((t, i) => (
          <p key={i} className="truncate font-mono text-[11px] text-muted-foreground">
            &gt; {t}
          </p>
        ))}
        <p className="break-words whitespace-pre-wrap">
          <Body text={c.body} />
        </p>
        {c.replies.map((r, i) => (
          <p key={i} className="break-words whitespace-pre-wrap">
            <span className="text-muted-foreground">↳ </span>
            <Author name={r.author} />
            <span className="text-muted-foreground">: </span>
            {r.body.trim()}
          </p>
        ))}
      </div>
    </div>
  )
}
