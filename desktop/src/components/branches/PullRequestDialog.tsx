import { useState } from "react"
import { formatKey } from "@/actions/keys"
import { Button } from "@/components/ui/button"
import { Checkbox } from "@/components/ui/checkbox"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Textarea } from "@/components/ui/textarea"
import type { PullRequestDraft } from "./store"

/**
 * PullRequestDialog asks for a title, description and draft flag. An empty title uses
 * `but pr new --default` (the title and description from the commits), as `N` does in the TUI.
 */
export function PullRequestDialog({
  branch,
  onDone,
}: {
  branch: string
  onDone: (value: PullRequestDraft | null) => void
}) {
  const [title, setTitle] = useState("")
  const [body, setBody] = useState("")
  const [draft, setDraft] = useState(false)

  function submit() {
    const t = title.trim()
    const message = t ? (body.trim() ? `${t}\n\n${body.trim()}` : t) : ""
    onDone({ message, draft })
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onDone(null)}>
      <DialogContent className="sm:max-w-lg" data-testid="pr-dialog">
        <form
          className="contents"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <DialogHeader>
            <DialogTitle>Create pull request</DialogTitle>
            <DialogDescription className="truncate">For {branch}.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-2">
            <Label htmlFor="pr-title">Title</Label>
            <Input
              id="pr-title"
              autoFocus
              placeholder="Leave empty to use the commits"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="pr-body">Description</Label>
            <Textarea
              id="pr-body"
              rows={6}
              placeholder="Optional"
              disabled={!title.trim()}
              value={body}
              onChange={(e) => setBody(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                  e.preventDefault()
                  submit()
                }
              }}
            />
          </div>
          <Label className="w-fit font-normal">
            <Checkbox checked={draft} onCheckedChange={(v) => setDraft(v === true)} />
            Create as a draft
          </Label>
          <DialogFooter className="items-center">
            <span className="mr-auto text-xs text-muted-foreground">{formatKey("mod+enter")} to create</span>
            <Button type="button" variant="outline" onClick={() => onDone(null)}>
              Cancel
            </Button>
            <Button type="submit">Create</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
