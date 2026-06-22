<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { EditorStateConfig } from "prosemirror-state"
import type { ShallowUnwrapRef } from "vue"

import type { ValidatedInput, ValidationError, ValidatorFn } from "@/types"

import {
  ArrowTurnDownLeftIcon,
  ArrowUturnLeftIcon,
  ArrowUturnRightIcon,
  BoldIcon,
  CodeBracketIcon,
  CodeBracketSquareIcon,
  H1Icon,
  H2Icon,
  H3Icon,
  ItalicIcon,
  LinkIcon,
  ListBulletIcon,
  MinusIcon,
  NumberedListIcon,
  PaperClipIcon,
  StrikethroughIcon,
  UnderlineIcon,
} from "@heroicons/vue/24/outline"
import { BlockquoteIcon, H4Icon, IndentIcon, OutdentIcon, PilcrowIcon } from "@sidekickicons/vue/24/outline"
import { baseKeymap, chainCommands, toggleMark } from "prosemirror-commands"
import { history, redo, undo } from "prosemirror-history"
import { keymap } from "prosemirror-keymap"
import { liftListItem, sinkListItem, splitListItem } from "prosemirror-schema-list"
import { EditorState, NodeSelection } from "prosemirror-state"
import { EditorView } from "prosemirror-view"
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowReadonly, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { CAN_EDIT_FILE, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import ButtonStyled from "@/components/ButtonStyled.vue"
import InputStyled from "@/components/InputStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import { classifyLink, LINK_CLASS_FILE } from "@/internal-links"
import InputBadges from "@/partials/InputBadges.vue"
import InputErrors from "@/partials/InputErrors.vue"
import { docToHtml, HEADING_LEVELS, htmlToDoc, schema } from "@/partials/input/InputHTML.schema"
import {
  canIndentAt,
  canOutdentAt,
  exitBlockquoteOnEmpty,
  exitPreformattedOnEmpty,
  findBlockquotePosAt,
  findLinkRangeAt,
  getCurrentLinkValue,
  indentCodeBlock,
  innermostListAt,
  insertLineBreak,
  isDocEmpty,
  isInside,
  isMarkActive,
  isNodeActive,
  isSelectionWithinLink,
  unindentCodeBlock,
} from "@/partials/input/InputHTML.state"
import type { EditPin, InsertSelection } from "@/partials/input/InputHTML.view"

import { activeRangeDecorationsPlugin, buildLinkMarkView, paragraphAroundHrPlugin, selectedLeafNodesPlugin } from "@/partials/input/InputHTML.plugins"
import {
  applyBlockquoteCite,
  applyInsertedLink,
  applyLinkHref,
  deleteLinkRange,
  indentList,
  insertHardBreak,
  insertHorizontalRule,
  outdentList,
  removeLinkAtRange,
  resolveLinkRange,
  setBlockquote,
  setBulletList,
  setHeading,
  setOrderedList,
  setParagraph,
  setPreformatted,
  toggleBold,
  toggleItalic,
  toggleMonospace,
  toggleStrikethrough,
  toggleUnderline,
  triggerRedo,
  triggerUndo,
} from "@/partials/input/InputHTML.view"
import InputLink from "@/partials/input/InputLink.vue"
import { useLock } from "@/progress"
import { uploadFile } from "@/upload"
import { useValidation, useValidationRegistry } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
  },
)

const model = defineModel<string>({ default: "" })
const errors = ref<ValidationError[]>([])

const emit = defineEmits<{ errors: [ValidationError[]] }>()
watch(errors, (v) => emit("errors", v), { flush: "sync" })

// Data modification and controls. lock is the external boundary
// (parent locks + this component's own non-validation locks); it
// cascades to descendants and feeds isInactive below.
//
// validationLock is a SEPARATE counter that we hand to useValidation
// instead of lock. useValidation increments / decrements its counter
// around every validate() call. Routing that cycle through lock would
// flip isInactive briefly during eager re-validation, which in turn
// flips the editor's contenteditable attribute via the editable() prop.
// Setting contenteditable="false" on a focused element causes browsers
// to blur it - the user would lose editor focus on the first keystroke
// after a "required" error, etc. Keeping validation out of the editable
// signal avoids that flicker without changing what the toolbar / parent
// forms see as locked.
const lock = useLock()
const validationLock = ref(0)

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// The bottom-toolbar's InputLink registers with the nearest validation
// registry. We open a sink registry at the InputHTML boundary so its
// dirty/error state does not bubble up to the surrounding form (the link
// editor is an internal sub-control, not a separate field). The InputHTML's
// own registration with the parent registry happens via its useValidation()
// call below.
useValidationRegistry()

const toolbarId = useId()
const toolbarEl = useTemplateRef<HTMLDivElement>("toolbarEl")
const editorRoot = useTemplateRef<HTMLDivElement>("editorRoot")
const escapeSentinel = useTemplateRef<HTMLSpanElement>("escapeSentinel")
const linkInputRef = useTemplateRef<ShallowUnwrapRef<ValidatedInput>>("linkInputRef")
const bottomToolbarEl = useTemplateRef<HTMLDivElement>("bottomToolbarEl")

let view: EditorView | null = null

// Tracks the currently active marks and block context so the toolbar can
// show pressed state. These mirror what is true for the current selection.
const activeMarks = ref<Record<string, boolean>>({
  bold: false,
  italic: false,
  underline: false,
  strikethrough: false,
  monospace: false,
  link: false,
})
const activeHeadingLevel = ref<number | null>(null)
const isPreformatted = ref(false)
// True only when every textblock the selection touches is a plain
// paragraph - i.e. setBlockType(paragraph) would be a no-op. Used by
// the block-type pill so the paragraph button is only pressed for a
// homogeneous paragraph selection; a mixed selection (paragraph +
// heading, etc.) leaves no block button pressed.
const isParagraph = ref(false)
const isBlockquote = ref(false)
// "Inside any list of this type at any nesting depth" - kept broad so the
// paragraph-button hide and similar checks treat any list level as "in a
// list".
const isBulletList = ref(false)
const isOrderedList = ref(false)
// The type of the innermost list at the cursor, or null if not in a list.
// Drives the pressed state of the bullet / ordered list buttons so the
// active button reflects the current nesting level only: a bullet inside
// an ordered list shows the bullet button pressed, not both.
const currentLevelList = ref<"bullet" | "ordered" | null>(null)
const hasFocus = ref(false)
const canInsertHorizontalRule = ref(false)
// True when the cursor sits in any textblock. Gates the line-break
// toolbar button. Inside an inline-accepting textblock (paragraph,
// heading, blockquote_paragraph) the click inserts a hard_break;
// inside a code-spec'd textblock (preformatted) it inserts a literal
// "\n" character instead.
const canInsertHardBreak = ref(false)

// True when the cursor / selection is in inline content that accepts a
// link mark (e.g. a paragraph, heading, or list item, but not
// preformatted which strips marks). Used by the Attach toolbar button
// (which does not care whether the cursor sits inside an existing
// link - attaching there just replaces / nests the link mark, same as
// what dropping a file there does).
const canApplyLinkMark = ref(false)

// The top-toolbar Link button additionally requires that the cursor is
// not already inside a link (existing links are edited through the
// bottom toolbar instead).
const canInsertLinkButton = ref(false)

// History gating for the undo / redo toolbar buttons. The prosemirror-history
// commands return false when nothing is on the stack, which is the
// natural disabled signal; updateActiveState refreshes these on every
// transaction so the buttons enable / disable as the user edits or
// scrubs through history.
const canUndo = ref(false)
const canRedo = ref(false)
// Indent / outdent buttons. Dry-runs of the same chained commands the
// Tab / Shift-Tab keymap entries use, so the buttons enable exactly
// when Tab / Shift-Tab would do something - i.e. inside a list (sink /
// liftListItem) OR inside a preformatted block (indent /
// unindentCodeBlock).
const canIndent = ref(false)
const canOutdent = ref(false)

// False when the selection is a NodeSelection on a leaf block,
// e.g. an hr that the user clicked on. In that state
// the block-type / list / blockquote buttons would just no-op
// (setBlockType / wrapInList only target textblocks), and we'd rather
// show them disabled than enabled-but-non-functional.
//
// True for TextSelection (cursor / text range) AND AllSelection.
// The mark and link buttons have their own gates (marksAllowedHere
// via toggleMark dry-run, canInsertLinkButton via toggleMark dry-run)
// that already disable themselves when no textblock in the range
// accepts marks - so we only need this flag to gate the block-type buttons.
const isTextblockSelection = ref(true)

// True when the cursor's textblock accepts inline marks. Drives the
// disabled state for the inline mark buttons (bold / italic / etc.).
const marksAllowedHere = ref(true)
// Per-mark dry-run for italic. Bold (used by marksAllowedHere above) is
// allowed everywhere paragraph-style marks are allowed - including
// blockquote_paragraph - but italic is not allowed inside
// blockquote_paragraph by the schema. The italic button needs its own
// probe so it stays disabled in that context while bold / underline /
// etc. stay enabled.
const italicAllowedHere = ref(true)

// True while the user is composing a new link via the bottom toolbar:
// the Link button was clicked, but the user has not yet pressed Insert
// or Cancel. While true, the bottom toolbar shows in insert mode
// (different buttons, empty starting value) and takes priority over the
// existing-link / blockquote contexts.
const insertingLink = ref<boolean>(false)

// Selection snapshot taken when the Link button was clicked. The
// contenteditable loses focus when the user clicks into the InputLink,
// but ProseMirror keeps the selection in its state - we save the
// positions here so Insert can apply the new mark (or insert URL-as-text
// for an empty selection) at the original spot, even after focus has
// moved away and back. Stored as a ref so the active-range decoration
// plugin can read .value at decoration time without a custom watcher.
const insertSelection = ref<InsertSelection | null>(null)

