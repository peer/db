<!--
InputFile uploads a selected (picked or dropped) file and
exposes the resulting StorageGet URL through v-model.

States:
  - Empty (v-model = ""): a single large non-primary button is shown which
    accepts both a click (opens the native file picker) and a file drop.
    During upload the button's progress bar is enabled.
  - Uploaded (v-model set): the button is replaced with a styled display of
    the uploaded file (a mock LinkClaim rendered via ClaimValue) and a Clear
    button.
-->

<script setup lang="ts">
import type { ValidationError, ValidatorFn } from "@/types"
import type { ComponentPublicInstance } from "vue"

import { Identifier } from "@tozd/identifier"
import { computed, onBeforeUnmount, ref, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { CAN_EDIT_FILES, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import InputStyled from "@/components/InputStyled.vue"
import WithLock from "@/components/WithLock.vue"
import { HighConfidence, LinkClaim } from "@/document"
import { classifyLink, LINK_CLASS_FILE } from "@/internal-links"
import ClaimValue from "@/partials/ClaimValue.vue"
import { getParentLock, useLock } from "@/progress"
import { uploadFile } from "@/upload"
import { useValidation } from "@/validation"

// Multi-root template, so we route fall-through attrs explicitly onto
// whichever element is visibly rendered.
defineOptions({
  inheritAttrs: false,
})

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

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Local upload progress, intentionally not stacked on the parent's progress
// chain (i.e. not useProgress()). An upload in flight should only drive the
// inline progress bar on this input's button, not the parent component's
// progress bar or top-level progress UI.
const progress = ref(0)
const total = ref<number | undefined>(undefined)

// Data modification and controls; useValidation writes to this lock during
// validation so the button locks itself while a validator is in flight.
const lock = useLock()
const inactive = computed(() => lock.value > 0 || props.readonly)

// We re-provide that bare parentLock via WithLock around the cancel button
// to keep the button interactive regardless of our own count.
const parentLock = getParentLock()
function getParentLockRef() {
  return parentLock
}

const fileInputEl = useTemplateRef<HTMLInputElement>("fileInputEl")
const browseButtonRef = useTemplateRef<ComponentPublicInstance>("browseButtonRef")
const isDragOver = ref(false)

// A file value is invalid if it is empty (when required) or does not resolve
// through the Vue router to a StorageGet route, i.e. classifyLink does not
// stamp it with LINK_CLASS_FILE. The required check is skipped on initial
// (no user interaction yet), but the file-route check is not - a
// pre-populated value pointing at something that is not a file should
// surface immediately. While an upload is in flight the model is "" but
// the user has already provided a file, so we treat that window as
// "value incoming" and skip the required check. A watcher on the upload
// state re-runs validation when the upload settles.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  if (value === "") {
    if (progress.value !== 0) {
      return []
    }
    if (!props.required || options.initial) {
      return []
    }
    // TODO: Use standard codes.
    return [{ code: "required" }]
  }
  if (!classifyLink(value, router).includes(LINK_CLASS_FILE)) {
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
  // The empty-state Button is the focus target. When required+empty
  // (the only failing case) the v-else Button is rendered and its $el is
  // the underlying <button>.
  () => (browseButtonRef.value?.$el as HTMLElement | null) ?? null,
  () => {
    model.value = ""
    errors.value = []
    if (fileInputEl.value) {
      fileInputEl.value.value = ""
    }
  },
)

defineExpose(validatedInput)

// Re-validate on the upload-in-flight transitions. We watch the boolean
// rather than progress itself so the watcher fires on start (0 -> non-zero)
// and end (non-zero -> 0) but not on every intermediate progress tick.
// Start: clears a stale required error now that the user has picked a
// file. End: re-evaluates against the post-upload state, which surfaces
// the required error again if the upload failed and left model empty.
watch(
  () => progress.value !== 0,
  async () => {
    await runValidation()
  },
)

// Set right before .click() on the hidden file input; consumed by the next
// blur on the browse Button. Clicking the Button to open the native picker
// can cause the browser to dispatch a blur on the trigger (Chrome does this
// when the picker takes focus), and we do not want that synthetic blur to
// fire validation while the user is actively in the middle of providing a
// value. The flag is also cleared on re-focus so it can never outlive its
// purpose in browsers that keep focus on the Button during the picker.
let openingPicker = false

// Run lazy validation when focus leaves either of the visible elements (the
// browse Button in empty state, or the Clear Button in uploaded state) so
// the required error appears as soon as the user tabs/clicks away. Skip
// the one blur caused by opening the file picker.
async function onBlur() {
  if (openingPicker) {
    openingPicker = false
    return
  }
  await runValidation()
}

function onBrowseFocus() {
  openingPicker = false
}

// AbortController for the currently active upload, or null when idle.
// Recreated per upload because once aborted an AbortController cannot
// be reused.
let abortController: AbortController | null = null
onBeforeUnmount(() => {
  abortController?.abort()
})

// Mock claim rendered by ClaimValue once a file has been uploaded.
// TODO: Store filename as sub-claim.
// TODO: Return claim as a whole from the component.
const mockClaim = computed<LinkClaim | null>(() => {
  if (!model.value) {
    return null
  }
  return new LinkClaim({
    id: Identifier.new().toString(),
    confidence: HighConfidence,
    prop: { id: Identifier.new().toString() },
    iri: model.value,
  })
})

async function onUpload(file: File) {
  // Handling of progress here is slightly different from the rest of the codebase
  // and we set it explicitly to 1 instead of increasing it (and in finally then to 0).
  // This is because uploadFile manages progress and total specially.
  if (!hasPermission(CAN_EDIT_FILES)) return
  if (progress.value !== 0) {
    throw new Error("upload already in progress")
  }
  // Setting progress to 1 shows the intermediate progress bar.
  progress.value = 1
  total.value = undefined
  // Lock the button via the local useLock boundary.
  lock.value += 1
  abortController = new AbortController()
  try {
    const fileId = await uploadFile(router, file, abortController.signal, progress, total)
    if (abortController.signal.aborted) {
      return
    }
    model.value = router.resolve({ name: "StorageGet", params: { id: fileId } }).href
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("InputFile.onUpload", err)
  } finally {
    progress.value = 0
    total.value = undefined
    lock.value -= 1
    abortController = null
  }
}

// Cancel the in-flight upload. Safe to call when no upload
// is running - it is a no-op then.
function onCancel() {
  abortController?.abort()
}

async function onFileInputChange() {
  const file = fileInputEl.value?.files?.[0]
  if (!file) {
    return
  }
  await onUpload(file)
}

function onBrowse() {
  if (inactive.value) return
  openingPicker = true
  fileInputEl.value?.click()
}

function onClear() {
  if (inactive.value) return
  model.value = ""
  if (fileInputEl.value) {
    fileInputEl.value.value = ""
  }
}

function onDragOver() {
  if (inactive.value) return
  isDragOver.value = true
}

function onDragLeave() {
  isDragOver.value = false
}

async function onDrop(e: DragEvent) {
  if (inactive.value) return
  isDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) {
    return
  }
  await onUpload(file)
}
</script>

