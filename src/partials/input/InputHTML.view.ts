import type { Attrs, Node, NodeType } from "prosemirror-model"
import type { Command, EditorState } from "prosemirror-state"
import type { EditorView } from "prosemirror-view"

import { chainCommands, setBlockType, toggleMark } from "prosemirror-commands"
import { redo, undo } from "prosemirror-history"
import { liftListItem, sinkListItem, wrapInList } from "prosemirror-schema-list"
import { AllSelection, TextSelection } from "prosemirror-state"

import { schema } from "@/partials/input/InputHTML.schema"
import {
  applyCommandToTr,
  dissolveContainersInTr,
  findBlockquotePosAt,
  findLinkRangeAt,
  findTextblockPositionInDoc,
  indentCodeBlock,
  innermostListAt,
  insertLeafNode,
  insertLineBreak,
  isInside,
  isNodeActive,
  liftBlockquotesInRangeInTr,
  liftListsInRangeInTr,
  rangeContainsInTr,
  removeHrsInRangeInTr,
  selectionSpansList,
  splitPreformattedInTr,
  unindentCodeBlock,
} from "@/partials/input/InputHTML.state"

// Bottom-toolbar edit-pin shape. The Vue side stores this in a ref and
// passes the current value down to the link (including attachment) /
// blockquote handlers and the active-range decoration plugin.
export type EditPin = { kind: "link"; from: number; to: number } | { kind: "blockquote"; pos: number }

// Snapshot of the selection captured when an insert-style flow starts -
// the Link button (link-insert mode) and the Attach file button both
// stash the current selection here so applyInsertedLink can place the
// new mark on the originally-selected range even after focus has moved
// to the bottom toolbar / native file picker.
export type InsertSelection = { from: number; to: number; empty: boolean }

// Apply a PM Command to the view and return focus.
export function runCommand(view: EditorView | null, command: Command) {
  if (!view) return
  command(view.state, view.dispatch, view)
  view.focus()
}

export function triggerUndo(view: EditorView | null) {
  runCommand(view, undo)
}

export function triggerRedo(view: EditorView | null) {
  runCommand(view, redo)
}

export function toggleBold(view: EditorView | null) {
  runCommand(view, toggleMark(schema.marks.bold))
}

export function toggleItalic(view: EditorView | null) {
  runCommand(view, toggleMark(schema.marks.italic))
}

export function toggleUnderline(view: EditorView | null) {
  runCommand(view, toggleMark(schema.marks.underline))
}

export function toggleStrikethrough(view: EditorView | null) {
  runCommand(view, toggleMark(schema.marks.strikethrough))
}

export function toggleMonospace(view: EditorView | null) {
  runCommand(view, toggleMark(schema.marks.monospace))
}

export function insertHorizontalRule(view: EditorView | null) {
  runCommand(view, insertLeafNode(schema.nodes.horizontal_rule))
}

export function insertHardBreak(view: EditorView | null) {
  runCommand(view, insertLineBreak)
}

// Toolbar indent / outdent. Same chained commands as the Tab /
// Shift-Tab keymap entries: inside a code block they prepend / strip
// a literal tab on each touched line, inside a list they nest the
// current list_item under the previous one (sinkListItem) or lift it
// one level (liftListItem). The toolbar buttons are gated by
// canIndent / canOutdent in updateActiveState, so they only enable
// when the cursor / selection sits in one of those two contexts -
// outside, the chained command would no-op anyway.
export function indentList(view: EditorView | null) {
  runCommand(view, chainCommands(indentCodeBlock, sinkListItem(schema.nodes.list_item)))
}

export function outdentList(view: EditorView | null) {
  runCommand(view, chainCommands(unindentCodeBlock, liftListItem(schema.nodes.list_item)))
}

