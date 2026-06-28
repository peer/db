import type { RefFilterResult, RefValueLike } from "@/types"

import { assert, describe, expect, test } from "vitest"

import { timeFloat64, validateTime } from "@/document/time"
import {
  addPrefixWildcard,
  amountRangeDecimals,
  amountRangeDisplay,
  amountStringFromFloat64,
  amountValueDecimals,
  buildRefTree,
  computeRefCheckStates,
  mergeRefOverlay,
  parseUrl,
  timePrecisionForRange,
  timePrecisionForValue,
  timeStringFromFloat64,
  toggleRefSelection,
  valuesNotShownMarkers,
} from "@/utils"

// Unix seconds for 2025-03-02 10:30:45 UTC.
const SAMPLE_SECONDS = Date.UTC(2025, 2, 2, 10, 30, 45) / 1000

describe("addPrefixWildcard", () => {
  test("appends a wildcard when the query ends with a letter or number", () => {
    assert.equal(addPrefixWildcard("germ"), "germ*")
    assert.equal(addPrefixWildcard("123"), "123*")
    assert.equal(addPrefixWildcard("united sta"), "united sta*")
    // Unicode letters count too.
    assert.equal(addPrefixWildcard("ljublj"), "ljublj*")
    assert.equal(addPrefixWildcard("ž"), "ž*")
  })

  test("leaves the query unchanged when it does not end with a letter or number", () => {
    assert.equal(addPrefixWildcard(""), "")
    assert.equal(addPrefixWildcard("germany "), "germany ")
    assert.equal(addPrefixWildcard("germany*"), "germany*")
    assert.equal(addPrefixWildcard('"exact phrase"'), '"exact phrase"')
    assert.equal(addPrefixWildcard("foo-"), "foo-")
  })
})

describe("timePrecisionForRange", () => {
  test("returns s for spans under an hour", () => {
    assert.equal(timePrecisionForRange(0, 0), "s")
    assert.equal(timePrecisionForRange(0, 30), "s")
    assert.equal(timePrecisionForRange(0, 60), "s")
    assert.equal(timePrecisionForRange(0, 60 * 59), "s")
  })

  test("returns min for spans from an hour up to a day", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60), "min")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 12), "min")
  })

  test("returns h for spans from a day up to a month", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24), "h")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 15), "h")
  })

  test("returns d for spans from a month up to a year", () => {
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 30), "d")
    assert.equal(timePrecisionForRange(0, 60 * 60 * 24 * 200), "d")
  })

  test("returns m for spans from a year up to a decade", () => {
    const year = 60 * 60 * 24 * 365
    assert.equal(timePrecisionForRange(0, year), "m")
    assert.equal(timePrecisionForRange(0, year * 5), "m")
  })

  test("returns coarser precisions for larger spans", () => {
    const year = 60 * 60 * 24 * 365
    assert.equal(timePrecisionForRange(0, year * 50), "y")
    assert.equal(timePrecisionForRange(0, year * 500), "10y")
    assert.equal(timePrecisionForRange(0, year * 5_000), "100y")
    assert.equal(timePrecisionForRange(0, year * 50_000), "k")
    assert.equal(timePrecisionForRange(0, year * 500_000), "10k")
    assert.equal(timePrecisionForRange(0, year * 5_000_000), "100k")
    assert.equal(timePrecisionForRange(0, year * 50_000_000), "M")
    assert.equal(timePrecisionForRange(0, year * 500_000_000), "10M")
    assert.equal(timePrecisionForRange(0, year * 5_000_000_000), "100M")
  })

  test("ignores argument order", () => {
    // 2 hours falls in the "min" tier under the current mapping.
    assert.equal(timePrecisionForRange(60 * 60 * 2, 0), "min")
    assert.equal(timePrecisionForRange(0, -60 * 60 * 2), "min")
  })
})

