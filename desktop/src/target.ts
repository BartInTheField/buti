// A port of internal/ui/target.go: what a verb does with its sources on each target.
// Pure functions over the workspace, shared by the target picker, the keyboard and drag-and-drop.
// Only type imports, so the e2e runner can load it without the app's path aliases.

import type { Branch, Commit, Placement, SquashMode, Workspace } from "@/api"
import type { Selection } from "@/selection"

export type Verb = "commit" | "squash" | "move" | "pick"
export const verbs: Verb[] = ["commit", "squash", "move", "pick"]

export type Side = "above" | "below"

/** A target is anything selectable, plus the "new branch" lane. */
export type TargetEntity = Selection | { kind: "new-branch" }

/** TargetState is a pending verb: targetMode in the TUI. */
export type TargetState = {
  verb: Verb
  sources: Selection[]
  side: Side
  /** squash: keep the target's message (`u`). */
  useTarget: boolean
  /** commit: skip the message composer (`e`). */
  emptyMsg: boolean
  /** commit and pick: a branch target gets a new branch stacked above it (`b`). */
  newBranchHere: boolean
}

/** PlanOp is the `but` operation a plan runs, as the arguments of an `ops.<name>` call. */
export type PlanOp =
  | { op: "commit"; changes: string[]; placement: Placement; empty: boolean }
  | { op: "amend"; target: string; changes: string[] }
  | { op: "uncommit"; sources: string[] }
  /** combined is set when the messages must be merged in the composer first. */
  | { op: "squash"; sources: string[]; target?: string; mode: SquashMode; combined?: string }
  | { op: "move"; sources: string[]; placement: Placement }
  | { op: "pick"; sources: string[]; placement: Placement }

/** Plan is what confirming a target would do. */
export type Plan = {
  /** Short, for the tag on the target: "Commit onto api". */
  label: string
  /** A sentence for the hint bar and the toast: "Commit README.md to branch api". */
  desc: string
  /** Where the result lands relative to a commit target, for the insertion marker. */
  insert?: Side
  op: PlanOp
}

export function targetKey(e: TargetEntity): string {
  return e.kind === "new-branch" ? "new-branch" : e.kind === "unstaged" ? "unstaged" : `${e.kind}:${e.id}`
}

const uncommitted = (e: TargetEntity) => e.kind === "unstaged" || e.kind === "file"

function allOf(es: Selection[], ...kinds: Selection["kind"][]): boolean {
  return es.length > 0 && es.every((e) => kinds.includes(e.kind))
}

export function describe(e: TargetEntity): string {
  switch (e.kind) {
    case "unstaged":
      return "all uncommitted changes"
    case "file":
      return e.hunk ? `hunk of ${e.path}` : `file ${e.path}`
    case "cfile":
      return `file ${e.path}`
    case "commit":
      return `commit “${e.subject}”`
    case "branch":
      return `branch ${e.name}`
    case "new-branch":
      return "a new branch"
  }
}

export function describeAll(es: Selection[]): string {
  if (es.length === 1) return describe(es[0])
  const s = es[0]
  return `${es.length} ${s.kind === "file" && s.hunk ? "hunk" : s.kind === "cfile" ? "file" : s.kind}s`
}

function subjectOf(message: string): string {
  return message.split("\n")[0]?.trim() || "(empty)"
}

export function findBranch(ws: Workspace, name: string): Branch | undefined {
  for (const st of ws.stacks) for (const b of st.branches) if (b.name === name) return b
  return undefined
}

export function findCommit(ws: Workspace, id: string): { commit: Commit; branch: Branch } | undefined {
  for (const st of ws.stacks)
    for (const b of st.branches) {
      const commit = b.commits.find((c) => c.cliId === id)
      if (commit) return { commit, branch: b }
    }
  return undefined
}