// Replace the preformatted containing the cursor with a list (bullet
// or ordered) whose items each carry one \n-delimited line as a
// paragraph. Lets setListType produce one list item per code-block
// line instead of one giant item with embedded newlines (which would
// render as one visual line under white-space: normal). Returns true
// when the replacement was dispatched, false when there was no
// preformatted at the cursor depth (caller falls back to the standard
// wrap path).
export function convertPreformattedToList(view: EditorView | null, targetType: NodeType): boolean {
  if (!view) return false
  const state = view.state
  // Ancestor walk for cursor-inside-preformatted, plus a range-walk
  // fallback for AllSelection / wide selections where $from sits at the
  // doc level (so the preformatted is a sibling-level child of doc, not
  // an ancestor of $from).
  const { $from } = state.selection
  let pfPos = -1
  for (let d = $from.depth; d >= 1; d--) {
    if ($from.node(d).type === schema.nodes.preformatted) {
      pfPos = $from.before(d)
      break
    }
  }
  if (pfPos < 0) {
    state.doc.nodesBetween(state.selection.from, state.selection.to, (node, pos) => {
      if (pfPos < 0 && node.type === schema.nodes.preformatted) {
        pfPos = pos
        return false
      }
      return undefined
    })
  }
  if (pfPos < 0) return false
  const pfNode = state.doc.nodeAt(pfPos)
  if (!pfNode || pfNode.type !== schema.nodes.preformatted) return false
  const text = pfNode.textContent
  const lines = text.split("\n")
  const items: Node[] = lines.map((line) =>
    schema.nodes.list_item.create(null, line.length > 0 ? schema.nodes.paragraph.create(null, schema.text(line)) : schema.nodes.paragraph.create()),
  )
  const list = targetType.create(null, items)
  // Selection preservation: same line/offset walk as splitPreformatted,
  // but mapped through a deeper position layout because each line ends
  // up wrapped as list > list_item > paragraph > text. lineToPos lands
  // inside the paragraph at the right character offset.
  const fromLocal = Math.max(0, state.selection.from - (pfPos + 1))
  const toLocal = Math.max(0, state.selection.to - (pfPos + 1))
  const lineToPos = (offset: number): number => {
    let lineIndex = 0
    let lineOffset = 0
    const clamped = Math.min(offset, text.length)
    for (let i = 0; i < clamped; i++) {
      if (text[i] === "\n") {
        lineIndex++
        lineOffset = 0
      } else {
        lineOffset++
      }
    }
    lineIndex = Math.min(lineIndex, items.length - 1)
    // pfPos: before the new list
    // pfPos + 1: inside list, before items[0]
    // +sum(items[0..i-1].nodeSize): before items[lineIndex]
    // +2: into list_item, into its paragraph
    // +lineOffset: into the paragraph's text content
    let pos = pfPos + 1
    for (let i = 0; i < lineIndex; i++) pos += items[i].nodeSize
    pos += 2 + Math.min(lineOffset, lines[lineIndex].length)
    return pos
  }
  let tr = state.tr.replaceWith(pfPos, pfPos + pfNode.nodeSize, list)
  // AllSelection over the original preformatted -> AllSelection over the
  // doc with the new list, so the user's "select everything" carries
  // through. Otherwise map cursor/range into the list-item's paragraph.
  if (state.selection instanceof AllSelection) {
    tr = tr.setSelection(new AllSelection(tr.doc))
  } else {
    tr = tr.setSelection(TextSelection.create(tr.doc, lineToPos(fromLocal), lineToPos(toLocal)))
  }
  view.dispatch(tr)
  view.focus()
  return true
}

