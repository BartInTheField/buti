import type { PickItem } from "@/dialogs/dialogs"
import { commitSubject } from "@/api"
import { detailsPane, getDetailsView, paneActionIds, setDetailsView } from "@/components/details/store"
import {
  authorLabel,
  loadComments,
  commentsApi,
  invalidateComments,
  isOpen,
  where,
  type Anchor,
  type LocatedComment,
} from "@/components/review/api"
import { toastError } from "./toast"
import { always, registerActions, type ActionContext } from "./registry"

// Review comments in the diff, mirroring internal/ui/comments.go. They sit in one store
// with the TUI's and `buti review`'s, through internal/desktop/comments.go.

const pane = () => detailsPane.get()

/** paneFocused: the line cursor counts while the pane has focus or fills the window (detailsLines). */
const paneFocused = () => Boolean(pane()?.focused() || getDetailsView().full)

function onComment(): boolean {
  return paneFocused() && pane()?.cursorComment() != null
}

function canComment(ctx: ActionContext): boolean {
  const s = ctx.sel.selection
  return paneFocused() && s !== null && pane()?.cursorComment() == null && pane()?.lineSelection() != null
}

/** anchorFor turns the line selection in the diff of the selection into an anchor (internal/ui anchorFor). */
function anchorFor(ctx: ActionContext): { anchor: Anchor; what: string } | string {
  const s = ctx.sel.selection
  const lines = pane()?.lineSelection()
  if (!lines) return "Put the line cursor on a line to comment on it"
  const a: Anchor = { path: lines.path, side: lines.side, line: lines.line, end_line: lines.endLine, line_text: lines.text }
  const commitOf = (cliId: string) => {
    for (const st of ctx.workspace.stacks) {
      for (const b of st.branches) {
        const c = b.commits.find((x) => x.cliId === cliId)
        if (c) return { c, branch: b.name }
      }
    }
    return null
  }
  switch (s?.kind) {
    case "commit":
    case "cfile": {
      const found = commitOf(s.kind === "commit" ? s.id : s.commit)
      if (!found) return "That commit is no longer in the workspace"
      Object.assign(a, { kind: "commit", change_id: found.c.changeId, commit_id: found.c.commitId, branch: found.branch })
      break
    }
    case "unstaged":
    case "file": {
      a.kind = "unassigned"
      // Assigned changes are known by their stack's top branch: `but status` has no stable stack id.
      for (const st of ctx.workspace.stacks) {
        if (st.assignedChanges?.some((c) => c.filePath === lines.path) && st.branches.length > 0) {
          a.kind = "assigned"
          a.branch = st.branches[0].name
        }
      }
      break
    }
    case "branch":
      // No kind: the API puts it on the branch's commit that has the lines.
      a.branch = s.name
      return { anchor: a, what: `on ${where(a)} in ${s.name}` }
    default:
      return "Comment on a diff: uncommitted changes, a commit or a branch"
  }
  return { anchor: a, what: `on ${where(a)}` }
}

async function write(ctx: ActionContext, done: string, fn: () => Promise<unknown>): Promise<boolean> {
  try {
    await fn()
    ctx.notify(done)
    return true
  } catch (err) {
    toastError(err)
    return false
  } finally {
    await invalidateComments(ctx.queryClient, ctx.ops.cfg.url)
  }
}

export function startComment(ctx: ActionContext) {
  const at = anchorFor(ctx)
  if (typeof at === "string") return ctx.notify(at)
  pane()?.compose({
    mode: "new",
    title: `Comment ${at.what}`,
    submit: (body) =>
      write(ctx, `Comment added on ${where(at.anchor)}`, () => commentsApi.add(ctx.ops.cfg, at.anchor, body)),
  })
}

export function editComment(ctx: ActionContext, c: LocatedComment) {
  pane()?.compose({
    mode: "edit",
    title: `Edit comment on ${where(c.at)}`,
    initial: c.body,
    submit: async (body) => {
      if (body.trim() === c.body.trim()) return true
      return write(ctx, "Comment edited", () => commentsApi.edit(ctx.ops.cfg, c.id, body))
    },
  })
}

export function replyComment(ctx: ActionContext, c: LocatedComment) {
  pane()?.compose({
    mode: "reply",
    title: `Reply to the comment on ${where(c.at)}`,
    submit: (body) => write(ctx, "Reply added", () => commentsApi.reply(ctx.ops.cfg, c.id, body)),
  })
}

