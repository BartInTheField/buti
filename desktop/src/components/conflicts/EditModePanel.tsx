import { keepPreviousData, useQuery } from "@tanstack/react-query"
import { CheckIcon, ExternalLinkIcon, FileIcon, XIcon } from "lucide-react"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card } from "@/components/ui/card"
import { Kbd } from "@/components/ui/kbd"
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from "@/components/ui/resizable"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import { fetchFile, type ApiConfig, type Workspace } from "@/api"
import { useActions } from "@/actions/context"
import {
  conflictFiles,
  openConflictFile,
  selectConflictFile,
  selectedConflictFile,
  useEditState,
  type ConflictFile,
} from "@/actions/conflicts"
import { formatKey } from "@/actions/keys"
import { allActions } from "@/actions/registry"

const fileRefetchMs = 3_000

/**
 * EditModePanel replaces the workspace while a conflicted commit is checked out, like the
 * TUI's editView: the commit, its files (Conflicted / Resolved), the selected file with its
 * markers, and the ways out. Every button runs the same action as its key.
 */
export function EditModePanel({ workspace, cfg }: { workspace: Workspace; cfg: ApiConfig }) {
  const actions = useActions()
  const { editing, selected } = useEditState()
  const files = conflictFiles(workspace)
  const current = selectedConflictFile(workspace, selected)
  const left = workspace.resolving?.conflicted_files?.length ?? 0

  function runAction(id: string) {
    const a = allActions().find((a) => a.id === id)
    if (a) actions.run(a, actions.contextFor())
  }

  return (
    <div className="min-h-0 flex-1" data-testid="edit-mode">
      <ResizablePanelGroup orientation="horizontal" className="h-full">
        <ResizablePanel defaultSize="40" minSize="25">
          <ScrollArea className="h-full">
            <div className="flex flex-col gap-4 p-5">
              <div>
                <h2 className="font-heading text-lg font-medium tracking-tight">
                  {editing ? (
                    <>
                      You are editing commit{" "}
                      <code className="rounded bg-muted px-1.5 py-0.5 font-mono text-sm">{editing.shortId}</code>
                    </>
                  ) : (
                    "You are editing a conflicted commit"
                  )}
                </h2>
                <p className="mt-1 text-sm text-muted-foreground">
                  Fix the files in your editor; this view updates as their markers go.
                </p>
              </div>

              {editing ? (
                <Card className="gap-1 px-4 py-3" data-testid="edited-commit">
                  <p className="truncate text-sm font-medium">{editing.subject}</p>
                  <p className="truncate text-xs text-muted-foreground">
                    {editing.shortId}
                    {editing.author ? ` · ${editing.author}` : ""}
                  </p>
                </Card>
              ) : null}

              <Card className="gap-0 py-2">
                <div className="flex items-center gap-2 px-4 py-1.5">
                  <span className="text-sm font-medium">Commit files</span>
                  <Badge variant="secondary" className="tabular-nums">
                    {files.length}
                  </Badge>
                </div>
                <ul className="flex flex-col gap-0.5 px-2 pt-1">
                  {files.map((f) => (
                    <FileRow
                      key={f.path}
                      file={f}
                      selected={current?.path === f.path}
                      onSelect={() => selectConflictFile(f.path)}
                      onOpen={() => void openConflictFile(actions.contextFor(), f.path)}
                    />
                  ))}
                </ul>
              </Card>

              <p className="text-xs text-muted-foreground">
                To exit edit mode, save and exit or cancel. Your uncommitted changes come back afterwards.
              </p>

              <div className="flex flex-wrap justify-end gap-2">
                <Button variant="outline" size="sm" onClick={() => runAction("conflict.cancel")}>
                  <XIcon />
                  Cancel
                  <Kbd>{formatKey("x")}</Kbd>
                </Button>
                {/* Disabled rather than hidden when nothing is left to open, so the row never reflows. */}
                <Button
                  variant="secondary"
                  size="sm"
                  disabled={left === 0}
                  onClick={() => runAction("conflict.open-all")}
                >
                  <ExternalLinkIcon />
                  Open conflicted files
                  <Kbd>{formatKey("o")}</Kbd>
                </Button>
                <Button size="sm" onClick={() => runAction("conflict.save")}>
                  <CheckIcon />
                  Save and exit
                  <Kbd>{formatKey("e")}</Kbd>
                </Button>
              </div>
            </div>
          </ScrollArea>
        </ResizablePanel>
        <ResizableHandle withHandle />
        <ResizablePanel defaultSize="60" minSize="30">
          <FileView cfg={cfg} file={current} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  )
}

