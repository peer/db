import type { Mark as PMMark } from "prosemirror-model"
import type { EditorState } from "prosemirror-state"
import type { Ref } from "vue"
import type { Router } from "vue-router"

import { Plugin } from "prosemirror-state"
import { Decoration, DecorationSet } from "prosemirror-view"

import { classifyLink } from "@/internal-links"
import { schema } from "@/partials/input/InputHTML.schema"
import { findLinkRangeAt, isMarkActive } from "@/partials/input/InputHTML.state"
import type { EditPin, InsertSelection } from "@/partials/input/InputHTML.view"

// Inline decorations only wrap text, so a TextSelection that drags over
// an hr leaves the hr visually un-selected even though the range
// formally covers it. Hand the same .ProseMirror-selectednode class
// that PM applies for a NodeSelection on the hr, but as a node
// decoration on every hr whose range falls inside the current
// selection - the existing ring style in theme.css picks the class up
// and the hr lights up the same way it does when clicked.
export function selectedLeafNodesPlugin(): Plugin {
  return new Plugin({
    props: {
      decorations(state) {
        const { from, to } = state.selection
        if (from === to) return null
        const decos: Decoration[] = []
        state.doc.nodesBetween(from, to, (node, pos) => {
          if (node.type === schema.nodes.horizontal_rule) {
            decos.push(Decoration.node(pos, pos + node.nodeSize, { class: "ProseMirror-selectednode" }))
          }
          return undefined
        })
        if (decos.length === 0) return null
        return DecorationSet.create(state.doc, decos)
      },
    },
  })
}

// hr is a leaf node, so when one sits at the very start or end of the
// doc (typical after Ctrl+A -> click hr, which replaces the whole doc
// with just an hr) there is no textblock the user can put a cursor in
// on that side - they cannot type anything before or after it. Listen
// for doc changes and append a trailing paragraph or prepend a leading
// paragraph whenever the first/last child is an hr. The schema allows
// paragraphs at the doc level, so these injections satisfy
// "(block | horizontal_rule)+".
export function paragraphAroundHrPlugin(): Plugin {
  return new Plugin({
    appendTransaction(transactions, _oldState, newState) {
      if (!transactions.some((tr) => tr.docChanged)) return null
      const doc = newState.doc
      const hr = schema.nodes.horizontal_rule
      const p = schema.nodes.paragraph
      let tr = newState.tr
      let changed = false
      // Append first so the prepend's position-0 insert does not need
      // to be remapped through it.
      if (doc.lastChild?.type === hr) {
        tr = tr.insert(doc.content.size, p.create())
        changed = true
      }
      if (doc.firstChild?.type === hr) {
        tr = tr.insert(0, p.create())
        changed = true
      }
      return changed ? tr : null
    },
  })
}

