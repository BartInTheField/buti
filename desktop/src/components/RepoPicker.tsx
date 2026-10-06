import { FolderOpenIcon, XIcon } from "lucide-react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
          <AlertTitle>{error.title}</AlertTitle>
          <AlertDescription>
            {repos.dir ? <p className="font-mono text-xs break-all">{repos.dir}</p> : null}
            <p>{error.message}</p>
          </AlertDescription>
        </Alert>
      ) : null}

      <section className="flex flex-col gap-3">
        <div>
          <h2 className="font-heading text-lg font-medium">Open a repository</h2>
          <p className="text-sm text-muted-foreground">
            Choose a folder with a GitButler workspace. Run <code>but setup</code> in a git repository to make one.
          </p>
        </div>
        <div>
          <Button
            onClick={() => void repos.choose().catch(toastError)}
            disabled={repos.switching}
          >
            <FolderOpenIcon />
            Choose folder…
            <Kbd className="bg-primary-foreground/15 text-primary-foreground">{formatKey("mod+o")}</Kbd>
          </Button>
        </div>
      </section>

      {recent.length > 0 ? (
        <section className="flex flex-col gap-2">
          <h3 className="text-xs font-medium text-muted-foreground">Recent</h3>
          <ul className="flex flex-col divide-y rounded-lg border" data-testid="recent-repos">
            {recent.map((dir) => (
              <li key={dir} className="group flex items-center">
                <button
                  type="button"
                  className="flex min-w-0 flex-1 flex-col items-start px-3 py-2 text-left hover:bg-muted/60 focus-visible:bg-muted/60 focus-visible:outline-none disabled:opacity-50"
                  disabled={repos.switching}
                  onClick={() => void repos.open(dir).catch(toastError)}
                >
                  <span className="text-sm font-medium">{repoName(dir)}</span>
                  <span className="max-w-full truncate font-mono text-xs text-muted-foreground">{dir}</span>
                </button>
                <Button
                  variant="ghost"
                  size="icon-sm"
                  className="mr-2 opacity-0 group-hover:opacity-100 focus-visible:opacity-100"
                  aria-label={`Remove ${repoName(dir)} from recent`}
                  onClick={() => repos.forget(dir)}
                >
                  <XIcon />
                </Button>
              </li>
            ))}
          </ul>
        </section>
      ) : null}
    </div>
  )
}