// When the user starts editing the URL of an existing link / blockquote
// cite (linkInputModel diverges from the editor's value), we pin the
// bottom toolbar to that specific link / blockquote: its position is
// snapshotted here, currentLinkValue stops following the live cursor,
// and bottomContext reports the pinned kind. Without this, moving the
// cursor off the link would close the toolbar and discard the pending
// edit. The pin clears only when Update or Remove commits the edit,
// or when focus leaves the bottom toolbar while the input is clean.
const editPin = ref<EditPin | null>(null)

// Active file upload state. When uploadingFile holds a File, the bottom
// toolbar swaps into upload mode: "Uploading" message, a progress bar
// across the editor's bottom edge, and a Cancel button that aborts via
// uploadAbort. Shared by both flows: the Attach flow (toolbar's Attach
// button or a drop on the editor); the Replace flow (file-edit
// toolbar's long button). uploadProgress / uploadTotal are passed by
// ref into uploadFile so it can swap them from indeterminate (total
// undefined) to determinate (chunk byte counts) without our intervention.
const uploadingFile = ref<File | null>(null)
const uploadProgress = ref<number>(0)
const uploadTotal = ref<number | undefined>(undefined)
let uploadAbort: AbortController | null = null

// Did upload fail and should should the error message be shown in the bottom toolbar?
// Set in the upload catch blocks, cleared when a new upload starts or the user dismisses it.
const uploadError = ref<boolean>(false)

// Visual feedback for dragging a file over the Replace button. Set on
// dragover/dragenter, cleared on dragleave/drop. Drives the Button's
// :active prop so the button highlights as a valid drop target.
const isReplaceDragOver = ref<boolean>(false)

// True while a file is being dragged over the editor / bottom-toolbar
// region. Drives a "drop-target" bottomContext that swaps the bottom
// toolbar for a "Drop file to upload it..." prompt (and accepts the
// drop). We only flip this for file drags - internal PM slice / text
// drags continue to use PM's native drop handling without UI change.
// TODO: Show that also the editor area itself is a valid drop area.
const isDraggingFile = ref<boolean>(false)

// dragenter and dragleave both fire for each descendant boundary
// crossing inside the wrapper, not just for its outer edge. Counting
// the events lets us detect a true wrapper enter / leave; drop and
// the editor's PM handleDrop reset the counter (neither fires a
// balancing dragleave).
let dragDepth = 0

// Hidden <input type="file"> the Attach button triggers via .click().
// Kept in template (not document.body) so it inherits the form's
// disabled / readonly chain and unmounts with the editor.
const fileInputRef = useTemplateRef<HTMLInputElement>("fileInputRef")

// Second hidden file input dedicated to the Replace flow. Kept separate
// from fileInputRef so each input's @change handler can stay
// flow-specific (onFilePicked applies a new link at the snapshotted
// selection; onFileReplacePicked swaps the href of the pinned link).
const fileReplaceInputRef = useTemplateRef<HTMLInputElement>("fileReplaceInputRef")

// The URL currently held by the link mark (or blockquote cite) at the
// cursor. Drives the bottom toolbar: it appears under the editor whenever
// this points at editable URL context, populates the InputLink with the
// existing value, and gates the Remove button (only enabled when there is
// something to remove).
const currentLinkValue = ref<string>("")

// What the user has typed into the bottom toolbar's InputLink. Synced
// from currentLinkValue on every context change (cursor moves to a
// different link / blockquote) so the input shows the editor's live
// value, then drifts as the user edits. Update commits the drift back
// into the editor; the revert badge snaps it back.
const linkInputModel = ref<string>("")

// Prevents the model watcher from clobbering the editor mid-dispatch.
let suppressModelWatcher = false

// Forces the model watcher to apply the model.value change to PM even
// when the editor has focus. Used by the revert override: the caller
// (InputField.onRevert) refocuses the contenteditable right after
// writing model.value, so the watcher's hasFocus gate (which exists
// to keep the watcher from clobbering the caret during normal
// editing) would otherwise skip the update. Revert is a deliberate
// user action; the caret jump is fine, and we need the displayed
// document to actually resync with the reverted value. The watcher
// consumes (clears) the flag inside its body; the revert override
// also clears it via nextTick as a defensive fallback for the case
// where the watcher does not run at all (model.value already equal
// to the checkpoint, so the setter does not trigger).
let forceModelSync = false

// True when the editor's document holds no actual content - no text and
// no leaf atom nodes (images, horizontal rules). We need this rather
// than !model.value because an empty editor serializes to "<p></p>",
// which is a truthy string. Driven by dispatchTransaction and the model
// watcher so the value tracks the current view state regardless of
// whether the change came from the user or from the parent v-model.
const isStructurallyEmpty = ref(true)

// Required-empty when isStructurallyEmpty is true. The required check is
// skipped on initial so a freshly mounted empty editor is not flagged before
// the user has interacted. The model value passed in is ignored - emptiness
// is decided by the doc structure, not by whether the HTML string is "" vs
// "<p></p>" vs the canonical empty paragraph form.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (_value, options) {
  if (!props.required || options.initial) return []
  // TODO: Use standard codes.
  return isStructurallyEmpty.value ? [{ code: "required" }] : []
}

const isInactive = computed(() => lock.value > 0 || props.readonly)
const invalid = computed(() => props.invalid || errors.value.length > 0)

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  validationLock,
  () => validator,
  // The ProseMirror contenteditable lives at view.dom; that is the actual
  // focus target, not our wrapper div. We fall back to the wrapper if
  // view is not constructed yet (pre-mount or post-unmount).
  () => view?.dom ?? editorRoot.value,
  () => {
    model.value = ""
    errors.value = []
    uploadError.value = false
  },
  // Reuse our structural emptiness ref (an empty editor serialises to
  // "<p></p>", a truthy string; the default !model.value check would say it
  // is non-empty).
  shallowReadonly(isStructurallyEmpty),
)

// Override revert so PM actually resyncs. The default
// validatedInput.revert only does model.value = checkpointValue -
// a plain Vue ref write. PM is a separate state container; nothing
// in PM is bound to model.value. The model.value watcher above is
// what mirrors external model changes into PM, but it normally
// skips while the editor has focus to avoid clobbering the caret
// during normal editing - and our caller (the surrounding
// InputField in DocumentEdit - InputField.onRevert in
// src/partials/InputField.vue, which we never invoke ourselves)
// refocuses the contenteditable right after invoking revert(), so
// by the time the watcher microtask runs hasFocus is true and the
// watcher would skip.
//
// We flip forceModelSync so the watcher runs once regardless of
// focus, then let the existing watcher path (createState + view.
// updateState) replace the doc and the plugins (incl. history).
// Reading model.value synchronously here would not work - the
// defineModel setter only updates the local ref after the parent
// re-renders, so a sync read returns the old (dirty) value;
// deferring to the watcher gives Vue time to propagate.
//
// We mutate validatedInput.revert in place rather than just
// exposing a wrapped copy: useValidation has already registered
// validatedInput with the parent validation registry, and revertAll
// iterates the registered objects and calls their .revert directly.
// Patching the same object means both call paths (the registry's
// revertAll and template-ref callers via defineExpose) go through
// the override.
const originalRevert = validatedInput.revert
validatedInput.revert = () => {
  forceModelSync = true
  originalRevert()
  // Defensive: the watcher (pre-flush) consumes the flag inside Vue's
  // current update cycle and clears it itself; this nextTick is the
  // fallback for when the watcher does not run at all - e.g. revert
  // was called with the model.value already at the checkpoint so the
  // setter does not trigger a change. nextTick guarantees this fires
  // AFTER Vue's flush (and therefore after the watcher had its
  // chance), so it cannot clear the flag before the watcher would
  // see it.
  void nextTick(() => {
    forceModelSync = false
  })
}
defineExpose(validatedInput)

