import type { Attrs, MarkType, Node, NodeType, Mark as PMMark } from "prosemirror-model"
import type { Command, EditorState, Transaction } from "prosemirror-state"

import { Fragment, Slice } from "prosemirror-model"
import { AllSelection, TextSelection } from "prosemirror-state"
import { ReplaceAroundStep } from "prosemirror-transform"

import { schema } from "@/partials/input/InputHTML.schema"

// Everything here operates purely on PM types - state, transaction,
// node, schema - without touching Vue refs, the editor's view
// instance, or any component-scoped state.

// True when the doc has no leaf content - i.e. only empty paragraphs,
// no text, no leaf atom nodes (images, horizontal rules). The Vue
// side uses this against the live state to keep isStructurallyEmpty in
// sync regardless of whether the change came from the user or v-model.
export function isDocEmpty(doc: Node): boolean {
  let nonEmpty = false
  doc.descendants((node) => {
    if (node.isLeaf) nonEmpty = true
  })
  return !nonEmpty
}

export function isMarkActive(state: EditorState, type: MarkType): boolean {
  const { from, to, empty, $from } = state.selection
  if (empty) {
    return !!type.isInSet(state.storedMarks ?? $from.marks())
  }
  return state.doc.rangeHasMark(from, to, type)
}

// True when every textblock the selection touches has the given type
// and attrs. For an empty cursor or a selection inside a single
// textblock, the fast path checks $from.parent directly. For a
// selection that spans past $from's textblock end (e.g. Ctrl+A puts
// $from at the doc level), we walk the textblocks in the range and
// require all of them to match - otherwise the toolbar's pressed-state
// indicator would be wrong after a setBlockType that converted every
// block in the doc.
export function isNodeActive(state: EditorState, type: NodeType, attrs?: Attrs | null): boolean {
  const { $from, to } = state.selection
  if (to <= $from.end() && $from.parent.isTextblock) {
    return $from.parent.hasMarkup(type, attrs ?? null)
  }
  let allMatch = true
  let count = 0
  state.doc.nodesBetween($from.pos, to, (node) => {
    if (!allMatch) return false
    if (node.isTextblock) {
      count++
      if (!node.hasMarkup(type, attrs ?? null)) allMatch = false
      return false
    }
    // Any non-textblock block - a wrapper (blockquote, list,
    // list_item, ...) or a leaf block (horizontal_rule) - never matches
    // a textblock type, so its presence in a mixed selection should
    // break the "everything matches" verdict. We also do NOT descend
    // into wrappers: doing so would surface their inner textblocks
    // (list_item > paragraph, blockquote > blockquote_paragraph) and
    // falsely match against them, so a [list, paragraph] selection
    // would report isNodeActive(paragraph) == true. The user is
    // looking at top-level block type, not the structural children of
    // a wrapper.
    if (node.isBlock) {
      count++
      allMatch = false
      return false
    }
    return undefined
  })
  return count > 0 && allMatch
}

// Locates a doc-level position relative to the textblocks within a given
// range, returning the index of the containing textblock (0-based among
// textblocks visited by nodesBetween) and the character offset inside
// that textblock's inline content. setBlockquote / setPreformatted use
// this to preserve the user's cursor / selection across a wrapping or
// merging conversion: the (index, offset) pair survives because each
// original textblock ends up as an identifiable child of the new node in
// the same order. Returns null when the position does not sit inside any
// textblock within the range (e.g. AllSelection's $from at doc level
// before the first child).
export function findTextblockPositionInDoc(doc: Node, pos: number, rangeStart: number, rangeEnd: number): { textblockIndex: number; offset: number } | null {
  let textblockIndex = -1
  let result: { textblockIndex: number; offset: number } | null = null
  doc.nodesBetween(rangeStart, rangeEnd, (node, nodePos) => {
    if (node.isTextblock) {
      textblockIndex++
      const innerStart = nodePos + 1
      const innerEnd = nodePos + node.nodeSize - 1
      if (innerStart <= pos && pos <= innerEnd && result === null) {
        result = { textblockIndex, offset: pos - innerStart }
      }
      return false
    }
    return undefined
  })
  return result
}

// Deletes any horizontal_rule nodes the selection range touches and
// appends the delete steps to the shared transaction. trySetBlockType
// uses this in its dissolve loop because hr is a leaf block, not a
// textblock, and setBlockType silently skips it - so a [paragraph, hr,
// paragraph] + heading conversion would leave the hr stranded between
// two headings. setListType uses it before wrapInList because
// list_item's content spec does not admit hr - the wrap would otherwise
// fail outright. Returns true when an hr was removed.
export function removeHrsInRangeInTr(tr: Transaction): boolean {
  const { from, to } = tr.selection
  const positions: number[] = []
  tr.doc.nodesBetween(from, to, (node, pos) => {
    if (node.type === schema.nodes.horizontal_rule) {
      positions.push(pos)
      return false
    }
    return undefined
  })
  if (positions.length === 0) return false
  const baseMapLen = tr.mapping.maps.length
  let changed = false
  for (let i = positions.length - 1; i >= 0; i--) {
    const mappedPos = tr.mapping.slice(baseMapLen).map(positions[i])
    const node = tr.doc.nodeAt(mappedPos)
    if (!node || node.type !== schema.nodes.horizontal_rule) continue
    tr.delete(mappedPos, mappedPos + node.nodeSize)
    changed = true
  }
  return changed
}

// tr-based variant of rangeContains.
export function rangeContainsInTr(tr: Transaction, type: NodeType): boolean {
  const { from, to } = tr.selection
  let found = false
  tr.doc.nodesBetween(from, to, (node) => {
    if (found) return false
    if (node.type === type) {
      found = true
      return false
    }
    return undefined
  })
  return found
}

// Runs a PM Command against the state derived from the in-progress
// transaction (so it sees prior steps' effects) and merges any steps
// the command produces back into the same transaction, instead of
// dispatching a separate transaction per command.
export function applyCommandToTr(initialState: EditorState, tr: Transaction, cmd: Command): boolean {
  const tempState = initialState.apply(tr)
  let succeeded = false
  cmd(tempState, (innerTr) => {
    succeeded = true
    for (const step of innerTr.steps) tr.step(step)
    if (innerTr.selectionSet) tr.setSelection(innerTr.selection)
    if (innerTr.scrolledIntoView) tr.scrollIntoView()
  })
  return succeeded
}

