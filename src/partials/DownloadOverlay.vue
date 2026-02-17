<script setup lang="ts">
import type { DownloadMode } from "@/download"

import { Dialog, DialogPanel } from "@headlessui/vue"
import { XMarkIcon } from "@heroicons/vue/20/solid"
import { computed } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"

const props = defineProps<{
  open: boolean
  mode: DownloadMode
  completed: number
  total: number
  currentFile: string
  error: string | null
}>()

const $emit = defineEmits<{
  cancel: []
}>()

const { t } = useI18n({ useScope: "global" })

const progressPercent = computed(() => {
  if (props.total === 0) {
    return 0
  }
  return (props.completed / props.total) * 100
})
</script>

<template>
  <Dialog as="div" class="relative z-50" :open="open" @close="$emit('cancel')">
    <!-- Backdrop. -->
    <div class="fixed inset-0 bg-black/30" aria-hidden="true" />

    <!-- Full-screen container to center the panel. -->
    <div class="fixed inset-0 flex items-center justify-center">
      <DialogPanel class="relative w-full max-w-md rounded-sm bg-white p-4 shadow-sm sm:p-6">
        <Button class="absolute! top-2 right-2 border-none! p-0! shadow-none!" :title="t('partials.DownloadOverlay.cancel')" @click="$emit('cancel')">
          <XMarkIcon class="size-5" />
        </Button>

        <div class="flex flex-col gap-y-3">
          <div v-if="error" class="text-error-600">{{ error }}</div>

          <template v-else>
            <div class="text-sm font-medium">
              {{ t("partials.DownloadOverlay.downloadingFile", { completed, total }) }}
            </div>

            <div v-if="currentFile" class="truncate text-xs text-gray-500">{{ currentFile }}</div>

            <!-- Determinate progress bar. -->
            <div class="relative h-2 w-full rounded-sm bg-slate-200">
              <div class="absolute inset-y-0 left-0 rounded-sm bg-secondary-400 transition-all duration-300" :style="{ width: progressPercent + '%' }" />
            </div>

            <div v-if="mode === 'zip' && completed === total && total > 0" class="text-xs text-gray-500">
              {{ t("partials.DownloadOverlay.creatingZip") }}
            </div>
          </template>
        </div>
      </DialogPanel>
    </div>
  </Dialog>
</template>