function updateActiveState(state: EditorState) {
  // linkActive (loose): the selection touches a link mark anywhere -
  // used to gate canInsertLinkButton so the top-toolbar Link button is
  // disabled when any part of the selection is already a link.
  const linkActive = isMarkActive(state, schema.marks.link)
  activeMarks.value = {
    bold: isMarkActive(state, schema.marks.bold),
    italic: isMarkActive(state, schema.marks.italic),
    underline: isMarkActive(state, schema.marks.underline),
    strikethrough: isMarkActive(state, schema.marks.strikethrough),
    monospace: isMarkActive(state, schema.marks.monospace),
    // link uses the stricter "fully within a single link mark range"
    // check so multi-block selections that merely touch a link do not
    // flip bottomContext to "link" and pop the bottom toolbar's link
    // form. Cursor on a link / selecting only inside a link still flip
    // it to true.
    link: isSelectionWithinLink(state),
  }

  let headingLevel: number | null = null
  for (const level of HEADING_LEVELS) {
    if (isNodeActive(state, schema.nodes.heading, { level })) {
      headingLevel = level
      break
    }
  }
  activeHeadingLevel.value = headingLevel
  isPreformatted.value = isNodeActive(state, schema.nodes.preformatted)
  isParagraph.value = isNodeActive(state, schema.nodes.paragraph)
  isBlockquote.value = isInside(state, schema.nodes.blockquote)
  isBulletList.value = isInside(state, schema.nodes.bullet_list)
  isOrderedList.value = isInside(state, schema.nodes.ordered_list)
  const innermost = innermostListAt(state)
  currentLevelList.value = innermost === null ? null : innermost.type === schema.nodes.bullet_list ? "bullet" : "ordered"
  // hr is only valid at the doc level (schema-wise). Two cases where
  // we can land an hr there:
  //   - Cursor in a top-level paragraph (depth 1, paragraph parent):
  //     replaceSelectionWith splits the paragraph at the cursor and
  //     inserts hr between.
  //   - Doc-level selection (depth 0 - Ctrl+A, the browser "Select
  //     All" menu, a manual drag from before the first block to after
  //     the last): insertHorizontalRule appends hr at the doc end so
  //     the existing content survives.
  // Headings / preformatted / list-paragraphs / blockquote-paragraphs
  // are not in either case - splitting them with hr is either invalid
  // (schema rejects hr inside list_item / blockquote) or weird UX
  // (chopping a heading in half).
  canInsertHorizontalRule.value = state.selection.$from.depth === 0 || (state.selection.$from.depth === 1 && state.selection.$from.parent.type === schema.nodes.paragraph)
  // hard_break can only go inside a textblock that admits inline
  // content. paragraph / heading / blockquote_paragraph all accept it;
  // preformatted's "text*" content spec does not (hard_break is not a
  // text node). AllSelection / NodeSelection at the doc level lands
  // with $from.parent === doc which is not a textblock either, so the
  // .isTextblock check covers both.
  canInsertHardBreak.value = state.selection.$from.parent.isTextblock
  // currentLinkValue stops following the cursor while an edit is pinned:
  // the bottom toolbar must keep showing the pinned link's URL until the
  // edit is committed or reverted, regardless of where the cursor goes.
  if (editPin.value === null) {
    currentLinkValue.value = getCurrentLinkValue(state)
  }
  // Allowed when the selection touches at least one textblock that accepts
  // link marks (so a code block alone is out, but a Ctrl-A selection that
  // spans paragraphs + code blocks is in). We use toggleMark as a dry-run
  // instead of looking at $from.parent: on Ctrl-A $from sits at depth 0
  // with parent === doc, and the doc node does not accept marks - so
  // the simpler check would disable the button on full-document
  // selections. The Link button additionally requires that the cursor
  // is not already inside a link; the Attach button uses canApplyLinkMark
  // directly so it stays enabled on existing links (same as drop does).
  canApplyLinkMark.value = toggleMark(schema.marks.link)(state)
  canInsertLinkButton.value = canApplyLinkMark.value && !linkActive
  // History dry-runs: undo / redo return true when something can be
  // undone / redone, false otherwise. Calling them with no dispatch
  // makes them a pure can-it-run probe.
  canUndo.value = undo(state)
  canRedo.value = redo(state)
  // Indent / outdent button gates. Stricter than "is in a list / code
  // block": the buttons disable when the corresponding action would
  // be a no-op even though Tab / Shift-Tab is in scope. Inside a code
  // block, Indent is always meaningful (indentCodeBlock inserts a
  // literal tab), but Outdent only when at least one touched line has
  // a leading tab to strip - otherwise it would silently consume the
  // event with no visible effect. Inside a list, Indent works only
  // when the cursor's list_item has a previous sibling list_item
  // (sinkListItem nests it under the previous one); Outdent works on
  // any list_item.
  canIndent.value = canIndentAt(state)
  canOutdent.value = canOutdentAt(state)
  // Mark-allowance probe via toggleMark's dry-run: returns true when at
  // least one textblock in the range accepts the mark. Picking any
  // mark (bold) works because heading / preformatted disallow all
  // marks (marks: "") while paragraph / blockquote-paragraph allow
  // all. Using the dry-run rather than $from.parent.allowsMarkType
  // handles selections that span past $from's textblock end - e.g.
  // the formatting buttons stay enabled across a mixed
  // paragraph+heading selection (toggleMark itself will apply the
  // mark only to the paragraph parts).
  marksAllowedHere.value = toggleMark(schema.marks.bold)(state)
  italicAllowedHere.value = toggleMark(schema.marks.italic)(state)
  isTextblockSelection.value = !(state.selection instanceof NodeSelection)
}

// Click handler for the top toolbar's Link button. The button is gated
// by canInsertLinkButton, so we know there is markable content here and
// no existing link. Snapshot the current selection (ProseMirror keeps it
// in state across blur, so the contenteditable losing focus to the
// InputLink is fine), switch the bottom toolbar into insert mode, then
// move focus to the InputLink after it has mounted so the user can type
// the URL immediately without a separate click.
async function onStartInsertLink() {
  if (!view) return
  const { from, to, empty } = view.state.selection
  insertSelection.value = { from, to, empty }
  insertingLink.value = true
  await nextTick().then(() => {
    linkInputRef.value?.el()?.focus()
  })
}

// Attach-file click. Snapshot the current selection so we can drop the
// resulting file link at the original spot once the upload finishes
// (the user can keep editing in the meantime, and the active-range
// decoration plugin reads the snapshot to highlight that target).
// Then trigger the hidden <input type="file"> which surfaces the OS
// file picker. The actual upload kicks off in onFilePicked ->
// startAttachUpload once the user confirms a file.
function onAttachFile() {
  if (!view || isInactive.value || uploadingFile.value !== null || isLinkInputDirty.value) return
  const { from, to, empty } = view.state.selection
  insertSelection.value = { from, to, empty }
  fileInputRef.value?.click()
}

// Shared Attach upload routine. Assumes insertSelection has already
// been snapshotted by the caller. Drives uploadingFile / progress refs
// so bottomContext switches to "upload" mode (label + Cancel + bottom
// progress bar) for the duration. Files are uploaded sequentially and
// each one's link is inserted as soon as its upload completes - so the
// user sees them appear in order. For files past the first we ask
// applyInsertedLink to prepend a separator (hard_break inside an inline
// textblock, "\n" inside a preformatted block). applyInsertedLink
// returns the InsertSelection the next file should aim at; we assign
// that back to insertSelection so the next iteration uses it - the
// returned position lands INSIDE the textblock the first file produced
// at doc-level drops, which is what keeps the whole batch in ONE
// paragraph instead of one paragraph per file. Errors (including the
// AbortError dispatched when the user clicks Cancel) stop the batch;
// files inserted before the error stay.
async function startAttachUpload(files: File[]) {
  if (files.length === 0) return
  if (!hasPermission(CAN_EDIT_FILE)) return
  uploadError.value = false
  uploadAbort = new AbortController()
  try {
    for (let i = 0; i < files.length; i++) {
      if (uploadAbort.signal.aborted) {
        return
      }
      const file = files[i]
      // TODO: Should we support showing total progress somewhere?
      uploadingFile.value = file
      uploadProgress.value = 1
      uploadTotal.value = undefined
      const id = await uploadFile(router, file, uploadAbort.signal, uploadProgress, uploadTotal)
      if (uploadAbort.signal.aborted || !id) {
        return
      }
      const href = router.resolve({ name: "StorageGet", params: { id } }).href
      const next = applyInsertedLink(view, insertSelection.value, href, file.name, i > 0)
      if (next) insertSelection.value = next
    }
  } catch (err) {
    // Cancel (AbortError) or network failure - stop the batch. Files
    // inserted before the error keep their place. The user's editor
    // content is otherwise untouched.
    if (uploadAbort?.signal.aborted) {
      return
    }
    uploadError.value = true
    console.error("InputHTML.startAttachUpload", err)
  } finally {
    uploadingFile.value = null
    uploadAbort = null
    insertSelection.value = null
    view?.focus()
  }
}

// Handler bound to the hidden file input's @change. Picks every file
// off the input (the input has multiple, so the user can pick more
// than one), resets the input value so re-picking the same file fires
// @change, then hands off to startAttachUpload.
async function onFilePicked(event: Event) {
  const input = event.target as HTMLInputElement
  const files = Array.from(input.files ?? [])
  input.value = ""
  if (files.length === 0) {
    insertSelection.value = null
    return
  }
  await startAttachUpload(files)
}

// Reset the drag-tracking state. Shared by every code path that ends
// the drag (drop on editor, drop on prompt) since drop is not paired
// with a balancing dragleave on the wrapper.
function resetDragState() {
  dragDepth = 0
  isDraggingFile.value = false
}

// Returns true if the drag carries one or more files (as opposed to
// text, an internal PM slice, etc.). We use this to gate the
// drop-target prompt - non-file drags fall through to PM's native
// handling without UI change.
function isFileDrag(event: DragEvent): boolean {
  const types = event.dataTransfer?.types
  if (!types) return false
  for (let i = 0; i < types.length; i++) {
    if (types[i] === "Files") return true
  }
  return false
}

function onWrapperDragEnter(event: DragEvent) {
  if (!isFileDrag(event)) return
  dragDepth++
  // Only advertise the drop-target prompt when we will actually
  // accept the file. Busy states (read-only, upload in flight, dirty
  // link / cite edit) still count toward the depth so dragleave stays
  // balanced and the drop is still claimed (in dragover / drop) - we
  // just do not light up a target the user cannot use.
  if (isInactive.value || uploadingFile.value !== null || isLinkInputDirty.value || !hasPermission(CAN_EDIT_FILE)) return
  isDraggingFile.value = true
}

function onWrapperDragOver(event: DragEvent) {
  if (!isFileDrag(event)) return
  // Always preventDefault on file drags, even in busy states.
  // Without preventDefault the drop event fires on the browser
  // (not the page) and the browser opens the file - which navigates
  // away from the SPA, destroying any in-progress edit. Silent
  // rejection (drop fires on us, we discard the file) is preferable
  // to surprise navigation.
  event.preventDefault()
}

function onWrapperDragLeave(event: DragEvent) {
  if (!isFileDrag(event)) return
  dragDepth--
  if (dragDepth <= 0) {
    dragDepth = 0
    isDraggingFile.value = false
  }
}

// Single dispatch point for every file drop landing anywhere inside
// the wrapper. We route by mode:
//
//   - file-edit (cursor on a storage link): the only meaningful
//     action is replacing that link's file, so any file drop in the
//     wrapper triggers the Replace flow on the link at the cursor;
//   - everything else: Attach a new file link at the drop
//     coordinates (or the current cursor if posAtCoords cannot
//     resolve them - e.g. a drop on the top toolbar's blank space).
//
// Busy states (read-only, in-flight upload, dirty link / cite edit)
// silently consume the drop after preventDefault, so the browser does
// not navigate to the file. Non-file drops (text, internal PM slice)
// are left for PM to handle as normal.
async function onWrapperDrop(event: DragEvent) {
  const files = Array.from(event.dataTransfer?.files ?? [])
  if (files.length === 0) return
  event.preventDefault()
  resetDragState()
  if (isInactive.value || uploadingFile.value !== null || isLinkInputDirty.value || !view || !hasPermission(CAN_EDIT_FILE)) return
  if (bottomMode.value === "file-edit") {
    const range = resolveLinkRange(view.state, editPin.value)
    if (!range) return
    editPin.value = { kind: "link", from: range.from, to: range.to }
    // Replace swaps the href of one existing link, so it only ever
    // consumes the first dropped file. Any extras are dropped on the
    // floor (the user intent was "replace this file", not "insert N
    // new ones after the existing link").
    await startReplaceUpload(files[0])
    return
  }
  const coords = view.posAtCoords({ left: event.clientX, top: event.clientY })
  if (coords) {
    insertSelection.value = { from: coords.pos, to: coords.pos, empty: true }
  } else {
    const { from, to, empty } = view.state.selection
    insertSelection.value = { from, to, empty }
  }
  await startAttachUpload(files)
}

