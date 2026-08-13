<script setup lang="ts">
import type { D } from "@/document"

import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { deleteFromCache, postJSON } from "@/api"
import { hasDocumentPermission } from "@/auth"
import { ACTION_DELETE } from "@/core"
import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import SearchResultDocument from "@/partials/SearchResultDocument.vue"
import { useBusy } from "@/progress"

const props = defineProps<{
  id: string
}>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const busy = useBusy()

const WithDocumentD = WithDocument<D>

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
    <div class="flex flex-col gap-y-4 rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <!--
        The delete action is decided on the document, because the document's own permission claims can
        grant it and the caller's roles alone cannot tell. The document is fetched once here and is what
        the result below renders, on the card this page already provides.
      -->
      <WithDocumentD :id="id" name="DocumentGet">
        <template #default="{ doc }">
          <form v-if="hasDocumentPermission(ACTION_DELETE, doc)" class="pd-documentdelete-form flex flex-col gap-y-4" @submit.prevent="onDelete">
            <div>
              <h1 id="documentdelete-title" class="text-3xl font-bold drop-shadow-xs">{{ t("views.DocumentDelete.title") }}</h1>
              <p id="documentdelete-text-confirm" class="mt-1 text-gray-700">{{ t("views.DocumentDelete.confirm") }}</p>
            </div>
            <SearchResultDocument :doc="doc" class="pd-documentdelete-result" />
            <div class="flex flex-row justify-between gap-4">
              <Button id="documentdelete-button-cancel" type="button" @click.prevent="onCancel">{{ t("common.buttons.cancel") }}</Button>
              <Button id="documentdelete-button-delete" type="submit" primary :progress="busy">{{ t("common.buttons.delete") }}</Button>
            </div>
          </form>
          <div v-else id="documentdelete-text-notallowed" class="my-1 text-center sm:my-4">{{ t("common.status.deletingNotAllowed") }}</div>
        </template>
        <template #loading>
          <div id="documentdelete-loading" class="my-1 text-center sm:my-4">{{ t("common.status.loading") }}</div>
        </template>
      </WithDocumentD>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
