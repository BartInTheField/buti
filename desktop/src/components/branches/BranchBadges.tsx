import { CloudIcon, CloudOffIcon, CloudUploadIcon, GitMergeIcon, GitPullRequestIcon, TriangleAlertIcon } from "lucide-react"
import { branchPR, type Branch } from "@/api"
import { Badge, type BadgeProps } from "@/components/reui/badge"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { pushState } from "./format"

/** The ReUI light variant for each push state: neutral, then warning, danger, done. */
const toneVariant = {
  local: "outline",
  ahead: "warning-light",
  force: "destructive-light",
  pushed: "success-light",
  integrated: "primary-light",
} as const satisfies Record<string, BadgeProps["variant"]>

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
              variant={toneVariant[push.tone]}
              size="sm"
              className={push.tone === "local" ? "max-w-24 text-muted-foreground" : "max-w-24"}
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