// Dissolve every list, blockquote, and horizontal_rule the selection
// touches. Used to bring selected textblocks to doc level - preformatted
// and blockquote both live at doc level under our schema, so a
// "click preformatted while inside a deeply nested list" needs to
// lift the selected paragraphs out first or the merge below would
// otherwise consume the whole containing list. Each helper invocation
// only changes tr.doc when its target structure is actually present,
// and rangeContainsInTr gates them so empty iterations exit fast.
// Termination relies on each helper returning true only when it
// strictly reduced the doc's wrapper count; the iteration cap is an
// assertion, not a real limit.
export function dissolveContainersInTr(tr: Transaction) {
  let iterations = 0
  while (true) {
    if (iterations++ >= 100) throw new Error("dissolveContainersInTr did not terminate")
    let progressed = false
    if (rangeContainsInTr(tr, schema.nodes.bullet_list) || rangeContainsInTr(tr, schema.nodes.ordered_list)) {
      if (liftListsInRangeInTr(tr)) progressed = true
    }
    if (rangeContainsInTr(tr, schema.nodes.blockquote)) {
      if (liftBlockquotesInRangeInTr(tr)) progressed = true
    }
    if (rangeContainsInTr(tr, schema.nodes.horizontal_rule)) {
      if (removeHrsInRangeInTr(tr)) progressed = true
    }
    if (!progressed) break
  }
}

// Innermost enclosing bullet_list / ordered_list at the cursor, or null
// if the cursor is not inside any list. Fast path walks ancestors of
// $from from deepest to shallowest so the first match is the deepest
// list (the "current level" for the list buttons). For AllSelection /
// cursor at doc-level $from this would always miss because no list is
// an ancestor of the doc node itself, so a range-walk fallback looks
// for a list whose range fully covers the selection - that's the
// equivalent of "the selection is inside this list" for selections
// without a meaningful $from ancestor (e.g. after wrapInList runs on
// an AllSelection, the post-wrap AllSelection covers the new list).
export function innermostListAt(state: EditorState): { pos: number; type: NodeType } | null {
  const { $from } = state.selection
  for (let d = $from.depth; d >= 1; d--) {
    const node = $from.node(d)
    if (node.type === schema.nodes.bullet_list || node.type === schema.nodes.ordered_list) {
      return { pos: $from.before(d), type: node.type }
    }
  }
  const { from, to } = state.selection
  let result: { pos: number; type: NodeType } | null = null
  state.doc.nodesBetween(from, to, (node, pos) => {
    if (result !== null) return false
    if ((node.type === schema.nodes.bullet_list || node.type === schema.nodes.ordered_list) && pos <= from && pos + node.nodeSize >= to) {
      result = { pos, type: node.type }
      return false
    }
    return undefined
  })
  return result
}

// True when the selection extends past at least one list boundary -
// i.e., some list whose nodesBetween-visited range is not fully
// contained inside [from, to]. Distinguishes "cursor / range stays
// inside the same list" (every visited list is an ancestor of the
// selection, so we swap only the innermost) from "selection crosses
// out of, into, or between lists" (we swap every list the range
// touches). Without this, a click on a list type only swaps the
// innermost list at $from even when the user has clearly selected
// several lists.
export function selectionSpansList(state: EditorState): boolean {
  const { from, to } = state.selection
  let spanned = false
  state.doc.nodesBetween(from, to, (node, pos) => {
    if (spanned) return false
    if (node.type === schema.nodes.bullet_list || node.type === schema.nodes.ordered_list) {
      // A list is an "ancestor" of the selection when it fully contains
      // [from, to]. Anything else (list starts after from, or ends
      // before to) means the selection enters or exits the list.
      if (pos > from || pos + node.nodeSize < to) {
        spanned = true
        return false
      }
    }
    return undefined
  })
  return spanned
}

// True when the selection sits inside a wrapper of the given type. Fast
// path walks ancestors of $from; for multi-textblock selections (Ctrl+A
// has $from at the doc level, where the upward walk cannot reach
// wrappers that sit BETWEEN doc and the textblocks), fall back to
// finding a single node of the target type whose range covers the
// entire selection.
export function isInside(state: EditorState, type: NodeType): boolean {
  const { $from, to } = state.selection
  for (let d = $from.depth; d >= 0; d--) {
    if ($from.node(d).type === type) return true
  }
  const from = $from.pos
  let found = false
  state.doc.nodesBetween(from, to, (node, pos) => {
    if (found) return false
    if (node.type === type && pos <= from && pos + node.nodeSize >= to) {
      found = true
      return false
    }
    return undefined
  })
  return found
}

// True when the Indent toolbar button (Tab equivalent) would do
// something visible at the current selection. Inside a code block,
// indentCodeBlock always inserts a literal tab, so true. Inside a
// list, sinkListItem nests the cursor's list_item under the previous
// sibling - applicable only when such a previous sibling exists, so
// the very first item in a list gives false. Anywhere else, false.
export function canIndentAt(state: EditorState): boolean {
  const { $from, $to } = state.selection
  if ($from.parent.type.spec.code) return $from.sameParent($to)
  for (let d = $from.depth; d >= 1; d--) {
    if ($from.node(d).type === schema.nodes.list_item) {
      return $from.index(d - 1) > 0
    }
  }
  return false
}

// True when the Outdent toolbar button (Shift-Tab equivalent) would
// do something visible. Inside a code block, only when at least one
// touched line has a leading tab to strip - unindentCodeBlock returns
// true unconditionally so it can consume Shift-Tab, but for the button
// we want a strict no-op to disable. Inside a list, liftListItem
// always applies to a list_item containing the cursor.
export function canOutdentAt(state: EditorState): boolean {
  const { $from, $to } = state.selection
  if ($from.parent.type.spec.code) {
    if (!$from.sameParent($to)) return false
    const text = $from.parent.textContent
    const parentStart = $from.start()
    const fromOffset = $from.pos - parentStart
    const toOffset = $to.pos - parentStart
    let firstLineStart = fromOffset
    while (firstLineStart > 0 && text[firstLineStart - 1] !== "\n") firstLineStart--
    if (text[firstLineStart] === "\t") return true
    for (let i = fromOffset; i < toOffset; i++) {
      if (text[i] === "\n" && text[i + 1] === "\t") return true
    }
    return false
  }
  for (let d = $from.depth; d >= 1; d--) {
    if ($from.node(d).type === schema.nodes.list_item) return true
  }
  return false
}