export async function deleteComment(ctx: ActionContext, c: LocatedComment) {
  const ok = await ctx.dialogs.confirm({
    title: `Delete comment on ${where(c.at)}?`,
    body: `“${c.body.trim().split("\n").slice(0, 3).join("\n")}”`,
    confirmLabel: "Delete",
    destructive: true,
  })
  if (ok) await write(ctx, "Comment deleted", () => commentsApi.remove(ctx.ops.cfg, c.id))
  pane()?.focus()
}

export function toggleResolved(ctx: ActionContext, c: LocatedComment) {
  return isOpen(c)
    ? write(ctx, "Comment resolved", () => commentsApi.resolve(ctx.ops.cfg, c.id))
    : write(ctx, "Comment reopened", () => commentsApi.reopen(ctx.ops.cfg, c.id))
}

/** jumpToComment selects what a comment is on and puts the line cursor on it (internal/ui jumpToComment). */
function jumpToComment(ctx: ActionContext, c: LocatedComment) {
  if (c.state === "orphaned") return ctx.notify("The commit of this comment is gone")
  const a = c.at
  let found = false
  if (a.kind === "commit") {
    for (const st of ctx.workspace.stacks) {
      for (const b of st.branches) {
        const cm = b.commits.find(
          (x) => (a.change_id && x.changeId === a.change_id) || (a.commit_id && x.commitId === a.commit_id),
        )
        if (cm) {
          ctx.sel.select({ kind: "commit", id: cm.cliId, subject: commitSubject(cm.message), branch: b.name })
          found = true
        }
      }
    }
  } else {
    const all = [...ctx.workspace.uncommittedChanges, ...ctx.workspace.stacks.flatMap((s) => s.assignedChanges ?? [])]
    const f = all.find((x) => x.filePath === a.path)
    if (f) {
      ctx.sel.select({ kind: "file", id: f.cliId, path: f.filePath })
      found = true
    }
  }
  if (!found) return ctx.notify(`${a.path} is no longer in the workspace`)
  ctx.sel.clearMarks()
  setDetailsView((v) => (v.full ? {} : { visible: true }))
  pane()?.jumpToComment(c.id)
}

const withComment = (fn: (ctx: ActionContext, c: LocatedComment) => unknown) => (ctx: ActionContext) => {
  const c = pane()?.cursorComment()
  if (c) void fn(ctx, c)
}

registerActions(
  {
    id: "review.comment",
    title: "Comment on the line…",
    group: "Review",
    keys: ["C"],
    when: canComment,
    run: startComment,
  },
  { id: "review.edit", title: "Edit comment…", group: "Review", keys: ["e"], when: onComment, run: withComment(editComment) },
  { id: "review.reply", title: "Reply to comment…", group: "Review", when: onComment, run: withComment(replyComment) },
  { id: "review.delete", title: "Delete comment…", group: "Review", keys: ["d"], when: onComment, run: withComment(deleteComment) },
  {
    id: "review.resolve",
    title: "Resolve / reopen comment",
    group: "Review",
    keys: ["x"],
    when: onComment,
    run: withComment(toggleResolved),
  },
  {
    id: "review.toggle-resolved",
    title: "Show / hide resolved comments",
    group: "Review",
    keys: ["z"],
    global: true,
    when: always,
    run: (ctx) => {
      setDetailsView((v) => ({ hideResolved: !v.hideResolved }))
      ctx.notify(getDetailsView().hideResolved ? "Resolved comments hidden" : "Resolved comments shown")
    },
  },
  {
    id: "review.list",
    title: "Review comments…",
    group: "Review",
    global: true,
    when: always,
    run: async (ctx) => {
      const items: PickItem<LocatedComment>[] = (await loadComments(ctx.queryClient, ctx.ops.cfg))
        .filter(isOpen)
        .map((c) => {
          let detail = c.target
          if (c.state === "orphaned") detail = "commit gone"
          else if (c.state === "outdated") detail = "outdated"
          else if (c.at.kind === "commit" && c.at.commit_id) detail += ` ${c.at.commit_id.slice(0, 7)}`
          const who = authorLabel(c.author)
          if (who !== "you") detail = `${who} · ${detail}`
          return {
            value: c,
            label: `${where(c.at)}  ${c.body.trim().split("\n")[0]}`,
            detail,
            keywords: `${c.id} ${c.body}`,
          }
        })
      const picked = await ctx.dialogs.pick({
        title: "Review comments · enter jumps to the line",
        placeholder: "Open review comments",
        items,
        empty: "No open comments",
      })
      if (picked) jumpToComment(ctx, picked)
    },
  },
)

for (const id of ["review.comment", "review.edit", "review.delete", "review.resolve"]) paneActionIds.add(id)