/** entityFor looks up a target by kind and id, as a drop target or drag source carries them. */
export function entityFor(ws: Workspace, kind: string, id: string): TargetEntity | null {
  switch (kind) {
    case "unstaged":
      return { kind: "unstaged" }
    case "new-branch":
      return { kind: "new-branch" }
    case "file": {
      const c = ws.uncommittedChanges.find((ch) => ch.cliId === id)
      return c ? { kind: "file", id, path: c.filePath } : null
    }
    case "commit": {
      const found = findCommit(ws, id)
      return found
        ? { kind: "commit", id, subject: subjectOf(found.commit.message), branch: found.branch.name }
        : null
    }
    case "cfile": {
      for (const st of ws.stacks)
        for (const b of st.branches)
          for (const c of b.commits) {
            const ch = c.changes?.find((x) => x.cliId === id)
            if (ch) return { kind: "cfile", id, path: ch.filePath, commit: c.cliId, branch: b.name }
          }
      return null
    }
    case "branch": {
      for (const st of ws.stacks)
        for (const b of st.branches) if (b.cliId === id) return { kind: "branch", id, name: b.name }
      return null
    }
  }
  return null
}

/** sourcesFor turns the subjects into sources for a verb, or explains why not. */
export function sourcesFor(v: Verb, subjects: Selection[]): { sources: Selection[] } | { why: string } {
  if (subjects.length === 0) return { why: "nothing selected" }
  switch (v) {
    case "commit":
      if (allOf(subjects, "unstaged", "file")) return { sources: subjects }
      // On a branch or commit, commit everything uncommitted (like `but tui`).
      return { sources: [{ kind: "unstaged" }] }
    case "squash":
      if (
        allOf(subjects, "unstaged", "file") ||
        allOf(subjects, "commit") ||
        allOf(subjects, "branch") ||
        allOf(subjects, "cfile")
      )
        return { sources: subjects }
      return { why: `can't squash ${subjects[0].kind}s` }
    case "move":
      if (allOf(subjects, "commit") || (subjects.length === 1 && subjects[0].kind === "branch"))
        return { sources: subjects }
      if (allOf(subjects, "branch")) return { why: "branches can only be moved one at a time" }
      return { why: "only commits and branches can be moved" }
    case "pick":
      if (allOf(subjects, "commit")) return { sources: subjects }
      return { why: "only commits can be cherry-picked" }
  }
}

export function newTargetState(verb: Verb, sources: Selection[]): TargetState {
  return {
    verb,
    sources,
    side: verb === "move" ? "above" : "below",
    useTarget: false,
    emptyMsg: false,
    newBranchHere: false,
  }
}

function isSource(t: TargetState, e: TargetEntity): boolean {
  const k = targetKey(e)
  return t.sources.some((s) => targetKey(s) === k)
}

/** selfTargetOK: squashing a branch into itself squashes all of its commits. */
function selfTargetOK(t: TargetState, e: TargetEntity): boolean {
  return t.verb === "squash" && e.kind === "branch"
}

/** sourceIDs: no ids means "everything" to commit and amend. */
function sourceIDs(t: TargetState): string[] {
  return t.sources.flatMap((s) => (s.kind === "unstaged" ? [] : [s.id]))
}

function placeRelative(side: Side, id: string): Placement {
  return side === "above" ? { above: id } : { below: id }
}

function branchCommits(ws: Workspace, e: TargetEntity): Commit[] {
  return e.kind === "branch" ? (findBranch(ws, e.name)?.commits ?? []) : []
}