// PM's drop handler. We claim file drops here purely to suppress PM's
// default behavior. Returning true makes PM call preventDefault and
// stand down. All the upload routing happens in onWrapperDrop.
// Non-file drops fall through so PM handles them normally.
function handleEditorDrop(_view: EditorView, event: DragEvent): boolean {
  return !!event.dataTransfer?.files?.length
}

// Cancel button on the upload toolbar. Aborts the in-flight uploadFile
// promise via AbortController. Whichever flow holds the upload
// (startAttachUpload or startReplaceUpload) has its own finally{}
// that resets uploadingFile / uploadProgress / uploadTotal /
// uploadAbort and any flow-specific snapshot once the rejection
// propagates.
function onCancelUpload() {
  uploadAbort?.abort()
}

// Dismiss the failed-upload message in the bottom toolbar, releasing it back to
// whatever the cursor is on.
function onDismissUploadError() {
  uploadError.value = false
  view?.focus()
}

// Set right before .click() on the hidden Replace file input, consumed
// by the next focusout on the bottom toolbar. Chrome (and others)
// dispatches a synthetic blur on the trigger when the native picker
// takes focus; without this guard that blur would bubble up to
// onBottomToolbarFocusOut and releaseEditPin() before the user even
// picks a file, so startReplaceUpload's editPin check would then bail
// and "nothing happens" after selection. Same trick InputFile.vue
// uses for its own browse button.
let openingReplacePicker = false

// Replace-file flow (file-edit toolbar's long button). Snapshot the link
// range into editPin so the in-flight upload still targets the right
// link even if the cursor wanders, then trigger the dedicated hidden
// <input type="file">. Once a file is picked the bottom toolbar switches
// to the shared "Uploading <name>" takeover (uploadingFile drives
// bottomContext) - the file-edit shape returns on completion / cancel
// when the pin is released and bottomContext re-resolves.
function onReplaceFileClick() {
  if (!view || isInactive.value || uploadingFile.value !== null) return
  const range = findLinkRangeAt(view.state)
  if (!range) return
  editPin.value = { kind: "link", from: range.from, to: range.to }
  openingReplacePicker = true
  fileReplaceInputRef.value?.click()
}

// Shared upload routine for both Replace flows (click-pick and drag-drop).
// Drives the same uploadingFile / progress refs as the Attach flow, so
// bottomContext switches to "upload" and the existing takeover toolbar
// (label + Cancel + bottom progress bar) shows for the duration. On
// success rewrites the pinned link's href via applyLinkHref; on cancel /
// failure leaves the editor untouched.
async function startReplaceUpload(file: File) {
  if (!view || editPin.value?.kind !== "link") return
  if (!hasPermission(CAN_EDIT_FILE)) return
  uploadError.value = false
  uploadingFile.value = file
  uploadProgress.value = 1
  uploadTotal.value = undefined
  uploadAbort = new AbortController()
  try {
    const id = await uploadFile(router, file, uploadAbort.signal, uploadProgress, uploadTotal)
    if (uploadAbort.signal.aborted || !id) return
    const href = router.resolve({ name: "StorageGet", params: { id } }).href
    applyLinkHref(view, editPin.value, href)
    // TODO: Show the user some success message. Because it is not really visible that the link changed.
  } catch (err) {
    // Cancel (AbortError) or network failure - the editor still holds the
    // original file link.
    if (uploadAbort?.signal.aborted) {
      return
    }
    uploadError.value = true
    console.error("InputHTML.startReplaceUpload", err)
  } finally {
    uploadingFile.value = null
    uploadAbort = null
    uploadProgress.value = 0
    uploadTotal.value = undefined
    // Drop the pin so bottomContext / currentLinkValue re-resolve from
    // the live cursor: if focus is still on the (newly-rewritten) link
    // the toolbar returns to file-edit; otherwise it closes.
    releaseEditPin()
  }
}

async function onFileReplacePicked(event: Event) {
  openingReplacePicker = false
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ""
  if (!file) {
    releaseEditPin()
    return
  }
  await startReplaceUpload(file)
}

// Drag-over visual feedback for the Replace button: highlights it as a
// valid drop target while the user is dragging a file over it. The
// actual drop is handled by onWrapperDrop on the surrounding
// InputStyled (which routes any file drop in file-edit mode to the
// Replace flow), so we only need to track the hover state here.
function onReplaceDragOver() {
  if (isInactive.value || uploadingFile.value !== null) return
  isReplaceDragOver.value = true
}

function onReplaceDragLeave() {
  isReplaceDragOver.value = false
}

// File-edit Unlink button: strip the link mark but keep the visible
// text in the document. Same call as onRemoveLink takes for link
// context, but exposed under a name that matches the file-edit button.
function onUnlinkFileLink() {
  removeLinkAtRange(view, editPin.value)
  releaseEditPin()
}

// File-edit Remove button: delete the entire link range (mark + text).
// This is the destructive sibling of Unlink - use when the user wants
// the file reference gone from the prose, not just demoted to plain
// text.
function onDeleteFileLink() {
  deleteLinkRange(view, editPin.value)
  releaseEditPin()
}

// Pressed-state value for the block-type buttons. "h1"-"h4" map to the
// heading level, "pre" to the preformatted/code block, "p" to a plain
// paragraph; "" means "no button pressed" - used when the selection
// spans multiple block types (e.g. paragraph + heading) and none of
// the buttons accurately represents the whole selection.
const blockType = computed<string>(() => {
  if (activeHeadingLevel.value !== null) return `h${activeHeadingLevel.value}`
  if (isPreformatted.value) return "pre"
  // Inside a list_item or blockquote, paragraph is just the structural
  // child the container requires - the user is "in a list / quote",
  // not "in a paragraph". Surface the wrapper instead so the paragraph
  // button stays unpressed there (and clicking it falls through to the
  // lift-out behavior in trySetBlockType, so it works as "leave the
  // wrapper").
  if (isBlockquote.value || isBulletList.value || isOrderedList.value) return ""
  if (isParagraph.value) return "p"
  return ""
})

// Distinguishes which URL the bottom-toolbar's InputLink targets. Insert
// wins over everything because the user explicitly opened that mode via
// the toolbar; the editPin is consulted next so a dirty edit survives the
// cursor wandering off the link / blockquote; finally we fall back to the
// live cursor context (link wins over blockquote so editing a link nested
// in a blockquote does not silently rewrite the cite).
const bottomContext = computed<"drop-target" | "upload" | "upload-error" | "insert" | "link" | "blockquote" | null>(() => {
  // Drop-target wins over everything else: while a file is being dragged
  // over the editor we replace whatever toolbar was open with a "drop
  // file" prompt that also accepts the drop. onWrapperDragEnter
  // already gates isDraggingFile on uploadingFile being null, so the
  // ordering vs upload below is for clarity only.
  if (isDraggingFile.value) return "drop-target"
  // Upload owns the bottom toolbar exclusively while it runs: showing
  // the InputLink form for some unrelated link the cursor happens to
  // pass over would confuse "what is this toolbar acting on". The
  // Attach and Replace flows share this state, so both swap the toolbar
  // for the "Uploading <name>" takeover; on completion they each
  // re-resolve back to their natural mode (file-edit for Replace via
  // editPin, or whatever the cursor lands in for Attach).
  if (uploadingFile.value !== null) return "upload"
  // A failed upload keeps the toolbar so the error is visible until the user dismisses it or
  // starts another upload.
  if (uploadError.value) return "upload-error"
  if (insertingLink.value) return "insert"
  if (editPin.value) return editPin.value.kind
  if (activeMarks.value.link) return "link"
  if (isBlockquote.value) return "blockquote"
  return null
})

// The bottom toolbar is a context surface: it appears under the editor
// whenever bottomContext is non-null and goes away otherwise. It extends
// the wrapper downward without shifting the editor content, since it sits
// after editorRoot in flow.
const showBottomToolbar = computed(() => bottomContext.value !== null)

// True when the link the cursor sits in is a same-origin storage URL
// (classifyLink stamps it with LINK_CLASS_FILE). In that case bottomMode
// resolves to "file-edit" instead of "link-edit". Only applies to
// existing-link editing - insert mode still uses InputLink so the user
// can paste any URL (including a storage URL, which becomes a storage
// link the next time the cursor returns to it).
const isStorageLinkContext = computed<boolean>(() => bottomContext.value === "link" && classifyLink(currentLinkValue.value, router).includes(LINK_CLASS_FILE))

// Visible label that names what the bottom toolbar is editing. Also used
// as the toolbar's aria-label so the visual label and the accessible name
// stay in sync. Insert and link share the same "Link" label; a storage
// link (file-edit mode) reports "File" - file-edit does not render the
// label column visibly, but the toolbar's aria-label still names what
// the row is acting on for screen readers.
const bottomLabel = computed<string>(() => {
  if (bottomContext.value === "drop-target") return t("partials.input.InputHTML.bottomToolbar.dropFileLabel")
  if (bottomContext.value === "upload") return t("partials.input.InputHTML.bottomToolbar.uploadingLabel", { name: uploadingFile.value?.name ?? "" })
  if (bottomContext.value === "upload-error") return t("common.errors.upload")
  if (isStorageLinkContext.value) return t("partials.input.InputHTML.bottomToolbar.fileLabel")
  if (bottomContext.value === "link" || bottomContext.value === "insert") return t("partials.input.InputHTML.bottomToolbar.linkLabel")
  if (bottomContext.value === "blockquote") return t("partials.input.InputHTML.bottomToolbar.blockquoteLabel")
  return ""
})

