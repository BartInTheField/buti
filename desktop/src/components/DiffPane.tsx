import { useEffect, useLayoutEffect, useMemo, useRef, useState, type KeyboardEvent, type MouseEvent } from "react"
import { useVirtualizer } from "@tanstack/react-virtual"
import { usePanelRef } from "react-resizable-panels"
import {
  ChevronDownIcon,
  ChevronUpIcon,
  CircleAlertIcon,
  EyeIcon,
  EyeOffIcon,
  FileCodeIcon,
  GitBranchIcon,
  GitCommitHorizontalIcon,
  LayersIcon,
  ListTreeIcon,
  Maximize2Icon,
  MessageSquareIcon,
  Minimize2Icon,
  MousePointerClickIcon,
  XIcon,
} from "lucide-react"
import { Alert, AlertDescription } from "@/components/reui/alert"
import { Badge } from "@/components/reui/badge"
import { IconTile } from "@/components/reui/icon-tile"
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable"
import { Separator } from "@/components/ui/separator"
import { Skeleton } from "@/components/ui/skeleton"
import { Spinner } from "@/components/ui/spinner"
import { cn } from "@/lib/utils"
import type { Diff, Workspace } from "@/api"
import { useActions } from "@/actions/context"
import { eventKeys, typingIn } from "@/actions/keys"
import { allActions, offers } from "@/actions/registry"
import { deleteComment, editComment, replyComment, toggleResolved } from "@/actions/review"
import { selectionKey, selectionTitle, type Selection } from "@/selection"
import { CommentComposer } from "./review/CommentComposer"
import { CommentThread } from "./review/CommentThread"
import { isOpen, useComments, type LocatedComment } from "./review/api"
import { fileOf, moveToLines } from "./review/locate"
import { DiffContextMenu } from "./details/DiffContextMenu"
import { FileTree } from "./details/FileTree"
import { HintButton } from "./details/HintButton"
import { PathLabel, StatusBadge } from "./details/StatusBadge"
import { useHighlight, type Highlighted } from "./details/highlight"
import {
  expandTabs,
  hunkSubject,
  isCursorRow,
  layoutDiff,
  notesShown,
  withComments,
  type DiffLayout,
  type HunkRef,
  type LineRow,
  type Row,
  type Shown,
} from "./details/rows"
import {
  detailsPane,
  paneActionIds,
  peekPendingJump,
  setDetailsView,
  setPendingJump,
  useDetailsView,
  type Composer,
  type DetailsPane,
  type LineSel,
} from "./details/store"
import "./details/diff.css"

type Props = {
  selection: Selection | null
  diff: Diff | undefined
  /** Nothing to show yet: the first load for this pane. */
  loading: boolean
  /** diff is the previous selection's, kept on screen while the new one loads. */
  stale?: boolean
  error: string | null
}

const rowHeight = { file: 32, hunk: 24, line: 20, gap: 12, info: 28 } as const

function estimate(r: Row): number {
  if (r.type === "note") {
    if (!isOpen(r.comment)) return 32
    return 44 + 18 * (r.comment.body.split("\n").length + r.comment.replies.length)
  }
  return rowHeight[r.type]
}

/** shownFor is what the diff is of, for picking its review comments. */
function shownFor(sel: Selection | null, ws: Workspace): Shown {
  const commit = (cliId: string) =>
    ws.stacks.flatMap((s) => s.branches.flatMap((b) => b.commits)).find((c) => c.cliId === cliId)
  switch (sel?.kind) {
    case "unstaged":
    case "file":
      return { kind: "uncommitted" }
    case "commit": {
      const c = commit(sel.id)
      return c ? { kind: "commit", changeId: c.changeId, commitId: c.commitId } : { kind: "none" }
    }
    case "cfile": {
      const c = commit(sel.commit)
      return c ? { kind: "commit", changeId: c.changeId, commitId: c.commitId, path: sel.path } : { kind: "none" }
    }
    case "branch":
      return { kind: "branch", name: sel.name }
    default:
      return { kind: "none" }
  }
}

type Cursor = { key: string; fallback?: string }

