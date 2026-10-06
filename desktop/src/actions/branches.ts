import { toast } from "sonner"
import { branchPR, fetchBranches, fetchReviewURL, type Branch, type Placement, type Workspace } from "@/api"
import { canPush, humanizeOp, relTime } from "@/components/branches/format"
import { askPullRequest, setOplogOpen } from "@/components/branches/store"
import type { PickItem } from "@/dialogs/dialogs"
import { workspaceKey } from "@/queries"
import type { Selection } from "@/selection"
import { always, anyOf, one, registerActions, type ActionContext } from "./registry"
import { toastError } from "./toast"

// Branch lifecycle and history: the TUI's Branch and History groups (internal/ui/actions.go).

/** branchOf is the branch a branch or commit subject belongs to. */
function branchOf(s: Selection | undefined): string | null {
  if (s?.kind === "branch") return s.name
  if (s?.kind === "commit") return s.branch
  return null
}

function findBranch(ws: Workspace, name: string): Branch | undefined {
  for (const st of ws.stacks) {
    const b = st.branches.find((b) => b.name === name)
    if (b) return b
  }
  return undefined
}

/** latestWorkspace is the freshest workspace in the cache: after an op it has the change. */
function latestWorkspace(ctx: ActionContext): Workspace {
  return ctx.queryClient.getQueryData<Workspace>(workspaceKey(ctx.ops.cfg.url)) ?? ctx.workspace
}

/** revealBranch selects a branch and scrolls its card into view, like the TUI's selectEntity. */
export function revealBranch(ctx: ActionContext, name: string) {
  const b = findBranch(latestWorkspace(ctx), name)
  if (!b) return
  ctx.sel.clearMarks()
  ctx.sel.select({ kind: "branch", id: b.cliId, name: b.name })
  requestAnimationFrame(() => {
    document
      .querySelector(`[data-testid="branch-card"][data-branch="${CSS.escape(name)}"]`)
      ?.scrollIntoView({ block: "nearest", inline: "nearest" })
  })
}

/** openUrl opens a link in the system browser: Tauri's opener in the window, a new tab in a browser. */
async function openUrl(url: string) {
  try {
    const { invoke, isTauri } = await import("@tauri-apps/api/core")
    if (isTauri()) {
      await invoke("plugin:opener|open_url", { url })
      return
    }
  } catch {
    // Fall through to the browser.
  }
  window.open(url, "_blank", "noopener,noreferrer")
}

/** dashes joins whitespace with dashes, as the server and the TUI do for branch names. */
const dashes = (s: string) => s.trim().split(/\s+/).filter(Boolean).join("-")

async function newBranch(ctx: ActionContext, placement: Placement, where: string) {
  const name = await ctx.dialogs.prompt({
    title: `New branch ${where}`,
    placeholder: "Leave empty for a generated name",
    hint: "Spaces become dashes.",
    submitLabel: "Create",
  })
  if (name === null) return
  const clean = dashes(name)
  const ok = await ctx.runOp(clean ? `Created branch ${clean}` : "Created a branch", () =>
    ctx.ops.run("branchNew", { name: clean, placement }),
  )
  if (ok && clean) revealBranch(ctx, clean)
}

async function push(ctx: ActionContext, name: string) {
  const status = findBranch(ctx.workspace, name)?.branchStatus
  if (!canPush(status)) {
    ctx.notify(`${name} has nothing to push`)
    return
  }
  let force = false
  if (status === "unpushedCommitsRequiringForce") {
    force = await ctx.dialogs.confirm({
      title: `Force push ${name}?`,
      body: "The remote branch has diverged; pushing rewrites it.",
      confirmLabel: "Force push",
      destructive: true,
    })
    if (!force) return
  }
  await ctx.runOp(force ? `Force pushed ${name}` : `Pushed ${name}`, () =>
    ctx.ops.run("push", { branch: name, force }),
  )
}