<template>
  <input ref="fileInputEl" type="file" class="hidden" @change="onFileInputChange" />
  <!--
    Grid wrapper with a single minmax(0,1fr) column so that long display labels
    actually clip with truncate.
  -->
  <div v-if="model" v-tw-merge v-bind="$attrs" :aria-invalid="invalid || undefined" class="pd-inputfile relative grid w-full grid-cols-[minmax(0,1fr)]">
    <!--
      pr-23 reserves space on the right for the Clear button overlay so
      the display label does not slide underneath it.
    -->
    <InputStyled as="div" :inactive="inactive" :invalid="invalid" class="w-full truncate" :class="readonly ? '' : 'pr-23'">
      <!--
        When the current value fails validation (e.g. it is not a route that
        classifies as a file link), rendering ClaimValue/Link could resolve
        the bad URL through the SPA router and produce a misleading link.
        Show the raw value instead so the user can see exactly what is wrong.
      -->
      <template v-if="invalid">{{ model }}</template>
      <ClaimValue v-else :claim="mockClaim" type="link" />
    </InputStyled>
    <div v-if="!readonly" class="absolute inset-y-0 right-0 flex items-center pr-2">
      <Button type="button" class="px-2.5 py-1" @click.prevent="onClear" @blur="onBlur">{{ t("common.buttons.clear") }}</Button>
    </div>
  </div>
  <div v-else-if="!hasPermission(CAN_EDIT_FILES)" v-tw-merge v-bind="$attrs" class="pd-inputfile text-gray-500 italic">{{
    t("partials.input.InputFile.noPermission")
  }}</div>
  <div v-else v-tw-merge v-bind="$attrs" class="pd-inputfile flex w-full flex-row gap-2">
    <Button
      ref="browseButtonRef"
      type="button"
      class="min-w-0 flex-1"
      :progress="progress"
      :total="total"
      :active="isDragOver"
      :disabled="readonly"
      :invalid="invalid"
      :aria-invalid="invalid || undefined"
      @click.prevent="onBrowse"
      @dragover.prevent="onDragOver"
      @dragenter.prevent="onDragOver"
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
      @focus="onBrowseFocus"
      @blur="onBlur"
      >{{ t("partials.input.InputFile.dropOrBrowse") }}</Button
    >
    <WithLock :lock="getParentLockRef">
      <Button v-if="progress !== 0" type="button" class="shrink-0" @click.prevent="onCancel">{{ t("common.buttons.cancel") }}</Button>
    </WithLock>
  </div>
</template>