// "Anchor" the input is compared against for dirty / revert. For an
// existing link or blockquote it is the editor's live href / cite; for a
// new link being inserted there is no editor value yet, so the anchor is
// the empty string (any typed text counts as dirty). We do not reach
// into InputLink's built-in isDirty/checkpoint/revert because v-model
// change propagation is not synchronous and calling checkpoint after setting
// v-model might not do the right thing. We would also manage state in two
// places. Anchoring here ties dirty/revert to live editor state.
const linkInputAnchor = computed(() => (bottomContext.value === "insert" ? "" : currentLinkValue.value))

// Has the user typed something that differs from the anchor. Drives the
// "changed" badge and gates the primary submit button.
//
// Computed separately from canPrimary so the badge stays visible while
// the editor is read-only - the user can still revert their pending edit
// even when the primary action is locked.
const isLinkInputDirty = computed(() => linkInputModel.value !== linkInputAnchor.value)

// Refines bottomContext into the action modes the bottom toolbar actually
// renders: drop-target (file being dragged over the editor), link-insert
// (toolbar Link button pressed), link-edit (cursor on a non-file link),
// file-edit (cursor on a storage file link), blockquote-add (cursor in
// blockquote with no cite yet), blockquote-edit (cursor in blockquote
// with cite). The link / file-edit split keys off isStorageLinkContext
// so file links get the Replace / Open / Unlink / Remove layout instead
// of the URL InputLink. The blockquote split keys off currentLinkValue
// so the moment a cite is applied the mode flips from add to edit and
// the primary label changes accordingly.
type BottomMode = "drop-target" | "link-insert" | "link-edit" | "file-edit" | "blockquote-add" | "blockquote-edit" | "upload" | "upload-error" | null
const bottomMode = computed<BottomMode>(() => {
  if (bottomContext.value === "drop-target") return "drop-target"
  if (bottomContext.value === "upload") return "upload"
  if (bottomContext.value === "upload-error") return "upload-error"
  if (bottomContext.value === "insert") return "link-insert"
  if (bottomContext.value === "link") return isStorageLinkContext.value ? "file-edit" : "link-edit"
  if (bottomContext.value === "blockquote") return currentLinkValue.value === "" ? "blockquote-add" : "blockquote-edit"
  return null
})

// Primary button label per mode. Insert / Add appear the first time a
// link / cite is being entered; Update appears once the underlying mark
// or attribute exists and the user is changing it. Upload mode does not
// render the form, so its label is unused.
const primaryLabel = computed<string>(() => {
  if (bottomMode.value === "link-insert") return t("common.buttons.insert")
  if (bottomMode.value === "blockquote-add") return t("common.buttons.add")
  if (bottomMode.value === "link-edit" || bottomMode.value === "blockquote-edit") return t("common.buttons.update")
  return ""
})

// Single enablement rule for the primary button across all four modes:
// dirty (changed from anchor) and editor not locked. For insert / add the
// anchor is "" so dirty <=> non-empty (blocks empty insert). For edit modes
// the anchor is the live href / cite so dirty includes the user emptying
// the field, which is the user-requested "Update on empty acts as Remove"
// path - handled in onConfirm.
const canPrimary = computed<boolean>(() => {
  if (isInactive.value) return false
  if (bottomMode.value === "link-insert") return isLinkInputDirty.value
  if (bottomMode.value === "blockquote-add") return isLinkInputDirty.value
  if (bottomMode.value === "link-edit") return isLinkInputDirty.value
  if (bottomMode.value === "blockquote-edit") return isLinkInputDirty.value
  return false
})

// Remove is enabled only when there is an existing href / cite to clear.
// Disabled in insert / add modes (there is nothing to remove yet) - in
// blockquote-add it is also hidden entirely, but the gate stays here as
// a single source of truth.
const canRemoveLink = computed(() => bottomContext.value !== "insert" && !isInactive.value && currentLinkValue.value !== "")

// Keeps the InputLink in lockstep with the anchor. Fires when the cursor
// moves to a different link, when the user clicks into a blockquote, when
// an Update / Remove changes the underlying value, when insert mode is
// entered or left, etc. Any unsaved edit is intentionally dropped on
// context change - the input always reflects the current anchor. While
// editPin is set, currentLinkValue is frozen so the anchor does not move
// either; the existing edit is preserved across cursor wandering.
watch([linkInputAnchor, bottomContext], () => {
  linkInputModel.value = linkInputAnchor.value
})

// Snapshots the current edit target (link mark range or blockquote node
// position) so the bottom toolbar can keep editing it after the cursor
// moves. Called when linkInputModel first diverges from the anchor.
function pinCurrentEdit() {
  if (!view) return
  if (activeMarks.value.link) {
    const range = findLinkRangeAt(view.state)
    if (range) editPin.value = { kind: "link", from: range.from, to: range.to }
  } else if (isBlockquote.value) {
    const pos = findBlockquotePosAt(view.state)
    if (pos !== null) editPin.value = { kind: "blockquote", pos }
  }
}

// Clears the pin and forces an immediate refresh of the live state. Used
// after Update / Remove / Revert so the bottom toolbar can collapse
// (cursor is off the link) or restyle for the new live context.
function releaseEditPin() {
  if (editPin.value === null) return
  editPin.value = null
  if (view) updateActiveState(view.state)
}

// Watch the input model for the dirty transition: when linkInputModel
// first diverges from the anchor in a non-insert context, pin the edit
// so the bottom toolbar keeps targeting this link / blockquote even if
// the cursor moves. We deliberately do NOT release on returning to
// clean: as long as focus stays in the toolbar, the user is still
// arguably "working on" this link (they may have just clicked the
// changed badge to revert and continue typing). The pin only goes away
// in onConfirm / onRemoveLink (the apply is done) or when the toolbar
// blurs while clean (handled in onBottomToolbarFocusOut). Insert mode
// is excluded because insertSelection already provides its own
// snapshot and bottomContext gives "insert" priority over the pin
// anyway.
watch(linkInputModel, (newValue) => {
  if (insertingLink.value) return
  if (newValue === linkInputAnchor.value) return
  if (editPin.value !== null) return
  pinCurrentEdit()
})

// When the bottom toolbar loses focus to something outside it (the
// editor, another input, browser chrome), drop whatever transient
// state was keeping it open - but only if the input is clean. A dirty
// edit (typed but not committed) must survive the blur so the user can
// come back and finish; a clean state has nothing left to protect, so
// the toolbar should snap to the live cursor context (edit mode) or
// just close (insert mode opened but never typed into).
//
// relatedTarget = null also counts as "left" (focus went to body, was
// killed by tab-out, etc.); a relatedTarget inside the toolbar is
// internal focus shuffle and we ignore it.
function onBottomToolbarFocusOut(event: FocusEvent) {
  const next = event.relatedTarget as HTMLElement | null
  if (next && bottomToolbarEl.value?.contains(next)) return
  if (isLinkInputDirty.value) return
  if (insertingLink.value) {
    // User opened insert mode but did not type anything before clicking
    // away - treat that as an implicit cancel and let the toolbar close.
    insertingLink.value = false
    insertSelection.value = null
    return
  }
  if (openingReplacePicker) {
    // Synthetic blur fired by the browser when the native file picker
    // took focus. Consume it so the editPin survives until the user
    // actually picks (or dismisses) the file.
    openingReplacePicker = false
    return
  }
  releaseEditPin()
}

// Plugin decorations close over editPin / insertingLink / insertSelection:
// when any of these toggle, kick PM to re-run the decoration plugin so the
// highlight points at the pinned range (or releases back to the live range,
// or shows / hides the insert-mode marker). Selection / content changes
// already drive a transaction, so cursor movement between pinned and
// unpinned states does not need a separate trigger.
watch([editPin, insertingLink, insertSelection, uploadingFile], () => {
  view?.dispatch(view.state.tr)
})

// Single form-submit handler. The form renders exactly one primary
// submit button - its label (Insert / Add / Update) and the mode-
// specific apply path are picked by bottomMode below - so every form
// submit routes through here. We validate first - the InputLink's
// validator also normalizes the URL (lowercases scheme/host, trims,
// etc.) so the value we read back has the canonical form. The
// dispatchTransaction path inside the apply* helpers triggers
// updateActiveState, which refreshes currentLinkValue and re-syncs
// linkInputModel via the watcher, so the dirty state naturally clears
// after a successful apply.
async function onConfirm() {
  if (!canPrimary.value) return
  if (!view || !linkInputRef.value) return
  await linkInputRef.value.validate()
  if (linkInputRef.value.errors.length > 0) return
  const value = linkInputModel.value
  switch (bottomMode.value) {
    case "link-insert":
      applyInsertedLink(view, insertSelection.value, value)
      insertingLink.value = false
      insertSelection.value = null
      view.focus()
      break
    case "link-edit":
      // Empty value on Update acts as Remove: the user emptied the URL
      // and submitted, meaning "drop the link mark" - applyLinkHref with
      // "" would persist an empty href which is meaningless.
      if (value === "") {
        removeLinkAtRange(view, editPin.value)
      } else {
        applyLinkHref(view, editPin.value, value)
      }
      releaseEditPin()
      break
    case "blockquote-add":
    case "blockquote-edit":
      // applyBlockquoteCite stores "" as null on the node, so an empty
      // value here is naturally the "remove cite" path - no branch
      // needed, just dispatch.
      applyBlockquoteCite(view, editPin.value, value)
      releaseEditPin()
      break
    case "drop-target":
    case "file-edit":
    case "upload":
    case "upload-error":
      // drop-target, file-edit, upload and upload-error modes do not render
      // the form / InputLink, so onConfirm (form @submit) cannot fire
      // here in practice - the v-else around the <form> branch
      // ensures it. Listed for exhaustive-switch lint coverage only.
      break
    case null:
      break
  }
}

