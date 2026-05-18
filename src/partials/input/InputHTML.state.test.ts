import type { Node, NodeType } from "prosemirror-model"
import type { Command } from "prosemirror-state"

import { setBlockType } from "prosemirror-commands"
import { AllSelection, EditorState, TextSelection } from "prosemirror-state"
import { describe, expect, test } from "vitest"

import { schema } from "@/partials/input/InputHTML.schema"
import {
  applyCommandToTr,
  canIndentAt,
  canOutdentAt,
  dissolveContainersInTr,
  exitBlockquoteOnEmpty,
  exitPreformattedOnEmpty,
  findBlockquotePosAt,
  findLinkRangeAt,
  findTextblockPositionInDoc,
  getCurrentLinkValue,
  indentCodeBlock,
  innermostListAt,
  insertLineBreak,
  isDocEmpty,
  isInside,
  isMarkActive,
  isNodeActive,
  liftBlockquotesInRangeInTr,
  liftListsInRangeInTr,
  rangeContainsInTr,
  removeHrsInRangeInTr,
  selectionSpansList,
  splitPreformattedInTr,
  unindentCodeBlock,
} from "@/partials/input/InputHTML.state"

// ---------------------------------------------------------------------------
// Doc-builder helpers. We construct PM docs through schema.nodes.X.create()
// rather than parsing HTML so the tests run in vitest's default node
// environment (no DOM) and so the structures we assert against are fully
// explicit. Comparisons use Node.toJSON() so vitest's pretty-printer shows
// readable structural diffs on mismatch.
// ---------------------------------------------------------------------------

function p(...content: (Node | string)[]): Node {
  const inline = content.map((c) => (typeof c === "string" ? schema.text(c) : c))
  return schema.nodes.paragraph.create(null, inline.length > 0 ? inline : undefined)
}

function h(level: number, text: string): Node {
  return schema.nodes.heading.create({ level }, schema.text(text))
}

function pre(text: string): Node {
  return schema.nodes.preformatted.create(null, text.length > 0 ? schema.text(text) : undefined)
}

function bp(text: string): Node {
  return schema.nodes.blockquote_paragraph.create(null, text.length > 0 ? schema.text(text) : undefined)
}

function bq(...children: Node[]): Node {
  return schema.nodes.blockquote.create(null, children)
}

function ul(...items: Node[]): Node {
  return schema.nodes.bullet_list.create(null, items)
}

function ol(...items: Node[]): Node {
  return schema.nodes.ordered_list.create(null, items)
}

function li(...children: Node[]): Node {
  return schema.nodes.list_item.create(null, children)
}

function hr(): Node {
  return schema.nodes.horizontal_rule.create()
}

function linked(href: string, text: string): Node {
  return schema.text(text, [schema.marks.link.create({ href })])
}

function doc(...children: Node[]): Node {
  return schema.topNodeType.create(null, children)
}

function stateWith(d: Node): EditorState {
  return EditorState.create({ doc: d, schema })
}

function withCursor(state: EditorState, pos: number): EditorState {
  return state.apply(state.tr.setSelection(TextSelection.create(state.doc, pos)))
}

function withSelection(state: EditorState, from: number, to: number): EditorState {
  return state.apply(state.tr.setSelection(TextSelection.create(state.doc, from, to)))
}

// Position of the first paragraph (or other named textblock) with the given
// text content. Returns the node's position; +1 lands the cursor inside.
function findParagraphPos(d: Node, text: string, type: NodeType = schema.nodes.paragraph): number {
  let pos = -1
  d.descendants((node, p) => {
    if (pos < 0 && node.type === type && node.textContent === text) {
      pos = p
      return false
    }
    return undefined
  })
  return pos
}

// Run a tr-mutating helper inside an EditorState and return the resulting
// doc. The state's selection is mapped through the steps so callers that
// also care about selection can pull it from the returned state.
function runMutator(state: EditorState, fn: (tr: import("prosemirror-state").Transaction) => unknown): EditorState {
  const tr = state.tr
  fn(tr)
  return state.apply(tr)
}