// Highlight the range the bottom toolbar is acting on. The contenteditable
// loses its native selection visibility when focus moves to the InputLink,
// so without this the user is blind to what the new / edited link will
// cover. In snapshot mode (link-insert OR an Attach file upload - both
// stash the original selection into insertSelection) we highlight the
// snapshotted selection - or, when the selection was empty, drop a small
// "ghost" marker at the insertion point. In edit mode we highlight the
// full link-mark range (even when the cursor sits in the middle of the
// link); this also covers the Replace upload flow, which uses editPin to
// pin the link being replaced rather than insertSelection. Blockquote
// mode is intentionally not highlighted because the cite covers the
// whole block - hard to communicate usefully through an inline
// decoration.
function buildActiveRangeDecorations(
  state: EditorState,
  insertingSnapshot: boolean,
  insertSelection: InsertSelection | null,
  editPin: EditPin | null,
): DecorationSet | null {
  if (insertingSnapshot && insertSelection) {
    if (insertSelection.empty) {
      // Zero-width content + the same padding/background as the inline
      // highlight: visually a small rectangle hovering at the cursor.
      // The zero-width-space gives the inline span a text baseline so it
      // picks up the line's natural height; the negative inline margin
      // it inherits from .pd-inputhtml-active-range cancels the padding's
      // layout width so neighbouring characters stay put. We do not
      // inherit surrounding marks - the marker should look the same in
      // bold / italic / etc.
      const widget = Decoration.widget(
        insertSelection.from,
        () => {
          const el = document.createElement("span")
          el.className = "pd-inputhtml-active-range"
          el.textContent = "​"
          el.setAttribute("aria-hidden", "true")
          return el
        },
        { marks: [], ignoreSelection: true },
      )
      return DecorationSet.create(state.doc, [widget])
    }
    return DecorationSet.create(state.doc, [Decoration.inline(insertSelection.from, insertSelection.to, { class: "pd-inputhtml-active-range" })])
  }
  // Highlight follows the pinned range when an edit is in progress so the
  // user can see exactly which link the bottom toolbar still targets even
  // after the cursor has moved away from it.
  if (editPin?.kind === "link") {
    const { from, to } = editPin
    if (from < to) {
      return DecorationSet.create(state.doc, [Decoration.inline(from, to, { class: "pd-inputhtml-active-range" })])
    }
  } else if (isMarkActive(state, schema.marks.link)) {
    const range = findLinkRangeAt(state)
    if (range && range.from < range.to) {
      return DecorationSet.create(state.doc, [Decoration.inline(range.from, range.to, { class: "pd-inputhtml-active-range" })])
    }
  }
  return null
}

// Factory: takes the Vue refs the decoration depends on and returns a
// Plugin whose decorations callback reads .value off them. The
// decoration recomputes on every dispatchTransaction (which is when
// editor state changes); the ref reads pick up the latest toolbar
// state without needing a Vue watcher of our own. The "snapshot is
// active" check fires for link-insert mode and for in-flight Attach
// file uploads - both stash where the new link should land in
// insertSelection and benefit from the highlight while focus is on
// the bottom toolbar. The Replace upload flow also flips
// uploadingFile but uses editPin (not insertSelection); it is
// handled by the edit-mode branch inside buildActiveRangeDecorations.
export function activeRangeDecorationsPlugin(
  insertingLink: Ref<boolean>,
  insertSelection: Ref<InsertSelection | null>,
  editPin: Ref<EditPin | null>,
  uploadingFile: Ref<File | null>,
): Plugin {
  return new Plugin({
    props: {
      decorations(state) {
        const insertingSnapshot = insertingLink.value || uploadingFile.value !== null
        return buildActiveRangeDecorations(state, insertingSnapshot, insertSelection.value, editPin.value)
      },
    },
  })
}

// Stamps the same icon classes the HTMLClaim anchors use
// (pd-link-internal, pd-link-internal-noview, pd-link-file,
// pd-link-external) onto the editor's <a> element for each link mark.
// Uses a MarkView (editor-only override of the schema's toDOM) so the
// class lives on the <a> itself rather than on a wrapping span -
// inline decorations get split into multiple spans whenever the
// underlying text has different inner marks (e.g. italic in the middle
// of a link), which would multiply the ::before icon. The schema's
// toDOM is untouched, so docToHtml / DOMSerializer still emit a bare
// <a href="..."> the way HTMLClaim.Validate expects.
export function buildLinkMarkView(router: Router) {
  return (mark: PMMark) => {
    const dom = document.createElement("a")
    let href = mark.attrs.href as string
    dom.setAttribute("href", href)
    let classes = classifyLink(href, router)
    if (classes.length > 0) dom.className = classes.join(" ")
    return {
      dom,
      contentDOM: dom,
      update: (newMark: PMMark): boolean => {
        if (newMark.type !== mark.type) return false
        const newHref = newMark.attrs.href as string
        if (newHref === href) return true
        href = newHref
        dom.setAttribute("href", newHref)
        classes = classifyLink(newHref, router)
        dom.className = classes.length > 0 ? classes.join(" ") : ""
        return true
      },
    }
  }
}