function FileRow({
  file,
  selected,
  onSelect,
  onOpen,
}: {
  file: ConflictFile
  selected: boolean
  onSelect: () => void
  onOpen: () => void
}) {
  const slash = file.path.lastIndexOf("/")
  const name = file.path.slice(slash + 1)
  const dir = slash > 0 ? file.path.slice(0, slash) : ""
  return (
    <li>
      <button
        type="button"
        data-testid="conflict-file"
        data-path={file.path}
        className={cn(
          "flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-xs",
          selected ? "bg-accent font-medium" : "hover:bg-muted/70",
        )}
        onClick={onSelect}
        onDoubleClick={onOpen}
        title="Double-click to open in your editor"
      >
        <FileIcon className="size-3.5 shrink-0 opacity-60" />
        <span className="min-w-0 flex-1 truncate">
          <span className="font-mono">{name}</span>
          {dir ? <span className="ml-2 text-muted-foreground">{dir}</span> : null}
        </span>
        {file.resolved ? (
          <Badge className="shrink-0 bg-emerald-600 text-white dark:bg-emerald-500">Resolved</Badge>
        ) : (
          <Badge variant="destructive" className="shrink-0">
            Conflicted
          </Badge>
        )}
      </button>
    </li>
  )
}

/** FileView shows the file as it is on disk, conflict markers and all: `but diff` is empty in edit mode. */
function FileView({ cfg, file }: { cfg: ApiConfig; file: ConflictFile | null }) {
  const q = useQuery({
    queryKey: ["file", cfg.url, file?.path],
    queryFn: () => fetchFile(cfg, file?.path ?? ""),
    enabled: file !== null,
    refetchInterval: fileRefetchMs,
    placeholderData: keepPreviousData,
    retry: false,
  })
  return (
    <div className="flex h-full min-h-0 flex-col border-l bg-background">
      <div className="flex items-center gap-2 border-b px-3 py-2">
        <h2 className="min-w-0 flex-1 truncate font-mono text-sm font-medium">{file?.path ?? "No files"}</h2>
        <span className="size-4 shrink-0">{q.isFetching && q.isPlaceholderData ? <Spinner /> : null}</span>
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <div className={cn("p-3 transition-opacity", q.isPlaceholderData && "opacity-60")}>
          {q.isError ? (
            <p className="text-sm text-destructive">{q.error instanceof Error ? q.error.message : "Could not read the file"}</p>
          ) : null}
          {q.data?.binary ? <p className="text-sm text-muted-foreground">Binary file.</p> : null}
          {q.data && !q.data.binary ? <MarkedText text={q.data.content} /> : null}
        </div>
      </ScrollArea>
    </div>
  )
}

type Region = "plain" | "ours" | "base" | "theirs" | "marker"

const regionClass: Record<Region, string> = {
  plain: "text-foreground",
  ours: "bg-sky-500/10 text-sky-800 dark:text-sky-300",
  base: "bg-muted/60 text-muted-foreground",
  theirs: "bg-emerald-500/10 text-emerald-800 dark:text-emerald-300",
  marker: "bg-rose-500/15 font-semibold text-rose-700 dark:text-rose-300",
}

/** regions tags each line with the side of a conflict it is on. */
function regions(lines: string[]): Region[] {
  let region: Region = "plain"
  return lines.map((line) => {
    let cls: Region = region
    if (line.startsWith("<<<<<<<")) [cls, region] = ["marker", "ours"]
    else if (line.startsWith("|||||||") && region !== "plain") [cls, region] = ["marker", "base"]
    else if (line.startsWith("=======") && region !== "plain") [cls, region] = ["marker", "theirs"]
    else if (line.startsWith(">>>>>>>") && region !== "plain") [cls, region] = ["marker", "plain"]
    return cls
  })
}

/** MarkedText colours the sides of each conflict: the new base, the common ancestor, and the commit. */
function MarkedText({ text }: { text: string }) {
  const lines = text.replace(/\n$/, "").split("\n")
  const tags = regions(lines)
  return (
    <pre className="overflow-x-auto rounded-md border font-mono text-[11px] leading-relaxed" data-testid="conflict-text">
      {lines.map((line, i) => {
        return (
          <div key={i} className={cn("flex", regionClass[tags[i]])}>
            <span className="w-10 shrink-0 pr-3 text-right text-muted-foreground/70 select-none">{i + 1}</span>
            <span className="pr-3">{line.length === 0 ? " " : line}</span>
          </div>
        )
      })}
    </pre>
  )
}
