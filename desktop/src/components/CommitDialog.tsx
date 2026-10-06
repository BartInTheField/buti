import { useState } from "react"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Textarea } from "@/components/ui/textarea"

type Props = {
  open: boolean
  title: string
  description: string
  pending?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: (message: string) => void
}

export function CommitDialog({
  open,
  title,
  description,
  pending,
  onOpenChange,
  onConfirm,
}: Props) {
  const [message, setMessage] = useState("")

  function submit() {
    onConfirm(message.trim())
    setMessage("")
  }

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) setMessage("")
        onOpenChange(next)
      }}
    >
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <Textarea
          autoFocus
          placeholder="Commit message"
          value={message}
          onChange={(e) => setMessage(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              submit()
            }
          }}
          rows={4}
        />
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={pending}>
            Cancel
          </Button>
          <Button onClick={submit} disabled={pending || !message.trim()}>
            Commit
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
