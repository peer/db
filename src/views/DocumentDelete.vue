<script setup lang="ts">
import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, postJSON } from "@/api"
import { CAN_DELETE_DOCUMENT, hasPermission } from "@/auth"
import Button from "@/components/Button.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import SearchResult from "@/partials/SearchResult.vue"
import { useBusy } from "@/progress"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const busy = useBusy()

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

// Cancel returns to the document without deleting it.
async function onCancel() {
  await router.push({ name: "DocumentGet", params: { id: props.id } })
}

async function onDelete() {
  if (abortController.signal.aborted) {
    return
  }

  busy.value += 1
  try {
    await postJSON(
      router.apiResolve({
        name: "DocumentDelete",
        params: {
          id: props.id,
        },
      }).href,
      {},
      abortController.signal,
      busy,
    )
    if (abortController.signal.aborted) {
      return
    }
    // The document no longer exists, so drop its cached response and leave the page.
    deleteFromCache(
      router.apiResolve({
        name: "DocumentGet",
        params: {
          id: props.id,
        },
      }).href,
    )
    await router.push({
      name: "Home",
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentDelete.onDelete", err)
  } finally {
    busy.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="pd-documentdelete mt-[var(--pd-navbar-offset)] flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:gap-y-4 sm:p-4">
    <template v-if="hasPermission(CAN_DELETE_DOCUMENT)">
      <div>
        <h1 class="text-3xl font-bold drop-shadow-xs">{{ t("views.DocumentDelete.title") }}</h1>
        <p class="mt-1 text-gray-700">{{ t("views.DocumentDelete.confirm") }}</p>
      </div>
      <SearchResult :result="{ id }" />
      <div class="flex flex-row justify-between gap-4">
        <Button id="documentdelete-button-cancel" type="button" @click.prevent="onCancel">{{ t("common.buttons.cancel") }}</Button>
        <Button id="documentdelete-button-delete" type="button" primary :progress="busy" @click.prevent="onDelete">{{ t("common.buttons.delete") }}</Button>
      </div>
    </template>
    <div v-else class="my-1 text-center sm:my-4">{{ t("common.status.deletingNotAllowed") }}</div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
