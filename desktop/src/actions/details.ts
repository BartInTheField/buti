import { commitSubject, type Commit, type Workspace } from "@/api"
import type { PickItem } from "@/dialogs/dialogs"
import { copyText } from "@/components/details/clipboard"
import { detailsPane, getDetailsView, paneActionIds, setDetailsView } from "@/components/details/store"
import { hunkSubject, type HunkRef } from "@/components/details/rows"
import type { Selection } from "@/selection"
import { always, one, registerActions, type ActionContext } from "./registry"

// The details pane: its line cursor, hunks, layout, the commit file lists (f / F), copy
// and go to. Mirrors the View actions in internal/ui/actions.go.

/** commitKey identifies a commit across reloads (internal/ui commitKey). */
export function commitKey(c: Commit): string {
  return c.changeId || c.commitId || c.cliId
}

export function findCommit(ws: Workspace, cliId: string): { commit: Commit; branch: string } | null {
  for (const st of ws.stacks) {
    for (const b of st.branches) {
      const commit = b.commits.find((c) => c.cliId === cliId)
      if (commit) return { commit, branch: b.name }
    }
  }
  return null
}

const pane = () => detailsPane.get()
const hasCursor = () => pane()?.hasCursor() ?? false

function hunkHeaderOf(h: HunkRef) {
  return `${h.path} ${h.header}`
}

/** hunkOf finds a hunk subject's hunk in the diff on screen. */
function hunkOf(s: Selection): HunkRef | null {
  if (s.kind !== "file" || !s.hunk) return null
  return pane()?.layout()?.hunks.find((h) => h.id === s.id) ?? null
}

function absPath(ctx: ActionContext, p: string): string {
  const repo = ctx.workspace.repo.replace(/[/\\]$/, "")
  return p.startsWith("/") || !repo ? p : `${repo}/${p}`
}

async function copy(ctx: ActionContext, text: string, what: string) {
  await copyText(text)
  ctx.notify(`Copied ${what}`)
}

const copyable = one("branch", "commit", "file", "cfile")

/** goTo selects an item and reveals it, like selectEntity in the TUI. */
function goTo(ctx: ActionContext, item: Selection) {
  ctx.sel.clearMarks()
  if (item.kind === "cfile") {
    const found = findCommit(ctx.workspace, item.commit)
    if (found) setDetailsView((v) => ({ showFiles: { ...v.showFiles, [commitKey(found.commit)]: true } }))
  }
  ctx.sel.select(item)
  if (item.kind === "branch") {
    document.querySelector(`[data-branch="${CSS.escape(item.name)}"]`)?.scrollIntoView({ block: "nearest", inline: "nearest" })
  }
}

