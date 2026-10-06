/// <reference types="node" />
// Unit tests of the recent repositories list: `npm test`.
import assert from "node:assert/strict"
import { test } from "node:test"
import { addRecent, parseRecent, removeRecent, repoName } from "./recent.ts"

test("addRecent puts the folder first once and caps the list", () => {
  assert.deepEqual(addRecent(["/a", "/b", "/c"], "/b"), ["/b", "/a", "/c"])
  assert.deepEqual(addRecent([], "/a"), ["/a"])
  assert.deepEqual(addRecent(["/a", "/b", "/c"], "/d", 3), ["/d", "/a", "/b"])
})

test("removeRecent drops the folder", () => {
  assert.deepEqual(removeRecent(["/a", "/b"], "/a"), ["/b"])
  assert.deepEqual(removeRecent(["/a"], "/x"), ["/a"])
})

test("parseRecent keeps only a list of paths", () => {
  assert.deepEqual(parseRecent(null), [])
  assert.deepEqual(parseRecent("not json"), [])
  assert.deepEqual(parseRecent('{"a":1}'), [])
  assert.deepEqual(parseRecent('["/a", 3, "", "/b"]'), ["/a", "/b"])
})

test("repoName is the last path segment", () => {
  assert.equal(repoName("/Users/ada/code/buti"), "buti")
  assert.equal(repoName("/Users/ada/code/buti/"), "buti")
  assert.equal(repoName("C:\\code\\buti"), "buti")
})