describe("timeStringFromFloat64", () => {
  test("formats at second precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "s"), "2025-03-02 10:30:45")
  })

  test("formats at minute precision (drops seconds)", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "min"), "2025-03-02 10:30")
  })

  test("formats at hour precision (minutes pinned to :00)", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "h"), "2025-03-02 10:00")
  })

  test("formats at day precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "d"), "2025-03-02")
  })

  test("formats at month precision with day=00", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "m"), "2025-03-00")
  })

  test("formats at year precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "y"), "2025")
  })

  test("rounds year down for decade precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "10y"), "2020")
  })

  test("rounds year down for century precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "100y"), "2000")
  })

  test("rounds year down for kiloyear precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "k"), "2000")
  })

  test("rounds year down for megayear precision", () => {
    assert.equal(timeStringFromFloat64(SAMPLE_SECONDS, "M"), "0000")
  })

  test("pads short positive years to four digits", () => {
    // Year 1 CE is unix epoch − ~62135596800 s.
    const seconds = -62_135_596_800 + 60 * 60 * 24
    const result = timeStringFromFloat64(seconds, "y")
    assert.equal(result, "0001")
  })

  test("formats negative years with leading minus and zero padding", () => {
    // Roughly -45 BCE (well before unix epoch).
    const year = 60 * 60 * 24 * 365
    const result = timeStringFromFloat64(-2_000 * year, "y")
    assert.match(result, /^-\d{4}$/)
  })

  test("output round-trips through the claim parser at the same precision", () => {
    for (const precision of ["s", "min", "h", "d", "m", "y", "10y", "100y", "k"] as const) {
      const s = timeStringFromFloat64(SAMPLE_SECONDS, precision)
      // validateTime throws on bad format or precision mismatch.
      validateTime(s, precision)
      // timeFloat64 (with explicit precision) re-derives a float that should
      // be the start of the precision window, i.e. <= the original.
      const roundTripped = timeFloat64(s, precision)
      assert.isAtMost(roundTripped, SAMPLE_SECONDS)
    }
  })

  test("throws for subsecond precisions", () => {
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "ms"), /subsecond/)
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "us"), /subsecond/)
    assert.throws(() => timeStringFromFloat64(SAMPLE_SECONDS, "ns"), /subsecond/)
  })
})

describe("timePrecisionForValue", () => {
  test("returns s for fractional-second values", () => {
    assert.equal(timePrecisionForValue(0.5), "s")
    assert.equal(timePrecisionForValue(0.001), "s")
    assert.equal(timePrecisionForValue(SAMPLE_SECONDS + 0.5), "s")
  })

  test("returns s for non-minute-divisible integer seconds", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10, 30, 45) / 1000), "s")
  })

  test("returns min when divisible by 60 but not 3600", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10, 30) / 1000), "min")
  })

  test("returns h when divisible by 3600 but not 86400", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2, 10) / 1000), "h")
  })

  test("returns d when divisible by 86400 and day > 1", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 2) / 1000), "d")
  })

  test("returns m when on day 1 of a non-January month", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 2, 1) / 1000), "m")
  })

  test("returns y on Jan 1 of a year not divisible by 10", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2025, 0, 1) / 1000), "y")
  })

  test("never returns coarser than y for four-digit years", () => {
    assert.equal(timePrecisionForValue(Date.UTC(2020, 0, 1) / 1000), "y")
    assert.equal(timePrecisionForValue(Date.UTC(2100, 0, 1) / 1000), "y")
    assert.equal(timePrecisionForValue(Date.UTC(2000, 0, 1) / 1000), "y")
    // Unix epoch year 1970.
    assert.equal(timePrecisionForValue(0), "y")
    assert.equal(timePrecisionForValue(Date.UTC(-2000, 0, 1) / 1000), "y")
  })

  test("uses the year divisibility walk for five-digit and larger years", () => {
    assert.equal(timePrecisionForValue(Date.UTC(12000, 0, 1) / 1000), "k")
    assert.equal(timePrecisionForValue(Date.UTC(10000, 0, 1) / 1000), "10k")
    assert.equal(timePrecisionForValue(Date.UTC(110000, 0, 1) / 1000), "10k")
    assert.equal(timePrecisionForValue(Date.UTC(100000, 0, 1) / 1000), "100k")
  })

  test("tolerates small float64 rounding error", () => {
    // 60 + 1e-9 should still be treated as exactly divisible by 60.
    assert.equal(timePrecisionForValue(60 + 1e-9), "min")
    // Likewise on the negative side.
    assert.equal(timePrecisionForValue(60 - 1e-9), "min")
  })
})

