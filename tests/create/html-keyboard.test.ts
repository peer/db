import type { Locator, Page } from "@playwright/test"

import { createNamed, documentIdOf, notesSlot, PROPERTY_IDS } from "../peerdb_utils"
import {
  activateHtmlToolbarButton,
  checkpoint,
  checkpointElement,
  expect,
  expectHtmlEditorValue,
  fieldRow,
  hideDuplicates,
  htmlEditorContent,
  htmlEditorValue,
  htmlFocusedButton,
  htmlTabbableButton,
  htmlToolbar,
  htmlToolbarButton,
  type HtmlToolbarButtonState,
  htmlToolbarState,
  LOADING_TIMEOUT,
  pressHtmlToolbarKey,
  saveEdit,
  selectedText,
  settleDocument,
  settleFormFocus,
  signIn,
  startEdit,
  test,
  volatile,
} from "../utils"

// Every document these tests create is named with this prefix, so that the documents of this file never
// collide with the ones another test file creates and a document left in the data set says which file
// made it.
const NAME_PREFIX = "HTML Keyboard"

// Every test here works on a species it creates itself, because that class needs nothing but a name in
// order to be saved and its notes are edited through the rich text editor these tests drive. The role
// which may create one is the researcher (ROLE_CREATES in peerdb_utils says which role opens which
// class).
const SPECIES_CLASS = "SPECIES"
const SPECIES_ROLE = "researcher"

// The document an internal link is pointed at, by its identifier, so that the link the editor is given
// is one the application has a view for and the test does not depend on a title.
const LINKED_DOCUMENT_ID = await documentIdOf("PLANET", "G1_HOLLIS_III")

// The addresses the link form is given. The first two are taken, the third is refused: the schemes a
// link may use are an allowlist which the editor shares with the backend, and a scheme which runs code
// is not on it. The contact scheme is taken for the address of a link and refused for the source of a
// quotation, which is the one place the two lists differ.
const EXTERNAL_URL = "https://anchor.ccx.example/notes/html-keyboard"
const CONTACT_URL = "tel:+38612345678"
const REFUSED_URL = "javascript:alert(1)"
const QUOTE_SOURCE_URL = "https://anchor.ccx.example/sources/tide-table"
const QUOTE_SOURCE_REFUSED_URL = "mailto:archive@anchor.ccx.example"

// A value set on the window before an internal link is clicked. A page which is loaded again gets a
// fresh window and loses it, while a route the application takes on its own leaves it in place, which
// is how a link the application routed is told from one which reloaded the page.
const ROUTING_MARKER = "peerdbHtmlKeyboardMarker"

// The toolbar buttons which are enabled with the cursor at the start of an empty note, in the order the
// toolbar renders them. The roving tabindex walks the enabled buttons and skips the disabled ones, so
// which button an arrow key moves to follows from this list. Undo and redo are disabled while there is
// nothing to undo, and outdent and indent are disabled outside a list and outside a code block.
const ENABLED_BUTTONS = [
  "paragraph",
  "heading1",
  "heading2",
  "heading3",
  "heading4",
  "preformatted",
  "bulletlist",
  "orderedlist",
  "blockquote",
  "bold",
  "italic",
  "underline",
  "strikethrough",
  "monospace",
  "link",
  "attachfile",
  "horizontalrule",
  "linebreak",
]

// The buttons which are disabled with the cursor at the start of an empty note, so that the test which
// walks the enabled ones also states which ones it expects to be skipped.
const DISABLED_BUTTONS = ["undo", "redo", "outdent", "indent"]

// Everything below addresses the one rich text editor of the form, which is the notes field. The logic is
// shared with the other applications built on PeerDB (utils.ts); what is local is only which part of the
// form holds the editor.

// The toolbar of the rich text editor of the notes field.
function toolbar(page: Page): Locator {
  return htmlToolbar(notesSlot(page))
}

// One toolbar button, addressed by the part of its class name which says what it does.
function toolbarButton(page: Page, name: string): Locator {
  return htmlToolbarButton(notesSlot(page), name)
}

// The element ProseMirror makes editable inside the mount point of the editor.
function editorContent(page: Page): Locator {
  return htmlEditorContent(notesSlot(page))
}

async function toolbarState(page: Page): Promise<Array<HtmlToolbarButtonState>> {
  return await htmlToolbarState(notesSlot(page))
}

async function tabbableButton(page: Page): Promise<string> {
  return await htmlTabbableButton(notesSlot(page))
}

async function focusedButton(page: Page): Promise<string> {
  return await htmlFocusedButton(notesSlot(page))
}