/** plan resolves what confirming target would do, or null when it is not a valid target. */
export function plan(ws: Workspace, t: TargetState, target: TargetEntity): Plan | null {
  if (isSource(t, target) && !selfTargetOK(t, target)) return null
  if (t.newBranchHere) return newBranchPlan(t, target)
  const src = t.sources[0]
  const what = describeAll(t.sources)
  const ids = sourceIDs(t)
  const hasCommits = (e: TargetEntity) => branchCommits(ws, e).length > 0

  switch (t.verb) {
    case "commit": {
      const op = (placement: Placement) =>
        ({ op: "commit", changes: ids, placement, empty: t.emptyMsg }) as const
      switch (target.kind) {
        case "branch":
          return {
            label: `Commit onto ${target.name}`,
            desc: `Commit ${what} to ${describe(target)}`,
            op: op({ branch: target.name }),
          }
        case "commit":
          return {
            label: `Commit ${t.side}`,
            desc: `Commit ${what} ${t.side} ${describe(target)}`,
            insert: t.side,
            op: op(placeRelative(t.side, target.id)),
          }
        case "new-branch":
          return {
            label: "Commit onto a new branch",
            desc: `Commit ${what} to a new branch`,
            op: op({ newBranch: true }),
          }
      }
      return null
    }

    case "squash": {
      if (uncommitted(src)) {
        if (target.kind !== "commit" && !(target.kind === "branch" && hasCommits(target))) return null
        return {
          label: target.kind === "commit" ? "Amend into this commit" : `Amend into ${target.name}`,
          desc: `Amend ${what} into ${describe(target)}`,
          op: { op: "amend", target: target.id, changes: ids },
        }
      }
      if (target.kind === "unstaged") {
        return {
          label: "Uncommit to Unstaged",
          desc: `Uncommit ${what}`,
          op: { op: "uncommit", sources: ids },
        }
      }
      if (src.kind === "branch" && target.kind === "branch" && isSource(t, target)) {
        if (t.sources.length > 1 || branchCommits(ws, target).length < 2) return null
        return squashPlan(ws, t, "Squash all commits", `Squash all commits of ${describe(target)}`, ids, target, "")
      }
      if (target.kind === "commit" || (target.kind === "branch" && hasCommits(target))) {
        // A committed file moves into another commit; its own commit is no target.
        if (src.kind === "cfile" && target.kind === "commit" && target.id === src.commit) return null
        const verb = src.kind === "cfile" ? "Move" : "Squash"
        const label = target.kind === "commit" ? `${verb} into this commit` : `${verb} into ${target.name}`
        return squashPlan(ws, t, label, `${verb} into ${describe(target)}: ${what}`, ids, target, target.id)
      }
      return null
    }

    case "move": {
      let placement: Placement
      let label: string
      let desc: string
      let insert: Side | undefined
      if (src.kind === "commit" && target.kind === "commit") {
        placement = placeRelative(t.side, target.id)
        insert = t.side
        label = `Move ${t.side}`
        desc = `Move ${what} ${t.side} ${describe(target)}`
      } else if (src.kind === "commit" && target.kind === "branch") {
        placement = { branch: target.name }
        label = `Move onto ${target.name}`
        desc = `Move ${what} to ${describe(target)}`
      } else if (src.kind === "branch" && target.kind === "branch") {
        placement = { branch: target.name }
        label = `Stack onto ${target.name}`
        desc = `Stack ${what} onto ${describe(target)}`
      } else if (target.kind === "new-branch") {
        placement = { newBranch: true }
        label = src.kind === "branch" ? "Unstack" : "Move onto a new branch"
        desc = src.kind === "branch" ? `Unstack ${what}` : `Move ${what} to a new branch`
      } else {
        return null
      }
      return { label, desc, insert, op: { op: "move", sources: ids, placement } }
    }

    case "pick": {
      const op = (placement: Placement) => ({ op: "pick", sources: ids, placement }) as const
      switch (target.kind) {
        case "branch":
          return {
            label: `Pick onto ${target.name}`,
            desc: `Cherry-pick ${what} to ${describe(target)}`,
            op: op({ branch: target.name }),
          }
        case "commit":
          return {
            label: `Pick ${t.side}`,
            desc: `Cherry-pick ${what} ${t.side} ${describe(target)}`,
            insert: t.side,
            op: op(placeRelative(t.side, target.id)),
          }
        case "new-branch":
          return {
            label: "Pick onto a new branch",
            desc: `Cherry-pick ${what} to a new branch`,
            op: op({ newBranch: true }),
          }
      }
      return null
    }
  }
}

/** newBranchPlan is `b` in commit and pick mode: a new branch stacked above the target branch. */
export function newBranchPlan(t: TargetState, target: TargetEntity): Plan | null {
  if (target.kind !== "branch" || (t.verb !== "commit" && t.verb !== "pick")) return null
  const ids = sourceIDs(t)
  const placement: Placement = { above: target.id }
  const label = `New branch above ${target.name}`
  if (t.verb === "commit") {
    return {
      label,
      desc: `Commit ${describeAll(t.sources)} to a new branch above ${target.name}`,
      op: { op: "commit", changes: ids, placement, empty: t.emptyMsg },
    }
  }
  return {
    label,
    desc: `Cherry-pick ${describeAll(t.sources)} to a new branch above ${target.name}`,
    op: { op: "pick", sources: ids, placement },
  }
}

