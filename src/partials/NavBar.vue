<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue"
import { useRouter } from "vue-router"
import { GlobeAltIcon } from "@heroicons/vue/24/outline"
import { ArrowUpTrayIcon } from "@heroicons/vue/20/solid"
import ProgressBar from "@/components/ProgressBar.vue"
import Button from "@/components/Button.vue"
import { useNavbar } from "@/navbar"
import { injectMainProgress, localProgress } from "@/progress"
import { uploadFile } from "@/upload"

const { ref: navbar, attrs: navbarAttrs } = useNavbar()

const mainProgress = injectMainProgress()

const router = useRouter()

const abortController = new AbortController()

const uploadProgress = localProgress(mainProgress)

const upload = ref<HTMLInputElement>()

onBeforeUnmount(() => {
  abortController.abort()
})

async function onUpload() {
  if (abortController.signal.aborted) {
    return
  }

  upload.value?.click()
}

async function onChange() {
  if (abortController.signal.aborted) {
    return
  }

  for (const file of upload.value?.files || []) {
    uploadProgress.value += 1
    try {
      await uploadFile(router, file, abortController.signal, uploadProgress)
      // TODO: Create a document for the file and redirect there.
    } catch (err) {
      if (abortController.signal.aborted) {
        return
      }
      // TODO: Show notification with error.
      console.error("NavBar.onChange", err)
    } finally {
      uploadProgress.value -= 1
    }

    // TODO: Support uploading multiple files.
    //       Input element does not have "multiple" set, so there should be only one file.
    break
  }
}
</script>

<template>
  <ProgressBar :progress="mainProgress" class="fixed inset-x-0 top-0 z-40 will-change-transform" />
  <div
    ref="navbar"
    class="z-30 flex w-full min-h-12 flex-grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow-md will-change-transform sm:gap-x-4 sm:p-4 sm:pl-0"
    v-bind="navbarAttrs"
  >
    <RouterLink
      :to="{ name: 'Home' }"
      class="p-1.5 sm:p-0 group -my-1 -ml-1 sm:ml-0 sm:-my-4 border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200"
    >
      <GlobeAltIcon class="m-1 sm:m-4 sm:h-10 sm:w-10 h-7 w-7 rounded group-focus:ring-2 group-focus:ring-primary-500" />
    </RouterLink>
    <slot />
    <input ref="upload" type="file" class="hidden" @change="onChange" />
    <Button :progress="uploadProgress" type="button" primary class="!px-3.5" @click.prevent="onUpload">
      <ArrowUpTrayIcon class="h-5 w-5 sm:hidden" alt="Upload" />
      <span class="hidden sm:inline">Upload</span>
    </Button>
  </div>
</template>