describe("amountRangeDecimals", () => {
  test("shows more digits for narrower spans", () => {
    assert.equal(amountRangeDecimals(0, 1_000_000), 0)
    assert.equal(amountRangeDecimals(0, 100), 0)
    assert.equal(amountRangeDecimals(3.14159, 9.87654), 2)
    assert.equal(amountRangeDecimals(1234.5678, 1234.9999), 3)
    assert.equal(amountRangeDecimals(0, 0.001), 5)
  })

  test("ignores argument order", () => {
    assert.equal(amountRangeDecimals(9.87654, 3.14159), 2)
  })

  test("clamps to [0, 12]", () => {
    assert.equal(amountRangeDecimals(0, 1e20), 0)
    assert.equal(amountRangeDecimals(0, 1e-20), 12)
  })

  test("falls back to the value's own precision for a zero-width or non-finite span", () => {
    assert.equal(amountRangeDecimals(5, 5), 0)
    assert.equal(amountRangeDecimals(3.5, 3.5), 1)
    assert.equal(amountRangeDecimals(0, Infinity), 0)
  })
})

describe("amountValueDecimals", () => {
  test("counts the fractional digits of the value", () => {
    assert.equal(amountValueDecimals(42), 0)
    assert.equal(amountValueDecimals(-42), 0)
    assert.equal(amountValueDecimals(3.1), 1)
    assert.equal(amountValueDecimals(3.14), 2)
    assert.equal(amountValueDecimals(3.142), 3)
    assert.equal(amountValueDecimals(-1.5), 1)
  })

  test("expands exponent notation used by very small values", () => {
    assert.equal(amountValueDecimals(1e-7), 7)
    assert.equal(amountValueDecimals(1.23e-7), 9)
  })

  test("returns 0 for non-finite values", () => {
    assert.equal(amountValueDecimals(Infinity), 0)
    assert.equal(amountValueDecimals(NaN), 0)
  })
})

describe("amountStringFromFloat64", () => {
  test("rounds to the given digits and trims trailing zeros", () => {
    assert.equal(amountStringFromFloat64(3.14159, 2), "3.14")
    assert.equal(amountStringFromFloat64(3.1, 2), "3.1")
    assert.equal(amountStringFromFloat64(1234.9999, 3), "1235")
    assert.equal(amountStringFromFloat64(42, 0), "42")
  })
})

describe("amountRangeDisplay", () => {
  test("rounds both edges to the span precision and trims trailing zeros", () => {
    assert.deepEqual(amountRangeDisplay(0, 1), { decimals: 2, from: "0", to: "1" })
    assert.deepEqual(amountRangeDisplay(3.14159, 9.87654), { decimals: 2, from: "3.14", to: "9.88" })
    assert.deepEqual(amountRangeDisplay(1234.5678, 1234.9999), { decimals: 3, from: "1234.568", to: "1235" })
    assert.deepEqual(amountRangeDisplay(0, 1_000_000), { decimals: 0, from: "0", to: "1000000" })
  })
})

