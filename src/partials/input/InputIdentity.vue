<!--
InputIdentity names one user of the site by their subject, which it validates by looking the user up
through the user API (see UserGetAPI in user.go): a subject the site cannot describe is not a user to
name. It is how a user is named where they cannot be picked from a list.

It has two visual states:

  1. Editing (and whenever there is no user): a text input holding the subject as typed.
  2. Named, not editing: the input area is replaced by a "chip" naming the user (their username when
     the site knows it, see IdentityInline). Clicking or tabbing into the chip returns to the input,
     with the subject in it again.

The Clear button on the right is there while the input holds the user it named: in the chip always, and
in the input until the subject in it is changed, because a changed subject is on its way to naming
somebody else and there is no way back to the named user (unlike InputRef, where Escape returns to the
selected document).

The typed subject becomes the value when focus leaves the input, and that is when the user is looked
up: the value is a subject somebody was named by, and not every keystroke on the way to it.

The chip is a contenteditable div (not a button) so that it is in the tab order, and so that it reads
like a native <input readonly> while there is nothing to switch to: focusing it keeps it when the input
is readonly or locked, with a real blinking caret and selectable text. Otherwise focusing it switches to
the input, so the caret is never seen there. Real edits are blocked (@beforeinput.prevent etc.), so the
chip is read-only despite contenteditable=true.
-->

<script setup lang="ts">
import type { ComponentPublicInstance } from "vue"

import type { Identity, ValidationError, ValidatorFn } from "@/types"

// We use v-model-text directive to mirror what Vue does on native <input> elements which we have to do
// ourselves because we use <input> element through InputStyled component.
import { computed, nextTick, onBeforeUnmount, ref, useTemplateRef, vModelText, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import Button from "@/components/Button.vue"
import InputStyled from "@/components/InputStyled.vue"
import ProgressBar from "@/components/ProgressBar.vue"
import IdentityInline from "@/partials/IdentityInline.vue"
import { useLock } from "@/progress"
import { useValidation } from "@/validation"

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

// Whether the value is a user the site described when it was last looked up (see the validator). It is
// what the chip names: a subject which resolved to nobody is text to fix and not a user to keep, so it
// stays in the input.
const resolved = computed(() => model.value !== "" && errors.value.length === 0)

// Whether what the input holds is still the user it resolved to. Typing anything else leaves nothing to
// clear: the named user is on the way out and there is no way back to them (unlike InputRef, where
// Escape returns to the selected document), so Clear goes as soon as the subject is changed.
const clearable = computed(() => resolved.value && text.value === model.value)

// We want all fallthrough attributes to be passed to the input element.
defineOptions({
  inheritAttrs: false,
})

const router = useRouter()
const { t } = useI18n({ useScope: "global" })

// The subject as typed, which becomes the value when focus leaves the input. It follows the value
// whenever that changes from the outside (a reset, or a parent writing it), so the input shows what
// the value is.
const text = ref(model.value)
watch(model, (value) => {
  text.value = value
})

// Local lookup progress, intentionally not stacked on the parent's progress chain (i.e. not
// useProgress()), like the search of InputRef: looking the user up should drive the inline progress
// bar under this input alone.
const lookupProgress = ref(0)

// Data modification and controls; useValidation writes to this lock during validation. An active
// enclosing lock behaves like a soft, temporary readonly: the input remains focusable but cannot be
// edited or cleared. The Clear button then visually appears but is disabled, which the harder readonly
// prop distinguishes by hiding it entirely.
const lock = useLock()
const inactive = computed(() => lock.value > 0 || props.readonly)

// Toggles between the two visual states: false shows the chip naming the user, true shows the input so
// the subject can be typed. There is nothing to show a chip for while the value is empty.
const editMode = ref(false)

const wrapperRef = useTemplateRef<HTMLElement>("wrapperRef")
const inputStyledRef = useTemplateRef<ComponentPublicInstance>("inputStyledRef")

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

// A subject is invalid when the site cannot describe the user it names, which the lookup tells, and
// empty is invalid only when the input is required. The required check is skipped on initial (no user
// interaction yet) and while eager, so it flags on the lazy blur pass and never mid-edit. The lookup
// runs on every other pass, eager ones included: the value changes only when focus leaves the input
// (see onWrapperFocusout), so there is no lookup per keystroke to avoid.
const validator: ValidatorFn<string> = async function (value, options) {
  if (value === "") {
    if (props.required && !options.initial && !options.eager) {
      // TODO: Use standard codes.
      return [{ code: "required" }]
    }
    return []
  }
  lookupProgress.value += 1
  try {
    await getURL<Identity>(router.apiResolve({ name: "UserGet", params: { id: value } }).href, null, options.signal, lookupProgress)
    return []
  } catch (err) {
    if (options.signal.aborted) {
      return []
    }
    // TODO: Use standard codes.
    return [{ code: "invalid", debugMessage: "identity lookup failed", debugError: err instanceof Error ? err : undefined }]
  } finally {
    lookupProgress.value -= 1
  }
}

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  lock,
  () => validator,
  // Focus target: whichever of the two visual states is currently mounted. role="textbox" is on the
  // contenteditable chip when a user is named, and the input itself otherwise.
  () => wrapperRef.value?.querySelector<HTMLElement>('input, [role="textbox"]') ?? null,
  () => {
    text.value = ""
    model.value = ""
    errors.value = []
    exitEditMode()
  },
)