// Cancel exits insert mode without touching the editor. We do not gate it
// on isInactive: even when the editor locks mid-insert, the user should
// still be able to dismiss the bottom toolbar.
function onCancelLink() {
  insertingLink.value = false
  insertSelection.value = null
  view?.focus()
}

// Remove workflow: for links, drop the mark; for blockquotes, clear the
// cite (the blockquote itself stays). Not applicable in insert mode.
// Release the pin afterwards so the bottom toolbar collapses (the link
// no longer exists / blockquote no longer has a cite to edit) or reverts
// to the live cursor context.
function onRemoveLink() {
  if (bottomContext.value === "link") removeLinkAtRange(view, editPin.value)
  else if (bottomContext.value === "blockquote") applyBlockquoteCite(view, editPin.value, "")
  releaseEditPin()
}

// Revert workflow: snap the InputLink back to the current anchor (the
// editor's live href / cite, or "" in insert mode). Used by the
// "changed" badge in the label column. Returns focus to the InputLink
// afterwards: the browser's default of focusing the badge button would
// otherwise leave focus on a now-invisible element (the badge hides
// itself once dirty clears), which both confuses keyboard users and
// trips the bottom-toolbar focusout handler into releasing the edit pin.
function onRevertLinkInput() {
  linkInputModel.value = linkInputAnchor.value
  linkInputRef.value?.el()?.focus()
}

// Label click focuses the InputLink, simulating <label for=...> behavior.
// We replicate HTML's "interactive content" exception so a click on the
// InputBadges revert button does its own thing instead of also moving
// focus into the input.
function onLabelClick(event: MouseEvent) {
  const target = event.target as HTMLElement | null
  if (target?.closest("a[href], button, input, select, textarea, details, [tabindex]:not([tabindex='-1'])")) return
  linkInputRef.value?.el()?.focus()
}

function buildKeymap() {
  return keymap({
    "Mod-b": toggleMark(schema.marks.bold),
    "Mod-i": toggleMark(schema.marks.italic),
    "Mod-u": toggleMark(schema.marks.underline),
    "Mod-z": undo,
    "Mod-y": redo,
    "Mod-Shift-z": redo,
    Enter: chainCommands(splitListItem(schema.nodes.list_item), exitBlockquoteOnEmpty, exitPreformattedOnEmpty, baseKeymap.Enter),
    // Shift-Enter inserts a visible line break. Inside a code block
    // (preformatted) that means a literal "\n" character; everywhere
    // else it inserts a hard_break inline node. The Line break toolbar
    // button shares this command so the two paths stay in sync. We
    // deliberately do not chain exitCode in front: pressing Shift-Enter
    // at the end of a code block stays inside the block and adds
    // another "\n", matching the "always a line break" intent. The
    // user can still leave the block via plain Enter on an empty
    // boundary line (exitPreformattedOnEmpty) or via a block-type
    // button.
    "Shift-Enter": insertLineBreak,
    "Mod-[": liftListItem(schema.nodes.list_item),
    "Mod-]": sinkListItem(schema.nodes.list_item),
    // Tab is captured inside the editor so it does not move focus out:
    // inside a code block it indents (inserts a literal tab on each line
    // that the selection touches, or just at the cursor when empty); in a
    // list it sinks the list item; elsewhere it is a no-op that still
    // consumes the event. Shift-Tab unindents the same line range inside
    // a code block and lifts list items elsewhere.
    Tab: chainCommands(indentCodeBlock, sinkListItem(schema.nodes.list_item), () => true),
    "Shift-Tab": chainCommands(unindentCodeBlock, liftListItem(schema.nodes.list_item), () => true),
    // Escape parks focus on the post-editor sentinel so the user can leave
    // the editor without tab-capture eating their next Tab. From the
    // sentinel the browser's natural Tab moves to whatever follows
    // InputHTML in document order; Shift-Tab is intercepted on the
    // sentinel (onEscapeSentinelKeyDown below) to send focus back to the
    // toolbar instead of re-entering the contenteditable.
    Escape: () => {
      escapeSentinel.value?.focus()
      return true
    },
  })
}

function onEscapeSentinelKeyDown(event: KeyboardEvent) {
  if (event.key !== "Tab" || !event.shiftKey) return
  event.preventDefault()
  // Roving tabindex on the toolbar keeps exactly one button at tabindex=0;
  // focus that one so the user lands on the same spot they left.
  const active = toolbarEl.value?.querySelector<HTMLButtonElement>('button[tabindex="0"]')
  active?.focus()
}

function createState(html: string): EditorState {
  const config: EditorStateConfig = {
    doc: htmlToDoc(html),
    schema,
    plugins: [
      history(),
      buildKeymap(),
      keymap(baseKeymap),
      activeRangeDecorationsPlugin(insertingLink, insertSelection, editPin, uploadingFile),
      selectedLeafNodesPlugin(),
      paragraphAroundHrPlugin(),
    ],
  }
  return EditorState.create(config)
}

onMounted(() => {
  if (!editorRoot.value) return

  view = new EditorView(editorRoot.value, {
    state: createState(model.value),
    editable: () => !isInactive.value,
    // Editor-only override of the link mark's rendering so the icon
    // classes go on <a> directly (avoids the multi-span split that
    // inline decorations cause when a link contains text with other
    // inner marks).
    markViews: {
      link: buildLinkMarkView(router),
    },
    // Drag-and-drop a file onto the editor: uploads it and inserts a
    // file link at the drop position. Plain text / internal slice
    // drops fall through to ProseMirror's default handling.
    handleDrop: handleEditorDrop,
    dispatchTransaction(transaction) {
      if (!view) return
      // Ride along with edits that happen while an insert-style snapshot
      // is live. insertSelection is a position snapshot captured by
      // any flow that needs to land content at the original selection
      // after focus has wandered (link-insert via the Link button,
      // file Attach via the toolbar button, file drop on the editor);
      // without mapping it would drift away from the user's intent as
      // soon as anything earlier in the doc is inserted or deleted.
      // Done before view.updateState so the next call to the
      // decoration plugin reads the up-to-date positions and renders
      // the marker at the mapped location. Edit-mode highlights do not
      // need this dance: findLinkRangeAt re-reads the live selection
      // on every plugin call. We only update on docChanged because
      // pure selection-change transactions leave positions identity-
      // mapped.
      if (insertSelection.value && transaction.docChanged) {
        const from = transaction.mapping.map(insertSelection.value.from)
        const to = transaction.mapping.map(insertSelection.value.to)
        insertSelection.value = { from, to, empty: from === to }
      }
      // Same mapping for an edit pin so the highlight and the apply path
      // continue to target the same link / blockquote after surrounding
      // text is added or removed.
      if (editPin.value && transaction.docChanged) {
        const pin = editPin.value
        if (pin.kind === "link") {
          editPin.value = { kind: "link", from: transaction.mapping.map(pin.from), to: transaction.mapping.map(pin.to) }
        } else {
          editPin.value = { kind: "blockquote", pos: transaction.mapping.map(pin.pos) }
        }
      }
      const newState = view.state.apply(transaction)
      view.updateState(newState)
      updateActiveState(newState)
      if (!transaction.docChanged) return
      isStructurallyEmpty.value = isDocEmpty(newState.doc)
      // Canonicalise the structurally-empty serialisation to "" rather than
      // PM's "<p></p>". This keeps isDirty=false for a row that the user
      // typed into and then cleared back out (whose checkpoint is the empty
      // default "") - structurally the row is unchanged, the value string
      // should reflect that. An existing claim seeded with non-empty HTML
      // still flips isDirty=true on clear, since its checkpoint is the
      // seeded HTML.
      const html = isDocEmpty(newState.doc) ? "" : docToHtml(newState.doc)
      if (html === model.value) return
      suppressModelWatcher = true
      model.value = html
      // useValidation's onInteraction (via its model watcher) fires as a
      // side effect of this model.value assignment.
      void nextTick(() => {
        suppressModelWatcher = false
      })
    },
    handleDOMEvents: {
      focus: () => {
        hasFocus.value = true
        return false
      },
      blur: () => {
        hasFocus.value = false
        // Run validation when the contenteditable loses focus, matching
        // TextArea/InputText's onBlur. We swallow the promise - errors land
        // in errors.value via useValidation's pipeline and flow out via
        // the errors emit.
        void runValidation()
        return false
      },
    },
  })
  updateActiveState(view.state)
  isStructurallyEmpty.value = isDocEmpty(view.state.doc)
})

onBeforeUnmount(() => {
  // Abort any in-flight upload before tearing down. Both Attach and
  // Replace share uploadAbort, so this covers both paths; their
  // finally{} blocks still run to clean up state, but on a destroyed
  // component that is harmless. Without this the upload would keep
  // running and eventually try to write into a torn-down view.
  uploadAbort?.abort()
  view?.destroy()
  view = null
})

// Sync external model updates back into the editor, but skip while the user
// is focused. Replacing the document would jump the caret. The
// forceModelSync flag bypasses the hasFocus gate when a deliberate
// caller (revert) wants the resync regardless of focus.
watch(
  () => model.value,
  (value) => {
    if (!view) return
    if (suppressModelWatcher) return
    if (hasFocus.value && !forceModelSync) return
    forceModelSync = false
    const currentHtml = docToHtml(view.state.doc)
    if (currentHtml === value) return
    const state = createState(value)
    view.updateState(state)
    updateActiveState(state)
    isStructurallyEmpty.value = isDocEmpty(state.doc)
  },
)

// Re-evaluate the editable flag when readonly/lock changes after mount.
watch(isInactive, () => {
  if (!view) return
  view.setProps({ editable: () => !isInactive.value })
})

// The active-range decoration plugin reads insertingLink + insertSelection
// from closures; selection / content changes already trigger a re-render
// via transactions, but flipping insert mode (Link button -> Insert /
// Cancel) does not. Kick an empty transaction so the plugin's decorations
// re-evaluate and the highlight appears or disappears.
watch(insertingLink, () => {
  view?.dispatch(view.state.tr)
})