// Inserts a leaf node at the current selection (used for hr and image).
export function insertLeafNode(type: NodeType, attrs?: Attrs | null): Command {
  return (state, dispatch) => {
    if (dispatch) {
      dispatch(state.tr.replaceSelectionWith(type.create(attrs ?? null)))
    }
    return true
  }
}

// Returns the contiguous link-mark range that contains $from, or null
// when the cursor is not inside (or adjacent to) a link. Walks the
// parent's children once and tracks the run of children whose marks
// include the same link instance as the cursor's neighbours.
export function findLinkRangeAt(state: EditorState): { from: number; to: number } | null {
  const linkType = schema.marks.link
  const { $from } = state.selection
  const linkMark: PMMark | undefined =
    $from.marks().find((m) => m.type === linkType) ?? $from.nodeBefore?.marks.find((m) => m.type === linkType) ?? $from.nodeAfter?.marks.find((m) => m.type === linkType)
  if (!linkMark) return null

  const parent = $from.parent
  const parentStart = $from.start()
  let cursor = parentStart
  let runFrom = -1
  let runTo = -1
  for (let i = 0; i < parent.childCount; i++) {
    const child = parent.child(i)
    const childStart = cursor
    const childEnd = cursor + child.nodeSize
    cursor = childEnd
    const sameLink = child.marks.some((m) => m.eq(linkMark))
    if (sameLink) {
      if (runFrom === -1) runFrom = childStart
      runTo = childEnd
    } else {
      if (runFrom !== -1 && runFrom <= $from.pos && $from.pos <= runTo) {
        return { from: runFrom, to: runTo }
      }
      runFrom = -1
      runTo = -1
    }
  }
  if (runFrom !== -1 && runFrom <= $from.pos && $from.pos <= runTo) {
    return { from: runFrom, to: runTo }
  }
  return null
}

// Position of the blockquote ancestor at the cursor, or null when not
// inside one.
export function findBlockquotePosAt(state: EditorState): number | null {
  const { $from } = state.selection
  for (let d = $from.depth; d >= 1; d--) {
    if ($from.node(d).type === schema.nodes.blockquote) return $from.before(d)
  }
  return null
}

// True only when the entire selection lies inside a single link mark
// range. Stricter than isMarkActive(state, link), which returns true
// for any selection that merely touches a link - e.g. a multi-block
// selection where one block happens to contain a link.
export function isSelectionWithinLink(state: EditorState): boolean {
  const range = findLinkRangeAt(state)
  if (!range) return false
  const { from, to } = state.selection
  return from >= range.from && to <= range.to
}

// Resolves the link href or blockquote cite at the cursor for the
// bottom toolbar's value tracking. Link wins when both apply (a
// blockquote can be the parent of a link, and editing the link is the
// more specific action).
export function getCurrentLinkValue(state: EditorState): string {
  const linkType = schema.marks.link
  const { $from } = state.selection
  const linkMark =
    $from.marks().find((m) => m.type === linkType) ?? $from.nodeBefore?.marks.find((m) => m.type === linkType) ?? $from.nodeAfter?.marks.find((m) => m.type === linkType)
  if (linkMark) {
    return (linkMark.attrs.href as string) || ""
  }
  for (let d = $from.depth; d >= 1; d--) {
    const node = $from.node(d)
    if (node.type === schema.nodes.blockquote) {
      return (node.attrs.cite as string | null) ?? ""
    }
  }
  return ""
}

// Replace the preformatted containing the cursor with one paragraph per
// \n-delimited line. setBlockType on its own would just retype the node
// and leave the literal \n characters in the resulting paragraph's text,
// where white-space: normal collapses them to spaces - so the user's
// three-line code block becomes one long visual line. Splitting first
// preserves the line structure; the caller (trySetBlockType below) then
// runs the normal setBlockType / lift / retry path on the resulting
// paragraphs to handle the actual target type.
//
// This is the tr-mutating form: callers append the split to a shared
// transaction so the whole conversion (split + retype + dissolve) ends
// up as a single undo step. Returns true when a preformatted was found
// and replaced.
export function splitPreformattedInTr(tr: Transaction): boolean {
  // Find the preformatted: first try the ancestor walk (cursor inside a
  // preformatted), then fall back to a range walk over the selection
  // for AllSelection / wide-selection cases where $from sits at the doc
  // level and the preformatted is a sibling child rather than an
  // ancestor of $from. We use tr.doc / tr.selection, not the original
  // editor state, because earlier steps in this transaction may have
  // already changed both.
  const { $from } = tr.selection
  let pfPos = -1
  for (let d = $from.depth; d >= 1; d--) {
    if ($from.node(d).type === schema.nodes.preformatted) {
      pfPos = $from.before(d)
      break
    }
  }
  if (pfPos < 0) {
    tr.doc.nodesBetween(tr.selection.from, tr.selection.to, (node, pos) => {
      if (pfPos < 0 && node.type === schema.nodes.preformatted) {
        pfPos = pos
        return false
      }
      return undefined
    })
  }
  if (pfPos < 0) return false
  const pfNode = tr.doc.nodeAt(pfPos)
  if (!pfNode || pfNode.type !== schema.nodes.preformatted) return false
  const text = pfNode.textContent
  const lines = text.split("\n")
  const paragraphs: Node[] = lines.map((line) => (line.length > 0 ? schema.nodes.paragraph.create(null, schema.text(line)) : schema.nodes.paragraph.create()))
  // Map the original selection (which sits in the preformatted's text)
  // into the equivalent paragraph + offset. fromLocal / toLocal are
  // character offsets within the preformatted's text content; walking
  // forward and counting \n boundaries gives (lineIndex, lineOffset),
  // which then converts to a doc position inside the matching new
  // paragraph.
  const fromLocal = Math.max(0, tr.selection.from - (pfPos + 1))
  const toLocal = Math.max(0, tr.selection.to - (pfPos + 1))
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
    lineIndex = Math.min(lineIndex, paragraphs.length - 1)
    let pos = pfPos
    for (let i = 0; i < lineIndex; i++) pos += paragraphs[i].nodeSize
    pos += 1 + Math.min(lineOffset, paragraphs[lineIndex].content.size)
    return pos
  }
  const wasAllSelection = tr.selection instanceof AllSelection
  tr.replaceWith(pfPos, pfPos + pfNode.nodeSize, paragraphs)
  // Preserve AllSelection across the split: a Ctrl+A -> code block ->
  // paragraph cycle should leave the user with "everything selected"
  // on the resulting paragraphs (so they can immediately re-target).
  if (wasAllSelection) {
    tr.setSelection(new AllSelection(tr.doc))
  } else {
    tr.setSelection(TextSelection.create(tr.doc, lineToPos(fromLocal), lineToPos(toLocal)))
  }
  return true
}