async function applyPicker(ctx: ActionContext) {
  const branches = await ctx.queryClient.fetchQuery({
    queryKey: ["branches", ctx.ops.cfg.url],
    queryFn: () => fetchBranches(ctx.ops.cfg),
    staleTime: 0,
  })
  const list = [...(branches.branches ?? [])].sort((a, b) => b.lastCommitAt - a.lastCommitAt)
  const items: PickItem<string>[] = list.map((b) => ({
    value: b.name,
    label: b.name,
    detail: [b.hasLocal ? "local" : "remote", b.lastCommitAt > 0 ? relTime(b.lastCommitAt) : ""]
      .filter(Boolean)
      .join(" · "),
    keywords: b.lastAuthor?.name,
  }))
  const name = await ctx.dialogs.pick({
    title: "Apply branch",
    placeholder: "Apply branch · type to search",
    items,
    empty: "No unapplied branches",
  })
  if (!name) return
  const ok = await ctx.runOp(`Applied ${name}`, () => ctx.ops.run("apply", { branch: name }))
  // A remote branch comes back under its local name (origin/x → x).
  if (ok) revealBranch(ctx, name.replace(/^[^/]+\//, ""))
}

async function gotoBranch(ctx: ActionContext) {
  type Target = { kind: "unstaged" } | { kind: "branch"; name: string }
  const items: PickItem<Target>[] = [
    { value: { kind: "unstaged" }, label: "Unstaged changes", detail: "workspace" },
  ]
  for (const st of ctx.workspace.stacks) {
    for (const b of st.branches) {
      items.push({
        value: { kind: "branch", name: b.name },
        label: b.name,
        detail: b.branchStatus ? humanizeOp(b.branchStatus[0].toUpperCase() + b.branchStatus.slice(1)) : undefined,
      })
    }
  }
  const t = await ctx.dialogs.pick({ title: "Go to branch", items })
  if (!t) return
  if (t.kind === "branch") return revealBranch(ctx, t.name)
  ctx.sel.clearMarks()
  ctx.sel.select({ kind: "unstaged" })
}

async function openPR(ctx: ActionContext, name: string, pr: string) {
  const url = await fetchReviewURL(ctx.ops.cfg, name)
  if (!url) throw new Error(`No pull request found for ${name}`)
  await openUrl(url)
  ctx.notify(`Opened ${pr} in the browser`)
}

/**
 * undoRedo runs `but undo`/`but redo` and names the operation in the toast: `but` prints
 * "Undid 5ea9a4a (2026-01-01 12:31:00): Created branch".
 */
async function undoRedo(ctx: ActionContext, op: "undo" | "redo") {
  let output = ""
  try {
    output = (await ctx.ops.run(op)).output ?? ""
  } catch (err) {
    toastError(err)
    return
  }
  const m = /^(Undid|Redid) (\S+) \([^)]*\): (.+)$/m.exec(output)
  if (m) {
    const again = op === "undo" ? "U redoes it" : "u undoes it again"
    toast.success(`${m[1]} “${m[3]}”`, { description: `Snapshot ${m[2].slice(0, 7)} · ${again}` })
    return
  }
  const last = output.split("\n").map((l) => l.trim()).filter(Boolean).pop()
  toast.message(last ?? (op === "undo" ? "Undone" : "Redone"))
}

const oneBranch = one("branch")
const branchName = (ctx: ActionContext) => branchOf(ctx.subjects[0]) ?? ""
const prOf = (ctx: ActionContext) => branchPR(findBranch(ctx.workspace, branchName(ctx))?.reviewId)

registerActions(
  {
    id: "branch.new",
    title: "New branch…",
    group: "Branch",
    keys: ["b"],
    when: always,
    run: (ctx) => {
      // On a branch or commit: stacked on top of that branch; otherwise a new lane.
      const name = one("branch", "commit")(ctx) ? branchOf(ctx.subjects[0]) : null
      const b = name ? findBranch(ctx.workspace, name) : undefined
      if (b) return newBranch(ctx, { above: b.cliId }, `stacked on ${b.name}`)
      return newBranch(ctx, {}, "as a new lane")
    },
  },
  {
    id: "branch.new-below",
    title: "New branch below…",
    group: "Branch",
    keys: ["B"],
    when: oneBranch,
    run: (ctx) => {
      const [s] = ctx.subjects
      return newBranch(ctx, { below: s.kind === "branch" ? s.id : "" }, `below ${branchName(ctx)}`)
    },
  },
  {
    id: "branch.new-lane",
    title: "New branch in a new lane…",
    group: "Branch",
    global: true,
    when: always,
    run: (ctx) => newBranch(ctx, {}, "as a new lane"),
  },
  {
    id: "branch.rename",
    title: "Rename branch…",
    group: "Branch",
    keys: ["enter"],
    when: oneBranch,
    run: async (ctx) => {
      const [s] = ctx.subjects
      if (s.kind !== "branch") return
      const name = await ctx.dialogs.prompt({
        title: "Rename branch",
        initial: s.name,
        placeholder: "branch-name",
        hint: "Spaces become dashes.",
        submitLabel: "Rename",
        validate: (v) => (dashes(v) ? null : "A branch needs a name"),
      })
      const clean = dashes(name ?? "")
      if (!clean || clean === s.name) return
      const ok = await ctx.runOp(`Renamed ${s.name} → ${clean}`, () =>
        ctx.ops.run("reword", { target: s.id, message: clean }),
      )
      if (ok) revealBranch(ctx, clean)
    },
  },
  {
    id: "branch.delete",
    title: "Delete branch…",
    group: "Branch",
    when: anyOf("branch"),
    run: async (ctx) => {
      const names = ctx.subjects.flatMap((s) => (s.kind === "branch" ? [s.name] : []))
      const what = names.length === 1 ? `branch ${names[0]}` : `${names.length} branches`
      const ok = await ctx.dialogs.confirm({
        title: `Delete ${what}?`,
        body:
          (names.length === 1
            ? "The branch and all of its commits are removed from the workspace."
            : names.map((n) => `• ${n}`).join("\n")) + "\n\nYou can bring it back with undo (u).",
        confirmLabel: "Delete",
        destructive: true,
      })
      if (ok) await ctx.runOp(`Deleted ${what}`, () => ctx.ops.run("branchDelete", { branches: names }))
    },
  },
  {
    id: "branch.push",
    title: "Push branch",
    group: "Branch",
    keys: ["P"],
    when: one("branch", "commit"),
    run: (ctx) => push(ctx, branchName(ctx)),
  },
  {
    id: "branch.pr-new",
    title: "Create pull request…",
    group: "Branch",
    keys: ["N"],
    when: (ctx) => oneBranch(ctx) && !prOf(ctx),
    run: async (ctx) => {
      const name = branchName(ctx)
      const pr = await askPullRequest(name)
      if (!pr) return
      await ctx.runOp(`Created ${pr.draft ? "a draft " : "a "}pull request for ${name}`, () =>
        ctx.ops.run("prNew", { branch: name, message: pr.message, draft: pr.draft }),
      )
    },
  },
  {
    id: "branch.open-pr",
    title: "Open pull request",
    group: "Branch",
    keys: ["o"],
    when: (ctx) => oneBranch(ctx) && Boolean(prOf(ctx)),
    run: (ctx) => openPR(ctx, branchName(ctx), prOf(ctx)),
  },
  {
    id: "branch.apply",
    title: "Apply branch…",
    group: "Branch",
    keys: ["a"],
    global: true,
    when: always,
    run: applyPicker,
  },
  {
    id: "branch.unapply",
    title: "Unapply stack…",
    group: "Branch",
    keys: ["S"],
    when: one("branch", "commit"),
    run: async (ctx) => {
      const name = branchName(ctx)
      const ok = await ctx.dialogs.confirm({
        title: `Unapply the stack with ${name}?`,
        body: "The stack leaves the workspace; its branches stay. Apply it again with a, or undo with u.",
        confirmLabel: "Unapply",
      })
      if (!ok) return
      await ctx.runOp(`Unapplied ${name}`, () => ctx.ops.run("unapply", { branch: name }))
    },
  },
  {
    id: "branch.land",
    title: "Land branch onto target…",
    group: "Branch",
    when: oneBranch,
    run: async (ctx) => {
      const name = branchName(ctx)
      const ok = await ctx.dialogs.confirm({
        title: `Land ${name}?`,
        body: `Merges ${name} straight into the target branch and pushes it.`,
        confirmLabel: "Land",
        destructive: true,
      })
      if (!ok) return
      if (await ctx.runOp(`Landed ${name}`, () => ctx.ops.run("land", { branch: name }))) {
        ctx.sel.select({ kind: "unstaged" })
      }
    },
  },
  {
    id: "branch.pull",
    title: "Pull (update from upstream)",
    group: "Branch",
    keys: ["L"],
    global: true,
    when: always,
    run: (ctx) => void ctx.runOp("Pulled upstream changes", () => ctx.ops.run("pull")),
  },
  {
    id: "branch.clean",
    title: "Clean up empty branches",
    group: "Branch",
    global: true,
    when: always,
    run: (ctx) => void ctx.runOp("Cleaned up empty branches", () => ctx.ops.run("clean")),
  },
  {
    id: "history.undo",
    title: "Undo",
    group: "History",
    keys: ["u"],
    global: true,
    when: always,
    run: (ctx) => undoRedo(ctx, "undo"),
  },
  {
    id: "history.redo",
    title: "Redo",
    group: "History",
    keys: ["U"],
    global: true,
    when: always,
    run: (ctx) => undoRedo(ctx, "redo"),
  },
  {
    id: "history.oplog",
    title: "Operation history…",
    group: "History",
    keys: ["H"],
    global: true,
    when: always,
    run: () => setOplogOpen(true),
  },
  {
    id: "branch.goto",
    title: "Go to branch…",
    group: "View",
    keys: ["t"],
    global: true,
    when: always,
    run: gotoBranch,
  },
)
