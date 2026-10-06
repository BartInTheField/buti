/// <reference types="node" />
// Unit tests of the comment line matching, a port of internal/review/locate.go: `npm test`.
import assert from "node:assert/strict"
import { test } from "node:test"
import type { Diff, FileDiff } from "@/api"
import { fileOf, moveToLines } from "./components/review/locate.ts"

const entry = (id: string, path: string, oldStart: number, newStart: number, diff: string): FileDiff => ({
  id,
  path,
  status: "modified",
  diff: { type: "patch", hunks: [{ oldStart, oldLines: 0, newStart, newLines: 0, diff }] },
})

// An uncommitted diff lists one entry per hunk.
const diff: Diff = {
  changes: [
    entry("f:1", "a.kt", 5, 5, "@@ -5,1 +5,2 @@\n import a\n+import b\n"),
    entry("g:1", "b.kt", 1, 1, "@@ -1,1 +1,1 @@\n-x\n+y\n"),
    entry("f:2", "a.kt", 290, 291, "@@ -290,1 +291,2 @@\n }\n+fun insert()\n"),
  ],
}

test("fileOf merges the hunks of a file listed once per hunk", () => {
  const fd = fileOf(diff, "a.kt")
  assert.equal(fd?.diff.hunks.length, 2)
  assert.equal(diff.changes[0].diff.hunks.length, 1, "the diff is left as it is")
  assert.equal(fileOf(diff, "b.kt")?.diff.hunks.length, 1)
  assert.equal(fileOf(diff, "c.kt"), undefined)
})

test("a comment on a later hunk of an uncommitted file is found", () => {
  const a = { kind: "unassigned" as const, path: "a.kt", side: "new" as const, line: 292, end_line: 292, line_text: "fun insert()" }
  assert.deepEqual(moveToLines(fileOf(diff, "a.kt"), a), a)
})