// PM's lift command lifts the block range that contains the selection.
// For a cursor / TextSelection inside blockquote > paragraph that range
// is the paragraph and the command works fine. But for AllSelection
// (Ctrl+A / browser Select All) the range PM picks is the whole doc;
// liftTarget cannot find a higher level to lift to, lift returns false,
// and the blockquote stays - so the user is stuck inside it. Walk the
// selection range, find every blockquote it touches, and lift each
// blockquote's contents up to the doc level explicitly. Iterating
// bottom-up keeps earlier mappings from invalidating later positions.
// Appends the unwrap steps to a shared transaction so trySetBlockType
// / setListType can bundle the whole conversion into a single undo
// step. Returns true when a blockquote was found and unwrapped.
export function liftBlockquotesInRangeInTr(tr: Transaction): boolean {
  const { from, to } = tr.selection
  const positions: number[] = []
  tr.doc.nodesBetween(from, to, (node, pos) => {
    if (node.type === schema.nodes.blockquote) {
      positions.push(pos)
      return false
    }
    return undefined
  })
  if (positions.length === 0) return false
  // Compute where the original selection endpoints should land after the
  // unwrap. tr.replaceWith's default position mapping treats the whole
  // blockquote range as "deleted" and pushes any cursor inside it to the
  // right side of the replacement - so a cursor that sat inside the
  // blockquote ends up just after where the blockquote was. Downstream
  // commands (setBlockType + wrapInList in setListType, the retry in
  // trySetBlockType) then operate on the wrong textblock.
  //
  // blockquote_paragraph.nodeSize === paragraph.nodeSize (both wrap the
  // same inline content with one open + one close), so every lift just
  // removes one wrapper level. For each blockquote the selection passes
  // through:
  //   - position strictly before its open  -> unchanged
  //   - position inside it                 -> shift -1 (lose the open)
  //   - position at/after its close        -> shift -2 (lose open + close)
  const shiftPosition = (originalPos: number): number => {
    let pos = originalPos
    for (const bqPos of positions) {
      const bqNode = tr.doc.nodeAt(bqPos)
      if (!bqNode || bqNode.type !== schema.nodes.blockquote) continue
      const bqEnd = bqPos + bqNode.nodeSize
      if (originalPos >= bqEnd) pos -= 2
      else if (originalPos > bqPos) pos -= 1
    }
    return pos
  }
  const newFrom = shiftPosition(from)
  const newTo = shiftPosition(to)
  const wasAllSelection = tr.selection instanceof AllSelection
  // baseMapLen captures the number of mapping entries tr already has
  // before our work - any earlier steps from the caller. Mapping each
  // collected position through tr.mapping.slice(baseMapLen) maps it
  // through only the steps WE add inside this loop, so positions
  // collected from tr.doc (current at function entry) remain
  // consistent as we add steps. Bottom-up iteration further ensures
  // earlier-position replacements do not shift later positions.
  const baseMapLen = tr.mapping.maps.length
  let changed = false
  for (let i = positions.length - 1; i >= 0; i--) {
    const mappedPos = tr.mapping.slice(baseMapLen).map(positions[i])
    const node = tr.doc.nodeAt(mappedPos)
    if (!node || node.type !== schema.nodes.blockquote) continue
    const inner: Node[] = []
    node.forEach((child) => {
      inner.push(schema.nodes.paragraph.create(null, child.content))
    })
    if (inner.length === 0) continue
    tr.replaceWith(mappedPos, mappedPos + node.nodeSize, inner)
    changed = true
  }
  if (changed) {
    // Preserve AllSelection across the unwrap: a Ctrl+A on a blockquote
    // is "everything is selected"; after the lift the doc has different
    // wrappers but the user's intent still maps to "everything is
    // selected". Downstream commands in setListType / trySetBlockType
    // (setBlockType, wrapInList) carry AllSelection through naturally
    // because AllSelection.map(newDoc) returns a fresh AllSelection on
    // the post-step doc.
    if (wasAllSelection) {
      tr.setSelection(new AllSelection(tr.doc))
    } else {
      tr.setSelection(TextSelection.create(tr.doc, newFrom, newTo))
    }
  }
  return changed
}

