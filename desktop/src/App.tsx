import { useEffect, useRef, useState, type ReactNode } from "react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { eventKeys, typingIn } from "@/actions/keys"
import { toastError } from "@/actions/toast"
import { ApiRequestError, type ApiConfig, type ApiError } from "./api"
import { BrandLogo } from "./components/BrandMark"
import { RepoPicker } from "./components/RepoPicker"
import { WorkspaceView } from "./components/WorkspaceView"
import { useApiConfig, useWorkspace } from "./queries"
import { RepoProvider } from "./RepoProvider"
import { useRepo, useRepoSwitcher } from "./repo"

export default function App() {
  const config = useApiConfig()

  if (config.isPending) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">Reading workspace…</p>
      </Shell>
    )
  }

  if (!config.data?.url) {
    return (
      <Shell>
        <Alert variant="destructive">
          <AlertTitle>No local API</AlertTitle>
          <AlertDescription>
            Start this window with <code>buti desktop</code> so it receives the
            loopback API address.
          </AlertDescription>
        </Alert>
      </Shell>
    )
  }

  return (
    <RepoProvider cfg={config.data}>
      <OpenFolderKey />
      <Screen cfg={config.data} />
    </RepoProvider>
  )
}

function Screen({ cfg }: { cfg: ApiConfig }) {
  const repo = useRepo(cfg)
  const repos = useRepoSwitcher()
  const workspace = useWorkspace(cfg.url, cfg.token)
  const reopen = useReopenLast(cfg, workspace.isError)

  // A workspace that loaded is worth offering again, here and on the next start.
  const shownDir = workspace.data?.repo
  useEffect(() => {
    if (shownDir) repos.shown(shownDir)
  }, [shownDir]) // eslint-disable-line react-hooks/exhaustive-deps

  if (workspace.isPending || reopen.pending || repos.switching) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">Reading workspace…</p>
      </Shell>
    )
  }

  // Once a workspace is on screen it stays: a failed background refetch is shown inline.
  if (workspace.data) {
    return (
      <WorkspaceView
        key={workspace.data.repo}
        workspace={workspace.data}
        apiUrl={cfg.url}
        apiToken={cfg.token}
        error={workspace.isError ? toApiError(workspace.error).message : null}
      />
    )
  }

  const err = toApiError(workspace.error)
  // Without `but` no folder helps: say how to install it rather than offering folders.
  if (err.code === "but_missing") {
    return (
      <Shell onRefresh={() => void workspace.refetch()} refreshing={workspace.isFetching}>
        <StatusAlert error={err} />
      </Shell>
    )
  }
  // A window started outside a repository (no -C) just asks for one.
  const quiet = repo.data && !repo.data.chosen
  return (
    <Shell onRefresh={() => void workspace.refetch()} refreshing={workspace.isFetching}>
      <RepoPicker error={quiet ? undefined : { title: "Could not read workspace", message: err.message }} />
    </Shell>
  )
}

/**
 * useReopenLast opens the most recent repository once, when `buti desktop` ran without -C and the
 * working directory is not a workspace: the window was not started in a folder the way the TUI is.
 */
function useReopenLast(cfg: ApiConfig, failed: boolean) {
  const repo = useRepo(cfg)
  const repos = useRepoSwitcher()
  const [tried, setTried] = useState(false)
  const last = repos.recent.find((d) => d !== repo.data?.dir)
  const pending = failed && !tried && repo.data?.chosen === false && Boolean(last)

  useEffect(() => {
    if (!pending || !last) return
    // A repository that is gone or no longer a workspace leaves the picker on screen.
    void repos.open(last).then(
      () => setTried(true),
      () => {
        setTried(true)
        repos.forget(last)
      },
    )
  }, [pending, last]) // eslint-disable-line react-hooks/exhaustive-deps

  return { pending }
}

/**
 * OpenFolderKey binds cmd/ctrl+O on every screen, the picker included. On the workspace the
 * "repo.open" action has the same key; whichever listener runs first prevents the other.
 */
function OpenFolderKey() {
  const repos = useRepoSwitcher()
  const latest = useRef(repos)
  useEffect(() => {
    latest.current = repos
  })
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      if (e.defaultPrevented || typingIn(e.target) || !eventKeys(e).includes("mod+o")) return
      e.preventDefault()
      if (!latest.current.switching) void latest.current.choose().catch(toastError)
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [])
  return null
}

function Shell({
  children,
  onRefresh,
  refreshing,
}: {
  children: ReactNode
  onRefresh?: () => void
  refreshing?: boolean
}) {
  return (
    <main className="mx-auto flex min-h-svh max-w-2xl flex-col gap-6 px-6 py-10">
      <header className="flex items-end justify-between gap-4">
        <div>
          <BrandLogo className="mb-2" />
          <h1 className="font-heading text-2xl font-medium tracking-tight">
            Workspace
          </h1>
        </div>
        {onRefresh ? (
          <Button variant="outline" onClick={onRefresh} disabled={refreshing}>
            Refresh
          </Button>
        ) : null}
      </header>
      {children}
    </main>
  )
}

function toApiError(err: unknown): ApiError {
  if (err instanceof ApiRequestError) {
    return { code: err.code, message: err.message, docsUrl: err.docsUrl }
  }
  return {
    code: "unreachable",
    message: err instanceof Error ? err.message : "Could not reach the local API.",
  }
}

function StatusAlert({ error }: { error: ApiError }) {
  const title =
    error.code === "but_missing"
      ? "GitButler CLI not found"
      : "Could not read workspace"
  return (
    <Alert variant="destructive">
      <AlertTitle>{title}</AlertTitle>
      <AlertDescription>
        <p>{error.message}</p>
        {error.docsUrl ? (
          <p className="mt-2">
            <a
              className="underline underline-offset-3"
              href={error.docsUrl}
              target="_blank"
              rel="noreferrer"
            >
              Install the GitButler CLI
            </a>
          </p>
        ) : null}
      </AlertDescription>
    </Alert>
  )
}