describe("parseUrl", () => {
  test.each([
    "https://example.com",
    "https://example.com/path?q=1#section",
    "http://example.com/foo",
    "HTTPS://Example.com",
    "mailto:test@example.com",
    "tel:+1234",
    "/foo",
    "/foo/bar?q=1#h",
    "/",
  ])("accepts %s", (input) => {
    const url = parseUrl(input)
    assert.instanceOf(url, URL)
  })

  test.each([
    ["", "empty"],
    ["#section", "Invalid URL"],
    ["../foo", "Invalid URL"],
    ["foo/bar", "Invalid URL"],
    ["//example.com/foo", "Invalid URL"],
    ["javascript:alert(1)", "disallowed URL scheme: javascript:"],
    ["ftp://example.com", "disallowed URL scheme: ftp:"],
    ["data:text/html,<x>", "disallowed URL scheme: data:"],
    ["http:///example.com", "missing host"],
    ["mailto:", "missing address"],
    ["tel:", "missing number"],
  ])("rejects %s", (input, fragment) => {
    expect(() => parseUrl(input)).toThrow(fragment)
  })

  test("allowContact=false rejects mailto: and tel: even when otherwise valid", () => {
    // Sanity-check: the same values are accepted with the default (allowContact=true).
    expect(parseUrl("mailto:test@example.com")).toBeInstanceOf(URL)
    expect(parseUrl("tel:+1234")).toBeInstanceOf(URL)
    expect(() => parseUrl("mailto:test@example.com", { allowContact: false })).toThrow("disallowed URL scheme: mailto:")
    expect(() => parseUrl("tel:+1234", { allowContact: false })).toThrow("disallowed URL scheme: tel:")
  })

  test("allowContact=false still accepts http / https / leading-slash paths", () => {
    assert.instanceOf(parseUrl("https://example.com", { allowContact: false }), URL)
    assert.instanceOf(parseUrl("http://example.com/foo", { allowContact: false }), URL)
    assert.instanceOf(parseUrl("/foo", { allowContact: false }), URL)
  })
})

describe("buildRefTree", () => {
  test("nests children under their placed ancestor, preserving input order", () => {
    const tree = buildRefTree([
      { id: "A", paths: [] },
      { id: "B", paths: [["A"]] },
      { id: "C", paths: [["A"]] },
    ])
    assert.lengthOf(tree, 1)
    assert.equal(tree[0].res.id, "A")
    assert.deepEqual(
      tree[0].children.map((n) => n.res.id),
      ["B", "C"],
    )
    assert.deepEqual(
      tree[0].children.map((n) => n.key),
      ["B", "C"],
    )
  })

  test("duplicates a value under each of its parents (diamond)", () => {
    const tree = buildRefTree([
      { id: "A", paths: [] },
      { id: "B", paths: [["A"]] },
      { id: "C", paths: [["A"]] },
      {
        id: "E",
        paths: [
          ["A", "B"],
          ["A", "C"],
        ],
      },
    ])
    const [b, c] = tree[0].children
    // E renders under both parents; the canonical (first) placement keeps key "E", the duplicate is suffixed.
    assert.deepEqual(
      b.children.map((n) => n.res.id),
      ["E"],
    )
    assert.deepEqual(
      c.children.map((n) => n.res.id),
      ["E"],
    )
    assert.equal(b.children[0].key, "E")
    assert.equal(c.children[0].key, "E|" + c.key)
  })

  test("values without a placed ancestor are roots", () => {
    const tree = buildRefTree([{ id: "A", paths: [] }, { id: "B" }])
    assert.deepEqual(
      tree.map((n) => n.res.id),
      ["A", "B"],
    )
  })
})