// Per-list_item classification. Each list_item whose content the user's
// selection touches falls into one of two buckets:
//
//   - lift: the FIRST direct paragraph child (the "primary" paragraph,
//     required by the schema's "paragraph ..." content spec) is touched
//     by the selection. The whole list_item is lifted out of its
//     containing list, taking all its content (including any non-
//     primary paragraphs and sub-lists) up to the list's parent level.
//   - extract: ONLY non-primary direct paragraph children are touched.
//     A contiguous range [firstIdx, lastIdx] of the list_item's
//     children is extracted: firstIdx is the lowest touched-paragraph
//     index, lastIdx is the highest touched-paragraph index extended
//     forward through any consecutive bullet_list/ordered_list siblings
//     (the "this paragraph plus its bullet list" group, since a
//     paragraph followed by a sub-list reads as one unit in the
//     editor). Any non-touched paragraphs / sub-lists between firstIdx
//     and lastIdx come along too, since the user's selection is
//     contiguous so they are in the range anyway. If the list_item is
//     the last in its parent list, the extracted nodes are placed
//     after the list, otherwise the parent list is split around the
//     list_item and the nodes land between the two halves. The
//     list_item itself stays in the (first half of the) list with its
//     primary paragraph and any non-extracted siblings intact.
//
// This distinction is what makes a second "click paragraph" on
// non-primary content lift only those paragraphs without dissolving
// the surrounding list, matching the user's "move one level out per
// click" mental model. Ancestor list_items further up the tree are
// not marked even when they geometrically contain the selection via
// nested lists - only the list_item whose own direct children are
// touched gets a mark.
//
// Lifts emit a tr.delete-per-inter-item-boundary merge followed by a
// single ReplaceAroundStep; the gap preserves the merged item's
// content, so PM's automatic selection mapping carries positions
// through correctly. Extractions emit a tr.delete per paragraph
// inside the list_item, an optional tr.split of the parent list (so
// the paragraph lands between the split halves at the list's parent
// level rather than after the entire list, matching the user's mental
// model of "move this paragraph out of the list right where it is"),
// then a tr.insert at the split point.
export function liftListsInRangeInTr(tr: Transaction): boolean {
  const userFrom = tr.selection.from
  const userTo = tr.selection.to
  const wasAllSelection = tr.selection instanceof AllSelection
  // Walk every list_item. For each one inspect its direct paragraph
  // children: if the primary (index 0) overlaps the selection mark
  // the whole li for lift; otherwise mark any non-primary overlapping
  // paragraphs for extraction. The two buckets are mutually exclusive
  // per li - lift's content includes the non-primary paragraphs
  // anyway, so we do not also extract them.
  const liftPlans = new Map<number, { listPos: number; startIdx: number; endIdx: number }>()
  const extractPlans = new Map<number, { liPos: number; parentListPos: number; liIdx: number; startPos: number; extractedNodes: Node[] }>()
  tr.doc.descendants((node, pos) => {
    if (node.type !== schema.nodes.list_item) return undefined
    const liContentStart = pos + 1
    let primaryOverlaps = false
    const nonPrimaryIdxs: number[] = []
    let childIdx = 0
    node.forEach((child) => {
      if (child.type === schema.nodes.paragraph) {
        const childOffset =
          childIdx === 0
            ? 0
            : (() => {
                let off = 0
                for (let k = 0; k < childIdx; k++) off += node.child(k).nodeSize
                return off
              })()
        const childPos = liContentStart + childOffset
        const childEnd = childPos + child.nodeSize
        const overlaps = childPos < userTo && childEnd > userFrom
        if (overlaps) {
          if (childIdx === 0) primaryOverlaps = true
          else nonPrimaryIdxs.push(childIdx)
        }
      }
      childIdx++
    })
    if (!primaryOverlaps && nonPrimaryIdxs.length === 0) return undefined
    const $li = tr.doc.resolve(pos)
    const parentList = $li.parent
    if (parentList.type !== schema.nodes.bullet_list && parentList.type !== schema.nodes.ordered_list) return undefined
    const parentListPos = $li.before($li.depth)
    if (primaryOverlaps) {
      const itemIdx = $li.index($li.depth)
      const plan = liftPlans.get(parentListPos)
      if (plan === undefined) {
        liftPlans.set(parentListPos, { listPos: parentListPos, startIdx: itemIdx, endIdx: itemIdx + 1 })
      } else {
        plan.startIdx = Math.min(plan.startIdx, itemIdx)
        plan.endIdx = Math.max(plan.endIdx, itemIdx + 1)
      }
    } else {
      // Extract a contiguous range from the list_item's children:
      // [firstIdx, lastIdx] where firstIdx is the lowest touched
      // non-primary paragraph and lastIdx is the highest, extended
      // forward through any consecutive bullet_list/ordered_list
      // siblings. The paragraph-plus-trailing-sub-list grouping reads
      // as one unit in the rendered list ("foobar with its bullets"),
      // so moving the paragraph out should bring the bullets along.
      // Any non-touched paragraph or sub-list children sitting between
      // firstIdx and lastIdx come along too - the user's selection is
      // contiguous so anything between two selected paragraphs is
      // already in the range.
      const firstIdx = nonPrimaryIdxs[0]
      let lastIdx = nonPrimaryIdxs[nonPrimaryIdxs.length - 1]
      while (lastIdx + 1 < node.childCount) {
        const nextChild = node.child(lastIdx + 1)
        if (nextChild.type !== schema.nodes.bullet_list && nextChild.type !== schema.nodes.ordered_list) break
        lastIdx++
      }
      let startPos = liContentStart
      for (let k = 0; k < firstIdx; k++) startPos += node.child(k).nodeSize
      const extractedNodes: Node[] = []
      for (let k = firstIdx; k <= lastIdx; k++) extractedNodes.push(node.child(k))
      const liIdx = $li.index($li.depth)
      extractPlans.set(pos, { liPos: pos, parentListPos, liIdx, startPos, extractedNodes })
    }
    return undefined
  })
  if (liftPlans.size === 0 && extractPlans.size === 0) return false
  const baseMapLen = tr.mapping.maps.length
  let changed = false
  // Cursor preservation for extractions. Delete + insert is two
  // unrelated steps from PM's mapping point of view: positions inside
  // the deleted paragraph map to the deletion boundary, not into the
  // re-inserted copy at the destination. So the cursor falls out of
  // the extracted paragraph and lands inside the now-shorter list_item.
  // Capture each endpoint that was inside an extracted paragraph and
  // its in-paragraph offset; after the insert we know where the new
  // paragraph lives in tr.doc and rewrite the position to "inside the
  // inserted copy at the same offset". The mapStart cursor lets us
  // pipe that intermediate position through any later steps so the
  // final selection lands where it belongs. (Lifts use PM's gap
  // semantics and do not need this fix-up - the gap preserves inside-
  // positions through the ReplaceAroundStep automatically.)
  let fromExtracted: { pos: number; mapStart: number } | null = null
  let toExtracted: { pos: number; mapStart: number } | null = null
  // Phase 1 (extractions, bottom-up by li position): for each affected
  // list_item, delete the marked non-primary paragraphs from it, then
  // either split the parent list (so the paragraphs land between the
  // li's slice and the rest of the list) or just insert the paragraphs
  // after the parent list (when the li is the last one and there's
  // nothing to split off). Bottom-up by li position keeps earlier-
  // visited lis' positions stable as later ones mutate higher-up
  // structures.
  for (const ePlan of Array.from(extractPlans.values()).sort((a, b) => b.liPos - a.liPos)) {
    const mappedLiPos = tr.mapping.slice(baseMapLen).map(ePlan.liPos)
    const liNode = tr.doc.nodeAt(mappedLiPos)
    if (!liNode || liNode.type !== schema.nodes.list_item) continue
    // Delete the [startPos, endPos] range from the list_item in one
    // step. The range covers all extracted nodes (paragraphs and
    // their associated trailing sub-lists), so a single tr.delete is
    // enough; no per-child loop needed.
    const extractedSize = ePlan.extractedNodes.reduce((s, n) => s + n.nodeSize, 0)
    const mappedStartPos = tr.mapping.slice(baseMapLen).map(ePlan.startPos)
    tr.delete(mappedStartPos, mappedStartPos + extractedSize)
    // After the delete, locate the (possibly shrunken) li and its
    // parent list. The parent list's childCount tells us whether to
    // split (li is not last) or just append (li is last). The split
    // path keeps a list_item's "in-place" feel: extracted nodes land
    // at the doc-level position where the li sat, not at the end of
    // the whole list (which can be far away for early lis).
    const mappedLiPosAfter = tr.mapping.slice(baseMapLen).map(ePlan.liPos)
    const liNodeAfter = tr.doc.nodeAt(mappedLiPosAfter)
    if (!liNodeAfter || liNodeAfter.type !== schema.nodes.list_item) continue
    const $li = tr.doc.resolve(mappedLiPosAfter)
    const parentListPostDelete = $li.parent
    if (parentListPostDelete.type !== schema.nodes.bullet_list && parentListPostDelete.type !== schema.nodes.ordered_list) continue
    const liIdxNow = $li.index($li.depth)
    const isLast = liIdxNow === parentListPostDelete.childCount - 1
    let insertPos: number
    if (isLast) {
      // No split needed: extracted nodes go right after the parent
      // list at the grandparent's content level.
      const parentListPos = $li.before($li.depth)
      insertPos = parentListPos + parentListPostDelete.nodeSize
    } else {
      // Split parent list at position right after this li so the
      // extracted nodes land between the two halves. tr.split inserts
      // a close + open boundary at the split position; after the
      // step, the doc has [first_list_half, second_list_half] at the
      // grandparent level, and the position "between" them is the
      // first-half's end position.
      const splitPos = mappedLiPosAfter + liNodeAfter.nodeSize
      tr.split(splitPos, 1)
      // Re-resolve to find the first half's end. The first half lives
      // at the same position as the original list (positions before
      // splitPos are unchanged by tr.split).
      const parentListPosResolved = tr.doc.resolve(mappedLiPosAfter).before($li.depth)
      const firstHalf = tr.doc.nodeAt(parentListPosResolved)
      if (!firstHalf) continue
      insertPos = parentListPosResolved + firstHalf.nodeSize
    }
    tr.insert(insertPos, Fragment.from(ePlan.extractedNodes))
    // When the insert lands inside a list_item, and the last
    // extracted node is a same-type sub-list as the split's second
    // half (the part of the parent list after our li), join them.
    // This matches what PM's chained liftListItem does for the
    // heading button: trailing bullets from inner-nested ancestors
    // collapse into one list at each higher level, instead of
    // leaving the doc littered with one-item sub-lists per ancestor.
    // The "inside a list_item" gate is what keeps two adjacent
    // sub-lists at doc level from getting collapsed - those are
    // semantically distinct (one is the extracted paragraph's
    // trailing bullets, the other is the rest of the original
    // outermost list) and the user does not want them merged.
    if (!isLast && ePlan.extractedNodes.length > 0) {
      const $insert = tr.doc.resolve(insertPos)
      if ($insert.parent.type === schema.nodes.list_item) {
        const lastNode = ePlan.extractedNodes[ePlan.extractedNodes.length - 1]
        if (lastNode.type === schema.nodes.bullet_list || lastNode.type === schema.nodes.ordered_list) {
          const joinPos = insertPos + extractedSize
          const secondHalfNode = tr.doc.nodeAt(joinPos)
          if (secondHalfNode && secondHalfNode.type === lastNode.type) {
            tr.join(joinPos)
          }
        }
      }
    }
    // Record where the user's cursor (or selection endpoint) landed
    // inside each newly-inserted paragraph. mapStart is the current
    // mapping length so any further steps in this function get
    // applied to these positions, not the baseline mapping. The
    // running offset walks through the inserted fragment so each
    // paragraph's content-start is insertPos + preceding-sizes + 1.
    // Non-paragraph children (sub-lists that came along for the
    // ride) do not host cursor positions in this flow - the cursor
    // was in a paragraph - so we only check paragraph children.
    const mapStart = tr.mapping.maps.length
    let runningOffset = 0
    let origOffset = 0
    for (const child of ePlan.extractedNodes) {
      const origChildPos = ePlan.startPos + origOffset
      if (child.type === schema.nodes.paragraph) {
        const origContentStart = origChildPos + 1
        const origContentEnd = origChildPos + child.nodeSize - 1
        const newContentStart = insertPos + runningOffset + 1
        if (fromExtracted === null && userFrom >= origContentStart && userFrom <= origContentEnd) {
          fromExtracted = { pos: newContentStart + (userFrom - origContentStart), mapStart }
        }
        if (toExtracted === null && userTo >= origContentStart && userTo <= origContentEnd) {
          toExtracted = { pos: newContentStart + (userTo - origContentStart), mapStart }
        }
      }
      runningOffset += child.nodeSize
      origOffset += child.nodeSize
    }
    changed = true
  }
  // Phase 2 (lifts, bottom-up by list position): use the existing
  // partial-lift logic - one merge step per list and one
  // ReplaceAroundStep to drop wrappers around the merged item.
  const plans = Array.from(liftPlans.values()).sort((a, b) => b.listPos - a.listPos)
  for (const plan of plans) {
    const mappedListPos = tr.mapping.slice(baseMapLen).map(plan.listPos)
    const listNode = tr.doc.nodeAt(mappedListPos)
    if (!listNode || (listNode.type !== schema.nodes.bullet_list && listNode.type !== schema.nodes.ordered_list)) continue
    if (listNode.childCount === 0) continue
    if (plan.startIdx >= listNode.childCount || plan.endIdx > listNode.childCount) continue
    const { startIdx, endIdx } = plan
    const atStart = startIdx === 0
    const atEnd = endIdx === listNode.childCount
    // Compute the positions of the lifted slice's first and last
    // boundaries. firstItemStart is the position of item[startIdx]'s
    // open token; lastItemEnd is the position just after
    // item[endIdx-1]'s close token. Both are pre-merge coordinates.
    let firstItemStart = mappedListPos + 1
    for (let k = 0; k < startIdx; k++) firstItemStart += listNode.child(k).nodeSize
    let lastItemEnd = firstItemStart
    for (let k = startIdx; k < endIdx; k++) lastItemEnd += listNode.child(k).nodeSize
    // Schema check: build the would-be replacement and verify the
    // parent's content spec accepts it before mutating.
    let mergedContent = Fragment.empty
    for (let k = startIdx; k < endIdx; k++) {
      mergedContent = mergedContent.append(listNode.child(k).content)
    }
    const $listPos = tr.doc.resolve(mappedListPos)
    const parent = $listPos.parent
    const parentIndex = $listPos.index()
    let replacement = Fragment.empty
    if (!atStart) {
      const leftover: Node[] = []
      for (let k = 0; k < startIdx; k++) leftover.push(listNode.child(k))
      replacement = replacement.append(Fragment.from(listNode.copy(Fragment.from(leftover))))
    }
    replacement = replacement.append(mergedContent)
    if (!atEnd) {
      const leftover: Node[] = []
      for (let k = endIdx; k < listNode.childCount; k++) leftover.push(listNode.child(k))
      replacement = replacement.append(Fragment.from(listNode.copy(Fragment.from(leftover))))
    }
    if (!parent.canReplace(parentIndex, parentIndex + 1, replacement)) continue
    // Step 1: merge the lifted slice's items by deleting each
    // inter-item boundary within [startIdx, endIdx). Iterate last to
    // first so earlier boundaries' positions stay valid.
    let mergePos = lastItemEnd
    for (let k = endIdx - 1; k > startIdx; k--) {
      mergePos -= listNode.child(k).nodeSize
      tr.delete(mergePos - 1, mergePos + 1)
    }
    // Step 2: drop the wrappers around the merged item.
    const mergedItem = tr.doc.nodeAt(firstItemStart)
    if (!mergedItem || mergedItem.type !== schema.nodes.list_item) continue
    const itemStart = firstItemStart
    const itemEnd = itemStart + mergedItem.nodeSize
    const sliceContent = (atStart ? Fragment.empty : Fragment.from(listNode.copy(Fragment.empty))).append(
      atEnd ? Fragment.empty : Fragment.from(listNode.copy(Fragment.empty)),
    )
    tr.step(
      new ReplaceAroundStep(
        itemStart - (atStart ? 1 : 0), // from: list's open if atStart, otherwise merged item's open
        itemEnd + (atEnd ? 1 : 0), // to: list's close if atEnd, otherwise merged item's close + 1
        itemStart + 1, // gapFrom: inside merged item, after its open
        itemEnd - 1, // gapTo: inside merged item, before its close
        new Slice(sliceContent, atStart ? 0 : 1, atEnd ? 0 : 1),
        atStart ? 0 : 1, // insert: gap before the slice content (atStart) or between the two list-copies
        true, // structure: enforce schema
      ),
    )
    changed = true
  }
  if (changed) {
    if (wasAllSelection) {
      tr.setSelection(new AllSelection(tr.doc))
    } else {
      // PM's automatic mapping carries positions through each
      // ReplaceAroundStep's gap and shifts everything outside by the
      // step's size delta - the user's original selection ends up at
      // the same logical place in the post-lift doc. For endpoints
      // that landed inside an extracted paragraph (where delete +
      // insert does not propagate the cursor), we use the position we
      // computed at insert time and pipe it through any later steps
      // via tr.mapping.slice(mapStart).
      const newFrom = fromExtracted !== null ? tr.mapping.slice(fromExtracted.mapStart).map(fromExtracted.pos) : tr.mapping.slice(baseMapLen).map(userFrom)
      const newTo = toExtracted !== null ? tr.mapping.slice(toExtracted.mapStart).map(toExtracted.pos) : tr.mapping.slice(baseMapLen).map(userTo)
      tr.setSelection(TextSelection.between(tr.doc.resolve(newFrom), tr.doc.resolve(newTo)))
    }
  }
  return changed
}

