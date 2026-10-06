import type { Placement } from "@/api"
import { commitSubject } from "@/api"
import type { PickItem } from "@/dialogs/dialogs"
import { describeSubjects, selectionKey, type Selection } from "@/selection"
import { formatKey } from "./keys"
import {
  allOf,
  always,
  anyOf,
  hasUncommitted,
  ids,
  one,
  registerActions,
  type ActionContext,
} from "./registry"

// Built-in actions: the verbs the workspace already had (commit, amend, move, uncommit,
// refresh) plus selection, palette and help. Feature modules add their own the same way.

/** branchTargets lists every applied branch, top of each lane first, for a target picker. */
export function branchTargets(ctx: ActionContext, opts: { newBranch?: boolean } = {}) {
  const items: PickItem<Placement>[] = ctx.workspace.stacks.flatMap((st) =>
    [...st.branches].reverse().map((b) => ({
      value: { branch: b.name },
      label: b.name,
      detail: `${b.commits.length} commit${b.commits.length === 1 ? "" : "s"}`,
      group: "Branches",
    })),
  )
  if (opts.newBranch) {
    items.push({ value: { newBranch: true }, label: "New branch", group: "New" })
  }
  return items
}

/** commitTargets lists every commit in the workspace, for amend and squash pickers. */
export function commitTargets(ctx: ActionContext, exclude: Selection[] = []) {
  const skip = new Set(exclude.map(selectionKey))
  return ctx.workspace.stacks.flatMap((st) =>
    st.branches.flatMap((b) =>
      b.commits
        .filter((c) => !skip.has(`commit:${c.cliId}`))
        .map(
          (c): PickItem<string> => ({
            value: c.cliId,
            label: commitSubject(c.message),
            detail: c.commitId?.slice(0, 7) ?? c.cliId,
            group: b.name,
          }),
        ),
    ),
  )
}

function placementLabel(p: Placement): string {
  return p.newBranch ? "a new branch" : (p.branch ?? "the workspace")
}

/** commitInto asks for a message and commits changes (all when empty) at placement. */
export async function commitInto(ctx: ActionContext, changes: string[], placement: Placement) {
  const message = await ctx.dialogs.prompt({
    title: "Commit",
    description: `Commit onto ${placementLabel(placement)}.`,
    placeholder: "Commit message",
    multiline: true,
    submitLabel: "Commit",
    hint: `${formatKey("mod+enter")} to commit`,
    validate: (v) => (v.trim() ? null : "A commit needs a message"),
  })
  if (message === null) return
  await ctx.runOp(`Committed to ${placementLabel(placement)}`, () =>
    ctx.ops.run("commit", { changes, message: message.trim(), placement }),
  )
}

const uncommittedKinds = ["unstaged", "file"] as const