defineExpose(validatedInput)

async function enterEditMode() {
  // The chip uses aria-readonly (no native disabled attribute) so it stays focusable for keyboard
  // navigation and text selection. That means click/focus events still fire even when conceptually
  // "disabled", so the gate has to live here rather than in markup.
  if (inactive.value) return
  if (editMode.value) return
  editMode.value = true
  // Wait for the input to render, then focus it.
  await nextTick()
  ;(inputStyledRef.value?.$el as HTMLInputElement | undefined)?.focus()
}

function exitEditMode() {
  editMode.value = false
}

// Held while clearSelection empties the value and re-focuses the input, so the focusout the Clear
// button dispatches as it unmounts does not run the required-validation: the user intentionally
// cleared the field.
let clearing = false

// Wired to focusout on the wrapper, so it catches focus leaving any of the inner focusables (the
// input, the chip, the Clear button). The nextTick wait is essential: entering edit mode unmounts the
// chip and focuses the new input programmatically, and Vue's re-render is one tick behind the
// synchronous focusout, so without the tick document.activeElement is still body and we would flip
// straight back out of edit mode.
//
// Leaving the input is what turns the typed subject into the value, and validating it is what looks
// the user up.
async function onWrapperFocusout() {
  await nextTick()
  if (abortController.signal.aborted) {
    return
  }
  if (clearing) {
    return
  }
  if (wrapperRef.value?.contains(document.activeElement)) {
    return
  }
  exitEditMode()
  const subject = text.value.trim()
  text.value = subject
  model.value = subject
  await runValidation()
}

async function clearSelection() {
  clearing = true
  text.value = ""
  model.value = ""
  errors.value = []
  await enterEditMode()
  // Focus the input so focus is not dropped to the body.
  await nextTick()
  ;(inputStyledRef.value?.$el as HTMLInputElement | undefined)?.focus()
  clearing = false
}
</script>

<template>
  <!--
    Default layout classes on the wrapper. inheritAttrs: false above means the parent's class does not
    auto-merge onto this root (fallthrough attrs target the input), so the wrapper needs explicit
    flex-item classes to stretch in a row-flex parent the way InputString et al. do.
  -->
  <div ref="wrapperRef" class="min-w-0 flex-auto grow" @focusout="onWrapperFocusout">
    <!--
      Grid with a single minmax(0,1fr) column. The "0" min track size propagates a min-content of 0 up
      through the flex ancestors, so the whole input chain can shrink and the chip's truncate actually
      clips a long name instead of forcing the input to grow.
    -->
    <div class="relative grid w-full grid-cols-[minmax(0,1fr)]">
      <!--
        A user is named and their subject is not being typed: the name inside a contenteditable div
        styled to look like a text input.

        pr-23 reserves space for the Clear button on the right; readonly mode has nothing to clear.
      -->
      <InputStyled
        v-if="resolved && !editMode"
        as="div"
        role="textbox"
        contenteditable="true"
        :inactive="inactive"
        :invalid="invalid"
        :aria-readonly="inactive || undefined"
        :aria-invalid="invalid || undefined"
        class="pd-inputidentity w-full truncate"
        :class="[readonly ? '' : 'pr-23', { 'pd-locked': lock > 0 }]"
        @click="enterEditMode"
        @focus="enterEditMode"
        @beforeinput.prevent
        @paste.prevent
        @drop.prevent
      >
        <IdentityInline :subject="model" />
      </InputStyled>

      <!-- No user named yet, their subject is being typed, or what was typed named nobody. -->
      <InputStyled
        v-else
        ref="inputStyledRef"
        v-model-text="text"
        as="input"
        type="text"
        :inactive="inactive"
        :invalid="invalid"
        :readonly="inactive"
        :aria-invalid="invalid || undefined"
        v-bind="$attrs"
        class="pd-inputidentity w-full"
        :class="[clearable && !readonly ? 'pr-23' : 'pr-3', { 'pd-locked': lock > 0 }]"
        @update:model-value="text = $event"
      />

      <!--
        The Clear button, absolutely positioned within the grid container. It is there for a user the
        site named and while the input still holds them (see clearable), and not for text which named
        nobody: that is fixed by typing. It is hidden entirely by readonly, and only disabled while a
        lock is held, matching the distinction between "the user cannot change the value" and "they can
        change it but not right now".
      -->
      <div v-if="!readonly && clearable" class="absolute inset-y-0 right-0 flex items-center gap-1 pr-2">
        <Button type="button" class="px-2.5 py-1" @click.prevent="clearSelection">{{ t("common.buttons.clear") }}</Button>
      </div>

      <!-- Indeterminate progress bar bound only to the lookup, which is the only work this input does. -->
      <ProgressBar :progress="lookupProgress" class="absolute inset-x-0 bottom-0 rounded-b" />
    </div>
  </div>
</template>
