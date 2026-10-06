import { CheckIcon, ChevronsUpDownIcon, FolderGit2Icon, FolderOpenIcon } from "lucide-react"
import { Button } from "@/components/ui/button"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { toastError } from "@/actions/toast"
import { formatKey } from "@/actions/keys"
import { repoName } from "@/recent"
import { useRepoSwitcher } from "@/repo"

/** RepoMenu is the header's repository name: a menu to switch to a recent repository or open a folder. */
export function RepoMenu({ dir }: { dir: string }) {
  const repos = useRepoSwitcher()
  const others = repos.recent.filter((d) => d !== dir)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="-ml-1 min-w-0 shrink justify-start gap-1.5 px-1.5"
          disabled={repos.switching}
          title={dir}
          data-testid="repo-menu"
        >
          <FolderGit2Icon className="text-muted-foreground" aria-hidden="true" />
          <span className="truncate font-heading text-sm font-medium tracking-tight">{repoName(dir)}</span>
          <ChevronsUpDownIcon className="text-muted-foreground/70" aria-hidden="true" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="w-80">
        <DropdownMenuGroup>
          <DropdownMenuLabel>Current</DropdownMenuLabel>
          <DropdownMenuItem disabled className="data-disabled:opacity-100">
            <CheckIcon className="text-primary" />
            <RepoLabel dir={dir} />
          </DropdownMenuItem>
        </DropdownMenuGroup>
        {others.length > 0 ? (
          <>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuLabel>Recent</DropdownMenuLabel>
              {others.map((d) => (
                <DropdownMenuItem key={d} onSelect={() => void repos.open(d).catch(toastError)}>
                  <span className="size-4 shrink-0" />
                  <RepoLabel dir={d} />
                </DropdownMenuItem>
              ))}
            </DropdownMenuGroup>
          </>
        ) : null}
        <DropdownMenuSeparator />
        <DropdownMenuItem onSelect={() => void repos.choose().catch(toastError)}>
          <FolderOpenIcon />
          Open folder…
          <DropdownMenuShortcut>{formatKey("mod+o")}</DropdownMenuShortcut>
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function RepoLabel({ dir }: { dir: string }) {
  return (
    <span className="flex min-w-0 flex-col gap-0.5">
      <span className="truncate font-medium">{repoName(dir)}</span>
      <span className="truncate font-mono text-[11px] text-muted-foreground" title={dir}>
        {dir}
      </span>
    </span>
  )
}