// Hierarchy artist > {painter, sculptor}, plus artist's "direct" entry and the top-level
// "missing" entry. Paths are ancestor chains from root to immediate parent; a "direct" entry and
// a root value list themselves as the parent of nothing else, "missing" has no paths. childCount is
// the exact number of distinct child values: artist has two real children loaded (painter, sculptor),
// so its childCount is 2 and all of its children are loaded, exercising the all-children promotion gate.
const ARTIST_VALUES: RefFilterResult[] = [
  { id: "artist", count: 9, childCount: 2 },
  { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
  { id: "sculptor", count: 4, childCount: 0, paths: [["artist"]] },
  { id: "__DIRECT__:artist", count: 3, childCount: 0, paths: [["artist"]] },
  { id: "__MISSING__", count: 5, childCount: 0 },
]

function checkedIds(values: readonly RefValueLike[], selected: Iterable<string>): string[] {
  const states = computeRefCheckStates(values, new Set(selected))
  return [...states.entries()].filter(([, s]) => s.checked).map(([id]) => id)
}

function indeterminateIds(values: readonly RefValueLike[], selected: Iterable<string>): string[] {
  const states = computeRefCheckStates(values, new Set(selected))
  return [...states.entries()].filter(([, s]) => s.indeterminate).map(([id]) => id)
}

describe("computeRefCheckStates", () => {
  test("nothing selected leaves every value unchecked and determinate", () => {
    assert.sameMembers(checkedIds(ARTIST_VALUES, []), [])
    assert.sameMembers(indeterminateIds(ARTIST_VALUES, []), [])
  })

  test("a selected leaf checks itself and leaves the parent indeterminate", () => {
    assert.sameMembers(checkedIds(ARTIST_VALUES, ["painter"]), ["painter"])
    assert.sameMembers(indeterminateIds(ARTIST_VALUES, ["painter"]), ["artist"])
  })

  test("all children selected checks the parent, none indeterminate", () => {
    const selected = ["painter", "sculptor", "__DIRECT__:artist"]
    assert.sameMembers(checkedIds(ARTIST_VALUES, selected), [...selected, "artist"])
    assert.sameMembers(indeterminateIds(ARTIST_VALUES, selected), [])
  })

  test("the parent value selected on its own checks the parent and all of its children", () => {
    // The API case: only the parent value is in the filter, yet the whole subtree reads as checked.
    assert.sameMembers(checkedIds(ARTIST_VALUES, ["artist"]), ["artist", "painter", "sculptor", "__DIRECT__:artist"])
    assert.sameMembers(indeterminateIds(ARTIST_VALUES, ["artist"]), [])
  })

  test("a partially covered parent is indeterminate", () => {
    const selected = ["painter", "__DIRECT__:artist"]
    assert.sameMembers(checkedIds(ARTIST_VALUES, selected), selected)
    assert.sameMembers(indeterminateIds(ARTIST_VALUES, selected), ["artist"])
  })

  test("a fully loaded parent (childCount == loaded) promotes to full when all children are checked", () => {
    // artist has count 9 and childCount 2, matching its two loaded real children (painter, sculptor), so the
    // all-children promotion fires.
    const selected = ["painter", "sculptor", "__DIRECT__:artist"]
    assert.isTrue(computeRefCheckStates(ARTIST_VALUES, new Set(selected)).get("artist")?.checked)
  })

  test("a truncated parent (childCount > loaded) does not promote, it stays indeterminate", () => {
    // artist claims three distinct children but only two are loaded, so the not-loaded child means the parent
    // can never be full from its loaded children alone.
    const values: RefFilterResult[] = [
      { id: "artist", count: 9, childCount: 3 },
      { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
      { id: "sculptor", count: 4, childCount: 0, paths: [["artist"]] },
      { id: "__DIRECT__:artist", count: 3, childCount: 0, paths: [["artist"]] },
    ]
    const selected = new Set(["painter", "sculptor", "__DIRECT__:artist"])
    const states = computeRefCheckStates(values, selected)
    assert.isFalse(states.get("artist")?.checked)
    assert.isTrue(states.get("artist")?.indeterminate)
  })

  test("a count-0 augment parent does not promote, it stays indeterminate", () => {
    // artist holds no documents of its own (count 0), so even with all of its children checked it is not full,
    // it only renders indeterminate and stays clickable to fully select.
    const values: RefFilterResult[] = [
      { id: "artist", count: 0, childCount: 2 },
      { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
      { id: "sculptor", count: 4, childCount: 0, paths: [["artist"]] },
    ]
    const selected = new Set(["painter", "sculptor"])
    const states = computeRefCheckStates(values, selected)
    assert.isFalse(states.get("artist")?.checked)
    assert.isTrue(states.get("artist")?.indeterminate)
  })
})

describe("toggleRefSelection", () => {
  test("clicking an unchecked parent selects its whole subtree", () => {
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "artist", new Set())], ["artist", "painter", "sculptor", "__DIRECT__:artist"])
  })

  test("clicking a checked parent clears its whole subtree", () => {
    const selected = new Set(["artist", "painter", "sculptor", "__DIRECT__:artist"])
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "artist", selected)], [])
  })

  test("deselecting a child decomposes the parent into its remaining siblings", () => {
    // From a UI selection (parent plus its children stored explicitly).
    const fromUI = new Set(["artist", "painter", "sculptor", "__DIRECT__:artist"])
    // From an API selection (only the parent value stored).
    const fromAPI = new Set(["artist"])
    const expected = ["sculptor", "__DIRECT__:artist"]
    // Both converge to the same selection: painter dropped, its siblings and "direct" kept.
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "painter", fromUI)], expected)
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "painter", fromAPI)], expected)
  })

  test("reselecting the last missing sibling re-checks the parent", () => {
    const afterDeselect = new Set(["sculptor", "__DIRECT__:artist"])
    const next = toggleRefSelection(ARTIST_VALUES, "painter", afterDeselect)
    assert.sameMembers([...next], ["painter", "sculptor", "__DIRECT__:artist"])
    assert.isTrue(computeRefCheckStates(ARTIST_VALUES, next).get("artist")?.checked)
  })

  test("the missing entry toggles independently of the value tree", () => {
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "__MISSING__", new Set(["painter"]))], ["painter", "__MISSING__"])
    assert.sameMembers([...toggleRefSelection(ARTIST_VALUES, "__MISSING__", new Set(["__MISSING__", "painter"]))], ["painter"])
  })

  test("deselecting through a multi-level hierarchy keeps the untouched branch", () => {
    // root > mid > {x, y}: selecting root then deselecting x must keep y.
    const values: RefValueLike[] = [{ id: "root" }, { id: "mid", paths: [["root"]] }, { id: "x", paths: [["root", "mid"]] }, { id: "y", paths: [["root", "mid"]] }]
    assert.sameMembers([...toggleRefSelection(values, "x", new Set(["root"]))], ["y"])
  })

  test("deselecting a diamond leaf reached through two parents clears it everywhere", () => {
    // root > {pa, pb}, both parents of the same leaf.
    const values: RefValueLike[] = [
      { id: "root" },
      { id: "pa", paths: [["root"]] },
      { id: "pb", paths: [["root"]] },
      {
        id: "leaf",
        paths: [
          ["root", "pa"],
          ["root", "pb"],
        ],
      },
    ]
    // A leaf covered by either parent is checked.
    assert.isTrue(computeRefCheckStates(values, new Set(["pa"])).get("leaf")?.checked)
    // Deselecting it from a root-wide selection leaves nothing, since it is the only value below.
    assert.sameMembers([...toggleRefSelection(values, "leaf", new Set(["root"]))], [])
  })
})

