import { ChevronRightIcon, CircleAlertIcon, FolderGit2Icon, FolderOpenIcon, XIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/reui/alert"
import { Frame, FrameHeader, FramePanel, FrameTitle } from "@/components/reui/frame"
import { IconTile } from "@/components/reui/icon-tile"
import { Button } from "@/components/ui/button"
import { Kbd } from "@/components/ui/kbd"
import { toastError } from "@/actions/toast"
import { formatKey } from "@/actions/keys"
import { repoName } from "@/recent"
import { useRepoSwitcher } from "@/repo"

/**
 * RepoPicker is the screen when there is no workspace to show: the folder the API started in is
 * not a GitButler repository. It offers the folder dialog and the recently opened repositories.
 */
export function RepoPicker({ error }: { error?: { title: string; message: string } }) {
  const repos = useRepoSwitcher()
  const recent = repos.recent.filter((d) => d !== repos.dir)

  return (
    <div className="flex flex-col gap-6" data-testid="repo-picker">
      {error ? (
        <Alert variant="destructive">
          <CircleAlertIcon />
          <AlertTitle>{error.title}</AlertTitle>
          <AlertDescription>
            {repos.dir ? <p className="font-mono text-xs break-all">{repos.dir}</p> : null}
            <p>{error.message}</p>
          </AlertDescription>
        </Alert>
      ) : null}

      <section className="flex flex-col items-center gap-4 text-center">
        <IconTile variant="frame" size="xl" aria-hidden="true">
          <FolderGit2Icon className="text-primary" />
        </IconTile>
        <div className="flex max-w-sm flex-col gap-1">
          <h2 className="font-heading text-lg font-medium tracking-tight">Open a repository</h2>
          <p className="text-sm text-balance text-muted-foreground">
            Choose a folder with a GitButler workspace. Run{" "}
            <code className="rounded-sm bg-muted px-1 py-0.5 font-mono text-xs text-foreground">but setup</code> in a
            git repository to make one.
          </p>
        </div>
        <Button onClick={() => void repos.choose().catch(toastError)} disabled={repos.switching}>
          <FolderOpenIcon />
          Choose folder…
          <Kbd className="-mr-1 bg-primary-foreground/15 text-primary-foreground">{formatKey("mod+o")}</Kbd>
        </Button>
      </section>

      {recent.length > 0 ? (
        <Frame stacked spacing="xs">
          <FrameHeader>
            <FrameTitle className="text-xs font-medium text-muted-foreground">Recent</FrameTitle>
          </FrameHeader>
          <FramePanel className="p-0">
            <ul className="flex flex-col divide-y" data-testid="recent-repos">
              {recent.map((dir) => (
                <li key={dir} className="group relative flex items-center">
                  <button
                    type="button"
                    className="flex min-w-0 flex-1 items-center gap-2.5 px-3 py-2 text-left transition-colors hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none disabled:opacity-50"
                    disabled={repos.switching}
                    onClick={() => void repos.open(dir).catch(toastError)}
                  >
                    <IconTile variant="outline" size="sm" aria-hidden="true">
                      <FolderGit2Icon className="text-muted-foreground" />
                    </IconTile>
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="text-sm font-medium">{repoName(dir)}</span>
                      <span className="truncate font-mono text-xs text-muted-foreground" title={dir}>
                        {dir}
                      </span>
                    </span>
                    <ChevronRightIcon
                      className="size-4 shrink-0 text-muted-foreground/60 group-hover:opacity-0"
                      aria-hidden="true"
                    />
                  </button>
                  <Button
                    variant="ghost"
                    size="icon-xs"
                    className="absolute right-2.5 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                    aria-label={`Remove ${repoName(dir)} from recent`}
                    onClick={() => repos.forget(dir)}
                  >
                    <XIcon />
                  </Button>
                </li>
              ))}
            </ul>
          </FramePanel>
        </Frame>
      ) : null}
    </div>
  )
}