// Line break command. Inside a code-spec'd textblock (preformatted)
// the hard_break node is not allowed by the schema, so we drop a
// literal "\n" character at the cursor instead - preformatted parses
// with preserveWhitespace: "full" so newlines render as actual line
// breaks. Outside code, insert a hard_break node so the line break
// participates in the inline mark chain normally.
export const insertLineBreak: Command = (state, dispatch) => {
  const $from = state.selection.$from
  if (!$from.parent.isTextblock) return false
  if (dispatch) {
    if ($from.parent.type.spec.code) {
      dispatch(state.tr.insertText("\n").scrollIntoView())
    } else {
      dispatch(state.tr.replaceSelectionWith(schema.nodes.hard_break.create()).scrollIntoView())
    }
  }
  return true
}

// Indents the selection inside a code block: with an empty selection it
// inserts a literal tab at the cursor; with a range it prepends a tab to
// every line that the selection touches (the line containing $from and
// each line that starts inside the selection). Returns false when the
// cursor is not inside a code-spec'd textblock or when the selection
// spans block boundaries, so chainCommands can move on to the next
// candidate (list sink / no-op consume).
export const indentCodeBlock: Command = (state, dispatch) => {
  const { $from, $to, empty } = state.selection
  if (!$from.parent.type.spec.code) return false
  if (!$from.sameParent($to)) return false

  if (empty) {
    if (dispatch) {
      dispatch(state.tr.insertText("\t").scrollIntoView())
    }
    return true
  }

  const parent = $from.parent
  const parentStart = $from.start()
  const text = parent.textContent
  const fromOffset = $from.pos - parentStart
  const toOffset = $to.pos - parentStart

  let firstLineStart = fromOffset
  while (firstLineStart > 0 && text[firstLineStart - 1] !== "\n") {
    firstLineStart--
  }

  const lineStarts = [firstLineStart]
  for (let i = fromOffset; i < toOffset; i++) {
    if (text[i] === "\n") {
      lineStarts.push(i + 1)
    }
  }

  if (dispatch) {
    const tr = state.tr
    // Walk highest-position first so earlier offsets stay valid.
    for (let i = lineStarts.length - 1; i >= 0; i--) {
      tr.insertText("\t", parentStart + lineStarts[i])
    }
    dispatch(tr.scrollIntoView())
  }
  return true
}

