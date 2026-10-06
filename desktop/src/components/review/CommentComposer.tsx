import { useState, type RefObject } from "react"
import { Button } from "@/components/ui/button"
import { Kbd } from "@/components/ui/kbd"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
import { Textarea } from "@/components/ui/textarea"
import { formatKey } from "@/actions/keys"

type Props = {
  open: boolean
  title: string
  initial?: string
  /** Where the popover points: the cursor line, measured when it opens. */
  anchor: RefObject<{ getBoundingClientRect: () => DOMRect }>
  /** Resolves true when the comment was saved, which closes the composer. */
  onSubmit: (body: string) => Promise<boolean>
  onClose: () => void
}

/**
 * CommentComposer is the comment editor, a popover at the line it is about. Cmd/ctrl+S
 * or cmd/ctrl+enter saves, Esc cancels (internal/ui newCommentComposer).
 */
export function CommentComposer({ open, title, initial, anchor, onSubmit, onClose }: Props) {
  return (
    <Popover open={open} onOpenChange={(o) => !o && onClose()}>
      <PopoverAnchor virtualRef={anchor} />
      {open ? (
        <Editor key={title + (initial ?? "")} title={title} initial={initial} onSubmit={onSubmit} onClose={onClose} />
      ) : null}
    </Popover>
  )
}

function Editor({ title, initial, onSubmit, onClose }: Omit<Props, "open" | "anchor">) {
  const [body, setBody] = useState(initial ?? "")
  const [saving, setSaving] = useState(false)
  const empty = body.trim() === ""

  async function save() {
    if (empty || saving) return
    setSaving(true)
    try {
      if (await onSubmit(body)) onClose()
    } finally {
      setSaving(false)
    }
  }

  return (
    <PopoverContent
      side="bottom"
      align="start"
      className="w-[min(32rem,calc(100vw-2rem))]"
      data-testid="comment-composer"
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && (e.key === "s" || e.key === "Enter")) {
          e.preventDefault()
          void save()
        }
      }}
    >
      <p className="truncate text-xs font-medium">{title}</p>
      <Textarea
        autoFocus
        value={body}
        onChange={(e) => setBody(e.target.value)}
        placeholder="Leave a comment"
        className="min-h-24 resize-y text-sm"
        aria-label="Comment"
      />
      <div className="flex items-center gap-2">
        <span className="flex-1 text-[11px] text-muted-foreground">
          <Kbd>{formatKey("mod+s")}</Kbd> saves · <Kbd>Esc</Kbd> cancels
        </span>
        <Button variant="ghost" size="sm" onClick={onClose}>
          Cancel
        </Button>
        <Button size="sm" onClick={() => void save()} disabled={empty || saving}>
          Save
        </Button>
      </div>
    </PopoverContent>
  )
}