// Try setBlockType, falling back to lifting out of any enclosing list
// item when the target node cannot sit there per the schema. Based
// on our schema, converting a list item's first paragraph to a heading
// or preformatted block is rejected by setBlockType. Lift the list
// item out (which moves the textblock to the parent level where the
// conversion is valid) and retry. For non-list contexts the first call
// either succeeds or no-ops (already that type) and we never reach the
// fallback; for the "already this type inside a list" case the lift
// still runs and the user exits the list, which is what a click on the
// paragraph button while inside a list ought to do.
//
// liftAllLists controls how aggressively we exit nested lists when the
// target node cannot sit inside a list_item. Paragraph (false) lifts one
// level per click so repeated clicks step out one wrapper at a time,
// matching the "click paragraph -> leave the list one step" UX. Heading
// (true) keeps lifting until the target textblocks reach a place where
// the schema accepts the new node, which for heading is the doc level -
// the user expects a heading click to fully escape the list and become
// a top-level heading in one go.
export function trySetBlockType(view: EditorView | null, type: NodeType, attrs?: Attrs | null, liftAllLists = false) {
  if (!view) return
  // All work happens on a single shared transaction. Without this the
  // conversion would dispatch multiple times (split, retype, lift,
  // retype, ...) and end up as several entries on the undo stack -
  // history's auto-grouping only merges adjacent-range transactions
  // within newGroupDelay, and our scattered replaces are not adjacent.
  const initialState = view.state
  const tr = initialState.tr
  const cmd = setBlockType(type, attrs ?? null)
  // Leaving a preformatted block: split its \n-delimited text into one
  // paragraph per line first, so the visual line structure survives.
  // Plain setBlockType below would leave \n characters embedded in a
  // single target node, which collapse to spaces under white-space:
  // normal.
  if (isNodeActive(initialState, schema.nodes.preformatted) && type !== schema.nodes.preformatted) {
    splitPreformattedInTr(tr)
  }
  // First pass of setBlockType. For mixed [paragraph, list] selections
  // this converts the top-level paragraph but leaves list_items'
  // paragraphs untouched (list_item's content spec rejects heading
  // etc.). The dissolve loop below handles any remaining wrappers.
  const succeededInitially = applyCommandToTr(initialState, tr, cmd)
  // Distinguish "cursor / range stays inside one textblock" from "the
  // selection spans multiple blocks". sameTextblock catches collapsed
  // cursors and small ranges within a single paragraph; it's false for
  // ranges crossing block boundaries, AllSelection ($from.parent is doc,
  // not a textblock), and NodeSelection on a leaf. Only the
  // refused-AND-sameTextblock case keeps the original single-level
  // lift fallback ("click paragraph exits the wrapper one level");
  // everything else goes through the dissolve loop so wrappers
  // anywhere in the range get unwrapped and retyped uniformly.
  const $from = initialState.selection.$from
  const $to = initialState.selection.$to
  const sameTextblock = $from.parent.isTextblock && $from.sameParent($to)
  if (!succeededInitially && sameTextblock) {
    // setBlockType refused, cursor stays inside one textblock: the
    // target is rejected by the surrounding wrapper's content spec
    // (e.g. list_item rejects heading because its first child must be
    // paragraph). Use the precise per-level lift to exit the wrapper,
    // then retry setBlockType - this matches the existing "click
    // paragraph exits the list" UX where only the cursor's textblock
    // is affected, not the entire list.
    const liftCmd = liftListItem(schema.nodes.list_item)
    const checkInList = (): boolean => {
      const s = initialState.apply(tr)
      return isInside(s, schema.nodes.bullet_list) || isInside(s, schema.nodes.ordered_list)
    }
    if (checkInList()) {
      if (liftAllLists) {
        // Heading: loop liftListItem until $from exits all enclosing
        // lists, or the command refuses (e.g. AllSelection where
        // blockRange's first child is the list itself, not a
        // list_item). The user expects a heading click to fully
        // escape the list and become a top-level heading.
        while (checkInList() && applyCommandToTr(initialState, tr, liftCmd)) {
          // empty: applyCommandToTr does the work
        }
        // If still inside a list (e.g. AllSelection), dissolve the
        // whole list as a fallback.
        if (checkInList()) {
          liftListsInRangeInTr(tr)
        }
      } else {
        // Paragraph: one lift per click. Route through the same
        // primary/non-primary classifier the selection path uses, so
        // a cursor in a non-primary paragraph extracts just that
        // paragraph out of its list_item (matching the "selected
        // non-primary paragraphs extract, primary paragraph lifts the
        // whole list_item" UX), while a cursor in a primary paragraph
        // lifts the whole list_item one level up. Repeated clicks
        // progressively flatten nesting instead of jumping all the
        // way to doc level.
        liftListsInRangeInTr(tr)
      }
    }
    if (isInside(initialState.apply(tr), schema.nodes.blockquote)) {
      liftBlockquotesInRangeInTr(tr)
    }
    applyCommandToTr(initialState, tr, cmd)
  } else {
    // Either setBlockType succeeded (mixed selection where some
    // textblocks were retyped, others left because their wrapper
    // rejected the target) OR refused but the selection spans multiple
    // blocks (everything is already the target type but the range
    // still contains wrappers that should be dissolved - e.g.
    // [paragraph, list] + paragraph: nothing to retype, but the list
    // should be unwrapped). Dissolve loop runs in both cases. Termination
    // is guaranteed by the !progressed check - each lift helper only
    // returns true when it actually removed a wrapper from tr.doc, so
    // the wrapper count is strictly decreasing. The counter is an
    // assertion: 100 iterations far exceeds any plausible nesting depth,
    // so hitting it means a helper returned true without making
    // progress (a bug) and we want to surface it loudly rather than
    // silently bail out.
    let iterations = 0
    // liftListsInRangeInTr does all its marking in one pass against the
    // selection's current shape, so we run it at most once per dissolve
    // loop unless liftAllLists is set. Re-calling it after the previous
    // pass dissolved an inner list would re-mark outer items (their
    // immediate-ancestor identity shifts as inner wrappers go away) and
    // cascade until every list is gone, which is not what the paragraph
    // button user asked for. Re-arm the pass only when a blockquote or
    // hr lift exposes new content inside list_items - those genuinely
    // change what should be marked. For the heading button (liftAllLists)
    // the cascade is the intent: keep dissolving until selected
    // paragraphs reach the doc level where heading is schema-valid.
    let listsHandled = false
    while (true) {
      if (iterations++ >= 100) throw new Error("trySetBlockType dissolve loop did not terminate")
      let progressed = false
      let nonListProgressed = false
      if (!listsHandled && (rangeContainsInTr(tr, schema.nodes.bullet_list) || rangeContainsInTr(tr, schema.nodes.ordered_list))) {
        if (liftListsInRangeInTr(tr)) progressed = true
        if (!liftAllLists) listsHandled = true
      }
      if (rangeContainsInTr(tr, schema.nodes.blockquote)) {
        if (liftBlockquotesInRangeInTr(tr)) {
          progressed = true
          nonListProgressed = true
        }
      }
      if (rangeContainsInTr(tr, schema.nodes.horizontal_rule)) {
        if (removeHrsInRangeInTr(tr)) {
          progressed = true
          nonListProgressed = true
        }
      }
      if (nonListProgressed) listsHandled = false
      if (!progressed) break
      applyCommandToTr(initialState, tr, cmd)
    }
  }
  if (tr.docChanged || tr.selectionSet) {
    view.dispatch(tr.scrollIntoView())
  }
  view.focus()
}

