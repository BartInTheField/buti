import { toast } from "sonner"
import { ApiRequestError } from "@/api"

/** toastError shows a failed op or action: the `but` message when there is one. */
export function toastError(err: unknown) {
  if (err instanceof ApiRequestError) {
    toast.error(err.message)
    return
  }
  toast.error(err instanceof Error ? err.message : "Operation failed")
}