function runCommand(state: EditorState, cmd: Command): { state: EditorState; ran: boolean } {
  let next = state
  let ran = false
  cmd(state, (tr) => {
    ran = true
    next = state.apply(tr)
  })
  return { state: next, ran }
}

// ---------------------------------------------------------------------------
// isDocEmpty
// ---------------------------------------------------------------------------

describe("isDocEmpty", () => {
  test("returns true for a doc with only an empty paragraph", () => {
    expect(isDocEmpty(doc(p()))).toBe(true)
  })
  test("returns false when the paragraph has text", () => {
    expect(isDocEmpty(doc(p("hi")))).toBe(false)
  })
  test("returns false when the doc has a leaf (hr)", () => {
    expect(isDocEmpty(doc(p(), hr()))).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// isMarkActive / isNodeActive
// ---------------------------------------------------------------------------

describe("isMarkActive", () => {
  test("true when cursor sits inside marked text", () => {
    const d = doc(p(linked("https://a", "linktext")))
    const state = stateWith(d)
    // linktext sits at content offset 0; doc.pos = 1 enters paragraph, +1 enters text.
    const cursor = findParagraphPos(d, "linktext") + 2
    expect(isMarkActive(withCursor(state, cursor), schema.marks.link)).toBe(true)
  })
  test("false for a cursor outside the marked range", () => {
    const d = doc(p("plain"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "plain") + 2)
    expect(isMarkActive(state, schema.marks.link)).toBe(false)
  })
})

describe("isNodeActive", () => {
  test("true when the cursor's textblock matches the type", () => {
    const d = doc(h(1, "title"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "title", schema.nodes.heading) + 1)
    expect(isNodeActive(state, schema.nodes.heading, { level: 1 })).toBe(true)
  })
  test("false when attrs differ (level mismatch)", () => {
    const d = doc(h(1, "title"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "title", schema.nodes.heading) + 1)
    expect(isNodeActive(state, schema.nodes.heading, { level: 2 })).toBe(false)
  })
  test("AllSelection over mixed textblocks does not match a single type", () => {
    const d = doc(p("a"), h(1, "b"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    expect(isNodeActive(state, schema.nodes.paragraph)).toBe(false)
    expect(isNodeActive(state, schema.nodes.heading)).toBe(false)
  })
  test("AllSelection over homogeneous paragraphs matches paragraph", () => {
    const d = doc(p("a"), p("b"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    expect(isNodeActive(state, schema.nodes.paragraph)).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// isInside / innermostListAt / selectionSpansList
// ---------------------------------------------------------------------------

describe("isInside", () => {
  test("true for cursor inside a blockquote", () => {
    const d = doc(bq(bp("inside")))
    const state = withCursor(stateWith(d), findParagraphPos(d, "inside", schema.nodes.blockquote_paragraph) + 1)
    expect(isInside(state, schema.nodes.blockquote)).toBe(true)
  })
  test("false for cursor outside any blockquote", () => {
    const d = doc(p("nope"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "nope") + 1)
    expect(isInside(state, schema.nodes.blockquote)).toBe(false)
  })
  test("AllSelection over a list reports isInside(list) = true", () => {
    const d = doc(ul(li(p("a")), li(p("b"))))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    expect(isInside(state, schema.nodes.bullet_list)).toBe(true)
  })
})

describe("innermostListAt", () => {
  test("returns the deepest enclosing list for a nested cursor", () => {
    const d = doc(ul(li(p("a"), ol(li(p("inner"))))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "inner") + 1)
    const innermost = innermostListAt(state)
    expect(innermost?.type).toBe(schema.nodes.ordered_list)
  })
  test("returns null when cursor is not in a list", () => {
    const d = doc(p("free"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "free") + 1)
    expect(innermostListAt(state)).toBeNull()
  })
})

describe("selectionSpansList", () => {
  test("false when selection stays inside one list", () => {
    const d = doc(ul(li(p("a")), li(p("b"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "a") + 1)
    expect(selectionSpansList(state)).toBe(false)
  })
  test("true when selection crosses out of a list", () => {
    const d = doc(ul(li(p("a"))), p("after"))
    const fromPos = findParagraphPos(d, "a") + 1
    const toPos = findParagraphPos(d, "after") + 1
    const state = withSelection(stateWith(d), fromPos, toPos)
    expect(selectionSpansList(state)).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// canIndentAt / canOutdentAt - gate the toolbar Indent/Outdent buttons.
// We want that the behavior matches what Tab/Shift-Tab would actually do.
// ---------------------------------------------------------------------------

describe("canIndentAt", () => {
  test("false at the first list_item (no previous sibling to nest under)", () => {
    const d = doc(ul(li(p("a")), li(p("b"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "a") + 1)
    expect(canIndentAt(state)).toBe(false)
  })
  test("true at a non-first list_item", () => {
    const d = doc(ul(li(p("a")), li(p("b"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "b") + 1)
    expect(canIndentAt(state)).toBe(true)
  })
  test("true inside a code block (indentCodeBlock always applies)", () => {
    const d = doc(pre("code"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "code", schema.nodes.preformatted) + 1)
    expect(canIndentAt(state)).toBe(true)
  })
  test("false at doc-level paragraph", () => {
    const d = doc(p("free"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "free") + 1)
    expect(canIndentAt(state)).toBe(false)
  })
})

describe("canOutdentAt", () => {
  test("true inside any list_item (liftListItem always applies)", () => {
    const d = doc(ul(li(p("a"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "a") + 1)
    expect(canOutdentAt(state)).toBe(true)
  })
  test("false inside a code block with no leading tabs to strip", () => {
    const d = doc(pre("nocode"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "nocode", schema.nodes.preformatted) + 1)
    expect(canOutdentAt(state)).toBe(false)
  })
  test("true inside a code block when a touched line has a leading tab", () => {
    const d = doc(pre("\tindented"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "\tindented", schema.nodes.preformatted) + 1)
    expect(canOutdentAt(state)).toBe(true)
  })
})

// ---------------------------------------------------------------------------
// findBlockquotePosAt / findLinkRangeAt / getCurrentLinkValue
// ---------------------------------------------------------------------------

describe("findBlockquotePosAt", () => {
  test("returns the position of the enclosing blockquote", () => {
    const d = doc(bq(bp("inside")))
    const state = withCursor(stateWith(d), findParagraphPos(d, "inside", schema.nodes.blockquote_paragraph) + 1)
    const pos = findBlockquotePosAt(state)
    expect(pos).not.toBeNull()
    expect(d.nodeAt(pos!)?.type).toBe(schema.nodes.blockquote)
  })
  test("returns null when cursor is not inside a blockquote", () => {
    const d = doc(p("free"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "free") + 1)
    expect(findBlockquotePosAt(state)).toBeNull()
  })
})

describe("findLinkRangeAt / getCurrentLinkValue", () => {
  test("finds the contiguous link-mark range and reports the href", () => {
    const d = doc(p("pre ", linked("https://x", "linked"), " post"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "pre linked post") + 6)
    const range = findLinkRangeAt(state)
    expect(range).not.toBeNull()
    expect(state.doc.textBetween(range!.from, range!.to)).toBe("linked")
    expect(getCurrentLinkValue(state)).toBe("https://x")
  })
  test("getCurrentLinkValue returns blockquote cite when not in a link", () => {
    const d = doc(schema.nodes.blockquote.create({ cite: "https://src" }, [bp("text")]))
    const state = withCursor(stateWith(d), findParagraphPos(d, "text", schema.nodes.blockquote_paragraph) + 1)
    expect(getCurrentLinkValue(state)).toBe("https://src")
  })
})

// ---------------------------------------------------------------------------
// rangeContainsInTr / removeHrsInRangeInTr
// ---------------------------------------------------------------------------

describe("rangeContainsInTr", () => {
  test("true when selection covers a node of the given type", () => {
    const d = doc(p("a"), hr(), p("b"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    expect(rangeContainsInTr(state.tr, schema.nodes.horizontal_rule)).toBe(true)
  })
  test("false when selection is entirely inside a single paragraph", () => {
    const d = doc(p("plain"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "plain") + 1)
    expect(rangeContainsInTr(state.tr, schema.nodes.horizontal_rule)).toBe(false)
  })
})

describe("removeHrsInRangeInTr", () => {
  test("deletes hrs the selection range touches", () => {
    const d = doc(p("a"), hr(), p("b"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => removeHrsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(p("a"), p("b")).toJSON())
  })
})

// ---------------------------------------------------------------------------
// splitPreformattedInTr / liftBlockquotesInRangeInTr
// ---------------------------------------------------------------------------

describe("splitPreformattedInTr", () => {
  test("splits a multi-line preformatted into one paragraph per line", () => {
    const d = doc(pre("line1\nline2\nline3"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "line1\nline2\nline3", schema.nodes.preformatted) + 1)
    const after = runMutator(state, (tr) => splitPreformattedInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(p("line1"), p("line2"), p("line3")).toJSON())
  })
  test("returns false when the cursor is not in a preformatted", () => {
    const d = doc(p("nope"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "nope") + 1)
    const tr = state.tr
    expect(splitPreformattedInTr(tr)).toBe(false)
  })
  test("AllSelection is preserved across the split", () => {
    const d = doc(pre("line1\nline2"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => splitPreformattedInTr(tr))
    expect(after.selection).toBeInstanceOf(AllSelection)
  })
})

describe("liftBlockquotesInRangeInTr", () => {
  test("unwraps every blockquote the range touches", () => {
    const d = doc(bq(bp("a"), bp("b")))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => liftBlockquotesInRangeInTr(tr))
    // blockquote_paragraph re-emits as paragraph at the doc level.
    expect(after.doc.toJSON()).toEqual(doc(p("a"), p("b")).toJSON())
  })
  test("AllSelection is preserved across the unwrap", () => {
    const d = doc(bq(bp("a"), bp("b")))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => liftBlockquotesInRangeInTr(tr))
    expect(after.selection).toBeInstanceOf(AllSelection)
  })
})

// ---------------------------------------------------------------------------
// liftListsInRangeInTr - the centerpiece. Each test below codifies a
// scenario we walked through during development.
// ---------------------------------------------------------------------------

describe("liftListsInRangeInTr", () => {
  test("cursor in primary paragraph lifts the entire list_item one level up", () => {
    // ul[li[p"a"]] + cursor in p"a" -> p"a" at doc level.
    const d = doc(ul(li(p("a"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "a") + 1)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(p("a")).toJSON())
  })

  test("cursor in non-primary paragraph extracts just that paragraph past the list", () => {
    // li[p"a", p"b"] + cursor in p"b" (li is last in its only-ul, so no split)
    // -> [li[p"a"]], p"b"
    const d = doc(ul(li(p("a"), p("b"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "b") + 1)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("a"))), p("b")).toJSON())
  })

  test("extraction takes the immediately following sub-list along with the paragraph", () => {
    // li_0[p"a", p"b" (cursor), ul[li[p"c"]]] -> li_0[p"a"], p"b", ul[li[p"c"]]
    const d = doc(ul(li(p("a"), p("b"), ul(li(p("c"))))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "b") + 1)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("a"))), p("b"), ul(li(p("c")))).toJSON())
  })

  test("extraction from a non-last li splits the parent list around it", () => {
    // ul[li_0[p"a", p"foobar"], li_1[p"c"]] + cursor in p"foobar" ->
    // ul[li_0[p"a"]], p"foobar", ul[li_1[p"c"]]
    const d = doc(ul(li(p("a"), p("foobar")), li(p("c"))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "foobar") + 1)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("a"))), p("foobar"), ul(li(p("c")))).toJSON())
  })

  test("non-primary extraction in a non-last li joins adjacent same-type sub-lists inside the enclosing list_item", () => {
    // The "click 2" shape from development:
    //   outer_li[p"P", middle_ul[middle_li_0[p"A", ul[li[p"B"]], p"foobar", ul[li[p"C"]]], middle_li_1[p"D"]]]
    // Cursor in p"foobar" (non-primary in middle_li_0). middle_li_0
    // is at idx 0 in middle_ul (not last). Extraction takes
    // [foobar, ul[li[p"C"]]] together with its trailing sub-list,
    // truncates middle_li_0 to [p"A", ul[li[p"B"]]], splits middle_ul,
    // and JOINS the extracted ul[li[p"C"]] with the split's second
    // half (which is the same-type middle_ul containing middle_li_1)
    // because the insert lands inside outer_li (a list_item).
    const d = doc(ul(li(p("P"), ul(li(p("A"), ul(li(p("B"))), p("foobar"), ul(li(p("C")))), li(p("D"))))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "foobar") + 1)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("P"), ul(li(p("A"), ul(li(p("B"))))), p("foobar"), ul(li(p("C")), li(p("D")))))).toJSON())
  })

  test("selection across primary paragraphs in nested lists lifts both list_items in one pass", () => {
    // ul[li[p"a", ul[li[p"b", ul[li[p"c"]]]]]] + selection from p"b" to p"c"
    // -> ul[li[p"a", p"b", p"c"]]
    // (Case A from the user-verified scenarios.)
    const d = doc(ul(li(p("a"), ul(li(p("b"), ul(li(p("c"))))))))
    const fromPos = findParagraphPos(d, "b") + 1
    const toPos = findParagraphPos(d, "c") + 2
    const state = withSelection(stateWith(d), fromPos, toPos)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("a"), p("b"), p("c")))).toJSON())
  })

  test("cursor preserved through extraction (lands inside the extracted paragraph)", () => {
    // ul[li[p"a", p"foobar"]] + cursor inside foobar
    // After extraction, cursor should still be inside the new foobar at doc level.
    const d = doc(ul(li(p("a"), p("foobar"))))
    const fooPos = findParagraphPos(d, "foobar")
    // Cursor in middle of "foobar" (offset 3).
    const state = withCursor(stateWith(d), fooPos + 4)
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    // Resolve the cursor in the after-doc and assert it lands inside the lifted p"foobar".
    const $sel = after.selection.$from
    expect($sel.parent.type).toBe(schema.nodes.paragraph)
    expect($sel.parent.textContent).toBe("foobar")
  })

  test("does nothing for a cursor not inside a list (returns false)", () => {
    const d = doc(p("free"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "free") + 1)
    const tr = state.tr
    expect(liftListsInRangeInTr(tr)).toBe(false)
  })

  test("AllSelection is preserved across the lift", () => {
    // Ctrl+A on a doc whose only content is a list. After the lift,
    // the structure is just plain paragraphs at the doc level and the
    // user's "everything is selected" intent should carry through to
    // a fresh AllSelection on the new doc - not collapse to a
    // TextSelection over the lifted content.
    const d = doc(ul(li(p("a"), ul(li(p("b"))))))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => liftListsInRangeInTr(tr))
    expect(after.selection).toBeInstanceOf(AllSelection)
  })
})

// ---------------------------------------------------------------------------
// dissolveContainersInTr - drives the trySetBlockType heading path and the
// setPreformatted / setBlockquote merges. Heading from a deeply
// nested list dissolves until selected paragraphs reach doc level.
// ---------------------------------------------------------------------------

describe("dissolveContainersInTr", () => {
  test("repeatedly lifts until the selection's textblock reaches doc level", () => {
    // ul[li[p"a", ul[li[p"sdfs"]]]] + cursor in p"sdfs". The first
    // liftListsInRangeInTr lifts inner_li (p"sdfs" merges into outer li),
    // the next pass extracts p"sdfs" out of outer_li (non-primary now).
    const d = doc(ul(li(p("a"), ul(li(p("sdfs"))))))
    const state = withCursor(stateWith(d), findParagraphPos(d, "sdfs") + 1)
    const after = runMutator(state, (tr) => dissolveContainersInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(ul(li(p("a"))), p("sdfs")).toJSON())
  })

  test("lifts blockquotes alongside lists", () => {
    const d = doc(bq(bp("inside")))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => dissolveContainersInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(p("inside")).toJSON())
  })

  test("strips hr from the selection range", () => {
    const d = doc(p("a"), hr(), p("b"))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => dissolveContainersInTr(tr))
    expect(after.doc.toJSON()).toEqual(doc(p("a"), p("b")).toJSON())
  })
  test("AllSelection survives a multi-pass dissolve through nested lists", () => {
    // Ctrl+A over a deeply nested list then heading-style dissolve.
    // Each pass of liftListsInRangeInTr preserves the AllSelection
    // through its own selection-set, and dissolveContainersInTr loops
    // until everything is at doc level - the final selection should
    // still be the doc-wide AllSelection rather than a TextSelection
    // collapsed to one of the lifted paragraphs.
    const d = doc(ul(li(p("a"), ul(li(p("b"), ul(li(p("c"))))))))
    const state = stateWith(d).apply(stateWith(d).tr.setSelection(new AllSelection(d)))
    const after = runMutator(state, (tr) => dissolveContainersInTr(tr))
    expect(after.selection).toBeInstanceOf(AllSelection)
    // Sanity: the dissolve still reaches doc-level paragraphs.
    expect(after.doc.toJSON()).toEqual(doc(p("a"), p("b"), p("c")).toJSON())
  })
})

// ---------------------------------------------------------------------------
// findTextblockPositionInDoc / applyCommandToTr / insertLeafNode
// ---------------------------------------------------------------------------

describe("findTextblockPositionInDoc", () => {
  test("locates a position inside the second textblock as (index=1, offset)", () => {
    const d = doc(p("alpha"), p("beta"))
    // Range covers the whole doc.
    const target = findParagraphPos(d, "beta") + 3
    const got = findTextblockPositionInDoc(d, target, 0, d.content.size)
    expect(got).toEqual({ textblockIndex: 1, offset: 2 })
  })
  test("returns null for a position not inside any textblock", () => {
    const d = doc(p("alpha"))
    expect(findTextblockPositionInDoc(d, 0, 0, d.content.size)).toBeNull()
  })
})

describe("applyCommandToTr", () => {
  test("applies a command's steps onto the shared transaction", () => {
    const d = doc(h(1, "title"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "title", schema.nodes.heading) + 1)
    const tr = state.tr
    const ok = applyCommandToTr(state, tr, setBlockType(schema.nodes.paragraph))
    expect(ok).toBe(true)
    expect(state.apply(tr).doc.toJSON()).toEqual(doc(p("title")).toJSON())
  })
  test("returns false for a no-op command", () => {
    // setBlockType(paragraph) on a paragraph is a no-op (hasMarkup
    // already matches), so applyCommandToTr reports false.
    const d = doc(p("plain"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "plain") + 1)
    const tr = state.tr
    expect(applyCommandToTr(state, tr, setBlockType(schema.nodes.paragraph))).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// indentCodeBlock / unindentCodeBlock / insertLineBreak / exit* commands
// ---------------------------------------------------------------------------

describe("indentCodeBlock", () => {
  test("inserts a tab at the cursor inside a code block", () => {
    const d = doc(pre("abc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "abc", schema.nodes.preformatted) + 2)
    const { state: after, ran } = runCommand(state, indentCodeBlock)
    expect(ran).toBe(true)
    expect(after.doc.toJSON()).toEqual(doc(pre("a\tbc")).toJSON())
  })

  test("returns false outside a code block", () => {
    const d = doc(p("abc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "abc") + 1)
    const { ran } = runCommand(state, indentCodeBlock)
    expect(ran).toBe(false)
  })
})

describe("unindentCodeBlock", () => {
  test("strips a leading tab from the current line", () => {
    const d = doc(pre("\tabc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "\tabc", schema.nodes.preformatted) + 2)
    const { ran, state: after } = runCommand(state, unindentCodeBlock)
    expect(ran).toBe(true)
    expect(after.doc.toJSON()).toEqual(doc(pre("abc")).toJSON())
  })
})

describe("insertLineBreak", () => {
  test("inserts a hard_break outside a code block", () => {
    const d = doc(p("abc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "abc") + 2)
    const { state: after } = runCommand(state, insertLineBreak)
    // Paragraph content becomes [text "a", hard_break, text "bc"].
    const para = after.doc.firstChild!
    expect(para.childCount).toBe(3)
    expect(para.child(1).type).toBe(schema.nodes.hard_break)
  })

  test("inserts a literal \\n inside a code block (no hard_break)", () => {
    const d = doc(pre("abc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "abc", schema.nodes.preformatted) + 2)
    const { state: after } = runCommand(state, insertLineBreak)
    expect(after.doc.toJSON()).toEqual(doc(pre("a\nbc")).toJSON())
  })
})

describe("exitPreformattedOnEmpty", () => {
  test("converts an empty preformatted to a paragraph", () => {
    const d = doc(pre(""))
    const state = withCursor(stateWith(d), 1)
    const { ran, state: after } = runCommand(state, exitPreformattedOnEmpty)
    expect(ran).toBe(true)
    expect(after.doc.toJSON()).toEqual(doc(p()).toJSON())
  })
  test("returns false in the middle of a code block", () => {
    const d = doc(pre("abc"))
    const state = withCursor(stateWith(d), findParagraphPos(d, "abc", schema.nodes.preformatted) + 2)
    const { ran } = runCommand(state, exitPreformattedOnEmpty)
    expect(ran).toBe(false)
  })
})

describe("exitBlockquoteOnEmpty", () => {
  test("converts a single-empty-bp blockquote to a paragraph", () => {
    const d = doc(bq(bp("")))
    const state = withCursor(stateWith(d), 2)
    const { ran, state: after } = runCommand(state, exitBlockquoteOnEmpty)
    expect(ran).toBe(true)
    expect(after.doc.toJSON()).toEqual(doc(p()).toJSON())
  })
  test("returns false on a non-empty blockquote_paragraph", () => {
    const d = doc(bq(bp("hi")))
    const state = withCursor(stateWith(d), 2)
    const { ran } = runCommand(state, exitBlockquoteOnEmpty)
    expect(ran).toBe(false)
  })
})

// ---------------------------------------------------------------------------
// Schema invariant: heading is not admissible inside a list_item.
// The toolbar's heading button has to dissolve selected content all the way
// to doc level precisely because the schema rejects heading inside list_item.
// ---------------------------------------------------------------------------

describe("schema: list_item content", () => {
  test("rejects heading as a list_item child", () => {
    expect(() => li(h(1, "no")).check()).toThrow()
  })
  test("rejects preformatted as a list_item child", () => {
    expect(() => li(pre("no")).check()).toThrow()
  })
  test("rejects blockquote as a list_item child", () => {
    expect(() => li(bq(bp("no"))).check()).toThrow()
  })
  test("accepts paragraph + sub-list pair", () => {
    expect(() => li(p("primary"), ul(li(p("sub")))).check()).not.toThrow()
  })
})
