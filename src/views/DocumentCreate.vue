<script setup lang="ts">
import type { ClassCreateResult, CreateOptionsResponse, DocumentCreateResponse } from "@/types"

import { computed, onBeforeMount, onBeforeUnmount, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL, postJSON } from "@/api"
import { INSTANCE_OF } from "@/core"
import { HighConfidence } from "@/document"
import ClassTreeList from "@/partials/ClassTreeList.vue"
import Footer from "@/partials/Footer.vue"
import NavBar from "@/partials/NavBar.vue"
import { useBusy } from "@/progress"
import { buildRefTree, encodeQuery, makeAddClaimChange } from "@/utils"

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Loading the class list and creating a document both feed the navbar progress bar and
// lock the buttons (via useLocked) while in flight, so the user cannot start two at once.
const busy = useBusy()

const abortController = new AbortController()

const classes = ref<ClassCreateResult[]>([])
const loaded = ref(false)

// The backend returns the classes already ordered for tree rendering (classes with more documents first,
// zero-document ones last); buildRefTree turns that flat list into the hierarchy, duplicating a class under
// each of its parents.
const tree = computed(() => buildRefTree(classes.value))

onBeforeUnmount(() => {
  abortController.abort()
})

async function loadClasses() {
  if (abortController.signal.aborted) {
    return
  }

  busy.value += 1
  try {
    const { doc } = await getURL<CreateOptionsResponse>(router.apiResolve({ name: "DocumentCreateOptions" }).href, null, abortController.signal, null)
    if (abortController.signal.aborted) {
      return
    }

    classes.value = doc.classes
    loaded.value = true
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentCreate.loadClasses", err)
  } finally {
    busy.value -= 1
  }
}

onBeforeMount(() => {
  loadClasses().catch((err) => {
    console.error("DocumentCreate.onBeforeMount", err)
  })
})

async function onCreate(classId: string) {
  if (abortController.signal.aborted) {
    return
  }

  busy.value += 1
  try {
    // Open a create session. The document is not yet inserted in the store;
    // the session holds all pending changes (starting with instance_of below)
    // and the backend materializes the document only on Save.
    const createResponse = await postJSON<DocumentCreateResponse>(router.apiResolve({ name: "DocumentCreate" }).href, {}, abortController.signal, null)
    if (abortController.signal.aborted) {
      return
    }

    // Add claim for "instance of" class as the first change in the session.
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: createResponse.session,
        },
        query: encodeQuery({ change: "1" }),
      }).href,
      await makeAddClaimChange(createResponse.base, createResponse.session, 1, {
        type: "ref",
        confidence: HighConfidence,
        prop: INSTANCE_OF,
        to: classId,
      }),
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return
    }

    // Navigate to edit page.
    await router.push({
      name: "DocumentEdit",
      params: {
        id: createResponse.id,
        session: createResponse.session,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("DocumentCreate.onCreate", err)
  } finally {
    busy.value -= 1
  }
}
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="pd-documentcreate mt-[var(--pd-navbar-height)] flex w-full flex-col p-1 sm:p-4 xl:px-16">
    <div v-if="!loaded" class="my-1 sm:my-4">{{ t("common.status.loading") }}</div>
    <div v-else-if="tree.length === 0" class="my-1 sm:my-4">{{ t("views.DocumentCreate.noClasses") }}</div>
    <div v-else class="flex w-full flex-col gap-y-2 sm:gap-y-4">
      <h1 class="text-3xl font-bold drop-shadow-xs">{{ t("views.DocumentCreate.title") }}</h1>
      <ClassTreeList :nodes="tree" :on-create="onCreate" />
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