/**
 * squashPlan decides how messages combine: keep the target's with `u`, reuse the
 * only non-empty one, or open the composer with all of them.
 */
function squashPlan(
  ws: Workspace,
  t: TargetState,
  label: string,
  desc: string,
  ids: string[],
  target: TargetEntity,
  targetID: string,
): Plan {
  const msgs: string[] = []
  const add = (msg: string) => {
    if (msg.trim()) msgs.push(msg.trim())
  }
  let targetMsg = ""
  if (target.kind === "commit") {
    targetMsg = findCommit(ws, target.id)?.commit.message ?? ""
  } else if (targetID !== "") {
    targetMsg = branchCommits(ws, target)[0]?.message ?? ""
  }
  add(targetMsg)
  for (const s of t.sources) {
    if (s.kind === "commit") add(findCommit(ws, s.id)?.commit.message ?? "")
    if (s.kind === "branch") for (const c of branchCommits(ws, s)) add(c.message)
  }
  const squashTarget = targetID || undefined
  let mode: SquashMode = "target"
  if (!t.useTarget && !targetMsg.trim() && msgs.length === 1 && t.sources[0].kind === "commit") {
    mode = "source"
  }
  if (t.useTarget || msgs.length <= 1) {
    return {
      label: t.useTarget ? `${label} (keep message)` : label,
      desc,
      op: { op: "squash", sources: ids, target: squashTarget, mode },
    }
  }
  return {
    label,
    desc,
    op: { op: "squash", sources: ids, target: squashTarget, mode: "combine", combined: msgs.join("\n\n") },
  }
}

/** dropVerb picks the natural verb for dragging source onto target (target.go dropVerb). */
export function dropVerb(source: Selection, target: TargetEntity): Verb | null {
  if (uncommitted(source)) {
    if (target.kind === "branch" || target.kind === "new-branch") return "commit"
    if (target.kind === "commit") return "squash" // amend
    return null
  }
  if (source.kind === "commit") {
    if (target.kind === "commit" || target.kind === "unstaged") return "squash"
    if (target.kind === "branch" || target.kind === "new-branch") return "move"
    return null
  }
  if (source.kind === "branch") {
    if (target.kind === "branch" || target.kind === "new-branch") return "move"
    if (target.kind === "unstaged" || target.kind === "commit") return "squash"
  }
  if (source.kind === "cfile") {
    if (target.kind === "commit" || target.kind === "branch" || target.kind === "unstaged") return "squash"
  }
  return null
}

/** dropPlan is what dropping sources onto target does, or null when it can't be dropped there. */
export function dropPlan(ws: Workspace, sources: Selection[], target: TargetEntity): Plan | null {
  if (sources.length === 0) return null
  const v = dropVerb(sources[0], target)
  if (!v) return null
  const s = sourcesFor(v, sources)
  if ("why" in s) return null
  return plan(ws, newTargetState(v, s.sources), target)
}

/** allTargets lists every target in the workspace: Unstaged, each lane top-down, then a new branch. */
export function allTargets(ws: Workspace): { entity: TargetEntity; group: string }[] {
  const out: { entity: TargetEntity; group: string }[] = [{ entity: { kind: "unstaged" }, group: "Workspace" }]
  for (const c of ws.uncommittedChanges) {
    out.push({ entity: { kind: "file", id: c.cliId, path: c.filePath }, group: "Unstaged" })
  }
  for (const st of ws.stacks)
    for (const b of [...st.branches].reverse()) {
      out.push({ entity: { kind: "branch", id: b.cliId, name: b.name }, group: b.name })
      for (const c of b.commits) {
        out.push({
          entity: { kind: "commit", id: c.cliId, subject: subjectOf(c.message), branch: b.name },
          group: b.name,
        })
      }
    }
  out.push({ entity: { kind: "new-branch" }, group: "Workspace" })
  return out
}
