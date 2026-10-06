import { toast } from "sonner"
import type { OpResult, Workspace } from "@/api"
import { startTarget } from "@/components/target/store"
import { workspaceKey } from "@/queries"
import type { Selection } from "@/selection"
import { describe, describeAll, findCommit, sourcesFor, type Plan, type Verb } from "@/target"
import { formatKey } from "./keys"
import { anyOf, hasUncommitted, ids, one, registerActions, type ActionContext } from "./registry"
import { toastError } from "./toast"

// The commit verbs of internal/ui/actions.go: c, r, m and p pick a target in the lanes
// (components/target), A absorbs, n inserts an empty commit, enter rewords, x discards.

/** runPlan runs what confirming a target does: the composer first when it needs a message. */
export async function runPlan(ctx: ActionContext, p: Plan) {
  const o = p.op
  switch (o.op) {
    case "commit": {
      let message = ""
      if (!o.empty) {
        const m = await ctx.dialogs.prompt({
          title: "Commit",
          description: `${p.desc}.`,
          placeholder: "Commit message",
          multiline: true,
          submitLabel: "Commit",
          hint: `${formatKey("mod+enter")} to commit`,
          validate: (v) => (v.trim() ? null : "A commit needs a message"),
        })
        if (m === null) return
        message = m.trim()
      }
      await ctx.runOp(p.desc, () =>
        ctx.ops.run("commit", { changes: o.changes, message, placement: o.placement }),
      )
      return
    }
    case "amend":
      await ctx.runOp(p.desc, () => ctx.ops.run("amend", { target: o.target, changes: o.changes }))
      return
    case "uncommit":
      await ctx.runOp(p.desc, () => ctx.ops.run("uncommit", { sources: o.sources }))
      return
    case "squash": {
      let message: string | undefined
      if (o.combined !== undefined) {
        const m = await ctx.dialogs.prompt({
          title: "Squash",
          description: `${p.desc}. The messages are combined; edit them into one.`,
          initial: o.combined,
          multiline: true,
          submitLabel: "Squash",
          hint: `${formatKey("mod+enter")} to squash`,
          validate: (v) => (v.trim() ? null : "A commit needs a message"),
        })
        if (m === null) return
        message = m.trim()
      }
      await ctx.runOp(p.desc, () =>
        ctx.ops.run("squash", { sources: o.sources, target: o.target, mode: o.mode, message }),
      )
      return
    }
    case "move":
      await ctx.runOp(p.desc, () => ctx.ops.run("move", { sources: o.sources, placement: o.placement }))
      return
    case "pick":
      await ctx.runOp(p.desc, () => ctx.ops.run("pick", { sources: o.sources, placement: o.placement }))
      return
  }
}

/** runUndoable runs a destructive op and offers Undo in its toast. */
async function runUndoable(
  ctx: ActionContext,
  title: string,
  fn: () => Promise<OpResult>,
  describeResult?: (res: OpResult) => string | undefined,
) {
  let res: OpResult
  try {
    res = await fn()
  } catch (err) {
    toastError(err)
    return
  }
  toast.success(title, {
    description: describeResult?.(res),
    classNames: { description: "whitespace-pre-line" },
    action: {
      label: "Undo",
      onClick: () => void ctx.runOp("Undone", () => ctx.ops.run("undo")),
    },
  })
}

/** verbAvailable mirrors the TUI's: the subjects can be sources, and commit needs changes. */
function verbAvailable(v: Verb) {
  return (ctx: ActionContext) =>
    !("why" in sourcesFor(v, ctx.subjects)) && (v !== "commit" || hasUncommitted(ctx.workspace))
}

function enterTarget(v: Verb) {
  return (ctx: ActionContext) => {
    const why = startTarget(v, ctx.subjects)
    if (why) ctx.notify(why)
  }
}

function shortId(ws: Workspace, id: string): string {
  return findCommit(ws, id)?.commit.commitId?.slice(0, 7) ?? id
}

function listLabels(subjects: Selection[]): string {
  const lines = subjects.slice(0, 8).map((s) => `• ${describe(s)}`)
  if (subjects.length > 8) lines.push(`… and ${subjects.length - 8} more`)
  return lines.join("\n")
}

function discardText(ctx: ActionContext): { title: string; body: string } {
  const [e] = ctx.subjects
  if (e.kind === "unstaged") {
    return {
      title: "Discard all uncommitted changes?",
      body: "Every uncommitted change in the workspace is thrown away.",
    }
  }
  if (ctx.subjects.length > 1) {
    return { title: `Discard ${describeAll(ctx.subjects)}?`, body: listLabels(ctx.subjects) }
  }
  switch (e.kind) {
    case "branch":
      return {
        title: `Discard branch ${e.name}?`,
        body: "The branch and all of its commits are removed from the workspace.",
      }
    case "commit":
      return {
        title: `Discard commit ${shortId(ctx.workspace, e.id)}?`,
        body: `“${e.subject}” and its changes are removed.`,
      }
    case "cfile":
      return {
        title: `Discard changes to ${e.path}?`,
        body: `The changes to this file are removed from commit ${shortId(ctx.workspace, e.commit)}.`,
      }
    default:
      return { title: `Discard ${describe(e)}?`, body: "The uncommitted changes are thrown away." }
  }
}

