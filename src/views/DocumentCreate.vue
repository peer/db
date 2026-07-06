<script setup lang="ts">
import type { ClassCreateResult, CreateOptionsResponse, DocumentCreateResponse } from "@/types"

import { computed, onBeforeUnmount, ref, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRoute, useRouter } from "vue-router"

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
const route = useRoute()

// Loading the class list and creating a document both feed the navbar progress bar and
// lock the buttons (via useLocked) while in flight, so the user cannot start two at once.
const busy = useBusy()

// "limit" restricts the offered classes to that class and its descendants (handled by the backend).
const limit = computed((): string => {
  const value = route.query.limit
  if (Array.isArray(value)) {
    return value[0] ?? ""
  }
  return value ?? ""
})

// Every other query param is a property=value pair (both IDs) added as an initial reference claim on the
// created document before navigating to the editor. This is plain query-string passing, compatible with
// search shortcuts but not parsed as one here.
const claimParams = computed((): { prop: string; to: string }[] => {
  const out: { prop: string; to: string }[] = []
  for (const [key, value] of Object.entries(route.query)) {
    if (key === "limit") {
      continue
    }
    for (const single of Array.isArray(value) ? value : [value]) {
      if (single !== null) {
        out.push({ prop: key, to: single })
      }
    }
  }
  return out
})

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

  // The limit is captured so a slower load for a previous limit cannot overwrite a newer one's result.
  const requested = limit.value

  busy.value += 1
  try {
    const { doc } = await getURL<CreateOptionsResponse>(
      router.apiResolve({ name: "DocumentCreateOptions", query: encodeQuery({ limit: requested || undefined }) }).href,
      null,
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted || requested !== limit.value) {
      return
    }

    classes.value = doc.classes
    loaded.value = true
  } catch (err) {
    if (abortController.signal.aborted || requested !== limit.value) {
      return
    }
    console.error("DocumentCreate.loadClasses", err)
  } finally {
    busy.value -= 1
  }
}

// Load on mount and whenever the limit changes, since the route query can change without the component
// being remounted (Vue Router reuses it on a query-only navigation).
// TODO: Better report the error.
watch(limit, () => loadClasses().catch((err) => console.error("DocumentCreate.loadClasses", err)), { immediate: true })

// saveRefClaim appends one reference claim (prop and to are IDs) to the create session as the given change.
async function saveRefClaim(createResponse: DocumentCreateResponse, change: number, prop: string, to: string) {
  await postJSON(
    router.apiResolve({
      name: "DocumentSaveChange",
      params: {
        session: createResponse.session,
      },
      query: encodeQuery({ change: String(change) }),
    }).href,
    await makeAddClaimChange(createResponse.base, createResponse.session, change, {
      type: "ref",
      confidence: HighConfidence,
      prop,
      to,
    }),
    abortController.signal,
    null,
  )
}

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

    // The first change is the "instance of" class; then any property=value query params become further
    // initial reference claims, before navigating to the editor.
    let change = 1
    await saveRefClaim(createResponse, change, INSTANCE_OF, classId)
    if (abortController.signal.aborted) {
      return
    }
    for (const { prop, to } of claimParams.value) {
      change += 1
      await saveRefClaim(createResponse, change, prop, to)
      if (abortController.signal.aborted) {
        return
      }
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
  <div class="pd-documentcreate mt-[var(--pd-navbar-offset)] flex w-full flex-col p-1 sm:p-4 xl:px-16">
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
