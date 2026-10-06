import { useState, type RefObject } from "react"
import { MessageSquarePlusIcon } from "lucide-react"
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupTextarea } from "@/components/ui/input-group"
import { Kbd } from "@/components/ui/kbd"
import { Popover, PopoverAnchor, PopoverContent } from "@/components/ui/popover"
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
      className="w-[min(32rem,calc(100vw-2rem))] gap-2"
      data-testid="comment-composer"
      onKeyDown={(e) => {
        if ((e.metaKey || e.ctrlKey) && (e.key === "s" || e.key === "Enter")) {
          e.preventDefault()
          void save()
        }
      }}
    >
      <p className="flex min-w-0 items-center gap-1.5 text-xs font-medium">
        <MessageSquarePlusIcon className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="truncate">{title}</span>
      </p>
      {/* ReUI c-input-group-26: the actions sit inside the field, under the text. */}
      <InputGroup>
        <InputGroupTextarea
          autoFocus
          value={body}
          onChange={(e) => setBody(e.target.value)}
          placeholder="Leave a comment"
          className="max-h-64 min-h-24 resize-y text-sm"
          aria-label="Comment"
        />
        <InputGroupAddon align="block-end" className="cursor-default border-t pt-2 font-normal">
          <span className="flex flex-1 items-center gap-1 text-[11px]">
            <Kbd>{formatKey("mod+s")}</Kbd> saves · <Kbd>Esc</Kbd> cancels
          </span>
          <InputGroupButton variant="ghost" size="sm" className="h-7 px-2.5" onClick={onClose}>
            Cancel
          </InputGroupButton>
          <InputGroupButton
            variant="default"
            size="sm"
            className="h-7 px-2.5"
            onClick={() => void save()}
            disabled={empty || saving}
          >
            Save
          </InputGroupButton>
        </InputGroupAddon>
      </InputGroup>
    </PopoverContent>
  )
}
