<!--
InputRef provides an autocomplete-style picker for a document reference.

It has three visual states:

  1. No selection: a regular text input with a chevron toggle on the right.
     Typing fires a search; results appear in a dropdown.
  2. Selected, not editing: the input area is replaced by a "chip" that
     renders the selected document's display label and shows an open-link
     icon + Clear button on the right. Tabbing into or clicking the chip
     enters edit mode.
  3. Selected, editing: the chip is replaced by the search input again,
     pre-filled with the user's previously typed query (if any). The
     Clear button stays visible. If the user defocuses without picking a
     new option, edit mode exits and the chip reappears with the prior
     selection intact.

The chip is a contenteditable div (not a button) so it has a real blinking
caret on focus, the closest match to a native <input readonly>. Real edits
are blocked (@beforeinput.prevent etc.), so the chip is read-only despite
contenteditable=true.
-->

<script setup lang="ts">
import type { D } from "@/document"
import type { Result } from "@/types"
import type { ComponentPublicInstance } from "vue"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import InputStyled from "@/components/InputStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { anySignal, loadingWidth } from "@/utils"

// Wildcard to see if a string ends with unicode letter or number.
const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

const props = withDefaults(
  defineProps<{
    progress?: number
    readonly?: boolean
  }>(),
  {
    progress: 0,
    readonly: false,
  },
)

const model = defineModel<string>({ default: "" })

// We want all fallthrough attributes to be passed to the combobox input element.
defineOptions({
  inheritAttrs: false,
})

// Local search progress, intentionally not stacked on the parent's progress
// chain (i.e. not useProgress()). A search in flight should only drive the
// inline progress bar under this input, never the parent component's progress
// bar or top-level progress UI.
const searchProgress = ref(0)

const router = useRouter()
const { t } = useI18n({ useScope: "global" })

// Two-way derived view over model.value. The getter constructs the chip
// payload from the current id; the setter writes the picked id back to model
// (and thus to the parent v-model). Selection state lives solely in model;
// selectedDocument is just a typed lens onto it.
const selectedDocument = computed<Result | null>({
  get: () => (model.value ? { id: model.value } : null),
  set: (value) => {
    model.value = value?.id || ""
  },
})
const query = ref("")
// Treats parent-supplied progress like a soft (temporary) readonly: input remains
// focusable and selectable but cannot be edited or cleared while it's set.
// The Clear button visually appears but is disabled, distinguishing this
// state from the harder readonly prop where Clear is hidden entirely.
const isInProgress = computed(() => props.progress > 0)
const searchResults = ref<Result[]>([])

// Toggles between the two "selected" visual states: false shows the chip,
// true shows the real combobox input so the user can search for a different
// document. The previously typed query is preserved across these toggles.
const editMode = ref(false)

const wrapperRef = useTemplateRef<HTMLElement>("wrapperRef")
const comboboxInputRef = useTemplateRef<ComponentPublicInstance>("comboboxInputRef")

const mainAbortController = new AbortController()
let searchAbortController = new AbortController()

async function search(q: string) {
  const signal = anySignal(mainAbortController.signal, searchAbortController.signal)

  if (signal.aborted) {
    return
  }

  // Add wildcard for prefix search if query ends with unicode letter or number.
  if (WILDCARD_SEARCH_REGEX.test(q)) {
    q = q + "*"
  }

  searchProgress.value += 1
  try {
    const response = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      {
        query: q,
      },
      signal,
      searchProgress,
    )
    if (signal.aborted) {
      return
    }

    // We use only the first 100 results.
    searchResults.value = response.slice(0, 100)
  } catch (err) {
    if (signal.aborted) {
      return
    }
    // TODO: Show error.
    console.error("InputRef.search", err)
  } finally {
    searchProgress.value -= 1
  }
}

watch(
  query,
  async (value) => {
    searchAbortController.abort()
    searchAbortController = new AbortController()
    await search(value)
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  searchAbortController.abort()
  mainAbortController.abort()
})

async function enterEditMode() {
  // The chip uses aria-readonly (no native disabled attribute) so it stays
  // focusable for keyboard navigation and text selection. That means
  // click/focus events still fire even when conceptually "disabled", so
  // the gate has to live here rather than in markup.
  if (props.readonly || isInProgress.value) return
  if (editMode.value) return
  editMode.value = true
  // Wait for the ComboboxInput to render, then focus its underlying input.
  await nextTick()
  const el = comboboxInputRef.value?.$el as HTMLInputElement | undefined
  el?.focus()
}

function exitEditMode() {
  editMode.value = false
}

// Wired to focusout on a real DOM wrapper (not the Combobox component
// itself, whose Headless UI root is a Vue Fragment that does not reliably
// dispatch attribute-attached focusout listeners). Catches Tab-out (or any
// focus loss) from any inner focusable: the input, the open-link, or the
// Clear button. The nextTick wait is essential: when entering edit mode the
// chip unmounts and the new input gets focused programmatically, and Vue's
// re-render is one tick behind the synchronous focusout. Without the tick,
// document.activeElement is still body and we would prematurely flip back
// out of edit mode. Selection is preserved either way. The user gets the
// chip back with the same document still picked.
async function onWrapperFocusout() {
  await nextTick()
  const wrapper = wrapperRef.value
  if (!wrapper) return
  if (wrapper.contains(document.activeElement)) {
    return
  }
  exitEditMode()
}

