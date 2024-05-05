<script setup lang="ts">
import type { PeerDBDocument, DocumentEndEditResponse } from "@/types"
import type { ComponentExposed } from "vue-component-type-helpers"

import { ref, computed } from "vue"
import { useRouter } from "vue-router"
import { CheckIcon } from "@heroicons/vue/20/solid"
import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import NavBar from "@/partials/NavBar.vue"
import Footer from "@/partials/Footer.vue"
import NavBarSearch from "@/partials/NavBarSearch.vue"
import PropertiesRows from "@/partials/PropertiesRows.vue"
import { getName } from "@/utils"
import { injectProgress } from "@/progress"
import { postJSON } from "@/api"

const props = defineProps<{
  id: string
  session: string
}>()

const router = useRouter()

const saveProgress = injectProgress()

const abortController = new AbortController()

const WithPeerDBDocument = WithDocument<PeerDBDocument>
const withDocument = ref<ComponentExposed<typeof WithPeerDBDocument> | null>(null)

const docName = computed(() => getName(withDocument.value?.doc?.claims))

async function onSave() {
  if (abortController.signal.aborted) {
    return
  }

  saveProgress.value += 1
  try {
    await postJSON<DocumentEndEditResponse>(
      router.apiResolve({
        name: "DocumentEndEdit",
        params: {
          session: props.session,
        },
      }).href,
      {},
      abortController.signal,
      saveProgress,
    )
    if (abortController.signal.aborted) {
      return
    }
    await router.push({
      name: "DocumentGet",
      params: {
        id: props.id,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentEdit.onSave", err)
  } finally {
    saveProgress.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar>
      <NavBarSearch />
      <Button :progress="saveProgress" type="button" primary class="!px-3.5" @click.prevent="onSave">
        <CheckIcon class="h-5 w-5 sm:hidden" alt="Save" />
        <span class="hidden sm:inline">Save</span>
      </Button>
    </NavBar>
  </Teleport>
  <div class="mt-12 flex w-full flex-col gap-y-1 border-t border-transparent p-1 sm:mt-[4.5rem] sm:gap-y-4 sm:p-4" :data-url="withDocument?.url">
    <div class="rounded border bg-white p-4 shadow">
      <WithPeerDBDocument :id="id" ref="withDocument" name="DocumentGet">
        <template #default="{ doc }">
          <h1 class="mb-4 text-4xl font-bold drop-shadow-sm" v-html="docName || '<i>no name</i>'"></h1>
          <table class="w-full table-auto border-collapse">
            <thead>
              <tr>
                <th class="border-r border-slate-200 px-2 py-1 text-left font-bold">Property</th>
                <th class="border-l border-slate-200 px-2 py-1 text-left font-bold">Value</th>
              </tr>
            </thead>
            <tbody>
              <PropertiesRows :claims="doc.claims" />
            </tbody>
          </table>
        </template>
        <template #error>
          <i class="text-error-600">loading data failed</i>
        </template>
      </WithPeerDBDocument>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow" />
  </Teleport>
</template>