export function setParagraph(view: EditorView | null) {
  trySetBlockType(view, schema.nodes.paragraph)
}

export function setHeading(view: EditorView | null, level: number) {
  trySetBlockType(view, schema.nodes.heading, { level }, true)
}

export function setPreformatted(view: EditorView | null) {
  if (!view) return
  // No-op when already in a preformatted block. Matches the paragraph /
  // heading buttons (setBlockType is a no-op when called for the type
  // the cursor already sits in); switching back out is done by clicking
  // the paragraph button (or any other block-type button).
  if (isNodeActive(view.state, schema.nodes.preformatted)) {
    view.focus()
    return
  }
  // Merge every textblock the selection touches into one preformatted
  // block. setBlockType would instead convert each touched textblock to
  // its own preformatted, leaving the user with three adjacent code
  // blocks for a three-paragraph selection. Joining the text with
  // newlines matches what "code block" typically means - a single
  // contiguous block, line breaks preserved by the text node
  // (preformatted has parseDOM with preserveWhitespace: "full").
  const state = view.state
  const tr = state.tr
  const wasAllSelection = state.selection instanceof AllSelection
  // Dissolve any lists/blockquotes/hr that hold the selection so the
  // doc-level rangeStart/rangeEnd we compute below only covers the
  // lifted textblocks, not the whole containing list. Without this,
  // a selection of a few paragraphs inside a deeply nested list would
  // consume every sibling and ancestor in the list when the merge
  // step replaces $from.before(1) to $to.after(1).
  dissolveContainersInTr(tr)
  const $from = tr.selection.$from
  const $to = tr.selection.$to
  const rangeStart = $from.depth === 0 ? 0 : $from.before(1)
  const rangeEnd = $to.depth === 0 ? tr.doc.content.size : $to.after(1)
  // Same snapshot as setBlockquote: capture (textblockIndex, offset)
  // before merging so we can re-derive the cursor / selection inside the
  // single resulting preformatted.
  const fromTb = findTextblockPositionInDoc(tr.doc, tr.selection.from, rangeStart, rangeEnd)
  const toTb = findTextblockPositionInDoc(tr.doc, tr.selection.to, rangeStart, rangeEnd)
  let text = ""
  const sizes: number[] = []
  let firstBlock = true
  tr.doc.nodesBetween(rangeStart, rangeEnd, (node) => {
    if (node.isTextblock) {
      if (!firstBlock) text += "\n"
      text += node.textContent
      sizes.push(node.textContent.length)
      firstBlock = false
      return false
    }
    return undefined
  })
  const preformatted = text.length > 0 ? schema.nodes.preformatted.create(null, schema.text(text)) : schema.nodes.preformatted.create()
  tr.replaceWith(rangeStart, rangeEnd, preformatted)
  // Map an original (textblockIndex, offset) into the merged text. The
  // preformatted's text content starts at rangeStart + 1; preceding
  // textblocks contributed their size plus one \n separator each, then
  // the original character offset is added (clamped to the contributing
  // textblock's size since the merged text matches the originals
  // character-for-character).
  const prePos = (tb: { textblockIndex: number; offset: number } | null): number => {
    if (tb === null || sizes.length === 0) return rangeStart + 1
    const idx = Math.min(tb.textblockIndex, sizes.length - 1)
    let pos = rangeStart + 1
    for (let i = 0; i < idx; i++) pos += sizes[i] + 1
    pos += Math.min(tb.offset, sizes[idx])
    return pos
  }
  // AllSelection preserved across the conversion: the user "selected
  // everything" before, so after the merge they should still see
  // everything selected - including the new preformatted's outer
  // boundary - rather than just the text content inside. A plain
  // TextSelection from first to last text position would visually
  // highlight all the characters but leave the preformatted's wrapper
  // unselected; AllSelection covers the whole doc.
  if (wasAllSelection) {
    tr.setSelection(new AllSelection(tr.doc))
  } else {
    tr.setSelection(TextSelection.create(tr.doc, prePos(fromTb), prePos(toTb)))
  }
  view.dispatch(tr.scrollIntoView())
  view.focus()
}