/** uncommittedPaths are the paths the subjects stand for: every change for Unstaged. */
function uncommittedPaths(ws: Workspace, subjects: Selection[]): string[] {
  if (subjects.some((s) => s.kind === "unstaged")) return ws.uncommittedChanges.map((c) => c.filePath)
  return subjects.flatMap((s) => (s.kind === "file" ? [s.path] : []))
}

/**
 * absorbResult is what `but absorb` printed (where each change went), without its hints.
 * Without output, it says which paths were absorbed and which had no commit to go to.
 */
function absorbResult(ctx: ActionContext, paths: string[], res: OpResult): string | undefined {
  // The API already drops the notice `but` prints under a coding agent (stripAgentNotice).
  const printed = (res.output ?? "")
    .split("\n")
    .filter((l) => l.trim() && !/^hint:/i.test(l.trim()))
    .join("\n")
  if (printed) return printed
  const after = ctx.queryClient.getQueryData<Workspace>(workspaceKey(ctx.ops.cfg.url))
  if (!after) return undefined
  const left = new Set(after.uncommittedChanges.map((c) => c.filePath))
  const absorbed = paths.filter((p) => !left.has(p))
  const stayed = paths.filter((p) => left.has(p))
  const parts: string[] = []
  if (absorbed.length > 0) parts.push(`Absorbed ${absorbed.join(", ")}.`)
  if (stayed.length > 0) parts.push(`No matching commit, still uncommitted: ${stayed.join(", ")}.`)
  return parts.join(" ") || undefined
}

registerActions(
  {
    id: "commit",
    title: "Commit…",
    group: "Commit",
    keys: ["c"],
    when: verbAvailable("commit"),
    run: enterTarget("commit"),
  },
  {
    id: "squash",
    title: "Squash / amend…",
    group: "Commit",
    keys: ["r"],
    when: verbAvailable("squash"),
    run: enterTarget("squash"),
  },
  {
    id: "absorb",
    title: "Absorb into matching commits",
    group: "Commit",
    keys: ["A"],
    when: (ctx) => hasUncommitted(ctx.workspace) && anyOf("unstaged", "file")(ctx),
    run: async (ctx) => {
      const what = describeAll(ctx.subjects)
      const ok = await ctx.dialogs.confirm({
        title: `Absorb ${what}?`,
        body:
          "Each change is amended into the commit that last touched those lines. " +
          "Changes with no matching commit stay uncommitted.\n\nYou can bring it back with Undo.",
        confirmLabel: "Absorb",
      })
      if (!ok) return
      const paths = uncommittedPaths(ctx.workspace, ctx.subjects)
      const sources = ids(ctx.subjects)
      await runUndoable(
        ctx,
        `Absorbed ${what}`,
        () => ctx.ops.run("absorb", { sources: sources.length > 0 ? sources : undefined }),
        (res) => absorbResult(ctx, paths, res),
      )
    },
  },
  {
    id: "empty-commit",
    title: "Insert empty commit",
    group: "Commit",
    keys: ["n"],
    when: one("branch", "commit"),
    run: (ctx) => {
      const [s] = ctx.subjects
      const placement = s.kind === "commit" ? { above: s.id } : s.kind === "branch" ? { branch: s.name } : {}
      void ctx.runOp("Inserted an empty commit", () => ctx.ops.run("emptyCommit", { placement }))
    },
  },
  {
    id: "reword",
    title: "Reword commit…",
    group: "Commit",
    keys: ["enter"],
    // Branches are renamed by branch.rename (branches.ts), also on enter.
    when: one("commit"),
    run: async (ctx) => {
      const [s] = ctx.subjects
      if (s.kind !== "commit") return
      const current = findCommit(ctx.workspace, s.id)?.commit.message ?? s.subject
      const message = await ctx.dialogs.prompt({
        title: "Reword commit",
        description: `Reword ${describe(s)}. The first line is the subject.`,
        initial: current.trim(),
        multiline: true,
        submitLabel: "Reword",
        hint: `${formatKey("mod+enter")} to save`,
        validate: (v) =>
          v.trim() ? null : "A commit message can't be empty; use Discard to drop the commit",
      })
      if (message === null || message.trim() === current.trim()) return
      await ctx.runOp("Reworded commit", () =>
        ctx.ops.run("reword", { target: s.id, message: message.trim() }),
      )
    },
  },
  {
    id: "discard",
    title: "Discard…",
    group: "Commit",
    keys: ["x"],
    when: (ctx) =>
      ctx.subjects.length > 0 && (ctx.subjects[0].kind !== "unstaged" || hasUncommitted(ctx.workspace)),
    run: async (ctx) => {
      const { title, body } = discardText(ctx)
      const ok = await ctx.dialogs.confirm({
        title,
        body: `${body}\n\nYou can bring it back with Undo.`,
        confirmLabel: "Discard",
        destructive: true,
      })
      if (!ok) return
      const targets = ids(ctx.subjects)
      await runUndoable(ctx, title.replace(/\?$/, ""), () =>
        ctx.ops.run("discard", { targets: targets.length > 0 ? targets : undefined }),
      )
    },
  },
  {
    id: "move",
    title: "Move…",
    group: "Branch",
    keys: ["m"],
    when: verbAvailable("move"),
    run: enterTarget("move"),
  },
  {
    id: "pick",
    title: "Cherry-pick…",
    group: "Branch",
    keys: ["p"],
    when: verbAvailable("pick"),
    run: enterTarget("pick"),
  },
)