describe("mergeRefOverlay", () => {
  test("a match already in primary keeps the primary data and is not duplicated", () => {
    const primary: RefFilterResult[] = [
      { id: "art", count: 10, childCount: 1 },
      { id: "painting", count: 4, childCount: 0, paths: [["art"]] },
    ]
    // The match carries a different count and extra paths for an id already loaded in primary.
    const matchResults: RefFilterResult[] = [{ id: "painting", count: 99, childCount: 0, paths: [["art"], ["other"]] }]
    const combined = mergeRefOverlay(primary, matchResults)
    // The combined list is exactly the primary list: the already-loaded id is not appended again.
    assert.deepEqual(combined, primary)
    // The primary entry wins, so its count and paths are kept and the match's differing data is ignored.
    const painting = combined.find((e) => e.id === "painting")
    assert.equal(painting?.count, 4)
    assert.deepEqual(painting?.paths, [["art"]])
  })

  test("a match not in primary is appended with its match-provided count and paths", () => {
    // sculpture is beyond the loaded primary list, so the combined list must reach it from the match data.
    const primary: RefFilterResult[] = [{ id: "art", count: 10, childCount: 0 }]
    const matchResults: RefFilterResult[] = [
      { id: "art", count: 10, childCount: 0 },
      { id: "sculpture", count: 2, childCount: 0, paths: [["art"]] },
    ]
    const combined = mergeRefOverlay(primary, matchResults)
    const sculpture = combined.find((e) => e.id === "sculpture")
    assert.isDefined(sculpture)
    assert.equal(sculpture?.count, 2)
    assert.deepEqual(sculpture?.paths, [["art"]])
  })

  test("combined order is primary first, then the not-in-primary matches in match order", () => {
    const primary: RefFilterResult[] = [
      { id: "a", count: 3, childCount: 0 },
      { id: "b", count: 2, childCount: 0 },
    ]
    const matchResults: RefFilterResult[] = [
      { id: "b", count: 2, childCount: 0 },
      { id: "c", count: 1, childCount: 0 },
      { id: "d", count: 1, childCount: 0 },
    ]
    const combined = mergeRefOverlay(primary, matchResults)
    assert.deepEqual(
      combined.map((e) => e.id),
      ["a", "b", "c", "d"],
    )
  })
})