async function pressToolbarKey(page: Page, key: string, expected: string): Promise<void> {
  await pressHtmlToolbarKey(page, notesSlot(page), key, expected)
}

async function activateToolbarButton(page: Page, name: string): Promise<void> {
  await activateHtmlToolbarButton(page, notesSlot(page), name)
}

async function editorHtml(page: Page): Promise<string> {
  return await htmlEditorValue(notesSlot(page))
}

async function expectEditorHtml(page: Page, expected: string, message: string): Promise<void> {
  await expectHtmlEditorValue(notesSlot(page), expected, message)
}

// The form at the bottom of the editor which takes the address of a link or the source of a quotation.
function linkForm(page: Page): Locator {
  return notesSlot(page).locator(".pd-inputhtml-form-link")
}

// The bar at the bottom of the editor which the form above sits in, and which is what the editor puts
// whatever the cursor is on into: the address of a link, the source of a quotation, the state of an
// upload.
function bottomToolbar(page: Page): Locator {
  return notesSlot(page).locator(".pd-inputhtml-toolbar-bottom")
}

// Where the editor these tests drive sits among the editors of the form, which is what focus is compared
// against below. The form holds one rich text editor per HTML field of the class and the notes may be
// stated more than once, so leaving the editor means landing outside this one, which is not the same as
// landing outside every one of them.
async function notesWidgetIndex(page: Page): Promise<number> {
  return await notesSlot(page)
    .locator(".pd-inputhtml")
    .first()
    .evaluate((element) => Array.from(document.querySelectorAll(".pd-inputhtml")).indexOf(element))
}

// What the browser has focused, described by the parts a keyboard test cares about: which region of the
// editor the focused element sits in, and enough of its identity to name it in a failure message. The
// editor it sits in is reported as the position of that editor among all of them, so that it can be
// compared against the one notesWidgetIndex named.
interface FocusInfo {
  id: string
  tag: string
  button: string
  widget: number
  inEditor: boolean
  inToolbar: boolean
  isSentinel: boolean
  editable: boolean
}

async function focusInfo(page: Page): Promise<FocusInfo> {
  return await page.evaluate(() => {
    const empty = { id: "", tag: "", button: "", widget: -1, inEditor: false, inToolbar: false, isSentinel: false, editable: false }
    const active = document.activeElement
    if (active === null) {
      return empty
    }
    const widgets = Array.from(document.querySelectorAll(".pd-inputhtml"))
    const widget = active.closest(".pd-inputhtml")
    return {
      id: active.id,
      tag: active.tagName.toLowerCase(),
      button:
        Array.from(active.classList)
          .find((name) => name.startsWith("pd-inputhtml-button-"))
          ?.replace("pd-inputhtml-button-", "") ?? "",
      widget: widget === null ? -1 : widgets.indexOf(widget),
      inEditor: active.closest(".pd-inputhtml-editor") !== null,
      inToolbar: active.closest(".pd-inputhtml-toolbar") !== null,
      isSentinel: active.classList.contains("pd-inputhtml-sentinel"),
      editable: active.getAttribute("contenteditable") === "true",
    }
  })
}

// Selects the line the cursor is on, which is what the buttons which wrap a selection (the marks and the
// link) are given to work on.
//
// A caret key is the browser's own and moves the browser's selection, which the editor reads back into
// its own state afterwards, and the commands the toolbar runs work on the state rather than on what the
// browser has selected. What is waited for is therefore both: that the browser has selected the line, and
// that the editor has read that selection. The editor says it has by the class it carries for a selection
// which spans a range, and the reading is waited for rather than assumed because the two are what the
// toolbar tells apart: a link made at a caret is the address written out as its own text, while a link
// made out of a selection wraps the selected text, and the toolbar offers both, so nothing about how the
// buttons look says which of the two the editor is about to make.
async function selectLine(page: Page, text: string): Promise<void> {
  await page.keyboard.press("End")
  await page.keyboard.press("Shift+Home")
  await expect.poll(() => selectedText(page), { message: `the line "${text}" is selected` }).toContain(text)
  await expect(notesSlot(page).locator(".pd-inputhtml-selection-range"), `the editor has read the selection of "${text}"`).toHaveCount(1)
  await expect(toolbarButton(page, "link"), `the link button once the editor has read the selection of "${text}"`).toBeEnabled()
}

// Moves the cursor to the line below the one it is on and selects that line.
async function selectNextLine(page: Page, text: string): Promise<void> {
  await page.keyboard.press("ArrowDown")
  await selectLine(page, text)
}

