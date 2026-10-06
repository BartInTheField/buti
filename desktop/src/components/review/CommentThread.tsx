import { BotIcon, CheckIcon, CircleCheckIcon, CircleXIcon, PencilIcon, ReplyIcon, RotateCcwIcon, Trash2Icon, UserIcon } from "lucide-react"
import { Badge } from "@/components/reui/badge"
import { IconTile } from "@/components/reui/icon-tile"
import { Button } from "@/components/ui/button"
import { HintButton } from "@/components/details/HintButton"
import { cn } from "@/lib/utils"
import { authorLabel, isOpen, type LocatedComment } from "./api"

/** Tones of the tags /buti-review starts a comment with (internal/ui severityStyles). */
const severity: Record<string, "destructive-light" | "warning-light" | "invert-light" | "info-light"> = {
  "[must-fix]": "destructive-light",
  "[suggestion]": "warning-light",
  "[nit]": "invert-light",
  "[question]": "info-light",
}

function Body({ text }: { text: string }) {
  const trimmed = text.trim()
  const tag = Object.keys(severity).find((t) => trimmed.startsWith(t))
  if (!tag) return <>{trimmed}</>
  return (
    <>
      {/* The brackets stay in the text, so copying and searching see the tag as written. */}
      <Badge size="sm" variant={severity[tag]} className="mr-0.5 align-[1px] font-semibold">
        <span className="sr-only">[</span>
        {tag.slice(1, -1)}
        <span className="sr-only">]</span>
      </Badge>
      {trimmed.slice(tag.length)}
    </>
  )
}

function Author({ name }: { name: string }) {
  const label = authorLabel(name)
  return <span className={cn("font-medium", label === "you" ? "text-foreground" : "text-info-foreground dark:text-info")}>{label}</span>
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
 * ones as a card with the body and replies, resolved and dismissed ones as one line.
 */
export function CommentThread({ comment: c, outdated, selected, onEdit, onReply, onToggleResolved, onDelete }: Props) {
  const stop = (fn: () => void) => (e: React.MouseEvent) => {
    e.stopPropagation()
    fn()
  }
  if (!isOpen(c)) {
    const dismissed = c.status === "dismissed"
    return (
      <div
        data-testid="comment-thread"
        data-comment={c.id}
        data-state={c.status}
        className={cn(
          "flex h-7 items-center gap-1.5 rounded-md border border-dashed bg-muted/30 pr-1 pl-2 text-xs text-muted-foreground",
          selected && "border-solid border-ring/60 bg-accent/60",
        )}
      >
        {dismissed ? <CircleXIcon className="size-3.5 shrink-0" /> : <CircleCheckIcon className="size-3.5 shrink-0 text-success" />}
        <span className="min-w-0 flex-1 truncate">
          <span className="font-medium">{dismissed ? "dismissed" : "resolved"}</span> · {c.body.split("\n")[0]}
          {c.resolution.summary ? ` — ${c.resolution.summary.split("\n")[0]}` : ""}
        </span>
        <Button variant="ghost" size="xs" onClick={stop(onToggleResolved)} title="Reopen (x)">
          <RotateCcwIcon />
          Reopen
        </Button>
        <HintButton label="Delete" hint="d" ariaLabel="Delete comment" size="icon-xs" onClick={stop(onDelete)}>
          <Trash2Icon />
        </HintButton>
      </div>
    )
  }
  const was = outdated ? c.at.line_text.replace(/\n$/, "").split("\n").slice(0, 3) : []
  const agent = authorLabel(c.author) !== "you"
  return (
    <div
      data-testid="comment-thread"
      data-comment={c.id}
      data-state={c.state}
      className={cn(
        "overflow-hidden rounded-lg border bg-card text-xs shadow-xs",
        // An open thread gets the warning edge, like the TUI's note gutter; an outdated one stays quiet.
        !outdated && "border-l-2 border-l-warning",
        selected && "ring-2 ring-ring/60",
      )}
    >
      <div className="flex h-8 items-center gap-1.5 border-b bg-muted/40 pr-1 pl-2">
        <IconTile size="xs" variant="outline" radius="full" className={cn("size-5", agent ? "text-info" : "text-muted-foreground")}>
          {agent ? <BotIcon className="size-3" /> : <UserIcon className="size-3" />}
        </IconTile>
        <span className="min-w-0 flex-1 truncate">
          <Author name={c.author} />
          <span className="text-muted-foreground"> · {at(c, outdated)}</span>
        </span>
        {outdated ? (
          <Badge size="sm" variant="warning-outline">
            outdated
          </Badge>
        ) : null}
        <span className="px-1 font-mono text-[10px] text-muted-foreground">{c.id}</span>
        <HintButton label="Edit" hint="e" ariaLabel="Edit comment" size="icon-xs" onClick={stop(onEdit)}>
          <PencilIcon />
        </HintButton>
        <HintButton label="Reply" ariaLabel="Reply" size="icon-xs" onClick={stop(onReply)}>
          <ReplyIcon />
        </HintButton>
        <HintButton
          label="Resolve"
          hint="x"
          ariaLabel="Resolve comment"
          size="icon-xs"
          onClick={stop(onToggleResolved)}
          className="hover:text-success"
        >
          <CheckIcon />
        </HintButton>
        <HintButton
          label="Delete"
          hint="d"
          ariaLabel="Delete comment"
          size="icon-xs"
          onClick={stop(onDelete)}
          className="hover:text-destructive"
        >
          <Trash2Icon />
        </HintButton>
      </div>
      <div className="flex flex-col gap-1.5 px-2.5 py-2 leading-relaxed">
        {was.length > 0 ? (
          <div className="border-l-2 pl-2">
            {was.map((t, i) => (
              <p key={i} className="truncate font-mono text-[11px] text-muted-foreground">
                {t}
              </p>
            ))}
          </div>
        ) : null}
        <p className="break-words whitespace-pre-wrap">
          <Body text={c.body} />
        </p>
        {c.replies.length > 0 ? (
          <div className="flex flex-col gap-1 border-l-2 pl-2">
            {c.replies.map((r, i) => (
              <p key={i} className="break-words whitespace-pre-wrap">
                <span className="sr-only">↳ </span>
                <Author name={r.author} />
                <span className="text-muted-foreground">: </span>
                {r.body.trim()}
              </p>
            ))}
          </div>
        ) : null}
      </div>
    </div>
  )
}
