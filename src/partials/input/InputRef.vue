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

We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts">
import type { D } from "@/document"
import type { Result, ValidationError, ValidatorFn } from "@/types"
import type { ComponentPublicInstance } from "vue"

import { Combobox, ComboboxButton, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { ArrowTopRightOnSquareIcon, CheckIcon, ChevronUpDownIcon } from "@heroicons/vue/20/solid"
import { Identifier } from "@tozd/identifier"
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { postJSON } from "@/api"
import Button from "@/components/Button.vue"
import InputStyled from "@/components/InputStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import WithDocument from "@/components/WithDocument.vue"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { useLock } from "@/progress"
import { anySignal, loadingWidth } from "@/utils"
import { useValidation } from "@/validation"

// Wildcard to see if a string ends with unicode letter or number.
const WILDCARD_SEARCH_REGEX = /[\p{L}\p{N}]$/u

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

const invalid = computed(() => props.invalid || errors.value.length > 0)

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
// Data modification and controls; useValidation writes to this lock during
// validation. An active enclosing lock (either inherited from a parent
// useLock or contributed locally) behaves like a soft, temporary readonly:
// input remains focusable and selectable but cannot be edited or cleared.
// The Clear button visually appears but is disabled, distinguishing this
// state from the harder readonly prop where Clear is hidden entirely.
const lock = useLock()
const inactive = computed(() => lock.value > 0 || props.readonly)
const searchResults = ref<Result[]>([])

// Toggles between the two "selected" visual states: false shows the chip,
// true shows the real combobox input so the user can search for a different
// document. The previously typed query is preserved across these toggles.
const editMode = ref(false)

const wrapperRef = useTemplateRef<HTMLElement>("wrapperRef")
const comboboxInputRef = useTemplateRef<ComponentPublicInstance>("comboboxInputRef")

// A reference is invalid if it is empty (when required) or does not parse as
// a valid document identifier. The required check is skipped on initial (no
// user interaction yet), but the identifier-shape check is not - a
// pre-populated value that is not a valid identifier should surface
// immediately.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  if (value === "") {
    if (!props.required || options.initial) {
      return []
    }
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  if (!Identifier.valid(value)) {
    // TODO: Use standard codes.
    return [{ code: "invalid" }]
  }
  return []
}

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  lock,
  () => validator,
  // Focus target: whichever of the two visual states is currently mounted.
  // role="textbox" is on the contenteditable chip when a doc is selected;
  // role="combobox" is on the HUI ComboboxInput when no doc is selected
  // (or the user is re-editing). Validation cares about the latter (the
  // only failing case is required+empty where the chip is not shown), but
  // programmatic focus moves (focusFirstInput on edit) hit the chip.
  () => wrapperRef.value?.querySelector<HTMLElement>('[role="textbox"], [role="combobox"]') ?? null,
  () => {
    query.value = ""
    model.value = ""
    errors.value = []
    exitEditMode()
  },
)

defineExpose(validatedInput)

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
  if (inactive.value) return
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
  if (wrapperRef.value?.contains(document.activeElement)) {
    return
  }
  exitEditMode()
  // Focus has actually left the component (not just moved between its inner
  // focusables). Run lazy validation now so the required error appears as
  // soon as the user tabs/clicks away from an empty required field.
  await runValidation()
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

const WithPeerDBDocument = WithDocument<D>
</script>