// Creates a species with the given name and opens it for editing, which is where every test here starts.
// Nothing is checkpointed on the way: what these tests are about is the editor of the saved document,
// and each of them takes its own screenshots of it.
async function createSpeciesAndEdit(page: Page, name: string): Promise<void> {
  await createNamed(page, SPECIES_CLASS, `${NAME_PREFIX} ${name}`)

  await startEdit(page)
  await hideDuplicates(page)
  await settleFormFocus(page)
  await expect(editorContent(page), "editable area of the notes editor").toBeVisible({ timeout: LOADING_TIMEOUT })
}

// The value of the notes of the saved document, which is what the editor wrote into the claim.
function savedNotes(page: Page): Locator {
  return fieldRow(page, PROPERTY_IDS.NOTES).locator(".pd-claimvaluehtml").first()
}

test.describe("PeerDB HTML Editor Keyboard Flows", () => {
  test("Test that Tab inside the editor is captured by the editor", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Tab")

    const content = editorContent(page)
    await content.click()
    await page.keyboard.type("A note about the keyboard.")
    await expectEditorHtml(page, "<p>A note about the keyboard.</p>", "typed text is a paragraph")

    // Tab is bound inside the editor so that it can indent, which means it cannot also move focus. In a
    // plain paragraph there is nothing to indent, so the binding consumes the key and does nothing: the
    // value is left alone and focus stays on the editable area.
    await page.keyboard.press("Tab")
    expect(await editorHtml(page), "Tab in a paragraph leaves the value alone").toBe("<p>A note about the keyboard.</p>")
    expect(await focusInfo(page), "Tab in a paragraph keeps focus on the editable area").toMatchObject({ inEditor: true, editable: true })

    // In a list Tab nests the item the cursor is in under the item above it, so the same key which does
    // nothing in a paragraph is the one which structures a list. The list is made through the toolbar,
    // whose buttons are reached and activated from the keyboard.
    await page.keyboard.press("Enter")
    await page.keyboard.type("First item")
    await activateToolbarButton(page, "bulletlist")
    await expectEditorHtml(page, "<ul><li><p>First item</p></li></ul>", "the block the cursor was in became a list")
    expect(await focusInfo(page), "activating a toolbar button gives focus back to the editable area").toMatchObject({ inEditor: true, editable: true })

    await page.keyboard.press("Enter")
    await page.keyboard.type("Second item")
    await expectEditorHtml(page, "<ul><li><p>First item</p></li><li><p>Second item</p></li></ul>", "the list holds two items")
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-tab-list-flat")

    await page.keyboard.press("Tab")
    await expectEditorHtml(page, "<ul><li><p>First item</p><ul><li><p>Second item</p></li></ul></li></ul>", "Tab nests the second item under the first")
    expect(await focusInfo(page), "Tab in a list keeps focus on the editable area").toMatchObject({ inEditor: true, editable: true })
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-tab-list-nested")

    // Shift-Tab is bound to the reverse, so the nesting can be undone with the keyboard which made it.
    await page.keyboard.press("Shift+Tab")
    await expectEditorHtml(page, "<ul><li><p>First item</p></li><li><p>Second item</p></li></ul>", "Shift-Tab lifts the nested item back out")
    expect(await focusInfo(page), "Shift-Tab in a list keeps focus on the editable area").toMatchObject({ inEditor: true, editable: true })

    // In a code block Tab is a literal tab, because that is what indenting means there. Enter on the
    // empty item which follows the last one leaves the list, so the code block is a block of its own.
    await page.keyboard.press("Enter")
    await page.keyboard.press("Enter")
    await page.keyboard.type("tide table")
    await activateToolbarButton(page, "preformatted")
    await page.keyboard.press("Tab")
    await expectEditorHtml(page, "tide table\t", "Tab in a code block inserts a tab character")
    expect(await focusInfo(page), "Tab in a code block keeps focus on the editable area").toMatchObject({ inEditor: true, editable: true })
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-tab-code-block")

    console.log("Successfully drove 4 Tab presses inside the editor, which indented a list item, lifted it back out, indented code and did nothing in a paragraph.")
  })

  test("Test the escape hatch out of the editor", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Escape")

    const content = editorContent(page)
    const widget = await notesWidgetIndex(page)
    await content.click()
    expect(await focusInfo(page), "focus starts on the editable area of the notes editor").toMatchObject({ widget, inEditor: true, editable: true })

    // Because the editor captures Tab, a keyboard user would be stuck in it without a way out. Escape is
    // that way out: it parks focus on a sentinel which sits after the editable area and outside it, so
    // the Tab which follows is the browser's own and not the editor's.
    await page.keyboard.press("Escape")
    expect(await focusInfo(page), "Escape parks focus on the sentinel after the editable area").toMatchObject({
      isSentinel: true,
      inEditor: false,
      widget,
    })
    // The sentinel itself is there only for the keyboard and shows nothing, so what a screenshot of the
    // parked state says is that the editor is no longer the focused control of the form.
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-escape-focus-parked")

    // From the sentinel Tab leaves the editor widget altogether and lands on what follows it on the form.
    await page.keyboard.press("Tab")
    const afterEscape = await focusInfo(page)
    expect(afterEscape, "Tab after Escape moves focus out of the editor it was pressed in").toMatchObject({ inEditor: false, inToolbar: false, editable: false })
    expect(afterEscape.widget, "Tab after Escape moves focus out of the editor widget").not.toBe(widget)

    // Shift-Tab from the sentinel is intercepted so that it does not fall back into the editable area,
    // which would be a way back into the trap. It goes to the toolbar instead, onto the button the user
    // last left it on, which is the button the roving tabindex holds as the tab stop.
    await toolbarButton(page, "bold").focus()
    expect(await tabbableButton(page), "focusing a toolbar button makes it the tab stop of the toolbar").toBe("bold")
    await page.keyboard.press("Tab")
    expect(await focusInfo(page), "Tab from the toolbar enters the editable area").toMatchObject({ widget, inEditor: true, editable: true })

    await page.keyboard.press("Escape")
    expect(await focusInfo(page), "Escape parks focus on the sentinel again").toMatchObject({ isSentinel: true })
    await page.keyboard.press("Shift+Tab")
    expect(await focusInfo(page), "Shift-Tab from the sentinel goes back to the toolbar and not into the editable area").toMatchObject({
      widget,
      inToolbar: true,
      inEditor: false,
      button: "bold",
    })
    await expect(toolbarButton(page, "bold"), "the toolbar button focus returned to").toBeFocused()
    await checkpointElement(page, toolbar(page), "htmlkeyboard-escape-focus-back-to-toolbar")

    console.log("Successfully left the editor with Escape and 1 Tab, and returned from the sentinel to the toolbar with Shift-Tab.")
  })

  test("Test the roving tabindex of the toolbar", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Roving")

    // The toolbar is walked with the arrow keys and not with Tab, so that the whole toolbar costs a
    // keyboard user one tab stop instead of one per button. Nothing is typed into the editor first, so
    // that which buttons are enabled follows only from the cursor sitting in an empty paragraph.
    const widget = await notesWidgetIndex(page)
    const state = await toolbarState(page)
    expect(
      state.filter((button) => !button.disabled).map((button) => button.name),
      "the buttons which are enabled in an empty note",
    ).toEqual(ENABLED_BUTTONS)
    expect(
      state.filter((button) => button.disabled).map((button) => button.name),
      "the buttons which are disabled in an empty note",
    ).toEqual(DISABLED_BUTTONS)
    expect(await tabbableButton(page), "the toolbar of an untouched editor offers its first enabled button as its tab stop").toBe(ENABLED_BUTTONS[0])

    await toolbarButton(page, ENABLED_BUTTONS[0]).focus()
    expect(await focusedButton(page), "the first enabled button takes focus").toBe(ENABLED_BUTTONS[0])
    await checkpointElement(page, toolbar(page), "htmlkeyboard-roving-first-button")

    // The arrow keys step through the enabled buttons, and the tab stop follows focus so that only one
    // button is ever tabbable.
    await pressToolbarKey(page, "ArrowRight", ENABLED_BUTTONS[1])
    await checkpointElement(page, toolbar(page), "htmlkeyboard-roving-arrow-right")
    await pressToolbarKey(page, "ArrowRight", ENABLED_BUTTONS[2])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[1])

    // Home and End jump to the ends of the enabled buttons, and stepping off either end wraps around.
    await pressToolbarKey(page, "End", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 1])
    await checkpointElement(page, toolbar(page), "htmlkeyboard-roving-end")
    await pressToolbarKey(page, "ArrowRight", ENABLED_BUTTONS[0])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 1])
    await pressToolbarKey(page, "Home", ENABLED_BUTTONS[0])

    // The disabled buttons are stepped over rather than focused, so a keyboard user never lands on a
    // button which would do nothing. Outdent and indent sit between monospace and link, so walking left
    // from the end reaches monospace one press after link.
    await pressToolbarKey(page, "End", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 1])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 2])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 3])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 4])
    await pressToolbarKey(page, "ArrowLeft", ENABLED_BUTTONS[ENABLED_BUTTONS.length - 5])
    expect(await focusedButton(page), "the arrow keys step over the disabled outdent and indent buttons").toBe("monospace")

    // One more Tab from anywhere in the toolbar has to reach the editable area, which is what having a
    // single tab stop is for.
    await page.keyboard.press("Tab")
    expect(await focusInfo(page), "Tab from the toolbar enters the editable area").toMatchObject({ widget, inEditor: true, editable: true })
    expect(await tabbableButton(page), "the toolbar keeps the button it was left on as its tab stop").toBe("monospace")

    console.log(`Successfully walked the roving tabindex of the toolbar over ${ENABLED_BUTTONS.length} enabled buttons with 12 arrow, Home and End presses.`)
  })

  test("Test the marks applied by a keyboard shortcut and by a button", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Marks")

    const content = editorContent(page)
    await content.click()
    await page.keyboard.type("A marked sentence.")
    // Select all is a shortcut of the editor itself, so what it selects is what the editor holds as its
    // selection. Selecting with the cursor keys instead would leave the mark shortcut below racing the
    // editor's reading of the browser's selection, and a mark applied to a selection the editor has not
    // seen yet only arms the mark for the next typed character.
    await page.keyboard.press("Control+a")
    await expect.poll(() => selectedText(page), { message: "the whole paragraph is selected" }).toContain("A marked sentence.")

    // The marks have keyboard shortcuts of their own, so the toolbar is not the only way to apply them.
    await page.keyboard.press("Control+b")
    await expectEditorHtml(page, "<p><b>A marked sentence.</b></p>", "Control-b makes the selected text bold")
    await expect(toolbarButton(page, "bold"), "the bold button of the toolbar shows the mark the shortcut applied").toHaveAttribute("aria-pressed", "true")
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-marks-bold")

    // Undo and redo are shortcuts as well, and they take the mark back out of the value and put it back.
    await page.keyboard.press("Control+z")
    await expect.poll(() => editorHtml(page), { message: "Control-z takes the mark back out" }).not.toContain("<b>")
    await page.keyboard.press("Control+y")
    await expectEditorHtml(page, "<p><b>A marked sentence.</b></p>", "Control-y puts the mark back")
    await expect.poll(() => selectedText(page), { message: "undo and redo restore the selection they were applied to" }).toContain("A marked sentence.")

    await page.keyboard.press("Control+i")
    await expectEditorHtml(page, "<p><b><i>A marked sentence.</i></b></p>", "Control-i adds the italic mark to the bold one")
    await expect(toolbarButton(page, "italic"), "the italic button of the toolbar shows the mark the shortcut applied").toHaveAttribute("aria-pressed", "true")

    // Underline has a shortcut of its own, while the two marks which have none are applied with their
    // buttons, which apply to the same selection and stack in the order the schema declares them.
    await page.keyboard.press("Control+u")
    await expectEditorHtml(page, "<p><b><i><u>A marked sentence.</u></i></b></p>", "Control-u adds the underline mark")
    await toolbarButton(page, "strikethrough").click()
    await expectEditorHtml(page, "<strike>A marked sentence.</strike>", "the strikethrough button adds its mark")
    await toolbarButton(page, "monospace").click()
    await expectEditorHtml(page, "<tt>A marked sentence.</tt>", "the monospace button adds its mark")
    for (const mark of ["bold", "italic", "underline", "strikethrough", "monospace"]) {
      await expect(toolbarButton(page, mark), `the ${mark} button of the toolbar shows its mark is on`).toHaveAttribute("aria-pressed", "true")
    }
    await checkpointElement(page, notesSlot(page), "htmlkeyboard-marks-all")

    // What the shortcuts and the buttons produced has to survive being written to the document, which is
    // what makes them a way of editing the value and not only of changing what the editor renders.
    await saveEdit(page)
    const saved = await savedNotes(page).innerHTML()
    expect(saved, "the saved note keeps the five marks which were applied").toContain("<p><b><i><u><strike><tt>A marked sentence.</tt></strike></u></i></b></p>")
    await checkpoint(page, "htmlkeyboard-marks-saved-document", { mask: volatile(page) })

    console.log("Successfully applied 5 marks, 3 of them with keyboard shortcuts, undid and redid one of them, and saved them into the document.")
  })

  test("Test the block buttons of the toolbar", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Blocks")

    const content = editorContent(page)
    const notes = notesSlot(page)
    await content.click()

    // A heading takes the block the cursor is in, so it is typed first and turned into one afterwards.
    await page.keyboard.type("A third level heading")
    await toolbarButton(page, "heading3").click()
    await expectEditorHtml(page, "<h3>A third level heading</h3>", "the heading button turns the block into a heading")
    await expect(toolbarButton(page, "heading3"), "the heading button shows the block it made").toHaveAttribute("aria-pressed", "true")
    await checkpointElement(page, notes, "htmlkeyboard-blocks-heading")

    // Every block type button is a way out of the one before it, so the block goes back to being an
    // ordinary paragraph and then becomes the heading the note keeps.
    await toolbarButton(page, "paragraph").click()
    await expectEditorHtml(page, "<p>A third level heading</p>", "the paragraph button turns the heading back into a paragraph")
    await expect(toolbarButton(page, "heading3"), "the heading button after the block became a paragraph again").toHaveAttribute("aria-pressed", "false")
    await toolbarButton(page, "heading3").click()
    await expectEditorHtml(page, "<h3>A third level heading</h3>", "the heading button makes the heading again")

    // A numbered list is the same block turned into a list, and its items are the blocks below it.
    await page.keyboard.press("Enter")
    await page.keyboard.type("First item")
    await toolbarButton(page, "orderedlist").click()
    await expectEditorHtml(page, "<ol><li><p>First item</p></li></ol>", "the ordered list button turns the block into a numbered list")
    await page.keyboard.press("Enter")
    await page.keyboard.type("Second item")

    // Indenting is what Tab does in a list, offered as a button for a user who does not know that, and it
    // is enabled only where there is something to indent.
    await expect(toolbarButton(page, "indent"), "the indent button inside a list").toBeEnabled()
    await toolbarButton(page, "indent").click()
    await expectEditorHtml(page, "<ol><li><p>First item</p><ol><li><p>Second item</p></li></ol></li></ol>", "the indent button nests the second item")
    await checkpointElement(page, notes, "htmlkeyboard-blocks-list-nested")
    await toolbarButton(page, "outdent").click()
    await expectEditorHtml(page, "<ol><li><p>First item</p></li><li><p>Second item</p></li></ol>", "the outdent button lifts the nested item back out")

    // A code block keeps what is typed into it as it is typed, which is why the marks are not offered
    // inside one: the schema allows no mark in a preformatted block.
    await page.keyboard.press("Enter")
    await page.keyboard.press("Enter")
    await page.keyboard.type("read_back = 2")
    await toolbarButton(page, "preformatted").click()
    await expectEditorHtml(page, "<pre>read_back = 2</pre>", "the preformatted button turns the block into a code block")
    await expect(toolbarButton(page, "bold"), "the bold button inside a code block").toBeDisabled()
    await expect(toolbarButton(page, "indent"), "the indent button inside a code block").toBeEnabled()
    await checkpointElement(page, notes, "htmlkeyboard-blocks-code")

    // Enter inside a code block is a line of its own, so a second Enter on the empty line it left behind
    // is what ends the block and starts an ordinary paragraph after it.
    await page.keyboard.press("Enter")
    await page.keyboard.press("Enter")
    await expectEditorHtml(page, "<pre>read_back = 2</pre>", "the code block keeps what was typed into it after the cursor left it")

    // The two inserts put a node at the cursor rather than changing the block it is in: a rule between
    // blocks, and a line break inside one.
    await page.keyboard.type("Before the rule")
    await toolbarButton(page, "linebreak").click()
    await page.keyboard.type("after the break")
    await expectEditorHtml(page, "<p>Before the rule<br>after the break</p>", "the line break button breaks the line inside the paragraph")
    // The rule is a leaf block, which the editor selects as it inserts it and renders with the attributes
    // it drags and selects such a node by, so it is counted rather than matched against the value.
    await toolbarButton(page, "horizontalrule").click()
    await expect(content.locator("hr"), "the horizontal rule button inserts a rule").toHaveCount(1)
    await checkpointElement(page, notes, "htmlkeyboard-blocks-inserts")

    await saveEdit(page)
    const saved = await savedNotes(page).innerHTML()
    expect(saved, "the saved note keeps the heading").toContain("<h3>A third level heading</h3>")
    expect(saved, "the saved note keeps the numbered list").toContain("<ol><li><p>First item</p></li><li><p>Second item</p></li></ol>")
    expect(saved, "the saved note keeps the code block").toContain("<pre>read_back = 2</pre>")
    expect(saved, "the saved note keeps the line break").toContain("<p>Before the rule<br>after the break</p>")
    expect(saved, "the saved note keeps the rule").toContain("<hr>")
    await checkpoint(page, "htmlkeyboard-blocks-saved-document", { mask: volatile(page) })

    console.log("Successfully built a note of 6 blocks with the heading, list, indent, outdent, code, line break and rule buttons of the toolbar.")
  })

  test("Test the quote button and the source it takes", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Quote")

    const content = editorContent(page)
    const notes = notesSlot(page)
    await content.click()
    await page.keyboard.type("The table was read back twice.")
    await toolbarButton(page, "blockquote").click()
    await expectEditorHtml(page, "<blockquote><p>The table was read back twice.</p></blockquote>", "the quote button turns the block into a quotation")
    await expect(toolbarButton(page, "blockquote"), "the quote button shows the block it made").toHaveAttribute("aria-pressed", "true")

    // A quotation is asked where it was quoted from, which is a second address the editor takes, in the
    // same form as the address of a link.
    const form = linkForm(page)
    await expect(form, "the form which takes the source of the quotation").toBeVisible()
    const input = form.locator(".pd-inputhtml-input-link .pd-inputlink-input")
    await expect(input, "the address input of the source form").toBeVisible()
    await checkpointElement(page, notes, "htmlkeyboard-quote-form")

    // The source of a quotation is a document which can be fetched, so the two schemes which reach a
    // person rather than a document are refused here, while a link may use them.
    await input.fill(QUOTE_SOURCE_REFUSED_URL)
    await form.locator(".pd-inputhtml-button-confirm").click()
    await expect(form.locator(".pd-inputerrors-error"), "the error the refused source is reported with").toBeVisible()
    expect(await editorHtml(page), "the refused source is not written into the quotation").not.toContain("cite=")
    await checkpointElement(page, notes, "htmlkeyboard-quote-refused")

    await input.fill(QUOTE_SOURCE_URL)
    await form.locator(".pd-inputhtml-button-confirm").click()
    await expectEditorHtml(page, `<blockquote cite="${QUOTE_SOURCE_URL}">`, "the accepted source is written into the quotation")
    await expect(form.locator(".pd-inputerrors-error"), "the error after the source was accepted").toHaveCount(0)
    await checkpointElement(page, notes, "htmlkeyboard-quote-cited")

    await saveEdit(page)
    const saved = await savedNotes(page).innerHTML()
    expect(saved, "the saved note keeps the quotation and its source").toContain(
      `<blockquote cite="${QUOTE_SOURCE_URL}"><p>The table was read back twice.</p></blockquote>`,
    )
    await checkpoint(page, "htmlkeyboard-quote-saved-document", { mask: volatile(page) })

    console.log("Successfully quoted 1 block, had 1 source refused for its scheme, and saved the quotation with the source which was accepted.")
  })

  test("Test the link button and the addresses it takes", async ({ context }) => {
    const page = await context.newPage()

    await signIn(page, [SPECIES_ROLE])
    await createSpeciesAndEdit(page, "Links")

    const content = editorContent(page)
    const notes = notesSlot(page)
    const form = linkForm(page)
    const input = form.locator(".pd-inputhtml-input-link .pd-inputlink-input")
    const confirmButton = form.locator(".pd-inputhtml-button-confirm")
    await content.click()

    // The three lines the links are made of are typed before any of them is linked. Typing and Enter are
    // handled by the editor itself, so the blocks are made while the editor and the browser agree on where
    // the cursor is, and no key which the editor handles is pressed on a selection a caret key has just
    // made, which the editor would still be reading.
    await page.keyboard.type("An outside page")
    await page.keyboard.press("Enter")
    await page.keyboard.type("A planet of the survey")
    await page.keyboard.press("Enter")
    await page.keyboard.type("The station")
    await expectEditorHtml(page, "<p>An outside page</p><p>A planet of the survey</p><p>The station</p>", "the note holds the three lines the links are made of")

    // A link is made out of a selection: the button opens a form at the bottom of the editor, the address
    // is typed into it, and confirming wraps the selected text into a link.
    await page.keyboard.press("Control+Home")
    await selectLine(page, "An outside page")
    await toolbarButton(page, "link").click()
    await expect(form, "the form which takes the address of the link").toBeVisible()
    await expect(input, "the address input of the link form").toBeVisible()
    // The two screenshots taken while the form is open are of the toolbar the form sits in rather than of
    // the whole field: opening the form takes focus off the editable area, and how much of the selection
    // the browser keeps painting in an editable area which no longer holds focus is not the same from one
    // run to the next.
    await checkpointElement(page, bottomToolbar(page), "htmlkeyboard-link-form")

    // The schemes a link may use are an allowlist, so an address which would run code where a document is
    // expected is refused and nothing is written into the note.
    await input.fill(REFUSED_URL)
    await confirmButton.click()
    await expect(form.locator(".pd-inputerrors-error"), "the error the refused address is reported with").toBeVisible()
    expect(await editorHtml(page), "the refused address is not written into the note").not.toContain("<a")
    await checkpointElement(page, bottomToolbar(page), "htmlkeyboard-link-refused")

    await input.fill(EXTERNAL_URL)
    await confirmButton.click()
    await expectEditorHtml(page, `<a href="${EXTERNAL_URL}"`, "the accepted address is written into the note")
    // The link is made out of the selection, so it is the selected text which carries the address. A link
    // made at a caret instead carries the address as its own text, which is what this tells apart.
    await expect(content.locator("a").first(), "the link is made out of the selected line").toHaveText("An outside page")
    // An address which leaves this site is marked as such while it is being written, which is what the
    // icon next to it is drawn from.
    await expect(content.locator("a").first(), "the class of the link which leaves the site").toHaveClass(/pd-link-external/)
    await checkpointElement(page, notes, "htmlkeyboard-link-external")

    // An address of this site which the application has a view for is marked as one it routes itself.
    await selectNextLine(page, "A planet of the survey")
    await toolbarButton(page, "link").click()
    await input.fill(`/d/${LINKED_DOCUMENT_ID}`)
    await confirmButton.click()
    await expectEditorHtml(page, `<a href="/d/${LINKED_DOCUMENT_ID}"`, "the address of a document of this site is written into the note")
    const internalLink = content.locator(`a[href="/d/${LINKED_DOCUMENT_ID}"]`)
    await expect(internalLink, "the link is made out of the selected line").toHaveText("A planet of the survey")
    await expect(internalLink, "the class of the link to a document of this site").toHaveClass(/pd-link-internal/)
    await expect(internalLink, "the link to a document of this site is one the application routes").not.toHaveClass(/pd-link-internal-noview/)

    // A contact scheme is taken for the address of a link, and it is neither of this site nor a page, so
    // it is left unmarked.
    await selectNextLine(page, "The station")
    await toolbarButton(page, "link").click()
    await input.fill(CONTACT_URL)
    await confirmButton.click()
    await expectEditorHtml(page, `<a href="${CONTACT_URL}"`, "the contact address is written into the note")
    await expect(content.locator(`a[href="${CONTACT_URL}"]`), "the link is made out of the selected line").toHaveText("The station")
    await expect(content.locator(`a[href="${CONTACT_URL}"]`), "the class of the contact link").not.toHaveClass(/pd-link/)
    // Every line the note was given has to still be a line of its own, each with the link it was given.
    await expect(content.locator("a"), "the links the note holds").toHaveCount(3)
    await expect(content.locator("p"), "the lines the note holds").toHaveCount(3)
    await checkpointElement(page, notes, "htmlkeyboard-link-all")

    await saveEdit(page)
    await checkpoint(page, "htmlkeyboard-link-saved-document", { mask: volatile(page) })

    // The saved note is classified again when it is rendered, so what the reader is shown carries the
    // same marks the editor did, and the link which leaves the site is stripped of the referrer.
    const saved = savedNotes(page)
    const savedExternal = saved.locator(`a[href="${EXTERNAL_URL}"]`)
    await expect(savedExternal, "the class of the saved link which leaves the site").toHaveClass(/pd-link-external/)
    await expect(savedExternal, "the saved link which leaves the site sends no referrer").toHaveAttribute("rel", /noreferrer/)
    await expect(saved.locator(`a[href="${CONTACT_URL}"]`), "the class of the saved contact link").not.toHaveClass(/pd-link/)

    const savedInternal = saved.locator(`a[href="/d/${LINKED_DOCUMENT_ID}"]`)
    await expect(savedInternal, "the class of the saved link to a document of this site").toHaveClass(/pd-link-internal/)

    // A link to a document of this site is followed without loading the page again, which is what the
    // class it carries stands for.
    await page.evaluate((marker) => ((window as unknown as Record<string, boolean>)[marker] = true), ROUTING_MARKER)
    await savedInternal.click()
    await settleDocument(page)
    expect(page.url(), "the address the internal link led to").toContain(`/d/${LINKED_DOCUMENT_ID}`)
    expect(
      await page.evaluate((marker) => (window as unknown as Record<string, boolean>)[marker] === true, ROUTING_MARKER),
      "the page was not loaded again to follow the internal link",
    ).toBe(true)
    await checkpoint(page, "htmlkeyboard-link-followed-document", { mask: volatile(page) })

    console.log("Successfully made 3 links of 3 kinds, had 1 address refused, and followed the internal one of the saved note without loading the page again.")
  })
})
