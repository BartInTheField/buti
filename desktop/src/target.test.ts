/// <reference types="node" />
// Unit tests of the target plans, a port of internal/ui/target.go. Node runs them
// directly (type stripping): `npm test`.
import assert from "node:assert/strict"
import { test } from "node:test"
import type { Workspace } from "@/api"
import type { Selection } from "@/selection"
import {
  dropPlan,
  newTargetState,
  plan,
  sourcesFor,
  type TargetEntity,
  type TargetState,
} from "./target.ts"

const ws: Workspace = {
  repo: "/r",
  uncommittedChanges: [{ cliId: "f1", filePath: "README.md", changeType: "modified" }],
  stacks: [
    {
      cliId: "s1",
      branches: [
        {
          cliId: "b1",
          name: "api",
          commits: [
            { cliId: "c2", message: "Add users\n\nWith a body" },
            { cliId: "c1", message: "Add routes" },
          ],
        },
        { cliId: "b2", name: "auth", commits: [] },
      ],
    },
  ],
}
const file: Selection = { kind: "file", id: "f1", path: "README.md" }
const c1: Selection = { kind: "commit", id: "c1", subject: "Add routes", branch: "api" }
const c2: Selection = { kind: "commit", id: "c2", subject: "Add users", branch: "api" }
const api: Selection = { kind: "branch", id: "b1", name: "api" }
const auth: Selection = { kind: "branch", id: "b2", name: "auth" }

function state(verb: TargetState["verb"], sources: Selection[], patch: Partial<TargetState> = {}) {
  const s = sourcesFor(verb, sources)
  if ("why" in s) throw new Error(s.why)
  return { ...newTargetState(verb, s.sources), ...patch }
}
const op = (t: TargetState, target: TargetEntity) => plan(ws, t, target)?.op ?? null

test("sources", () => {
  assert.deepEqual(sourcesFor("commit", [api]), { sources: [{ kind: "unstaged" }] })
  assert.deepEqual(sourcesFor("move", [api, auth]), { why: "branches can only be moved one at a time" })
  assert.deepEqual(sourcesFor("pick", [file]), { why: "only commits can be cherry-picked" })
  assert.equal(newTargetState("move", [c1]).side, "above")
  assert.equal(newTargetState("pick", [c1]).side, "below")
})

test("commit", () => {
  const t = state("commit", [file])
  assert.deepEqual(plan(ws, t, api), {
    label: "Commit onto api",
    desc: "Commit file README.md to branch api",
    op: { op: "commit", changes: ["f1"], placement: { branch: "api" }, empty: false },
  })
  assert.deepEqual(plan(ws, { ...t, side: "above" }, c1)?.insert, "above")
  assert.deepEqual(op({ ...t, side: "above" }, c1), {
    op: "commit",
    changes: ["f1"],
    placement: { above: "c1" },
    empty: false,
  })
  assert.deepEqual(op({ ...t, emptyMsg: true }, { kind: "new-branch" }), {
    op: "commit",
    changes: ["f1"],
    placement: { newBranch: true },
    empty: true,
  })
  assert.equal(plan(ws, { ...t, newBranchHere: true }, auth)?.label, "New branch above auth")
  assert.deepEqual(op({ ...t, newBranchHere: true }, auth), {
    op: "commit",
    changes: ["f1"],
    placement: { above: "b2" },
    empty: false,
  })
  assert.equal(op({ ...t, newBranchHere: true }, c1), null)
  assert.equal(op(t, file), null) // a source cancels
  // On a branch, c commits everything: no change ids.
  assert.deepEqual(op(state("commit", [c1]), api), {
    op: "commit",
    changes: [],
    placement: { branch: "api" },
    empty: false,
  })
})

test("squash, amend and uncommit", () => {
  assert.deepEqual(op(state("squash", [file]), c1), { op: "amend", target: "c1", changes: ["f1"] })
  assert.equal(op(state("squash", [file]), auth), null) // no commit to amend
  assert.deepEqual(op(state("squash", [c1]), { kind: "unstaged" }), { op: "uncommit", sources: ["c1"] })
  assert.deepEqual(op(state("squash", [c1]), c2), {
    op: "squash",
    sources: ["c1"],
    target: "c2",
    mode: "combine",
    combined: "Add users\n\nWith a body\n\nAdd routes",
  })
  assert.equal(plan(ws, state("squash", [c1], { useTarget: true }), c2)?.label, "Squash into this commit (keep message)")
  assert.deepEqual(op(state("squash", [c1], { useTarget: true }), c2), {
    op: "squash",
    sources: ["c1"],
    target: "c2",
    mode: "target",
  })
  // A branch onto itself squashes all of its commits.
  assert.equal(plan(ws, state("squash", [api]), api)?.label, "Squash all commits")
  assert.equal(op(state("squash", [auth]), auth), null)
  assert.equal(op(state("squash", [c1]), c1), null)
})

test("move and pick", () => {
  assert.equal(plan(ws, state("move", [api]), { kind: "new-branch" })?.label, "Unstack")
  assert.equal(plan(ws, state("move", [api]), auth)?.label, "Stack onto auth")
  assert.equal(op(state("move", [api]), c1), null)
  assert.deepEqual(op(state("move", [c1]), c2), { op: "move", sources: ["c1"], placement: { above: "c2" } })
  assert.deepEqual(op(state("pick", [c1]), auth), { op: "pick", sources: ["c1"], placement: { branch: "auth" } })
  assert.deepEqual(op(state("pick", [c1], { newBranchHere: true }), auth), {
    op: "pick",
    sources: ["c1"],
    placement: { above: "b2" },
  })
})

test("drops use the same plans (dropVerb)", () => {
  assert.equal(dropPlan(ws, [file], c1)?.label, "Amend into this commit")
  assert.equal(dropPlan(ws, [file], api)?.label, "Commit onto api")
  assert.equal(dropPlan(ws, [c1], c2)?.label, "Squash into this commit")
  assert.equal(dropPlan(ws, [c1], { kind: "unstaged" })?.label, "Uncommit to Unstaged")
  assert.equal(dropPlan(ws, [c1], auth)?.label, "Move onto auth")
  assert.equal(dropPlan(ws, [api], { kind: "new-branch" })?.label, "Unstack")
  assert.equal(dropPlan(ws, [api], { kind: "unstaged" })?.label, "Uncommit to Unstaged")
  assert.equal(dropPlan(ws, [file], { kind: "unstaged" }), null)
})

test("hunks and committed files", () => {
  const hunk: Selection = { kind: "file", id: "h1", path: "README.md", hunk: "@@ -1 +1 @@" }
  assert.equal(plan(ws, state("commit", [hunk]), api)?.desc, "Commit hunk of README.md to branch api")
  assert.deepEqual(op(state("commit", [hunk]), api), {
    op: "commit",
    changes: ["h1"],
    placement: { branch: "api" },
    empty: false,
  })

  const cf: Selection = { kind: "cfile", id: "cf1", path: "go.mod", commit: "c1", branch: "api" }
  const t = state("squash", [cf])
  assert.deepEqual(op(t, { kind: "unstaged" }), { op: "uncommit", sources: ["cf1"] })
  assert.equal(plan(ws, t, c1), null, "its own commit is no target")
  assert.equal(plan(ws, t, c2)?.label, "Move into this commit")
  assert.deepEqual(op(t, c2), { op: "squash", sources: ["cf1"], target: "c2", mode: "target" })
  assert.equal(dropPlan(ws, [cf], { kind: "unstaged" })?.label, "Uncommit to Unstaged")
  assert.equal(dropPlan(ws, [cf], api)?.label, "Move into api")
})