describe("valuesNotShownMarkers", () => {
  test("a parent with unloaded children yields a marker carrying the document gap", () => {
    // artist has five distinct children but only two real ones (painter, sculptor) plus its "direct" entry are
    // loaded, so a marker is produced. docGap = 100 - (2 + 4 + 3) = 91, where the "direct" entry counts toward
    // loaded documents but is not counted as a real loaded child.
    const results: RefFilterResult[] = [
      { id: "artist", count: 100, childCount: 5 },
      { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
      { id: "sculptor", count: 4, childCount: 0, paths: [["artist"]] },
      { id: "__DIRECT__:artist", count: 3, childCount: 0, paths: [["artist"]] },
    ]
    const markers = valuesNotShownMarkers(results)
    assert.lengthOf(markers, 1)
    assert.equal(markers[0].id, "__MORE__:artist")
    assert.equal(markers[0].count, 91)
  })

  test("a fully loaded parent yields no marker", () => {
    const results: RefFilterResult[] = [
      { id: "artist", count: 9, childCount: 2 },
      { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
      { id: "sculptor", count: 4, childCount: 0, paths: [["artist"]] },
      { id: "__DIRECT__:artist", count: 3, childCount: 0, paths: [["artist"]] },
    ]
    assert.lengthOf(valuesNotShownMarkers(results), 0)
  })

  test("the direct entry is excluded from the real loaded child count but included in the document gap", () => {
    // Only one real child (painter) is loaded next to the "direct" entry, while childCount is 2, so a child is
    // missing and a marker is produced. docGap = 10 - (2 + 3) = 5, with the "direct" entry counted in the sum.
    const results: RefFilterResult[] = [
      { id: "artist", count: 10, childCount: 2 },
      { id: "painter", count: 2, childCount: 0, paths: [["artist"]] },
      { id: "__DIRECT__:artist", count: 3, childCount: 0, paths: [["artist"]] },
    ]
    const markers = valuesNotShownMarkers(results)
    assert.lengthOf(markers, 1)
    assert.equal(markers[0].count, 5)
  })
})