function onSelect(value: Result | null) {
  editMode.value = false
  // Once the user has committed to a selection there's no point finishing
  // the in-flight search for the prior query.
  searchAbortController.abort()
  // Intentionally not resetting query here: keeping it preserves the user's
  // last typed text, which is restored via display-value the next time edit
  // mode is entered. Query is only reset on explicit clear, see below.
  selectedDocument.value = value
}

function clearSelection() {
  query.value = ""
  model.value = ""
  exitEditMode()
}

// HUI 1.7.x intentionally disabled its "immediate" prop in the context
// (immediate: computed(() => false)), so ":immediate="true"" on Combobox does
// nothing. To still get "open the dropdown on focus" behavior, we dispatch a
// synthetic ArrowDown keydown on the input when it gains focus and the
// dropdown is closed. HUI's keydown handler sees ArrowDown with state===1
// (closed) and runs openCombobox() itself.
// This workaround has a downside: when "immediate" is honored properly,
// HUI's onFocus path also calls setActivationTrigger(1), which makes its
// Tab handler skip selectActiveOption(). Our ArrowDown dispatch only opens
// the combobox; it leaves activationTrigger at the default value of 2
// ("Other"). Combined with HUI's openCombobox() setting its internal "just
// opened" flag to true (which makes activeOptionIndex computed auto-active
// the first option when no option has been manually navigated to), the
// first Tab after a focus-open commits that first option via selectActiveOption()
// and the chip mounts; the user then needs a second Tab to actually move forward.
// Typing or arrow-key navigation does not suffer from this because typing's
// pending search empties the options momentarily and arrowing resets "just opened"
// flag to false, in both cases leaving activeOptionIndex null at Tab time so
// selectActiveOption() is a no-op.
// TODO: Remove this workaround when a never version is released.
//       See: https://github.com/tailwindlabs/headlessui/issues/3862
function onInputFocus(open: boolean, event: FocusEvent) {
  if (open) return
  const inputEl = event.target as HTMLInputElement | null
  inputEl?.dispatchEvent(
    new KeyboardEvent("keydown", {
      key: "ArrowDown",
      code: "ArrowDown",
      bubbles: true,
      cancelable: true,
    }),
  )
}

const WithPeerDBDocument = WithDocument<D>
</script>

