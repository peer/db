<script setup lang="ts">
import type { DownloadingPhase } from "@/types"

import { Dialog, DialogPanel } from "@headlessui/vue"
import { computed } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import ProgressBar from "@/components/ProgressBar.vue"

const props = defineProps<{
  open: boolean
  downloadingPhase: DownloadingPhase | null
  completed: number
  total: number
  currentFile: string
  error: string | null
}>()

const $emit = defineEmits<{
  cancel: []
}>()

const { t } = useI18n({ useScope: "global" })

// Worker reports completed as the index of the file currently being fetched (0-based, so 0
// while the first file is downloading). Shift to a 1-based "current file" for the user, and
// cap at total so the brief final progress message before the overlay closes does not show
// e.g. "6 of 5".
const currentIndex = computed(() => Math.min(props.completed + 1, props.total))

// Hide the progress bar when there's nothing meaningful to show. The empty notice is a
// terminal state, and any error is shown as a separate block.
const showProgress = computed(() => !props.error && props.downloadingPhase !== "empty")

// The action button doubles as Close in any terminal state (error or the empty notice).
const closeOnly = computed(() => props.error !== null || props.downloadingPhase === "empty")

function onClose() {
  // We allow closing with esc key and clicking outside only in terminal states.
  if (!closeOnly.value) {
    return
  }
  $emit("cancel")
}

function onCancel() {
  $emit("cancel")
}
</script>

<template>
  <Dialog as="div" class="relative z-50" :open="open" @close="onClose">
    <!-- Backdrop. -->
    <div class="fixed inset-0 bg-black/30" aria-hidden="true" />

    <!-- Full-screen container to center the panel. -->
    <div class="fixed inset-0 flex items-center justify-center">
      <DialogPanel class="relative flex w-full max-w-md flex-col gap-y-4 rounded-sm bg-white p-4 shadow-sm sm:p-6">
        <div class="flex flex-col gap-y-2">
          <div v-if="downloadingPhase === 'preparing'" class="font-medium">
            {{ t("partials.DownloadOverlay.preparing") }}
          </div>
          <div v-else-if="downloadingPhase === 'downloading'" class="font-medium">
            {{ t("partials.DownloadOverlay.downloadingFile", { completed: currentIndex, total }) }}
          </div>
          <div v-else-if="downloadingPhase === 'empty'" class="font-medium">
            {{ t("partials.DownloadOverlay.noFiles") }}
          </div>

          <div v-if="currentFile" class="truncate text-sm text-neutral-500">{{ currentFile }}</div>

          <!-- Determinate progress bar. -->
          <ProgressBar v-if="showProgress" :progress="completed" :total="total" class="h-2 bg-slate-200" />

          <div v-if="error" class="text-error-600">{{ t("partials.DownloadOverlay.error") }}</div>
        </div>

        <div class="flex flex-row justify-end">
          <Button @click="onCancel">{{ closeOnly ? t("common.buttons.close") : t("common.buttons.cancel") }}</Button>
        </div>
      </DialogPanel>
    </div>
  </Dialog>
</template>