// Counterpart to indentCodeBlock: removes a single leading tab from every
// line the selection touches (including the line containing the cursor
// when the selection is empty). Lines without a leading tab are left
// alone. Still returns true when inside a code block so Shift-Tab is
// captured even when nothing was removed (matches Tab's "always consume
// inside a code block" behaviour).
export const unindentCodeBlock: Command = (state, dispatch) => {
  const { $from, $to } = state.selection
  if (!$from.parent.type.spec.code) return false
  if (!$from.sameParent($to)) return false

  const parent = $from.parent
  const parentStart = $from.start()
  const text = parent.textContent
  const fromOffset = $from.pos - parentStart
  const toOffset = $to.pos - parentStart

  let firstLineStart = fromOffset
  while (firstLineStart > 0 && text[firstLineStart - 1] !== "\n") {
    firstLineStart--
  }

  const lineStarts = [firstLineStart]
  for (let i = fromOffset; i < toOffset; i++) {
    if (text[i] === "\n") {
      lineStarts.push(i + 1)
    }
  }

  if (dispatch) {
    const tr = state.tr
    for (let i = lineStarts.length - 1; i >= 0; i--) {
      const offset = lineStarts[i]
      if (text[offset] === "\t") {
        tr.delete(parentStart + offset, parentStart + offset + 1)
      }
    }
    dispatch(tr.scrollIntoView())
  }
  return true
}