<template>
  <div ref="wrapperRef" @focusout="onWrapperFocusout">
    <Combobox v-slot="{ open }" :model-value="selectedDocument" as="div" @update:model-value="onSelect">
      <!--
        Grid with a single minmax(0, 1fr) column. The "0" min track size
        propagates a min-content of 0 up through the flex ancestors, so the
        whole input chain can shrink and the chip's truncate actually clips
        long display labels instead of forcing the input to grow.

        The icon stack, progress bar, and dropdown are all position:absolute,
        so they do not contribute to flow height; the only flow child of this
        grid is the chip/input, which means the container's height tracks the
        input's height exactly.
      -->
      <div class="relative grid w-full grid-cols-[minmax(0,_1fr)]">
        <!--
          Selected + not editing: render the display label inside a
          contenteditable div styled to look like a text input. The
          contenteditable gives us a real blinking caret on focus, while
          @beforeinput / @paste / @drop prevention keeps the content
          immutable. aria-readonly conveys the readonly semantic without the
          native disabled attribute, so the element stays focusable and its
          text remains selectable.

          truncate on the chip itself is what actually clips overflowing
          labels. With the grid track above, the chip is constrained and
          truncate clips with an ellipsis.

          pr-29 reserves space for the open-link icon + Clear button stack
          on the right; pr-9 is the narrower variant for readonly mode where
          Clear is hidden, leaving only the open-link icon.
        -->
        <WithPeerDBDocument v-if="selectedDocument?.id && !editMode" :id="selectedDocument.id" name="DocumentGet">
          <template #default="{ doc }">
            <InputStyled
              as="div"
              role="textbox"
              contenteditable="true"
              :inactive="readonly || isInProgress"
              :aria-readonly="readonly || isInProgress || undefined"
              class="w-full truncate"
              :class="readonly ? 'pr-9' : 'pr-29'"
              @click="enterEditMode"
              @focus="enterEditMode"
              @beforeinput.prevent
              @paste.prevent
              @drop.prevent
            >
              <DisplayLabel :doc="doc" />
            </InputStyled>
          </template>
        </WithPeerDBDocument>

        <!--
          Either no selection yet, or the user is re-editing a selection.
          display-value returns the local query so the input is pre-filled
          with the previously typed text on re-entry to edit mode.

          readonly + isInProgress both make the input look and behave like
          a read-only field. Searching keeps the input editable so the user
          can keep refining the query; only the inline progress bar
          communicates the in-flight state. The pr-23/pr-9 split mirrors
          the chip's: full padding when Clear is visible, narrow when only
          the chevron is shown.
        -->
        <InputStyled
          v-else
          ref="comboboxInputRef"
          :as="ComboboxInput"
          :inactive="readonly || isInProgress"
          :readonly="readonly || isInProgress"
          v-bind="$attrs"
          class="w-full"
          :class="{
            'pr-23': selectedDocument?.id && !readonly,
            'pr-9': !selectedDocument?.id || readonly,
          }"
          :display-value="() => query"
          @input="query = ($event.target as HTMLInputElement).value"
          @focus="onInputFocus(open, $event)"
          @keydown.escape="exitEditMode"
        />

        <!--
          Right-side icon stack absolutely positioned within the grid
          container.

          When a document is selected we show an open-link (only when not in
          edit mode, since the chip-pretty representation is gone during a
          search) and a Clear button. Clear is hidden by readonly entirely
          but only disabled by isInProgress, matching the distinction between
          "user cannot change the value" and "user can change it but not
          right now".

          With no selection, the chevron ComboboxButton is shown; clicking
          it toggles the dropdown. Cursor-not-allowed when readonly or
          in-progress, to match the input's own disabled-ish look.
        -->
        <div class="absolute inset-y-0 right-0 flex items-center gap-1 pr-2">
          <template v-if="selectedDocument?.id">
            <RouterLink v-if="!editMode" :to="{ name: 'DocumentGet', params: { id: selectedDocument.id } }" class="link">
              <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
            </RouterLink>
            <Button v-if="!readonly" type="button" :disabled="isInProgress" class="px-2 py-0.5" @click.prevent="clearSelection">{{ t("common.buttons.clear") }}</Button>
          </template>
          <ComboboxButton v-else class="inline-flex items-center">
            <ChevronUpDownIcon
              class="size-5 text-gray-400"
              :class="{
                'cursor-not-allowed': readonly || isInProgress,
              }"
              aria-hidden="true"
            />
          </ComboboxButton>
        </div>

        <!--
          Indeterminate progress bar bound only to searchProgress, not to
          props.progress. Parent-level progress has its own UI in the parent;
          this bar exists solely to indicate that the inline search is in
          flight.
        -->
        <ProgressBar :progress="searchProgress" class="absolute inset-x-0 bottom-0 rounded-b" />

        <!--
          Visibility is driven by Headless UI's own "open" slot prop,
          exposed via v-slot on Combobox. The chevron toggles it via HUI's
          built-in ComboboxButton onClick, typing into the input opens it
          via HUI's onInput, and HUI's blur logic closes it on
          click-outside. Auto-open on focus is achieved by dispatching a
          synthetic ArrowDown keydown from onInputFocus (see script), since
          HUI 1.7.x's ":immediate" prop is intentionally disabled at the
          library level.

          top-full anchors the dropdown to the bottom of the grid container
          rather than its top-left corner. In a relative block parent, an
          absolutely-positioned descendant without explicit positioning would
          fall to its "static position" after the input in flow; in a grid
          container the default for absolute descendants is the grid's
          top-left padding edge, so the explicit top: 100% is necessary for
          the "below the input" placement.
        -->
        <ComboboxOptions
          v-if="open && !isInProgress && !readonly"
          static
          class="absolute top-full z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <ComboboxOption v-if="searchResults.length === 0">
            <li class="p-2"
              ><i>{{ t("partials.input.InputRef.noResults") }}</i></li
            >
          </ComboboxOption>

          <template v-if="searchResults.length > 0">
            <WithPeerDBDocument v-for="result in searchResults" :id="result.id" :key="result.id" name="DocumentGet">
              <template #default="{ doc }">
                <ComboboxOption v-slot="{ active }" :value="result" as="template">
                  <li class="p-1 outline-none select-none">
                    <!--
                      We have an additional div so that the ring has the space to be shown.
                      li element has p-1 for ring space, together with py-1 and px-2 we get the effective padding
                      for option content of py-2 and px-3, same what InputText and ListboxButton have.
                    -->
                    <div class="flex flex-row items-center justify-between rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                      <div
                        class="w-full cursor-pointer truncate"
                        :class="{
                          'font-medium': result.id === selectedDocument?.id,
                        }"
                      >
                        <DisplayLabel :doc="doc" />
                      </div>

                      <CheckIcon v-if="result.id === selectedDocument?.id" class="mr-2 size-5 text-primary-600" aria-hidden="true" />

                      <!--
                        HUI's ComboboxOption listens on mousedown (not click) to trigger
                        selection, so @mousedown.stop prevents the option from being picked
                        when the user actually wanted to open the link. The click event is
                        independent and still fires, letting RouterLink navigate normally.
                      -->
                      <RouterLink v-if="result?.id" :to="{ name: 'DocumentGet', params: { id: result.id } }" class="link" @mousedown.stop>
                        <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
                      </RouterLink>
                    </div>
                  </li>
                </ComboboxOption>
              </template>
              <template #loading="{ url }">
                <li class="p-1 outline-none select-none">
                  <i class="h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></i>
                </li>
              </template>
            </WithPeerDBDocument>
          </template>
        </ComboboxOptions>
      </div>
    </Combobox>
  </div>
</template>