registerActions(
  {
    id: "details.mark-hunk",
    title: "Mark / unmark hunk",
    group: "View",
    keys: ["space"],
    when: () => Boolean(pane()?.cursorHunk()?.id),
    run: (ctx) => {
      const h = pane()?.cursorHunk()
      if (!h?.id) return
      const err = ctx.sel.toggleMark(hunkSubject(h))
      if (err) return ctx.notify(err)
      pane()?.stepHunk(1)
    },
  },
  {
    id: "details.range",
    title: "Select a range of lines",
    group: "View",
    keys: ["v"],
    when: hasCursor,
    run: () => pane()?.toggleRange(),
  },
  {
    id: "details.next-hunk",
    title: "Next hunk",
    group: "View",
    keys: ["]"],
    global: true,
    when: hasCursor,
    run: () => pane()?.stepHunk(1),
  },
  {
    id: "details.prev-hunk",
    title: "Previous hunk",
    group: "View",
    keys: ["["],
    global: true,
    when: hasCursor,
    run: () => pane()?.stepHunk(-1),
  },
  {
    id: "details.files",
    title: "Show files in commit",
    group: "View",
    keys: ["f"],
    when: one("commit", "cfile"),
    run: (ctx) => {
      const s = ctx.subjects[0]
      const found = findCommit(ctx.workspace, s.kind === "cfile" ? s.commit : s.kind === "commit" ? s.id : "")
      if (!found || (found.commit.changes?.length ?? 0) === 0) return ctx.notify("This commit has no files")
      const k = commitKey(found.commit)
      const showing = Boolean(getDetailsView().showFiles[k])
      setDetailsView((v) => ({ showFiles: { ...v.showFiles, [k]: !showing } }))
      if (showing && s.kind === "cfile") {
        const { commit, branch } = found
        ctx.sel.select({ kind: "commit", id: commit.cliId, subject: commitSubject(commit.message), branch })
      }
    },
  },
  {
    id: "details.all-files",
    title: "Show files in all commits",
    group: "View",
    keys: ["F"],
    global: true,
    when: always,
    run: () => setDetailsView((v) => ({ showAllFiles: !v.showAllFiles })),
  },
  {
    id: "details.toggle",
    title: "Toggle details pane",
    group: "View",
    keys: ["d"],
    global: true,
    when: always,
    run: () => setDetailsView((v) => (v.full ? { full: false, visible: true } : { visible: !v.visible })),
  },
  {
    id: "details.full",
    title: "Full-screen details",
    group: "View",
    keys: ["D"],
    global: true,
    when: always,
    run: () => {
      setDetailsView((v) => ({ full: !v.full }))
      if (getDetailsView().full) setTimeout(() => pane()?.focus(), 0)
    },
  },
  {
    id: "details.leave-full",
    title: "Leave full-screen details",
    group: "View",
    keys: ["escape"],
    global: true,
    when: () => getDetailsView().full,
    run: () => setDetailsView({ full: false }),
  },
  {
    id: "details.tree",
    title: "Show / hide the file tree",
    group: "View",
    keys: ["T"],
    global: true,
    when: () => getDetailsView().full,
    run: () => setDetailsView((v) => ({ tree: !v.tree })),
  },
  {
    id: "copy",
    title: "Copy",
    group: "View",
    keys: ["y"],
    when: copyable,
    run: async (ctx) => {
      const s = ctx.subjects[0]
      switch (s.kind) {
        case "branch":
          return copy(ctx, s.name, s.name)
        case "commit": {
          const c = findCommit(ctx.workspace, s.id)?.commit
          const id = c?.changeId && c.changeId.length >= 8 ? c.changeId.slice(0, 8) : (c?.commitId?.slice(0, 7) ?? s.id)
          return copy(ctx, id, id)
        }
        case "file": {
          const h = hunkOf(s)
          if (h) return copy(ctx, h.text, hunkHeaderOf(h))
          return copy(ctx, s.path, s.path)
        }
        case "cfile":
          return copy(ctx, s.path, s.path)
      }
    },
  },
  {
    id: "copy-pick",
    title: "Copy…",
    group: "View",
    keys: ["Y"],
    when: copyable,
    run: async (ctx) => {
      const s = ctx.subjects[0]
      const items: PickItem<string>[] = []
      const add = (label: string, value: string | undefined) => {
        if (value) items.push({ value, label, detail: value.split("\n")[0] })
      }
      if (s.kind === "commit") {
        const c = findCommit(ctx.workspace, s.id)?.commit
        add("Commit ID", c?.commitId)
        add("Short commit ID", c?.commitId?.slice(0, 7))
        add("Change ID", c?.changeId)
        add("Message title", c ? commitSubject(c.message) : undefined)
        add("Whole message", c?.message.trim())
        add("Author", c?.authorName ? `${c.authorName} <${c.authorEmail ?? ""}>` : undefined)
      } else if (s.kind === "branch") {
        add("Branch name", s.name)
        add("Short ID", s.id)
      } else if (s.kind !== "unstaged") {
        add("File path", absPath(ctx, s.path))
        add("Relative path", s.path)
        add("Short ID", s.id)
      }
      // The hunk under the line cursor, whatever the diff is of.
      const h = (s.kind === "file" && hunkOf(s)) || pane()?.cursorHunk()
      if (h) add("Hunk", h.text)
      const lines = pane()?.lineSelection()
      if (lines) add(lines.endLine > lines.line ? "Selected lines" : "Line", lines.text)
      const picked = await ctx.dialogs.pick({ title: "Copy", items })
      if (picked !== null) {
        const it = items.find((i) => i.value === picked)
        await copy(ctx, picked, (it?.label ?? "text").toLowerCase())
      }
    },
  },
  {
    id: "goto",
    title: "Go to…",
    group: "View",
    keys: ["/"],
    global: true,
    when: always,
    run: async (ctx) => {
      const ws = ctx.workspace
      const items: PickItem<Selection>[] = [
        { value: { kind: "unstaged" }, label: "Unstaged changes", detail: "zz", group: "Uncommitted" },
      ]
      for (const c of ws.uncommittedChanges) {
        items.push({ value: { kind: "file", id: c.cliId, path: c.filePath }, label: c.filePath, detail: c.cliId, group: "Uncommitted" })
      }
      for (const st of ws.stacks) {
        for (const b of [...st.branches].reverse()) {
          items.push({ value: { kind: "branch", id: b.cliId, name: b.name }, label: `⑂ ${b.name}`, detail: b.cliId, group: "Branches & commits" })
          for (const c of b.commits) {
            const subject = commitSubject(c.message)
            items.push({
              value: { kind: "commit", id: c.cliId, subject, branch: b.name },
              label: `● ${subject}`,
              detail: `${c.cliId} ${c.commitId?.slice(0, 7) ?? ""}`,
              keywords: b.name,
              group: "Branches & commits",
            })
            for (const ch of c.changes ?? []) {
              items.push({
                value: { kind: "cfile", id: ch.cliId, path: ch.filePath, commit: c.cliId, branch: b.name },
                label: `    ${ch.filePath}`,
                detail: ch.cliId,
                keywords: `${subject} ${b.name}`,
                group: "Branches & commits",
              })
            }
          }
        }
      }
      const picked = await ctx.dialogs.pick({ title: "Go to", placeholder: "Files, branches and commits", items })
      if (picked) goTo(ctx, picked)
    },
  },
)

paneActionIds.add("details.mark-hunk")
