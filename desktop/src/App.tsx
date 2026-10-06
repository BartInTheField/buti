import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { StatusRequestError, type StatusError, type Summary } from "./api"
import { useApiConfig, useWorkspaceStatus } from "./queries"

export default function App() {
  const config = useApiConfig()
  const status = useWorkspaceStatus(config.data?.url, config.data?.token)
  const waiting =
    config.isPending || (Boolean(config.data?.url) && status.isPending)

  function refresh() {
    if (!config.data?.url) {
      void config.refetch()
      return
    }
    void status.refetch()
  }

  return (
    <main className="mx-auto flex min-h-svh max-w-2xl flex-col gap-6 px-6 py-10">
      <header className="flex items-end justify-between gap-4">
        <div>
          <p className="text-sm text-muted-foreground">buti desktop</p>
          <h1 className="font-heading text-2xl font-medium tracking-tight">
            Workspace
          </h1>
        </div>
        <Button
          variant="outline"
          onClick={refresh}
          disabled={waiting || status.isFetching}
        >
          Refresh
        </Button>
      </header>
      {waiting ? (
        <p className="text-sm text-muted-foreground">Reading status…</p>
      ) : null}
      {config.isSuccess && !config.data.url ? (
        <Alert variant="destructive">
          <AlertTitle>No local API</AlertTitle>
          <AlertDescription>
            Start this window with <code>buti desktop</code> so it receives the
            loopback API address.
          </AlertDescription>
        </Alert>
      ) : null}
      {status.isError ? <StatusAlert error={toStatusError(status.error)} /> : null}
      {status.data ? <SummaryCard summary={status.data} /> : null}
    </main>
  )
}

function toStatusError(err: unknown): StatusError {
  if (err instanceof StatusRequestError) {
    return { code: err.code, message: err.message, docsUrl: err.docsUrl }
  }
  return {
    code: "unreachable",
    message: err instanceof Error ? err.message : "Could not reach the local API.",
  }
}

function StatusAlert({ error }: { error: StatusError }) {
  const title =
    error.code === "but_missing"
      ? "GitButler CLI not found"
      : "Could not read status"
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

function SummaryCard({ summary }: { summary: Summary }) {
  const name = summary.repo.split(/[/\\]/).filter(Boolean).pop() ?? summary.repo
  const hidden = summary.uncommitted - summary.uncommittedPaths.length
  return (
    <Card>
      <CardHeader>
        <CardTitle>{name}</CardTitle>
        <CardDescription className="break-all">{summary.repo}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {summary.resolving ? (
          <Alert>
            <AlertTitle>Resolving a conflicted commit</AlertTitle>
            <AlertDescription>
              {summary.resolving.conflicted} conflicted,{" "}
              {summary.resolving.resolved} resolved.
            </AlertDescription>
          </Alert>
        ) : null}
        <dl className="grid grid-cols-3 gap-3 text-sm">
          <Stat label="Uncommitted" value={String(summary.uncommitted)} />
          <Stat label="Stacks" value={String(summary.stacks.length)} />
          <Stat label="Upstream ahead" value={String(summary.upstreamBehind)} />
        </dl>
        {summary.uncommittedPaths.length > 0 ? (
          <section>
            <h2 className="mb-1 text-sm font-medium">Uncommitted</h2>
            <ul className="font-mono text-xs text-muted-foreground">
              {summary.uncommittedPaths.map((path) => (
                <li key={path}>{path}</li>
              ))}
              {hidden > 0 ? <li>and {hidden} more</li> : null}
            </ul>
          </section>
        ) : null}
        <section>
          <h2 className="mb-1 text-sm font-medium">Stacks</h2>
          {summary.stacks.length === 0 ? (
            <p className="text-sm text-muted-foreground">No applied stacks.</p>
          ) : (
            <ul className="flex flex-col gap-2">
              {summary.stacks.map((stack, i) => (
                <li key={i} className="text-sm">
                  {stack.branches.map((branch, j) => (
                    <span key={branch.name}>
                      {j > 0 ? (
                        <span className="text-muted-foreground"> ← </span>
                      ) : null}
                      <span className="font-medium">{branch.name}</span>
                      <span className="text-muted-foreground">
                        {" "}
                        {branch.commits}
                        {branch.upstream > 0 ? `+${branch.upstream}` : ""}
                        {branch.pr ? ` ${branch.pr}` : ""}
                        {branch.status ? ` ${branch.status}` : ""}
                      </span>
                    </span>
                  ))}
                </li>
              ))}
            </ul>
          )}
        </section>
      </CardContent>
      <CardFooter className="text-xs text-muted-foreground">
        From <code>but status --json</code> via the local API.
      </CardFooter>
    </Card>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg bg-muted/60 px-3 py-2">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="text-lg font-medium tabular-nums">{value}</dd>
    </div>
  )
}