registerActions(
  {
    id: "commit",
    title: "Commit…",
    group: "Commit",
    keys: ["c"],
    when: (ctx) =>
      hasUncommitted(ctx.workspace) &&
      (allOf(ctx.subjects, ...uncommittedKinds) || one("branch", "commit")(ctx)),
    run: async (ctx) => {
      const [s] = ctx.subjects
      // On a branch or commit, commit everything uncommitted onto that branch (like `but tui`).
      if (s.kind === "branch" || s.kind === "commit") {
        const branch = s.kind === "branch" ? s.name : s.branch
        return commitInto(ctx, [], { branch })
      }
      const placement = await ctx.dialogs.pick({
        title: `Commit ${describeSubjects(ctx.subjects)} onto`,
        items: branchTargets(ctx, { newBranch: true }),
      })
      if (placement) await commitInto(ctx, ids(ctx.subjects), placement)
    },
  },
  {
    id: "amend-into",
    title: "Amend into commit…",
    group: "Commit",
    keys: ["r"],
    when: (ctx) => anyOf("file")(ctx) && commitTargets(ctx).length > 0,
    run: async (ctx) => {
      const target = await ctx.dialogs.pick({
        title: `Amend ${describeSubjects(ctx.subjects)} into`,
        items: commitTargets(ctx),
      })
      if (target) {
        await ctx.runOp("Amended into commit", () =>
          ctx.ops.run("amend", { target, changes: ids(ctx.subjects) }),
        )
      }
    },
  },
  {
    id: "amend-all",
    title: "Amend all changes into this",
    group: "Commit",
    keys: ["R"],
    when: (ctx) => {
      if (!one("commit", "branch")(ctx) || !hasUncommitted(ctx.workspace)) return false
      const s = ctx.subjects[0]
      if (s.kind === "commit") return true
      return ctx.workspace.stacks.some((st) =>
        st.branches.some((b) => b.cliId === (s.kind === "branch" ? s.id : "") && b.commits.length > 0),
      )
    },
    run: (ctx) =>
      void ctx.runOp(`Amended all changes into ${describeSubjects(ctx.subjects)}`, () =>
        ctx.ops.run("amend", { target: ids(ctx.subjects)[0] }),
      ),
  },
  {
    id: "move",
    title: "Move…",
    group: "Branch",
    keys: ["m"],
    when: (ctx) => anyOf("commit")(ctx) || one("branch")(ctx),
    run: async (ctx) => {
      const placement = await ctx.dialogs.pick({
        title: `Move ${describeSubjects(ctx.subjects)} onto`,
        items: branchTargets(ctx, { newBranch: true }).filter(
          (it) => !ctx.subjects.some((s) => s.kind === "branch" && s.name === it.value.branch),
        ),
      })
      if (placement) {
        await ctx.runOp(`Moved onto ${placementLabel(placement)}`, () =>
          ctx.ops.run("move", { sources: ids(ctx.subjects), placement }),
        )
      }
    },
  },
  {
    id: "uncommit",
    title: "Uncommit to Unstaged",
    group: "Commit",
    when: anyOf("commit", "branch"),
    run: (ctx) =>
      void ctx.runOp(`Uncommitted ${describeSubjects(ctx.subjects)}`, () =>
        ctx.ops.run("uncommit", { sources: ids(ctx.subjects) }),
      ),
  },

  // Selection and view.
  {
    id: "mark",
    title: "Mark / unmark",
    group: "View",
    keys: ["space"],
    when: (ctx) => ctx.sel.selection !== null,
    run: (ctx) => {
      const s = ctx.sel.selection
      if (!s) return
      if (s.kind === "unstaged") {
        // Toggle every uncommitted file, as in the TUI.
        const files: Selection[] = ctx.workspace.uncommittedChanges.map((c) => ({
          kind: "file",
          id: c.cliId,
          path: c.filePath,
        }))
        const all = files.length > 0 && files.every((f) => ctx.sel.isMarked(f))
        ctx.sel.setMarks(all ? [] : files)
        return
      }
      const err = ctx.sel.toggleMark(s)
      if (err) ctx.notify(err)
    },
  },
  {
    id: "clear-marks",
    title: "Clear marks",
    group: "View",
    keys: ["escape"],
    global: true,
    resolving: true,
    when: (ctx) => ctx.sel.marks.length > 0,
    run: (ctx) => ctx.sel.clearMarks(),
  },
  {
    id: "reload",
    title: "Reload",
    group: "View",
    keys: ["ctrl+r"],
    global: true,
    resolving: true,
    when: always,
    run: (ctx) =>
      void ctx.ops.refresh({ sync: true }).catch((err: unknown) => {
        ctx.notify(err instanceof Error ? err.message : "Reload failed")
      }),
  },
  {
    id: "context-actions",
    title: "Actions for selection…",
    group: "View",
    keys: ["."],
    global: true,
    resolving: true,
    when: always,
    run: (ctx) => ctx.ui.openPalette("selection"),
  },
  {
    id: "help",
    title: "Help & all commands",
    group: "View",
    keys: ["?"],
    global: true,
    resolving: true,
    when: always,
    run: (ctx) => ctx.ui.openPalette("help"),
  },
  {
    id: "palette",
    title: "Command palette",
    group: "View",
    keys: ["mod+k", "ctrl+p"],
    global: true,
    resolving: true,
    when: always,
    run: (ctx) => ctx.ui.openPalette("all"),
  },
)