// "Switch to bullet list" / "switch to ordered list" / "switch to
// blockquote" - mirror the setBlockType-style behavior of the paragraph
// and heading buttons. No-op when already in the target wrapper;
// otherwise convert the cursor's textblock to a paragraph (lists
// require their items to start with a paragraph; blockquote is
// consistent with this so a heading becomes plain text inside) and
// then wrap. For list-type transitions we lift the existing list item
// first so the result is the target list type rather than a nested
// list.
//
// Each step is a separate dispatch (so multiple undo steps when going
// from heading -> bullet); accepted trade-off for not having to write a
// custom command that mixes setBlockType / lift / wrap into a single
// transaction.
export function setBulletList(view: EditorView | null) {
  setListType(view, schema.nodes.bullet_list, schema.nodes.ordered_list)
}

export function setOrderedList(view: EditorView | null) {
  setListType(view, schema.nodes.ordered_list, schema.nodes.bullet_list)
}

// Switch the selection to targetType (bullet_list or ordered_list).
// Two cases:
//   1. The selection touches an existing list of the OTHER type
//      (otherType). Swap that list's node markup to targetType
//      directly - simpler than lift+rewrap, and works for AllSelection
//      where lift-based commands cannot operate because the doc-level
//      range's first child is the list itself, not a list_item.
//   2. Otherwise, lift any enclosing blockquote (blockquote's
//      rejects lists as children, so wrapInList would fail),
//      make sure each textblock is a paragraph (list_item requires
//      paragraph as the first child and admits no other textblock
//      types as later children), then wrap in the target list.
export function setListType(view: EditorView | null, targetType: NodeType, otherType: NodeType) {
  if (!view) return
  const state = view.state
  // Branch on whether the selection actually spans list boundaries.
  //   - No spanning (collapsed cursor, or range fully contained in one
  //     list): swap only the innermost enclosing list. Outer / sibling
  //     lists are left alone so clicking "ordered" while in a bullet
  //     nested under an ordered list flips just the inner bullet, not
  //     the outer ordered list.
  //   - Spanning (range crosses out of, into, or between lists): walk
  //     the whole range and swap every otherType list, including ones
  //     nested inside already-targetType lists. This is what the user
  //     means by "convert this selection to ordered" when the
  //     selection covers multiple lists.
  if (!selectionSpansList(state)) {
    const innermost = innermostListAt(state)
    if (innermost !== null) {
      if (innermost.type === targetType) {
        view.focus()
        return
      }
      view.dispatch(state.tr.setNodeMarkup(innermost.pos, targetType))
      view.focus()
      return
    }
  }
  // Preformatted: same line-preservation concern as setParagraph /
  // setHeading. The standard wrap path below would convert the
  // preformatted to a paragraph (\n characters surviving as text),
  // wrap it as a single list item containing one long line. Build the
  // list directly with one item per \n-delimited line instead.
  if (isNodeActive(state, schema.nodes.preformatted) && convertPreformattedToList(view, targetType)) return
  // Wide-range path: swap any otherType list the selection touches,
  // strip stray hr, dissolve any blockquote, ensure paragraphs, then
  // wrap. All four sub-operations are accumulated into one shared
  // transaction so the whole click ends up as a single undo step -
  // same reasoning as trySetBlockType above.
  const initialState = state
  const tr = initialState.tr
  // A list is "in scope" only when one of its direct list_items has a
  // direct paragraph child whose range overlaps the selection. An
  // ancestor list whose own list_items only contain the selection
  // VIA NESTED lists - i.e., no direct paragraph touched - does not
  // get swapped. This provides the heading/paragraph behavior we want:
  // clicking "ordered" while a selection sits 2 levels deep should
  // flip those 2 levels, not also the outer list whose list_items
  // only contain the inner list (no overlapping paragraph of its own).
  const { from: selFrom, to: selTo } = initialState.selection
  const listsInScope = new Set<number>()
  initialState.doc.nodesBetween(selFrom, selTo, (node, pos) => {
    if (node.type !== schema.nodes.paragraph) return undefined
    const $node = initialState.doc.resolve(pos)
    for (let d = $node.depth; d >= 1; d--) {
      const ancestor = $node.node(d)
      if (ancestor.type === schema.nodes.list_item) {
        const parent = $node.node(d - 1)
        if (parent.type === otherType) listsInScope.add($node.before(d - 1))
        break
      }
    }
    return undefined
  })
  for (const listPos of listsInScope) {
    // setNodeMarkup uses a same-size ReplaceAroundStep so siblings'
    // positions do not shift - safe to keep iterating initialState.doc
    // while stacking setNodeMarkup calls onto tr.
    tr.setNodeMarkup(listPos, targetType)
  }
  // Strip any horizontal_rule from the range first. hr is not admitted
  // by list_item's content spec, so leaving it in would make
  // wrapInList below fail outright (the whole click would no-op even
  // though the swap above may have succeeded). Doing it unconditionally
  // here is safe - no-op when no hr is present.
  removeHrsInRangeInTr(tr)
  // If, after any swap, the selection is already inside a list of the
  // target type, there is nothing left to wrap; bail out before the
  // wrap path would try to nest another list around it. Check against
  // tr's post-swap state, not the original view.state.
  if (isInside(initialState.apply(tr), targetType)) {
    if (tr.docChanged) view.dispatch(tr.scrollIntoView())
    view.focus()
    return
  }
  // Blockquote in range: dissolve it. blockquote's content spec rejects
  // lists, so wrapInList would fail unless we unwrap first.
  liftBlockquotesInRangeInTr(tr)
  // Ensure each textblock is a paragraph, then wrap.
  applyCommandToTr(initialState, tr, setBlockType(schema.nodes.paragraph))
  applyCommandToTr(initialState, tr, wrapInList(targetType))
  if (tr.docChanged || tr.selectionSet) {
    view.dispatch(tr.scrollIntoView())
  }
  view.focus()
}