// Roving tabindex (WAI-ARIA "toolbar" pattern). Without this every button
// in the toolbar is its own tab stop - Tab would walk through every
// button in the toolbar pills before reaching the editor. With roving
// focus only one button at a time carries tabindex=0, so one Tab lands
// somewhere in the toolbar and the next Tab moves to the contenteditable.
// ArrowLeft / ArrowRight / Home / End navigate between buttons inside the toolbar.
function getToolbarButtons(): HTMLButtonElement[] {
  if (!toolbarEl.value) return []
  return Array.from(toolbarEl.value.querySelectorAll<HTMLButtonElement>("button"))
}

function setToolbarActiveButton(target: HTMLButtonElement | null) {
  for (const btn of getToolbarButtons()) {
    btn.tabIndex = btn === target ? 0 : -1
  }
}

// Keeps the invariant: exactly one enabled button has tabindex=0 (or none
// when every button is disabled). Called on mount and after any reactive
// state change that could re-disable the currently-active button.
function ensureToolbarActiveButton() {
  const buttons = getToolbarButtons()
  const currentActive = buttons.find((b) => b.tabIndex === 0)
  if (currentActive && !currentActive.disabled) return
  const firstEnabled = buttons.find((b) => !b.disabled) ?? null
  setToolbarActiveButton(firstEnabled)
}

function onToolbarKeyDown(event: KeyboardEvent) {
  if (event.key !== "ArrowRight" && event.key !== "ArrowLeft" && event.key !== "Home" && event.key !== "End") return
  const enabled = getToolbarButtons().filter((b) => !b.disabled)
  if (enabled.length === 0) return
  const current = enabled.indexOf(event.target as HTMLButtonElement)
  if (current === -1) return

  let next: number
  switch (event.key) {
    case "ArrowRight":
      next = (current + 1) % enabled.length
      break
    case "ArrowLeft":
      next = (current - 1 + enabled.length) % enabled.length
      break
    case "Home":
      next = 0
      break
    case "End":
      next = enabled.length - 1
      break
    default:
      return
  }
  event.preventDefault()
  enabled[next].focus()
}

function onToolbarFocusIn(event: FocusEvent) {
  const target = event.target
  if (!(target instanceof HTMLButtonElement)) return
  if (!toolbarEl.value?.contains(target)) return
  setToolbarActiveButton(target)
}

onMounted(() => {
  ensureToolbarActiveButton()
})

// Repair after Vue re-renders the toolbar's :disabled bindings (e.g.
// isInactive flips, the link mark becomes active/inactive, the cursor
// moves so canInsertHorizontalRule changes). flush: "post" runs after
// the DOM update so b.disabled reflects the new state.
watch(
  () => [
    isInactive.value,
    canInsertHorizontalRule.value,
    canInsertHardBreak.value,
    activeMarks.value.link,
    canUndo.value,
    canRedo.value,
    canInsertLinkButton.value,
    canApplyLinkMark.value,
    canIndent.value,
    canOutdent.value,
    marksAllowedHere.value,
    italicAllowedHere.value,
    isTextblockSelection.value,
  ],
  () => {
    ensureToolbarActiveButton()
  },
  { flush: "post" },
)
</script>