export function DiffPane({ selection, diff, loading, stale, error }: Props) {
  const actions = useActions()
  const { workspace, sel, ops } = actions.contextFor()
  const view = useDetailsView()
  const comments = useComments(ops.cfg, workspace)

  // The selection the diff on screen belongs to: while a new one loads, the old diff stays.
  const [shownSel, setShownSel] = useState(selection)
  const sameKey = (selection ? selectionKey(selection) : "") === (shownSel ? selectionKey(shownSel) : "")
  if (selection !== shownSel && (sameKey || (diff && !stale))) setShownSel(selection)
  const shownKey = shownSel ? selectionKey(shownSel) : ""
  const uncommitted = shownSel?.kind === "unstaged" || shownSel?.kind === "file"
  const shown = shownFor(shownSel, workspace)
  const shownId = JSON.stringify(shown)

  const base = useMemo(() => (diff ? layoutDiff(diff, uncommitted) : null), [diff, uncommitted])
  const notes = useMemo(() => {
    if (!diff) return []
    const fileDiff = (path: string) => fileOf(diff, path)
    return notesShown(comments.data ?? [], JSON.parse(shownId) as Shown, view.hideResolved, fileDiff, moveToLines)
  }, [diff, comments.data, shownId, view.hideResolved])
  const layout = useMemo(() => (base ? withComments(base, notes) : null), [base, notes])
  const hl = useHighlight(ops.cfg, base?.hunks ?? [])
  const rows = useMemo(() => layout?.rows ?? [], [layout])

  const [cursor, setCursor] = useState<Cursor | null>(null)
  const [anchor, setAnchor] = useState<string | null>(null)
  const [focused, setFocused] = useState(false)
  const [composer, setComposer] = useState<Composer | null>(null)
  const [jumpTick, setJumpTick] = useState(0)
  const [cursorFor, setCursorFor] = useState(shownKey)
  if (cursorFor !== shownKey) {
    setCursorFor(shownKey)
    setCursor(null)
    setAnchor(null)
  }

  const index = useMemo(() => new Map(rows.map((r, i) => [r.key, i])), [rows])
  let cur = -1
  if (cursor) cur = index.get(cursor.key) ?? (cursor.fallback ? (index.get(cursor.fallback) ?? -1) : -1)
  const curRow = cur >= 0 ? rows[cur] : null
  const curHunk = curRow && "hunk" in curRow && curRow.hunk >= 0 ? (layout?.hunks[curRow.hunk] ?? null) : null
  let anc = anchor !== null ? (index.get(anchor) ?? -1) : -1
  if (anc >= 0 && (rows[anc].type !== "line" || curRow?.type !== "line" || (rows[anc] as LineRow).hunk !== curRow.hunk)) anc = -1
  const lo = anc >= 0 ? Math.min(cur, anc) : cur
  const hi = anc >= 0 ? Math.max(cur, anc) : cur

  const scrollRef = useRef<HTMLDivElement>(null)
  const [viewportW, setViewportW] = useState(0)
  useEffect(() => {
    const el = scrollRef.current
    if (!el) return
    const ro = new ResizeObserver(() => setViewportW(el.clientWidth))
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  useEffect(() => {
    scrollRef.current?.scrollTo({ top: 0 })
  }, [shownKey])

  const virt = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scrollRef.current,
    estimateSize: (i) => estimate(rows[i]),
    getItemKey: (i) => rows[i].key,
    overscan: 24,
  })

  // The hunk under the cursor is what c / r / x act on while the pane has focus (internal/ui subjects).
  const detailHunk = focused && uncommitted && curHunk?.id ? curHunk : null
  const setDetail = sel.setDetail
  const detailKey = detailHunk ? `${detailHunk.id}\0${detailHunk.header}` : ""
  // Layout effects: a key pressed right after the click that moved the cursor acts on its hunk.
  useLayoutEffect(() => {
    setDetail(detailHunk ? hunkSubject(detailHunk) : null)
    // detailKey identifies detailHunk.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [detailKey, setDetail])
  useEffect(() => () => setDetail(null), [setDetail])

  // Scrolls are applied after the render that moved the cursor, so the virtualizer
  // measures the rows they go to; scrolling inside the key handler gets lost.
  const scrollTo = useRef<{ key: string; align: "auto" | "start" | "center"; hunk?: [string, string] } | null>(null)
  useEffect(() => {
    const t = scrollTo.current
    const el = scrollRef.current
    if (!t || !el) return
    scrollTo.current = null
    const i = index.get(t.key)
    if (i === undefined) return
    const first = t.hunk && index.get(t.hunk[0])
    const last = t.hunk && index.get(t.hunk[1])
    if (first !== undefined && last !== undefined) {
      // Like the TUI's selectHunk: the whole hunk when it fits, else from its header.
      const top = virt.measurementsCache[first]?.start ?? 0
      const end = virt.measurementsCache[last]?.end ?? 0
      if (top < el.scrollTop || end > el.scrollTop + el.clientHeight) {
        el.scrollTop = Math.min(top, Math.max(end - el.clientHeight, 0))
        return
      }
    }
    virt.scrollToIndex(i, { align: t.align })
  })

  function moveTo(i: number, opts: { keepRange?: boolean; scroll?: boolean } = {}) {
    const r = rows[i]
    if (!r) return
    setCursor({ key: r.key, fallback: r.type === "note" ? r.after : undefined })
    if (!opts.keepRange) setAnchor(null)
    if (opts.scroll !== false) scrollTo.current = { key: r.key, align: "auto" }
  }

  function firstLine(h: HunkRef): number {
    for (let i = h.first; i < h.end; i++) if (rows[i].type === "line" && isCursorRow(rows[i])) return i
    return -1
  }

  function selectHunk(h: number) {
    if (!layout || layout.hunks.length === 0) return
    const ref = layout.hunks[Math.max(0, Math.min(h, layout.hunks.length - 1))]
    const first = firstLine(ref)
    if (first < 0) return
    moveTo(first, { scroll: false })
    // The header with the line: the hunk's top, unless that would put the line out of view.
    scrollTo.current = { key: rows[first].key, align: "auto", hunk: [rows[ref.first].key, rows[ref.end - 1].key] }
  }

  /** nextRow is the closest row the cursor can stop on; a range stays in its hunk and skips comments. */
  function nextRow(from: number, dir: number): number {
    for (let i = from + dir; i >= 0 && i < rows.length; i += dir) {
      const r = rows[i]
      if (anc >= 0) {
        if (!("hunk" in r) || r.hunk !== (rows[anc] as LineRow).hunk) return -1
        if (r.type === "line" && isCursorRow(r)) return i
        continue
      }
      if (isCursorRow(r)) return i
    }
    return -1
  }

  function step(dir: number) {
    if (!layout || layout.hunks.length === 0) return
    if (cur < 0) return selectHunk(0)
    const next = nextRow(cur, dir)
    if (next >= 0) moveTo(next, { keepRange: true })
    else if (anc < 0) scrollRef.current?.scrollBy({ top: dir * rowHeight.line })
  }

  function stepHunk(dir: number) {
    if (!curHunk) return selectHunk(0)
    selectHunk(curHunk.index + dir)
  }

  function lineSelection(): LineSel | null {
    if (!curRow || curRow.type !== "line" || !curHunk) return null
    const span = rows.slice(lo, hi + 1).filter((r): r is LineRow => r.type === "line" && r.sign !== "\\")
    const side = span.some((r) => r.sign !== "-") ? "new" : "old"
    const s: LineSel = { path: curHunk.path, side, line: 0, endLine: 0, text: "", hunk: curHunk }
    const text: string[] = []
    for (const r of span) {
      const n = side === "old" ? r.old : r.new
      if (n === 0) continue // a removed line inside a range on the new side
      if (s.line === 0) s.line = n
      s.endLine = n
      text.push(r.code)
    }
    s.text = text.join("\n")
    return s.line > 0 ? s : null
  }

  function cursorComment(): LocatedComment | null {
    return curRow?.type === "note" ? curRow.comment : null
  }

  function focusPane() {
    scrollRef.current?.focus({ preventScroll: true })
  }

  function scrollToFile(path: string) {
    const f = layout?.files.find((x) => x.path === path)
    if (!f || !layout) return
    if (f.hunk >= 0) {
      const first = firstLine(layout.hunks[f.hunk])
      if (first >= 0) moveTo(first, { scroll: false })
    }
    scrollTo.current = { key: rows[f.row].key, align: "start" }
  }

  // Actions reach the pane through detailsPane; it reads this render's state.
  const api: DetailsPane = {
    layout: () => layout,
    focused: () => focused,
    hasCursor: () => cur >= 0,
    ranged: () => anc >= 0,
    cursorHunk: () => curHunk,
    cursorComment,
    lineSelection,
    stepHunk,
    toggleRange: () => {
      if (anc >= 0) setAnchor(null)
      else if (curRow?.type === "line") setAnchor(curRow.key)
    },
    cancelRange: () => setAnchor(null),
    focus: focusPane,
    jumpToComment: (id: string) => {
      setPendingJump(id)
      setJumpTick((t) => t + 1)
    },
    compose: (c: Composer) => setComposer(c),
    scrollToFile,
  }
  const apiRef = useRef<DetailsPane>(api)
  useLayoutEffect(() => {
    apiRef.current = api
  })
  useEffect(() => {
    detailsPane.set(apiRef)
    return () => detailsPane.set(null)
  }, [])

  // Jump to a comment (picked from Review comments…) once the diff showing it is here.
  useEffect(() => {
    const id = peekPendingJump()
    if (!id || stale || !layout) return
    const i = index.get(`note:${id}`)
    setPendingJump(null)
    if (i === undefined) return
    setCursor({ key: rows[i].key, fallback: (rows[i] as { after?: string }).after })
    setAnchor(null)
    focusPane()
    scrollTo.current = { key: rows[i].key, align: "center" }
    // virt is stable for the pane's life.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [layout, stale, jumpTick, index, rows])

  function onKeyDown(e: KeyboardEvent<HTMLDivElement>) {
    if (e.defaultPrevented || e.nativeEvent.isComposing || typingIn(e.target)) return
    if (!e.metaKey && !e.ctrlKey && !e.altKey) {
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault()
          return step(1)
        case "k":
        case "ArrowUp":
          e.preventDefault()
          return step(-1)
        case "J":
        case "K":
          e.preventDefault()
          scrollRef.current?.scrollBy({ top: (e.key === "J" ? 5 : -5) * rowHeight.line })
          return
        // Handled here rather than through the deferred action run, so v then j selects as typed.
        case "v":
          e.preventDefault()
          return api.toggleRange()
        case "[":
        case "]":
          e.preventDefault()
          return stepHunk(e.key === "]" ? 1 : -1)
        case "Escape":
          if (anc >= 0) {
            e.preventDefault()
            setAnchor(null)
            return
          }
      }
    }
    // Keys on the line cursor (comment, edit, delete, resolve, mark the hunk) win over
    // the workspace's actions bound to the same key, as the TUI lists them first.
    const ctx = actions.contextFor()
    const mine = allActions().filter((a) => paneActionIds.has(a.id))
    for (const key of eventKeys(e.nativeEvent)) {
      const a = mine.find((x) => x.keys?.includes(key) && offers(x, ctx))
      if (a) {
        e.preventDefault()
        actions.run(a, ctx)
        return
      }
    }
  }

  function onRowMouseDown(i: number, e: MouseEvent) {
    if (e.button !== 0 && e.button !== 2) return
    if ((e.target as HTMLElement).closest("button, a, textarea")) return
    const r = rows[i]
    if (e.shiftKey) e.preventDefault() // no text selection on shift-click
    focusPane()
    if (r.type === "hunk") {
      if (e.button === 0 || curHunk?.index !== r.hunk) selectHunk(r.hunk)
      return
    }
    if (r.type === "note") {
      setAnchor(null)
      setCursor({ key: r.key, fallback: r.after })
      return
    }
    if (r.type !== "line" || r.sign === "\\") return
    if (e.button === 2 && i >= lo && i <= hi) return // right-click inside the range keeps it
    const sameHunk = curRow?.type === "line" && curRow.hunk === r.hunk
    if (anc >= 0 && !sameHunk) setAnchor(null)
    if (e.shiftKey && anc < 0 && sameHunk && curRow) setAnchor(curRow.key)
    if (e.button === 0 && !e.shiftKey) {
      // A plain press starts a drag: moving onto other lines selects the range, like `v`.
      e.preventDefault() // the line range replaces the browser's text selection
      setAnchor(null)
      dragRef.current = { start: i, at: i, hunk: r.hunk, ranged: false }
    }
    setCursor({ key: r.key })
  }

  /** The line nearest to row i, walking back towards the drag start, in the drag's hunk. */
  function dragLine(i: number, d: { start: number; hunk: number }): number {
    const step = i > d.start ? -1 : 1
    for (let j = i; j !== d.start; j += step) {
      const r = rows[j]
      if (r?.type === "line" && r.hunk === d.hunk && r.sign !== "\\") return j
    }
    return d.start
  }

  function onRowMouseEnter(i: number) {
    const d = dragRef.current
    if (!d) return
    const j = dragLine(i, d)
    if (j === d.at) return
    d.at = j
    if (!d.ranged) {
      d.ranged = true
      setAnchor(rows[d.start].key)
    }
    setCursor({ key: rows[j].key })
  }

  const dragRef = useRef<{ start: number; at: number; hunk: number; ranged: boolean } | null>(null)
  useEffect(() => {
    const end = () => {
      const d = dragRef.current
      dragRef.current = null
      // Dragging back onto the start line is a click, not a one-line range.
      if (d?.ranged && d.at === d.start) setAnchor(null)
    }
    window.addEventListener("mouseup", end)
    return () => window.removeEventListener("mouseup", end)
  }, [])

  // While dragging a range, holding the pointer near the top or bottom edge scrolls the diff.
  function onScrollMouseMove(e: MouseEvent) {
    const el = scrollRef.current
    if (!dragRef.current || !el || e.buttons !== 1) return
    const box = el.getBoundingClientRect()
    const edge = 24
    if (e.clientY < box.top + edge) el.scrollTop -= edge
    else if (e.clientY > box.bottom - edge) el.scrollTop += edge
  }

  const anchorRef = useRef({
    getBoundingClientRect: () => {
      const el = scrollRef.current
      const c = el?.querySelector<HTMLElement>("[data-cursor=true]")
      return (c ?? el)?.getBoundingClientRect() ?? new DOMRect()
    },
  })

  const subjectForMenu = (): Selection | null => {
    const h = apiRef.current.cursorHunk()
    if (h?.id && (shownSel?.kind === "unstaged" || shownSel?.kind === "file")) return hunkSubject(h)
    return sel.selection
  }

  const openCount = notes.filter(isOpen).length
  const stats = useMemo(() => diffStats(base), [base])
  const activeFile = layout ? fileAt(layout, virt.range?.startIndex ?? 0) : ""
  const treeShown = view.full && view.tree && layout !== null && layout.files.length > 0
  const gutterCh = (layout?.numW ?? 1) * 2 + 3
  const treeRef = usePanelRef()
  useEffect(() => {
    const t = treeRef.current
    if (!t) return
    if (!treeShown) return t.collapse()
    t.expand()
    // Starting collapsed, expand would only reach the minimum width.
    if (t.getSize().inPixels < 200) t.resize(260)
  }, [treeShown, treeRef])

  const body = (
    <div className="relative flex h-full min-h-0 flex-col">
      <DiffContextMenu subject={subjectForMenu}>
        <div
          ref={scrollRef}
          tabIndex={0}
          data-testid="diff-scroll"
          role="grid"
          aria-label="Diff"
          className="min-h-0 flex-1 overflow-auto outline-none [contain:strict]"
          onKeyDown={onKeyDown}
          onMouseMove={onScrollMouseMove}
          onFocus={() => setFocused(true)}
          onBlur={(e) => {
            const to = e.relatedTarget as HTMLElement | null
            if (to && scrollRef.current?.contains(to)) return
            // Focus moving into a dialog, menu or the composer keeps the hunk as the subject.
            if (to?.closest('[role="dialog"], [role="alertdialog"], [role="menu"], [data-slot="popover-content"]')) return
            setFocused(false)
          }}
        >
          {selection === null ? (
            <div className="flex h-full flex-col items-center justify-center gap-2 p-6 text-center">
              <IconTile variant="frame" size="sm" className="text-muted-foreground">
                <MousePointerClickIcon />
              </IconTile>
              <p className="text-sm text-muted-foreground">Select something in the workspace to see its diff.</p>
            </div>
          ) : null}
          {loading && !diff ? <DiffSkeleton /> : null}
          {error ? (
            <Alert variant="destructive" className="m-3 w-auto">
              <CircleAlertIcon />
              <AlertDescription className="text-destructive">{error}</AlertDescription>
            </Alert>
          ) : null}
          {layout ? (
            <div
              className={cn("relative font-mono text-xs transition-opacity", stale && "opacity-60")}
              style={{
                height: virt.getTotalSize() + 48,
                width: `max(100%, calc(${gutterCh + layout.maxLen + 2}ch + 24px))`,
              }}
            >
              {virt.getVirtualItems().map((item) => {
                const r = rows[item.index]
                const inHunk = "hunk" in r && r.hunk >= 0
                const hunkSelected = inHunk && curHunk !== null && (r as { hunk: number }).hunk === curHunk.index
                const hunk = inHunk ? layout.hunks[(r as { hunk: number }).hunk] : null
                const marked = hunk?.id ? sel.isMarked(hunkSubject(hunk)) : false
                return (
                  <div
                    key={item.key}
                    data-index={item.index}
                    data-key={r.key}
                    data-cursor={item.index === cur ? true : undefined}
                    ref={r.type === "note" ? virt.measureElement : undefined}
                    className="absolute top-0 left-0 w-full"
                    style={{ transform: `translateY(${item.start}px)`, height: r.type === "note" ? undefined : item.size }}
                    onMouseDown={(e) => onRowMouseDown(item.index, e)}
                    onMouseEnter={() => onRowMouseEnter(item.index)}
                  >
                    <RowView
                      row={r}
                      hl={hl.data}
                      hunk={hunk}
                      numW={layout.numW}
                      viewportW={viewportW}
                      hunkSelected={hunkSelected}
                      marked={marked}
                      focused={focused}
                      cursor={item.index === cur}
                      inRange={anc >= 0 && item.index >= lo && item.index <= hi}
                      ranged={anc >= 0}
                      onComment={(verb) => {
                        if (r.type !== "note") return
                        setCursor({ key: r.key, fallback: r.after })
                        const fn = { edit: editComment, reply: replyComment, resolve: toggleResolved, delete: deleteComment }[verb]
                        void fn(actions.contextFor(), r.comment)
                      }}
                    />
                  </div>
                )
              })}
            </div>
          ) : null}
        </div>
      </DiffContextMenu>
      <CommentComposer
        open={composer !== null}
        title={composer?.title ?? ""}
        initial={composer && composer.mode !== "new" ? composer.initial : undefined}
        anchor={anchorRef}
        onSubmit={async (b) => {
          const ok = (await composer?.submit(b)) ?? false
          if (ok) setAnchor(null)
          return ok
        }}
        onClose={() => {
          setComposer(null)
          focusPane()
        }}
      />
    </div>
  )

  return (
    <div className="flex h-full min-h-0 flex-col border-t bg-background" data-testid="diff-pane">
      <div className="flex h-10 shrink-0 items-center gap-1.5 border-b bg-muted/30 pr-1.5 pl-3">
        <SelectionIcon selection={selection} />
        <h2 className="min-w-0 truncate text-sm font-medium">{selectionTitle(selection)}</h2>
        {stats.files > 0 ? (
          <span className="flex shrink-0 items-center gap-1" data-testid="diff-stats">
            {stats.files > 1 ? (
              <span className="text-xs text-muted-foreground tabular-nums">{stats.files} files</span>
            ) : null}
            {stats.added > 0 ? (
              <Badge size="sm" variant="success-light" className="font-mono tabular-nums">
                +{stats.added}
              </Badge>
            ) : null}
            {stats.removed > 0 ? (
              <Badge size="sm" variant="destructive-light" className="font-mono tabular-nums">
                −{stats.removed}
              </Badge>
            ) : null}
          </span>
        ) : null}
        {/* Reserved space, so the spinner appearing never shifts the title. */}
        <span className="size-4 shrink-0 text-muted-foreground">{stale || loading ? <Spinner /> : null}</span>
        <span className="flex-1" />
        <Badge
          variant="warning-light"
          className={cn("gap-1 tabular-nums", openCount === 0 && "invisible")}
          data-testid="open-comments"
          title="Open review comments"
        >
          <MessageSquareIcon />
          {openCount}
        </Badge>
        <HintButton label="Previous hunk" hint="[" onClick={() => stepHunk(-1)} disabled={!layout?.hunks.length}>
          <ChevronUpIcon />
        </HintButton>
        <HintButton label="Next hunk" hint="]" onClick={() => stepHunk(1)} disabled={!layout?.hunks.length}>
          <ChevronDownIcon />
        </HintButton>
        <Separator orientation="vertical" className="mx-0.5 my-2.5" />
        <HintButton
          label={view.hideResolved ? "Show resolved comments" : "Hide resolved comments"}
          hint="z"
          onClick={() => setDetailsView((v) => ({ hideResolved: !v.hideResolved }))}
          pressed={view.hideResolved}
        >
          {view.hideResolved ? <EyeOffIcon /> : <EyeIcon />}
        </HintButton>
        {view.full ? (
          <HintButton
            label="File tree"
            hint="T"
            onClick={() => setDetailsView((v) => ({ tree: !v.tree }))}
            pressed={view.tree}
          >
            <ListTreeIcon />
          </HintButton>
        ) : null}
        <HintButton
          label={view.full ? "Leave full screen" : "Full screen"}
          hint="D"
          onClick={() => setDetailsView((v) => ({ full: !v.full }))}
        >
          {view.full ? <Minimize2Icon /> : <Maximize2Icon />}
        </HintButton>
        <HintButton
          label={view.full ? "Leave full screen" : "Hide details"}
          hint={view.full ? "Esc" : "d"}
          onClick={() => setDetailsView(view.full ? { full: false } : { visible: false })}
        >
          <XIcon />
        </HintButton>
      </div>
      {/* The tree panel collapses rather than unmounts, so the diff keeps its scroll and focus. */}
      <ResizablePanelGroup orientation="horizontal" className="min-h-0 flex-1">
        <ResizablePanel panelRef={treeRef} defaultSize={treeShown ? 260 : 0} minSize={140} maxSize="50%" collapsible>
          {treeShown ? (
            <FileTree
              files={layout.files.map((f) => ({ path: f.path, status: statusOf(layout, f.row) }))}
              active={activeFile}
              onSelect={scrollToFile}
            />
          ) : null}
        </ResizablePanel>
        <ResizableHandle withHandle className={cn(!treeShown && "hidden")} />
        <ResizablePanel minSize="30%">{body}</ResizablePanel>
      </ResizablePanelGroup>
    </div>
  )
}

function statusOf(l: DiffLayout, row: number): string {
  const r = l.rows[row]
  return r?.type === "file" ? r.status : ""
}

/** fileAt is the file the row at the top of the viewport is in. */
function fileAt(l: DiffLayout, top: number): string {
  let path = l.files[0]?.path ?? ""
  for (const f of l.files) if (f.row <= top) path = f.path
  return path
}

/** diffStats counts the files and lines the diff adds and removes, for the header. */
function diffStats(l: DiffLayout | null): { files: number; added: number; removed: number } {
  const out = { files: 0, added: 0, removed: 0 }
  for (const r of l?.rows ?? []) {
    if (r.type !== "file") continue
    out.files++
    out.added += r.added
    out.removed += r.removed
  }
  return out
}

/** SelectionIcon tells a file, commit, branch and the Unstaged changes apart in the header. */
function SelectionIcon({ selection }: { selection: Selection | null }) {
  const Icon = {
    unstaged: LayersIcon,
    file: FileCodeIcon,
    cfile: FileCodeIcon,
    commit: GitCommitHorizontalIcon,
    branch: GitBranchIcon,
  }[selection?.kind ?? "unstaged"]
  return (
    <IconTile size="xs" variant="outline" className={cn("text-muted-foreground", !selection && "invisible")}>
      <Icon />
    </IconTile>
  )
}

type RowProps = {
  row: Row
  hl: Highlighted | undefined
  hunk: HunkRef | null
  numW: number
  viewportW: number
  hunkSelected: boolean
  marked: boolean
  focused: boolean
  cursor: boolean
  inRange: boolean
  ranged: boolean
  onComment: (verb: "edit" | "reply" | "resolve" | "delete") => void
}

/** Widths of the placeholder lines, in ch: enough variety to read as code. */
const skeletonLines = [34, 52, 18, 44, 61, 27, 0, 39, 56, 22, 47, 31]

/**
 * DiffSkeleton has the diff's shape (file header, hunk header, numbered lines) at the
 * rows' heights, so the first diff replaces it without the pane jumping.
 */
function DiffSkeleton() {
  return (
    <div className="font-mono text-xs" data-testid="diff-skeleton" aria-busy="true" aria-label="Loading diff">
      {[0, 1].map((f) => (
        <div key={f}>
          <div className="flex h-8 items-center gap-2 border-y bg-muted/60 px-3" data-testid="diff-skeleton-file">
            <Skeleton className="size-4 rounded-sm bg-foreground/10" />
            <Skeleton className={cn("h-3 bg-foreground/10", f === 0 ? "w-48" : "w-36")} />
          </div>
          <div className="flex h-6 items-center bg-info/5 px-3">
            <Skeleton className="h-2.5 w-40 bg-info/15" />
          </div>
          {skeletonLines.slice(0, f === 0 ? undefined : 6).map((w, i) => (
            <div key={i} className="flex h-5 items-center">
              <span className="h-full w-[6ch] shrink-0 border-r bg-muted/50" />
              {w > 0 ? <Skeleton className="ml-[2.5ch] h-2.5" style={{ width: `${w}ch` }} /> : null}
            </div>
          ))}
          <div className="h-3" />
        </div>
      ))}
    </div>
  )
}

function RowView({ row: r, hl, hunk, numW, viewportW, hunkSelected, marked, focused, cursor, inRange, ranged, onComment }: RowProps) {
  // Bars in the gutter: marked hunks, then the selected one (internal/ui drawHunk).
  const bar = (
    <span
      className={cn(
        "sticky left-0 z-10 w-1 shrink-0 self-stretch",
        marked ? "bg-warning" : hunkSelected ? (focused ? "bg-primary" : "bg-primary/40") : "bg-transparent",
      )}
    />
  )
  const stick = { width: viewportW ? viewportW - 4 : undefined }
  switch (r.type) {
    case "file":
      return (
        <div className="sticky left-0 flex h-8 items-center gap-2 border-y bg-muted/60 px-3 font-sans backdrop-blur-sm" style={stick}>
          <StatusBadge status={r.status} />
          <PathLabel path={r.path} className="text-xs font-medium" />
          <span className="ml-auto flex shrink-0 items-center gap-1.5 font-mono text-[11px] tabular-nums">
            {r.added > 0 ? <span className="text-success-foreground dark:text-success">+{r.added}</span> : null}
            {r.removed > 0 ? <span className="text-destructive-foreground dark:text-destructive">−{r.removed}</span> : null}
          </span>
        </div>
      )
    case "gap":
      return null
    case "info":
      return (
        <p className="sticky left-0 px-3 py-1.5 font-sans text-sm text-muted-foreground" style={stick}>
          {r.text}
        </p>
      )
    case "hunk":
      return (
        <div
          data-testid="hunk-header"
          className={cn(
            "flex h-6 cursor-pointer items-center text-info-foreground dark:text-info",
            hunkSelected && focused ? "bg-primary/12" : "bg-info/6 hover:bg-info/10",
          )}
        >
          {bar}
          <span className="sticky left-1 flex min-w-0 items-center gap-2 px-2" style={stick}>
            <span className="truncate">{r.header}</span>
            {marked ? (
              <Badge size="xs" variant="warning-light" className="font-sans">
                ● marked
              </Badge>
            ) : null}
          </span>
        </div>
      )
    case "note":
      return (
        <div className="flex py-1">
          {bar}
          <div className="sticky left-1 pr-3" style={{ ...stick, paddingLeft: `calc(${numW * 2 + 3}ch + 4px)` }}>
            <div className="font-sans">
              <CommentThread
                comment={r.comment}
                outdated={r.outdated}
                selected={cursor}
                onEdit={() => onComment("edit")}
                onReply={() => onComment("reply")}
                onToggleResolved={() => onComment("resolve")}
                onDelete={() => onComment("delete")}
              />
            </div>
          </div>
        </div>
      )
    case "line": {
      const tokens = r.src && hl && hunk ? hl[hunk.index]?.[r.src.side]?.[r.src.i] : undefined
      const num = (n: number) => (n === 0 ? "" : String(n))
      return (
        <div
          data-testid="diff-line"
          data-sign={r.sign}
          data-range={inRange || undefined}
          className={cn(
            "flex h-5 items-stretch leading-5 whitespace-pre",
            // ReUI code-block's diff tints: 8% light, 12% dark. Left off the cursor and range,
            // whose backgrounds the dark: tint would otherwise win over.
            !cursor && !inRange && r.sign === "+" && "bg-success/8 dark:bg-success/12",
            !cursor && !inRange && r.sign === "-" && "bg-destructive/8 dark:bg-destructive/12",
            cursor && (focused ? "bg-primary/15 ring-1 ring-primary/60 ring-inset" : "bg-primary/8"),
            inRange && "bg-warning/18",
            inRange && cursor && "bg-warning/28 ring-warning/70",
          )}
        >
          {bar}
          <span
            className={cn(
              "sticky left-1 z-10 flex shrink-0 justify-end gap-[1ch] border-r bg-muted px-[0.5ch] text-muted-foreground/70 tabular-nums select-none",
              r.sign === "+" && "bg-[color-mix(in_oklab,var(--success)_10%,var(--muted))] text-success-foreground/70 dark:text-success/70",
              r.sign === "-" && "bg-[color-mix(in_oklab,var(--destructive)_10%,var(--muted))] text-destructive-foreground/70 dark:text-destructive/70",
              cursor && focused && "text-foreground",
            )}
            style={{ width: `${numW * 2 + 2}ch` }}
          >
            <span className="text-right" style={{ width: `${numW}ch` }}>{num(r.old)}</span>
            <span className="text-right" style={{ width: `${numW}ch` }}>{num(r.new)}</span>
          </span>
          <span
            className={cn(
              "w-[2ch] shrink-0 pl-[0.5ch] select-none",
              r.sign === "+" && "text-success-foreground dark:text-success",
              r.sign === "-" && "text-destructive-foreground dark:text-destructive",
              cursor && ranged && "text-warning-foreground dark:text-warning",
            )}
          >
            {r.sign === "\\" ? " " : r.sign}
          </span>
          <span className={cn("pr-4", r.sign === "\\" && "text-muted-foreground italic")}>
            {tokens
              ? tokens.map(([cls, text], i) => (
                  <span key={i} className={cls ? `tok-${cls}` : undefined}>
                    {text}
                  </span>
                ))
              : expandTabs(r.code) || " "}
          </span>
        </div>
      )
    }
  }
}
