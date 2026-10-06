import { CloudIcon, CloudOffIcon, CloudUploadIcon, GitMergeIcon, GitPullRequestIcon, TriangleAlertIcon } from "lucide-react"
import { branchPR, type Branch } from "@/api"
import { Badge } from "@/components/ui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { cn } from "@/lib/utils"
import { pushState } from "./format"

const toneClass = {
  local: "text-muted-foreground",
  ahead: "border-amber-500/40 text-amber-700 dark:text-amber-400",
  force: "border-destructive/40 text-destructive",
  pushed: "border-teal-500/40 text-teal-700 dark:text-teal-400",
  integrated: "border-primary/40 text-primary",
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
            <Badge variant="outline" className="text-[10px]" data-testid="pr-badge">
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
              className={cn("max-w-24 text-[10px]", toneClass[push.tone])}
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
