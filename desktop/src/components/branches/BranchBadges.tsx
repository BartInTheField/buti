import { CloudIcon, CloudOffIcon, CloudUploadIcon, GitMergeIcon, GitPullRequestIcon, TriangleAlertIcon } from "lucide-react"
import { branchPR, type Branch } from "@/api"
import { cn } from "cn"
import { Badge } from "@/components/reui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { pushState } from "./format"

/**
 * The color of each push state, matching branchColor in internal/ui/styles.go: grey while
 * local, yellow with commits to push (diverged too: it needs a force push, it isn't an
 * error), teal once pushed, the accent once integrated. In dark mode teal-400 and
 * yellow-400 are the TUI's exact colors; light mode darkens the text to stay readable.
 */
const toneClass = {
  local: "text-muted-foreground",
  ahead: "border-yellow-500/30 bg-yellow-400/15 text-yellow-700 dark:border-yellow-400/25 dark:text-yellow-400",
  force: "border-yellow-500/30 bg-yellow-400/15 text-yellow-700 dark:border-yellow-400/25 dark:text-yellow-400",
  pushed: "border-teal-500/30 bg-teal-400/15 text-teal-700 dark:border-teal-400/25 dark:text-teal-400",
  integrated: "border-primary/25 bg-primary/15 text-primary",
} as const

const toneIcon = {
  local: CloudOffIcon,
  ahead: CloudUploadIcon,
  force: TriangleAlertIcon,
  pushed: CloudIcon,
  integrated: GitMergeIcon,
} as const

/**
 * BranchBadges show a branch's pull request and push state, each with a tooltip. They
 * only change when the branch does, never on hover, and keep a fixed height.
 */
export function BranchBadges({ branch }: { branch: Branch }) {
  const pr = branchPR(branch.reviewId)
  const push = pushState(branch.branchStatus)
  const PushIcon = push ? toneIcon[push.tone] : null
  return (
    <>
      {pr ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge variant="info-light" size="sm" data-testid="pr-badge">
              <GitPullRequestIcon />
              {pr}
            </Badge>
          </TooltipTrigger>
          <TooltipContent>Pull request {pr} · o opens it</TooltipContent>
        </Tooltip>
      ) : null}
      {push && PushIcon ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge
              variant="outline"
              size="sm"
              className={cn("max-w-24", toneClass[push.tone])}
              data-testid="push-badge"
            >
              <PushIcon />
              <span className="truncate">{push.label}</span>
            </Badge>
          </TooltipTrigger>
          <TooltipContent>{push.tip}</TooltipContent>
        </Tooltip>
      ) : null}
    </>
  )
}
