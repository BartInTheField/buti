import type { ReactNode } from "react"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import { ApiRequestError, type ApiError } from "./api"
import { WorkspaceView } from "./components/WorkspaceView"
import { useApiConfig, useWorkspace } from "./queries"

export default function App() {
  const config = useApiConfig()
  const workspace = useWorkspace(config.data?.url, config.data?.token)
  const waiting =
    config.isPending || (Boolean(config.data?.url) && workspace.isPending)

  function refresh() {
    if (!config.data?.url) {
      void config.refetch()
      return
    }
    void workspace.refetch()
  }

  if (waiting) {
    return (
      <Shell>
        <p className="text-sm text-muted-foreground">Reading workspace…</p>
      </Shell>
    )
  }

  if (config.isSuccess && !config.data.url) {
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

  // Once a workspace is on screen it stays: a failed background refetch is shown inline.
  if (workspace.data && config.data) {
    return (
      <WorkspaceView
        workspace={workspace.data}
        apiUrl={config.data.url}
        apiToken={config.data.token}
        error={workspace.isError ? toApiError(workspace.error).message : null}
      />
    )
  }

  if (workspace.isError) {
    return (
      <Shell onRefresh={refresh} refreshing={workspace.isFetching}>
        <StatusAlert error={toApiError(workspace.error)} />
      </Shell>
    )
  }

  return (
    <Shell onRefresh={refresh}>
      <p className="text-sm text-muted-foreground">No workspace data.</p>
    </Shell>
  )
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
          <p className="text-sm text-muted-foreground">buti desktop</p>
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
