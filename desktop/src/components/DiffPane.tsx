import { ScrollArea } from "@/components/ui/scroll-area"
import type { Diff } from "@/api"
import { selectionTitle, type Selection } from "@/selection"

type Props = {
  selection: Selection | null
  diff: Diff | undefined
  loading: boolean
  error: string | null
}

export function DiffPane({ selection, diff, loading, error }: Props) {
  return (
    <div className="flex h-full min-h-0 flex-col border-t bg-background">
      <div className="flex items-center border-b px-3 py-2">
        <h2 className="truncate text-sm font-medium">{selectionTitle(selection)}</h2>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className="p-3">
          {selection === null ? (
            <p className="text-sm text-muted-foreground">
              Select something in the workspace to see its diff.
            </p>
          ) : null}
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading diff…</p>
          ) : null}
          {error ? <p className="text-sm text-destructive">{error}</p> : null}
          {diff && !loading ? <DiffView diff={diff} /> : null}
        </div>
      </ScrollArea>
    </div>
  )
}

function DiffView({ diff }: { diff: Diff }) {
  if (diff.changes.length === 0) {
    return <p className="text-sm text-muted-foreground">No changes.</p>
  }
  return (
    <div className="flex flex-col gap-4">
      {diff.changes.map((file) => (
        <section key={file.id || file.path} className="overflow-hidden rounded-md border">
          <header className="flex items-center gap-2 border-b bg-muted/40 px-3 py-1.5">
            <span className="font-mono text-xs font-medium">{file.path}</span>
            <span className="text-[10px] uppercase text-muted-foreground">
              {file.status}
            </span>
          </header>
          <pre className="overflow-x-auto p-3 font-mono text-[11px] leading-relaxed">
            {(file.diff.hunks ?? []).map((h, i) => (
              <HunkLines key={i} text={h.diff} />
            ))}
          </pre>
        </section>
      ))}
    </div>
  )
}

function HunkLines({ text }: { text: string }) {
  const lines = text.split("\n")
  return (
    <>
      {lines.map((line, i) => {
        let cls = "text-foreground"
        if (line.startsWith("+") && !line.startsWith("+++")) {
          cls = "bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"
        } else if (line.startsWith("-") && !line.startsWith("---")) {
          cls = "bg-rose-500/10 text-rose-700 dark:text-rose-400"
        } else if (line.startsWith("@@")) {
          cls = "text-sky-700 dark:text-sky-400"
        }
        return (
          <div key={i} className={cls}>
            {line.length === 0 ? " " : line}
          </div>
        )
      })}
    </>
  )
}