<template>
  <!--
    Outer frame reuses InputStyled so the ring / hover behavior matches other inputs.
    focusWithin opts into painting the focused ring when any descendant (the contenteditable,
    or a toolbar button) is focused. p-0 overrides InputStyled's default: the toolbar and
    editor children own their own padding. Inline-size containment so that truncate works
    for uploading label.
  -->
  <InputStyled
    as="div"
    :inactive="isInactive"
    :invalid="invalid"
    focus-within
    :aria-readonly="isInactive || undefined"
    class="pd-inputhtml p-0 contain-inline-size"
    @dragenter="onWrapperDragEnter"
    @dragover="onWrapperDragOver"
    @dragleave="onWrapperDragLeave"
    @drop="onWrapperDrop"
  >
    <!--
      Toolbar visual style mirrors SearchResultsHeader: each logical group
      of controls is its own pill, separated by gap on a transparent outer
      toolbar row. Buttons follow SelectButton's selected-state styling.
      Stateless action buttons use plain hover because they have no aria-pressed.

      Sticky positioning keeps the toolbar visible when the editor's
      content scrolls past it. top offset = navbar height plus
      --pd-navbar-top so the toolbar follows the navbar even when it
      auto-hides. z-index to layer it above editor content while
      sliding over it.
    -->
    <div
      :id="toolbarId"
      ref="toolbarEl"
      role="toolbar"
      :aria-label="t('partials.input.InputHTML.toolbar.label')"
      class="sticky top-[calc(var(--pd-navbar-height)+var(--pd-navbar-top,0px))] z-10 flex flex-wrap items-center gap-1 rounded-t-sm border-b border-neutral-200 bg-slate-100 px-1 py-1"
      @keydown="onToolbarKeyDown"
      @focusin="onToolbarFocusIn"
    >
      <!--
        History pill.
      -->
      <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canUndo"
          :aria-label="t('partials.input.InputHTML.toolbar.undo')"
          :title="t('partials.input.InputHTML.toolbar.undo')"
          @click.prevent="triggerUndo(view)"
        >
          <ArrowUturnLeftIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canRedo"
          :aria-label="t('partials.input.InputHTML.toolbar.redo')"
          :title="t('partials.input.InputHTML.toolbar.redo')"
          @click.prevent="triggerRedo(view)"
        >
          <ArrowUturnRightIcon class="size-6" aria-hidden="true" />
        </button>
      </div>

      <!--
        Block type pill.
      -->
      <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'p'"
          :aria-label="t('partials.input.InputHTML.toolbar.paragraph')"
          :title="t('partials.input.InputHTML.toolbar.paragraph')"
          @click.prevent="setParagraph(view)"
        >
          <PilcrowIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'h1'"
          :aria-label="t('partials.input.InputHTML.toolbar.heading', { level: 1 })"
          :title="t('partials.input.InputHTML.toolbar.heading', { level: 1 })"
          @click.prevent="setHeading(view, 1)"
        >
          <H1Icon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'h2'"
          :aria-label="t('partials.input.InputHTML.toolbar.heading', { level: 2 })"
          :title="t('partials.input.InputHTML.toolbar.heading', { level: 2 })"
          @click.prevent="setHeading(view, 2)"
        >
          <H2Icon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'h3'"
          :aria-label="t('partials.input.InputHTML.toolbar.heading', { level: 3 })"
          :title="t('partials.input.InputHTML.toolbar.heading', { level: 3 })"
          @click.prevent="setHeading(view, 3)"
        >
          <H3Icon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'h4'"
          :aria-label="t('partials.input.InputHTML.toolbar.heading', { level: 4 })"
          :title="t('partials.input.InputHTML.toolbar.heading', { level: 4 })"
          @click.prevent="setHeading(view, 4)"
        >
          <H4Icon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="blockType === 'pre'"
          :aria-label="t('partials.input.InputHTML.toolbar.preformatted')"
          :title="t('partials.input.InputHTML.toolbar.preformatted')"
          @click.prevent="setPreformatted(view)"
        >
          <CodeBracketSquareIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="currentLevelList === 'bullet'"
          :aria-label="t('partials.input.InputHTML.toolbar.bulletList')"
          :title="t('partials.input.InputHTML.toolbar.bulletList')"
          @click.prevent="setBulletList(view)"
        >
          <ListBulletIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="currentLevelList === 'ordered'"
          :aria-label="t('partials.input.InputHTML.toolbar.orderedList')"
          :title="t('partials.input.InputHTML.toolbar.orderedList')"
          @click.prevent="setOrderedList(view)"
        >
          <NumberedListIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !isTextblockSelection"
          :aria-pressed="isBlockquote"
          :aria-label="t('partials.input.InputHTML.toolbar.blockquote')"
          :title="t('partials.input.InputHTML.toolbar.blockquote')"
          @click.prevent="setBlockquote(view)"
        >
          <BlockquoteIcon class="size-6" aria-hidden="true" />
        </button>
      </div>

      <!-- Formatting pill. -->
      <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !marksAllowedHere"
          :aria-pressed="activeMarks.bold"
          :aria-label="t('partials.input.InputHTML.toolbar.bold')"
          :title="t('partials.input.InputHTML.toolbar.bold')"
          @click.prevent="toggleBold(view)"
        >
          <BoldIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !italicAllowedHere"
          :aria-pressed="activeMarks.italic"
          :aria-label="t('partials.input.InputHTML.toolbar.italic')"
          :title="t('partials.input.InputHTML.toolbar.italic')"
          @click.prevent="toggleItalic(view)"
        >
          <ItalicIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !marksAllowedHere"
          :aria-pressed="activeMarks.underline"
          :aria-label="t('partials.input.InputHTML.toolbar.underline')"
          :title="t('partials.input.InputHTML.toolbar.underline')"
          @click.prevent="toggleUnderline(view)"
        >
          <UnderlineIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !marksAllowedHere"
          :aria-pressed="activeMarks.strikethrough"
          :aria-label="t('partials.input.InputHTML.toolbar.strikethrough')"
          :title="t('partials.input.InputHTML.toolbar.strikethrough')"
          @click.prevent="toggleStrikethrough(view)"
        >
          <StrikethroughIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none not-aria-pressed:hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:not-aria-pressed:hover:bg-transparent aria-pressed:bg-white aria-pressed:shadow-xs disabled:aria-pressed:bg-slate-100"
          :disabled="isInactive || !marksAllowedHere"
          :aria-pressed="activeMarks.monospace"
          :aria-label="t('partials.input.InputHTML.toolbar.monospace')"
          :title="t('partials.input.InputHTML.toolbar.monospace')"
          @click.prevent="toggleMonospace(view)"
        >
          <CodeBracketIcon class="size-6" aria-hidden="true" />
        </button>
      </div>

      <!--
        Indent / outdent pill.
      -->
      <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canOutdent"
          :aria-label="t('partials.input.InputHTML.toolbar.outdent')"
          :title="t('partials.input.InputHTML.toolbar.outdent')"
          @click.prevent="outdentList(view)"
        >
          <OutdentIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canIndent"
          :aria-label="t('partials.input.InputHTML.toolbar.indent')"
          :title="t('partials.input.InputHTML.toolbar.indent')"
          @click.prevent="indentList(view)"
        >
          <IndentIcon class="size-6" aria-hidden="true" />
        </button>
      </div>

      <!--
        Inserts pill.
      -->
      <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canInsertLinkButton || uploadingFile !== null"
          :aria-label="t('partials.input.InputHTML.toolbar.link')"
          :title="t('partials.input.InputHTML.toolbar.link')"
          @click.prevent="onStartInsertLink"
        >
          <LinkIcon class="size-6" aria-hidden="true" />
        </button>
        <template v-if="hasPermission(CAN_EDIT_FILE)">
          <button
            type="button"
            class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
            :disabled="isInactive || !canApplyLinkMark || uploadingFile !== null || isLinkInputDirty"
            :aria-label="t('partials.input.InputHTML.toolbar.attachFile')"
            :title="t('partials.input.InputHTML.toolbar.attachFile')"
            @click.prevent="onAttachFile"
          >
            <PaperClipIcon class="size-6" aria-hidden="true" />
          </button>
          <!--
            Hidden file input the Attach button triggers programmatically.
            translate="no" mirrors the contenteditable root so any browser
            translation layer leaves the filename alone.
          -->
          <input ref="fileInputRef" type="file" multiple class="hidden" @change="onFilePicked" />
        </template>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canInsertHorizontalRule || !isTextblockSelection"
          :aria-label="t('partials.input.InputHTML.toolbar.horizontalRule')"
          :title="t('partials.input.InputHTML.toolbar.horizontalRule')"
          @click.prevent="insertHorizontalRule(view)"
        >
          <MinusIcon class="size-6" aria-hidden="true" />
        </button>
        <button
          type="button"
          class="rounded-sm px-2 py-0.5 outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 disabled:cursor-not-allowed disabled:text-gray-500 disabled:hover:bg-transparent"
          :disabled="isInactive || !canInsertHardBreak || !isTextblockSelection"
          :aria-label="t('partials.input.InputHTML.toolbar.lineBreak')"
          :title="t('partials.input.InputHTML.toolbar.lineBreak')"
          @click.prevent="insertHardBreak(view)"
        >
          <ArrowTurnDownLeftIcon class="size-6" aria-hidden="true" />
        </button>
      </div>
    </div>

    <!--
      Mirroring how rendered HTMLClaim content is styled.
    -->
    <div ref="editorRoot" class="pd-inputhtml-editor prose max-w-none px-3 py-2 prose-gray"></div>

    <!--
      Post-editor focus park: ProseMirror's Escape handler focuses this span
      so the user can leave the editor without Tab-capture eating their next
      keypress. tabindex="-1" keeps it out of the normal tab order - focus
      lands here only via the Escape command. Sitting between the editor
      and the bottom toolbar means Tab from the sentinel lands on the
      bottom toolbar's controls (when shown) before exiting the wrapper;
      Shift-Tab is intercepted so it returns to the top toolbar rather
      than diving back into the contenteditable.
    -->
    <span ref="escapeSentinel" tabindex="-1" aria-hidden="true" class="sr-only" @keydown="onEscapeSentinelKeyDown"></span>
    <!--
      Context-sensitive bottom toolbar. Shown only when showBottomToolbar
      is true - i.e. bottomMode resolves to something non-null.
      Sticky positioning keeps it pinned to the viewport's bottom edge
      while the user scrolls up through tall editor content. z-index
      layers it above the editor as it slides over.
    -->
    <div
      v-if="showBottomToolbar"
      ref="bottomToolbarEl"
      role="toolbar"
      :aria-label="bottomLabel"
      class="sticky bottom-0 z-10 overflow-hidden rounded-b-sm border-t border-neutral-200 bg-slate-100"
      @focusout="onBottomToolbarFocusOut"
    >
      <!--
        Drop-target mode: a file is currently being dragged over the
        editor / toolbar. The drop itself is handled by onWrapperDrop
        on the surrounding InputStyled.
      -->
      <div v-if="bottomMode === 'drop-target'" class="flex flex-row items-center justify-center px-4 py-3">
        <span class="text-sm text-gray-700">{{ bottomLabel }}</span>
      </div>
      <!--
        Upload mode owns the toolbar exclusively while either an Attach
        or a Replace upload is in flight.
      -->
      <template v-else-if="bottomMode === 'upload'">
        <div class="flex flex-row items-center gap-2 py-1 pr-2 pl-4">
          <span class="min-w-0 flex-1 truncate text-sm text-gray-700" :title="bottomLabel">{{ bottomLabel }}</span>
          <Button type="button" class="shrink-0 px-3 py-2" @click.prevent="onCancelUpload">{{ t("common.buttons.cancel") }}</Button>
        </div>
        <ProgressBar :progress="uploadProgress" :total="uploadTotal" />
      </template>
      <!--
        Upload-error mode: the most recent upload failed. The message stays until
        the user dismisses it or starts another upload.
      -->
      <div v-else-if="bottomMode === 'upload-error'" class="flex flex-row items-center gap-2 py-1 pr-2 pl-4" role="alert">
        <span class="min-w-0 flex-1 truncate text-sm text-error-600" :title="bottomLabel">{{ bottomLabel }}</span>
        <Button type="button" class="shrink-0 px-3 py-2" @click.prevent="onDismissUploadError">{{ t("common.buttons.close") }}</Button>
      </div>
      <!--
        File-edit mode: the cursor is on a same-origin storage link.
      -->
      <div v-else-if="bottomMode === 'file-edit'" class="flex flex-row items-center gap-2 px-2 py-1">
        <!--
          Open is a real <a> (via ButtonStyled as="a"), not a <button>
          with a click handler: this preserves right-click "Open in new
          tab", middle-click, Ctrl/Cmd-click, and screen-reader link
          semantics. target="_blank" sends the user to the file in a fresh tab.
        -->
        <ButtonStyled as="a" :href="currentLinkValue" target="_blank" class="shrink-0 px-3 py-2">{{ t("common.buttons.open") }}</ButtonStyled>
        <template v-if="hasPermission(CAN_EDIT_FILE)">
          <Button
            type="button"
            class="min-w-0 flex-1 px-3 py-2"
            :active="isReplaceDragOver"
            :disabled="isInactive"
            @click.prevent="onReplaceFileClick"
            @dragover.prevent="onReplaceDragOver"
            @dragenter.prevent="onReplaceDragOver"
            @dragleave.prevent="onReplaceDragLeave"
            >{{ t("partials.input.InputHTML.toolbar.replaceFile") }}</Button
          >
        </template>
        <Button type="button" class="shrink-0 px-3 py-2" :disabled="isInactive" @click.prevent="onUnlinkFileLink">{{ t("common.buttons.unlink") }}</Button>
        <Button type="button" class="shrink-0 px-3 py-2" :disabled="isInactive" @click.prevent="onDeleteFileLink">{{ t("common.buttons.remove") }}</Button>
        <!--
          Hidden <input type="file"> dedicated to the Replace flow. Kept
          separate from fileInputRef (which is wired to onFilePicked for
          the Attach flow) so each path has its own @change handler.
        -->
        <input ref="fileReplaceInputRef" type="file" class="hidden" @change="onFileReplacePicked" />
      </div>
      <!--
        The form lets Enter inside the InputLink submit via the primary
        button (type=submit) without us hooking into key events.
        Forms in HTML are not supposed to nest, but browsers route Enter
        to the innermost ancestor form, so this works even when InputHTML
        is rendered inside an outer claim form.
      -->
      <form v-else class="flex flex-row gap-2 px-2 py-1" @submit.prevent="onConfirm">
        <!--
          Label column.
        -->
        <div class="flex shrink-0 flex-col items-start gap-1 pt-0.5" @click="onLabelClick">
          <span class="leading-none text-gray-700">{{ bottomLabel }}</span>
          <div class="flex flex-row flex-wrap gap-1">
            <InputBadges :changed="isLinkInputDirty" @revert="onRevertLinkInput" />
          </div>
        </div>
        <div class="flex min-w-0 flex-1 flex-col">
          <InputErrors v-slot="errorProps">
            <div class="flex flex-row items-start gap-2">
              <div class="flex min-w-0 flex-1 flex-col">
                <!--
                  Plain URL input. File links never reach this branch - they
                  take the file-edit toolbar above instead, which is why there
                  is no InputFile swap here.

                  Neither input is required: link href and blockquote cite
                  are both optional. In insert / add mode the primary
                  button is gated on non-empty via canPrimary (dirty
                  against an empty anchor); in edit modes an emptied
                  value is intentionally allowed and treated as Remove
                  inside onConfirm.

                  Contact schemes (mailto and tel) are refused for blockquote
                  cite, in sync with the backend, which validates cite with
                  allowContact false and link href with allowContact true
                  (validateURL in urls.go).
                -->
                <InputLink
                  ref="linkInputRef"
                  v-model="linkInputModel"
                  v-bind="errorProps"
                  :readonly="isInactive"
                  :allow-contact="bottomContext !== 'blockquote'"
                  class="w-full"
                />
              </div>
              <!--
                Per-mode button arrangement.
              -->
              <Button type="submit" primary class="shrink-0 self-center px-3 py-2" :disabled="!canPrimary">{{ primaryLabel }}</Button>
              <Button v-if="bottomMode === 'link-insert'" type="button" class="shrink-0 self-center px-3 py-2" @click.prevent="onCancelLink">{{
                t("common.buttons.cancel")
              }}</Button>
              <Button
                v-else-if="bottomMode === 'link-edit' || bottomMode === 'blockquote-edit'"
                type="button"
                class="shrink-0 self-center px-3 py-2"
                :disabled="!canRemoveLink"
                @click.prevent="onRemoveLink"
                >{{ t("common.buttons.remove") }}</Button
              >
            </div>
          </InputErrors>
        </div>
      </form>
    </div>
  </InputStyled>
</template>