// Blockquote does not behave like the textblock buttons (heading,
// paragraph, ...) which are happy to leave the existing block structure
// alone and just retype each textblock. The user's expectation here is
// that selecting a mix of blocks (e.g. a bullet item plus an existing
// blockquote) and clicking blockquote produces ONE blockquote that
// holds every touched textblock's content - no list wrappers, no
// per-textblock sibling blockquotes, all the inline content lifted
// into one blockquote. setBlockType on its own would turn each block
// into its own blockquote (and silently fail inside list_item, which
// forbids blockquote as the first child) - so we do the merge
// ourselves.
//
// We extend the operation up to the doc-level edges of the touched
// children so the new blockquote can land at the doc level (the only
// place the schema allows it). Each touched textblock is re-emitted
// as its own blockquote_paragraph inside the new blockquote. We also
// carry over the first cite attribute we encounter so an existing
// blockquote in the selection keeps its source URL.
export function setBlockquote(view: EditorView | null) {
  if (!view) return
  // Already inside a blockquote - no-op so we preserve cite and the
  // existing paragraph structure.
  if (isInside(view.state, schema.nodes.blockquote)) return
  const state = view.state
  const tr = state.tr
  const wasAllSelection = state.selection instanceof AllSelection
  // Capture cite from the first blockquote in the original selection
  // BEFORE dissolving: dissolveContainersInTr below lifts blockquotes
  // along with lists, so an existing blockquote's source URL would
  // disappear if we waited until after the dissolve to scan for it.
  let cite: string | null = null
  state.doc.nodesBetween(state.selection.from, state.selection.to, (node) => {
    if (node.type === schema.nodes.blockquote && cite === null) {
      cite = (node.attrs.cite as string | null) ?? null
    }
    return undefined
  })
  // Dissolve any lists/blockquotes/hr that hold the selection so the
  // doc-level rangeStart/rangeEnd we compute below only covers the
  // lifted textblocks, not the whole containing list. blockquote lives
  // at doc level only, so a click while inside a deeply nested list
  // needs to lift the selected paragraphs out first.
  dissolveContainersInTr(tr)
  const $from = tr.selection.$from
  const $to = tr.selection.$to
  const rangeStart = $from.depth === 0 ? 0 : $from.before(1)
  const rangeEnd = $to.depth === 0 ? tr.doc.content.size : $to.after(1)
  // Snapshot where the cursor / selection lived (which textblock in the
  // range, and how far inside) before we rebuild the range. After the
  // replacement we map (textblockIndex, offset) into the same-indexed
  // blockquote_paragraph so the cursor visibly stays on the same
  // characters.
  const fromTb = findTextblockPositionInDoc(tr.doc, tr.selection.from, rangeStart, rangeEnd)
  const toTb = findTextblockPositionInDoc(tr.doc, tr.selection.to, rangeStart, rangeEnd)
  // For each textblock the selection touches, re-emit its inline content
  // as a blockquote_paragraph inside the new blockquote. Regular paragraphs
  // all flatten to blockquote_paragraph. Italic marks are stripped because
  // blockquote_paragraph's marks allowlist excludes italic - leaving an
  // italic-bearing text node in would make replaceWith throw.
  const paragraphs: Node[] = []
  const italicType = schema.marks.italic
  tr.doc.nodesBetween(rangeStart, rangeEnd, (node) => {
    if (node.isTextblock) {
      const inline: Node[] = []
      node.content.forEach((child) => {
        const stripped = child.marks.some((m) => m.type === italicType) ? child.mark(child.marks.filter((m) => m.type !== italicType)) : child
        inline.push(stripped)
      })
      paragraphs.push(schema.nodes.blockquote_paragraph.create(null, inline))
      return false
    }
    return undefined
  })
  if (paragraphs.length === 0) paragraphs.push(schema.nodes.blockquote_paragraph.create())
  const blockquote = schema.nodes.blockquote.create(cite === null ? null : { cite }, paragraphs)
  tr.replaceWith(rangeStart, rangeEnd, blockquote)
  // Map an original textblock-relative position to its equivalent inside
  // the new blockquote. Start at rangeStart + 1 (inside the blockquote),
  // skip over preceding blockquote_paragraphs' full nodeSize, step into
  // the target child (+1), then add the original character offset
  // clamped to that child's content size (in case italic stripping or a
  // missing textblock - tb null - leaves us without a sensible offset).
  const bpPos = (tb: { textblockIndex: number; offset: number } | null): number => {
    if (tb === null) return rangeStart + 2
    const idx = Math.min(tb.textblockIndex, paragraphs.length - 1)
    let pos = rangeStart + 1
    for (let i = 0; i < idx; i++) pos += paragraphs[i].nodeSize
    pos += 1
    pos += Math.min(tb.offset, paragraphs[idx].content.size)
    return pos
  }
  // The user's "everything is selected" intent carries through to the
  // new blockquote and the doc that contains it, so they can immediately
  // chain another conversion (paragraph, list, etc.) on the whole thing.
  if (wasAllSelection) {
    tr.setSelection(new AllSelection(tr.doc))
  } else {
    tr.setSelection(TextSelection.create(tr.doc, bpPos(fromTb), bpPos(toTb)))
  }
  view.dispatch(tr.scrollIntoView())
  view.focus()
}