// Enter on an empty boundary line in a preformatted block: same idea
// as exitBlockquoteOnEmpty below, but adapted to preformatted's
// structure. Preformatted is a single text* textblock, so "boundary
// lines" are not separate child nodes - they're \n characters at the
// edges of the text. Cases:
//   - Empty preformatted (text === "") + cursor at offset 0:
//     replace the whole preformatted with a paragraph.
//   - Cursor at the end of text AND text ends with "\n" (i.e. the
//     trailing line is empty): strip the trailing \n and insert a
//     paragraph after the preformatted.
//   - Cursor at offset 0 AND text starts with "\n" (i.e. the leading
//     line is empty): strip the leading \n and insert a paragraph
//     before the preformatted.
// If the strip would leave the preformatted empty, replace the whole
// preformatted with a paragraph instead of keeping an empty <pre>.
// Falls through (returns false) for non-boundary positions, so the
// default newlineInCode behavior (Enter inserts a literal \n) still
// runs in the middle of a code block.
export const exitPreformattedOnEmpty: Command = (state, dispatch) => {
  const { $from, $to } = state.selection
  if ($from.pos !== $to.pos) return false
  if ($from.parent.type !== schema.nodes.preformatted) return false
  const text = $from.parent.textContent
  const offset = $from.parentOffset
  const atStart = offset === 0
  const atEnd = offset === text.length
  let action: "empty" | "first" | "last" | null = null
  if (text.length === 0 && atStart) action = "empty"
  else if (atEnd && text.endsWith("\n")) action = "last"
  else if (atStart && text.startsWith("\n")) action = "first"
  if (action === null) return false
  if (!dispatch) return true
  const pfPos = $from.before()
  const pfEnd = pfPos + $from.parent.nodeSize
  let tr = state.tr
  if (action === "empty") {
    tr = tr.replaceWith(pfPos, pfEnd, schema.nodes.paragraph.create())
    tr = tr.setSelection(TextSelection.create(tr.doc, pfPos + 1))
  } else {
    const newText = action === "last" ? text.slice(0, -1) : text.slice(1)
    if (newText.length === 0) {
      // The strip would leave the preformatted empty - skip the
      // intermediate empty <pre> and just convert the whole thing to a
      // paragraph in one step.
      tr = tr.replaceWith(pfPos, pfEnd, schema.nodes.paragraph.create())
      tr = tr.setSelection(TextSelection.create(tr.doc, pfPos + 1))
    } else {
      const newPf = schema.nodes.preformatted.create(null, schema.text(newText))
      tr = tr.replaceWith(pfPos, pfEnd, newPf)
      const insertPos = action === "last" ? pfPos + newPf.nodeSize : pfPos
      tr = tr.insert(insertPos, schema.nodes.paragraph.create())
      tr = tr.setSelection(TextSelection.create(tr.doc, insertPos + 1))
    }
  }
  dispatch(tr.scrollIntoView())
  return true
}

// Enter on an empty blockquote_paragraph at the first or last position
// of a blockquote behaves like Enter on an empty boundary list_item:
// lift out of the wrapper so the user can leave the blockquote. Without
// this the blockquote is a one-way door - typing Enter inside it only
// ever adds new bps, never exits. The command no-ops in any other
// context (cursor not in a blockquote_paragraph, bp non-empty, or bp
// not at one of the blockquote's edges) so plain Enter elsewhere falls
// through to the default split-paragraph behavior.
export const exitBlockquoteOnEmpty: Command = (state, dispatch) => {
  const { $from, $to } = state.selection
  if ($from.pos !== $to.pos) return false
  if ($from.parent.type !== schema.nodes.blockquote_paragraph) return false
  if ($from.depth < 2) return false
  const blockquoteNode = $from.node(-1)
  if (blockquoteNode.type !== schema.nodes.blockquote) return false
  if ($from.parent.content.size !== 0) return false
  const isFirst = $from.index(-1) === 0
  const isLast = $from.indexAfter(-1) === blockquoteNode.childCount
  if (!isFirst && !isLast) return false
  if (!dispatch) return true
  const bqPos = $from.before(-1)
  const bqEnd = bqPos + blockquoteNode.nodeSize
  let tr = state.tr
  if (blockquoteNode.childCount === 1) {
    // The empty bp is the blockquote's only child - replace the whole
    // blockquote with an empty paragraph rather than leaving a blank
    // blockquote in the doc.
    tr = tr.replaceWith(bqPos, bqEnd, schema.nodes.paragraph.create())
    tr = tr.setSelection(TextSelection.create(tr.doc, bqPos + 1))
  } else {
    // Delete just the empty boundary bp, then splice a paragraph in
    // immediately before or after the (now-shrunk) blockquote. For the
    // trailing case the post-delete position of "just after the
    // blockquote" is bqEnd - bp.nodeSize (the blockquote shrank by
    // open + close = 2 for an empty bp). For the leading case the
    // blockquote's starting position bqPos is unchanged by the delete
    // (the delete happened strictly inside it), so we just insert at
    // bqPos.
    const bpStart = $from.before()
    const bpEnd = $from.after()
    tr = tr.delete(bpStart, bpEnd)
    const insertPos = isLast ? bqEnd - (bpEnd - bpStart) : bqPos
    tr = tr.insert(insertPos, schema.nodes.paragraph.create())
    tr = tr.setSelection(TextSelection.create(tr.doc, insertPos + 1))
  }
  dispatch(tr.scrollIntoView())
  return true
}
