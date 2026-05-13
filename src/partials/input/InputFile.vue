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
import { Identifier } from "@tozd/identifier"
import { computed, onBeforeUnmount, ref, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import InputStyled from "@/components/InputStyled.vue"
import { HighConfidence, LinkClaim } from "@/document"
import ClaimValue from "@/partials/ClaimValue.vue"
import { useLock } from "@/progress"
import { uploadFile } from "@/upload"

const model = defineModel<string>({ default: "" })

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Local upload progress, intentionally not stacked on the parent's progress
// chain (i.e. not useProgress()). An upload in flight should only drive the
// inline progress bar on this input's button, not the parent component's
// progress bar or top-level progress UI.
const progress = ref(0)
const total = ref<number | undefined>(undefined)

// Data modification and controls.
const lock = useLock()

const fileInputEl = useTemplateRef<HTMLInputElement>("fileInputEl")
const isDragOver = ref(false)

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
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
  if (progress.value !== 0) {
    throw new Error("upload already in progress")
  }
  // Setting progress to 1 shows the intermediate progress bar.
  progress.value = 1
  total.value = undefined
  // Lock the button via the local useLock boundary.
  lock.value += 1
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
  }
}

async function onFileInputChange() {
  const file = fileInputEl.value?.files?.[0]
  if (!file) {
    return
  }
  await onUpload(file)
}

function onBrowse() {
  fileInputEl.value?.click()
}

function onClear() {
  model.value = ""
  if (fileInputEl.value) {
    fileInputEl.value.value = ""
  }
}

function onDragOver() {
  isDragOver.value = true
}

function onDragLeave() {
  isDragOver.value = false
}

async function onDrop(e: DragEvent) {
  isDragOver.value = false
  const file = e.dataTransfer?.files?.[0]
  if (!file) {
    return
  }
  await onUpload(file)
}
</script>

<template>
  <div class="pd-inputfile flex w-full flex-row gap-x-1 sm:gap-x-4">
    <input ref="fileInputEl" type="file" class="hidden" @change="onFileInputChange" />
    <template v-if="model">
      <!--
        Grid wrapper with a single minmax(0,1fr) column so that long display labels
        actually clips overflowing labels. With the grid track, the display label is
        constrained and truncate clips with an ellipsis.
      -->
      <div class="grid min-w-0 flex-auto grow grid-cols-[minmax(0,1fr)]">
        <InputStyled as="div" class="w-full truncate">
          <ClaimValue :claim="mockClaim" type="link" />
        </InputStyled>
      </div>
      <Button type="button" @click.prevent="onClear">{{ t("common.buttons.clear") }}</Button>
    </template>
    <Button
      v-else
      type="button"
      class="w-full"
      :progress="progress"
      :total="total"
      :active="isDragOver"
      @click.prevent="onBrowse"
      @dragover.prevent="onDragOver"
      @dragenter.prevent="onDragOver"
      @dragleave.prevent="onDragLeave"
      @drop.prevent="onDrop"
      >{{ t("partials.input.InputFile.dropOrBrowse") }}</Button
    >
  </div>
</template>