// Returns the link range we should operate on: the pinned range when an
// edit is in progress (so Update / Remove targets the originally-edited
// link even after the cursor wanders), otherwise the contiguous link-mark
// run at the cursor.
export function resolveLinkRange(state: EditorState, editPin: EditPin | null): { from: number; to: number } | null {
  if (editPin?.kind === "link") {
    return { from: editPin.from, to: editPin.to }
  }
  return findLinkRangeAt(state)
}

// Replaces the href of the link mark at the cursor by removing the existing
// mark across its full range and re-adding it with the new attrs. Doing it
// in one transaction keeps history collapsed to a single undo step.
export function applyLinkHref(view: EditorView | null, editPin: EditPin | null, href: string) {
  if (!view) return
  const { state } = view
  const linkType = schema.marks.link
  const range = resolveLinkRange(state, editPin)
  if (!range || range.from >= range.to) return
  let tr = state.tr.removeMark(range.from, range.to, linkType)
  tr = tr.addMark(range.from, range.to, linkType.create({ href }))
  view.dispatch(tr)
  view.focus()
}

// Strips the link mark from the entire range at the cursor (not just from
// the selection), so a cursor in the middle of a link clears the whole
// thing.
export function removeLinkAtRange(view: EditorView | null, editPin: EditPin | null) {
  if (!view) return
  const { state } = view
  const range = resolveLinkRange(state, editPin)
  if (!range || range.from >= range.to) return
  view.dispatch(state.tr.removeMark(range.from, range.to, schema.marks.link))
  view.focus()
}

// Deletes the entire link range (mark + visible text), as opposed to
// removeLinkAtRange which only strips the mark. Used by the file-edit
// toolbar's Remove button to drop the file link from the document.
export function deleteLinkRange(view: EditorView | null, editPin: EditPin | null) {
  if (!view) return
  const { state } = view
  const range = resolveLinkRange(state, editPin)
  if (!range || range.from >= range.to) return
  view.dispatch(state.tr.delete(range.from, range.to))
  view.focus()
}

// Sets (or clears) the cite attribute on the enclosing blockquote. An
// empty string is stored as null so the DOM serializer drops the attribute
// entirely (matching how blockquotes without cite parse back to null).
// Pinned-pos wins so an edit committed after the cursor wandered out
// still targets the original blockquote; defensive nodeAt check rejects
// pins whose blockquote has since been deleted out from under them.
export function applyBlockquoteCite(view: EditorView | null, editPin: EditPin | null, cite: string) {
  if (!view) return
  const { state } = view
  const pos = editPin?.kind === "blockquote" ? editPin.pos : findBlockquotePosAt(state)
  if (pos === null) return
  const node = state.doc.nodeAt(pos)
  if (!node || node.type !== schema.nodes.blockquote) return
  const nextAttrs = { ...node.attrs, cite: cite === "" ? null : cite }
  view.dispatch(state.tr.setNodeMarkup(pos, undefined, nextAttrs))
  view.focus()
}