<template>
  <div ref="wrapperRef" @focusout="onWrapperFocusout">
    <Combobox v-slot="{ open }" :model-value="selectedDocument" as="div" :immediate="true" by="id" @update:model-value="onSelect">
      <!--
        Grid with a single minmax(0,1fr) column. The "0" min track size
        propagates a min-content of 0 up through the flex ancestors, so the
        whole input chain can shrink and the chip's truncate actually clips
        long display labels instead of forcing the input to grow.

        The icon stack, progress bar, and dropdown are all position:absolute,
        so they do not contribute to flow height; the only flow child of this
        grid is the chip/input, which means the container's height tracks the
        input's height exactly.
      -->
      <div class="relative grid w-full grid-cols-[minmax(0,1fr)]">
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
        <!--
          Invalid value (non-empty + validation failed): do not attempt to
          load the doc; show the red "invalid value" placeholder inside the
          chip. The chip retains its click/focus to enter edit mode so the
          user can search for a new doc, and the right-side Clear button
          still works. Combined with the selectedDocument?.id guard,
          invalid here can only mean the value failed the identifier-shape
          check (the required check only fires on empty).
        -->
        <InputStyled
          v-if="selectedDocument?.id && !editMode && invalid"
          as="div"
          role="textbox"
          contenteditable="true"
          :inactive="inactive"
          :invalid="invalid"
          :aria-readonly="inactive || undefined"
          :aria-invalid="invalid || undefined"
          class="w-full truncate"
          :class="readonly ? '' : 'pr-23'"
          @click="enterEditMode"
          @focus="enterEditMode"
          @beforeinput.prevent
          @paste.prevent
          @drop.prevent
        >
          <i class="text-error-600">{{ t("partials.input.InputRef.invalidValue") }}</i>
        </InputStyled>

        <!--
          One stable chip wrapper across the default / loading / error doc
          states (WithPeerDBDocument swaps only its inner slot content). A
          stable wrapper keeps role="textbox" continuously present so
          focusFirstInput can land on the chip even while the doc is still
          fetching, and keeps focus across the slot transitions.
        -->
        <InputStyled
          v-else-if="selectedDocument?.id && !editMode"
          as="div"
          role="textbox"
          contenteditable="true"
          :inactive="inactive"
          :invalid="invalid"
          :aria-readonly="inactive || undefined"
          :aria-invalid="invalid || undefined"
          class="w-full truncate"
          :class="readonly ? 'pr-9' : 'pr-29'"
          @click="enterEditMode"
          @focus="enterEditMode"
          @beforeinput.prevent
          @paste.prevent
          @drop.prevent
        >
          <WithPeerDBDocument :id="selectedDocument.id" name="DocumentGet">
            <template #default="{ doc }">
              <DisplayLabel :doc="doc" />
            </template>
            <template #error="{ url }">
              <i class="pd-withdocument-error text-error-600" :data-url="url">{{ t("common.status.loadingDataFailed") }}</i>
            </template>
            <template #loading="{ url }">
              <i class="block h-4 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(selectedDocument?.id ?? '')]"></i>
            </template>
          </WithPeerDBDocument>
        </InputStyled>

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
          :inactive="inactive"
          :invalid="invalid"
          :readonly="inactive"
          :aria-invalid="invalid || undefined"
          v-bind="$attrs"
          class="w-full"
          :class="{
            'pr-23': selectedDocument?.id && !readonly,
            'pr-9': !selectedDocument?.id || readonly,
          }"
          :display-value="() => query"
          @input="query = ($event.target as HTMLInputElement).value"
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
            <RouterLink v-if="!editMode && !invalid" :to="{ name: 'DocumentGet', params: { id: selectedDocument.id } }" class="link">
              <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
            </RouterLink>
            <Button v-if="!readonly" type="button" class="px-2.5 py-1" @click.prevent="clearSelection">{{ t("common.buttons.clear") }}</Button>
          </template>
          <ComboboxButton v-else class="inline-flex items-center">
            <ChevronUpDownIcon
              class="size-5 text-gray-400"
              :class="{
                'cursor-not-allowed': inactive,
              }"
              aria-hidden="true"
            />
          </ComboboxButton>
        </div>

        <!--
          Indeterminate progress bar bound only to searchProgress.
          Parent-level loading has its own UI in the parent; this bar exists
          solely to indicate that the inline search is in flight.
        -->
        <ProgressBar :progress="searchProgress" class="absolute inset-x-0 bottom-0 rounded-b" />

        <!--
          Visibility is driven by Headless UI's own "open" slot prop,
          exposed via v-slot on Combobox. The chevron toggles it via HUI's
          built-in ComboboxButton onClick, typing into the input opens it
          via HUI's onInput, and HUI's blur logic closes it on
          click-outside. Auto-open on focus is achieved using ":immediate"
          prop.

          top-full anchors the dropdown to the bottom of the grid container
          rather than its top-left corner. In a relative block parent, an
          absolutely-positioned descendant without explicit positioning would
          fall to its "static position" after the input in flow; in a grid
          container the default for absolute descendants is the grid's
          top-left padding edge, so the explicit top: 100% is necessary for
          the "below the input" placement.
        -->
        <ComboboxOptions
          v-if="open && !inactive"
          static
          class="absolute top-full z-10 mt-1 max-h-40 w-full overflow-auto rounded-sm bg-white shadow-sm ring-2 ring-neutral-300 outline-none"
        >
          <ComboboxOption v-if="searchResults.length === 0">
            <li class="p-2"
              ><i>{{ t("partials.input.InputRef.noResults") }}</i></li
            >
          </ComboboxOption>

          <template v-if="searchResults.length > 0">
            <!--
              ComboboxOption is the outer wrapper so that rows register with
              HUI as soon as searchResults arrives, independent of the per-row
              doc fetch. That lets the user arrow-navigate and pick a row
              whose document is still loading, and keeps the active ring and
              the selected-check icon consistent across the loading, error,
              and loaded slot variants below.
            -->
            <ComboboxOption v-for="result in searchResults" :key="result.id" v-slot="{ active }" :value="result" as="template">
              <li class="p-1 outline-none select-none">
                <!--
                  We have an additional div so that the ring has the space to be shown.
                  li element has p-1 for ring space, together with py-1 and px-2 we get the effective padding
                  for option content of py-2 and px-3, same what InputText and ListboxButton have.
                -->
                <div class="flex flex-row items-center justify-between rounded-sm px-2 py-1" :class="active ? 'ring-2 ring-primary-500' : ''">
                  <WithPeerDBDocument :id="result.id" name="DocumentGet">
                    <template #default="{ doc }">
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
                      <RouterLink :to="{ name: 'DocumentGet', params: { id: result.id } }" class="link" @mousedown.stop>
                        <ArrowTopRightOnSquareIcon class="size-5" aria-hidden="true" />
                      </RouterLink>
                    </template>
                    <template #loading="{ url }">
                      <div class="w-full">
                        <i class="block h-4 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(result.id)]"></i>
                      </div>

                      <CheckIcon v-if="result.id === selectedDocument?.id" class="size-5 text-primary-600" aria-hidden="true" />
                    </template>
                    <template #error="{ url }">
                      <div class="w-full truncate">
                        <i class="pd-withdocument-error text-error-600" :data-url="url">{{ t("common.status.loadingDataFailed") }}</i>
                      </div>

                      <CheckIcon v-if="result.id === selectedDocument?.id" class="size-5 text-primary-600" aria-hidden="true" />
                    </template>
                  </WithPeerDBDocument>
                </div>
              </li>
            </ComboboxOption>
          </template>
        </ComboboxOptions>
      </div>
    </Combobox>
  </div>
</template>
