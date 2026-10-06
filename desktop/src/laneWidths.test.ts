/// <reference types="node" />
// Unit tests of the stored lane widths: `npm test`.
import assert from "node:assert/strict"
import { test } from "node:test"
import { defaultLaneWidth, maxLaneWidth, minLaneWidth, parseLaneWidths, setLaneWidth } from "./laneWidths.ts"

test("setLaneWidth clamps and drops the default", () => {
  assert.deepEqual(setLaneWidth({}, "api", 400.4), { api: 400 })
  assert.deepEqual(setLaneWidth({}, "api", 10), { api: minLaneWidth })
  assert.deepEqual(setLaneWidth({}, "api", 5000), { api: maxLaneWidth })
  assert.deepEqual(setLaneWidth({ api: 400, b: 300 }, "api", defaultLaneWidth), { b: 300 })
})

test("parseLaneWidths keeps only widths", () => {
  assert.deepEqual(parseLaneWidths(null), {})
  assert.deepEqual(parseLaneWidths("nope"), {})
  assert.deepEqual(parseLaneWidths("[1]"), {})
  assert.deepEqual(parseLaneWidths('{"a":300,"b":"x","c":1}'), { a: 300, c: minLaneWidth })
})