// Applies a new link mark to the snapshotted selection and returns the
// InsertSelection the caller should use for any follow-up link in the
// same batch. The multi-file Attach flow drives this in a loop, calling
// once per uploaded file so users see each link appear progressively;
// the form's link-insert flow makes a single call and ignores the
// return value.
//
// Empty-selection branch: insert displayText (default: the href itself;
// the file-upload caller passes the filename so the visible text is
// human-readable rather than the storage URL). withLeadingSeparator
// requests a separator between this insert and the previous one - a
// hard_break inside an inline-content textblock, a literal "\n" inside
// a preformatted block.
//
// Non-empty selection: wrap the existing text with the link mark;
// displayText / withLeadingSeparator are ignored - the user already
// has visible content. Always returns null in this branch since the
// form path never chains.
//
// Position-fitting notes (empty branch):
//   - Inside a textblock (paragraph, heading, blockquote_paragraph)
//     the marked text node splices inline, the hard_break separator
//     too. The returned InsertSelection sits right after the inserted
//     text in the SAME textblock, so the next call's hard_break also
//     splices into the same block.
//   - At a doc-level / list / blockquote wrapper position PM's Fitter
//     wraps the marked text node in a new paragraph (it cannot host
//     inline content directly). The returned InsertSelection is then
//     shifted by -1 from the post-dispatch mapping so it lands INSIDE
//     that new paragraph's content end - so the next call in the
//     batch (which will request withLeadingSeparator) appends its
//     hard_break + text INTO the same paragraph. A withLeadingSeparator
//     on the first file at doc level is dropped (the parent's
//     content spec rejects hard_break and the marked text would
//     have wrapped it in its own paragraph anyway).
//   - Inside a preformatted block link marks cannot apply and
//     hard_break is not allowed - insert the URL itself as plain
//     text and use "\n" as the cross-file separator.
//
// Pre-marked text node (NOT tr.insertText + tr.addMark): when from is
// a position PM cannot host inline content at directly, the Fitter
// shifts the actual insertion - and a follow-up addMark(from, from
// + text.length) on the original from would cover the original
// position (no character there) and fall one position short of the
// inserted text, leaving the last character unmarked. Pre-marked
// text preserves the mark on every character regardless of any
// position fitting the replaceRange does.
export function applyInsertedLink(
  view: EditorView | null,
  insertSelection: InsertSelection | null,
  href: string,
  displayText?: string,
  withLeadingSeparator = false,
): InsertSelection | null {
  if (!view || !insertSelection) return null
  const linkType = schema.marks.link
  const hardBreakType = schema.nodes.hard_break
  const { from, to, empty } = insertSelection
  let tr = view.state.tr
  if (!empty) {
    tr = tr.addMark(from, to, linkType.create({ href }))
    // When the original selection extended past a textblock boundary
    // (Ctrl+A puts $from at the doc level, before the first paragraph),
    // the post-addMark selection's $from sits outside any text node.
    // getCurrentLinkValue and findLinkRangeAt both reach for marks at
    // $from / $from.nodeAfter and miss the newly-applied link, so the
    // bottom toolbar would resolve the href as empty and treat the
    // just-inserted link as "no link active". Snap to a TextSelection
    // covering the same logical range so $from lands inside text
    // content and the edit-mode code paths see the mark.
    const $from = tr.doc.resolve(from)
    const $to = tr.doc.resolve(to)
    if ($from.depth === 0 || $to.depth === 0) {
      tr = tr.setSelection(TextSelection.between($from, $to))
    }
    view.dispatch(tr)
    return null
  }
  const $from = tr.doc.resolve(from)
  const inPreformatted = $from.parent.type === schema.nodes.preformatted
  if (inPreformatted) {
    const text = (withLeadingSeparator ? "\n" : "") + href
    tr = tr.replaceRangeWith(from, from, schema.text(text))
  } else {
    let insertPos = from
    if (withLeadingSeparator && $from.parent.type.contentMatch.matchType(hardBreakType)) {
      tr = tr.replaceRangeWith(insertPos, insertPos, hardBreakType.create())
      // mapping(from, 1) is the position right after the hard_break,
      // accounting for any position fitting replaceRangeWith did.
      insertPos = tr.mapping.map(from, 1)
    }
    const text = displayText ?? href
    tr = tr.replaceRangeWith(insertPos, insertPos, schema.text(text, [linkType.create({ href })]))
  }
  view.dispatch(tr)
  // Where the next per-file insert should land. mapping(from, 1) sits
  // at the end of all inserted content in the post-dispatch doc. If
  // that ends up at a wrapper level (doc, list, blockquote) because
  // PM wrapped our inline content in a new textblock, shift one
  // position left to be INSIDE that textblock's content end so the
  // next call's hard_break splices in instead of getting wrapped
  // again. The shift is only safe when the previous sibling is a
  // textblock that admits inline content - which is exactly what PM
  // would have just created when it wrapped us, and what is true
  // already for normal inline drops where parent is unchanged.
  const newPos = tr.mapping.map(from, 1)
  const $new = tr.doc.resolve(newPos)
  let nextFrom = newPos
  if (!$new.parent.inlineContent && $new.nodeBefore && $new.nodeBefore.inlineContent) {
    nextFrom = newPos - 1
  }
  return { from: nextFrom, to: nextFrom, empty: true }
}
