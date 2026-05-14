<script setup lang="ts">
import { HighConfidence, type D } from "@/document"
import type { DocumentBeginEditResponse, DocumentCreateResponse, Result } from "@/types"

import { PlusIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL, postJSON } from "@/api"
import Button from "@/components/Button.vue"
import { CLASS, INSTANCE_OF } from "@/core"
import { hasFields, isAbstractClass } from "@/fields"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import { localCounter, useLock } from "@/progress"
import { encodeQuery, makeAddClaimChange } from "@/utils"

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

// Data modification and controls. busy holds a local count that drives
// the Create button's :progress visual; writes also propagate into the
// useLock combined ref so descendants and the button itself cascade-lock,
// but ancestor lock contributions are not reflected in the visual.
const busy = localCounter(useLock())

const abortController = new AbortController()

const showDropdown = ref(false)
const classesWithFields = ref<D[]>([])
const initial = ref(true)
const loaded = ref(false)
const loading = ref(false)

onBeforeUnmount(() => {
  abortController.abort()
})

async function loadClasses() {
  if (loaded.value || loading.value || abortController.signal.aborted) {
    return
  }

  initial.value = false
  loading.value = true
  try {
    // Get search results for documents that are instances of CLASS.
    const results = await postJSON<Result[]>(
      router.apiResolve({ name: "SearchJustResults" }).href,
      {
        query: "",
        filters: { and: [{ ref: { prop: INSTANCE_OF, value: CLASS } }] },
      },
      abortController.signal,
      null,
    )
    if (abortController.signal.aborted) {
      return
    }

    // Fetch each class document and check for fields.
    const classes: D[] = []
    for (const result of results) {
      try {
        const { doc } = await getURL<D>(router.apiResolve({ name: "DocumentGet", params: { id: result.id } }).href, null, abortController.signal, null)
        if (abortController.signal.aborted) {
          return
        }
        if (hasFields(doc.claims) && !isAbstractClass(doc.claims)) {
          classes.push(doc)
        }
      } catch (err) {
        // TODO: Do something better?
        console.error("CreateDropdown.loadClasses", err)
      }
    }

    classesWithFields.value = classes
    loaded.value = true
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("CreateDropdown.loadClasses", err)
  } finally {
    loading.value = false
  }
}

function onToggle() {
  showDropdown.value = !showDropdown.value
  if (showDropdown.value && !loaded.value) {
    loadClasses().catch((err) => {
      console.error("CreateDropdown.onToggle", err)
    })
  }
}

async function onCreate(classId: string) {
  if (abortController.signal.aborted) {
    return
  }

  showDropdown.value = false
  busy.value += 1
  try {
    // Create a new document.
    const createResponse = await postJSON<DocumentCreateResponse>(router.apiResolve({ name: "DocumentCreate" }).href, {}, abortController.signal, busy)
    if (abortController.signal.aborted) {
      return
    }

    // Begin editing.
    const editResponse = await postJSON<DocumentBeginEditResponse>(
      router.apiResolve({
        name: "DocumentBeginEdit",
        params: {
          id: createResponse.id,
        },
      }).href,
      {},
      abortController.signal,
      busy,
    )
    if (abortController.signal.aborted) {
      return
    }

    // Add claim for "instance of" class.
    await postJSON(
      router.apiResolve({
        name: "DocumentSaveChange",
        params: {
          session: editResponse.session,
        },
        query: encodeQuery({ change: "1" }),
      }).href,
      await makeAddClaimChange(createResponse.base, editResponse.session, 1, {
        type: "ref",
        confidence: HighConfidence,
        prop: INSTANCE_OF,
        to: classId,
      }),
      // await makeAddClaimChange(latestChange + 1, makePatch()),
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
        session: editResponse.session,
      },
    })
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    console.error("CreateDropdown.onCreate", err)
  } finally {
    busy.value -= 1
  }
}

function onClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest(".pd-create-dropdown")) {
    showDropdown.value = false
  }
}

onMounted(() => {
  document.addEventListener("click", onClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener("click", onClickOutside)
})
</script>

<template>
  <div v-if="initial || loading || (loaded && classesWithFields.length > 0)" class="pd-create-dropdown relative shrink-0 self-center">
    <Button :progress="busy" type="button" primary class="px-3.5" @click.prevent="onToggle">
      <PlusIcon class="size-5 sm:hidden" :alt="t('common.buttons.create')" />
      <span class="hidden sm:inline">{{ t("common.buttons.create") }}</span>
      <svg class="ml-1 hidden size-4 sm:inline" viewBox="0 0 20 20" fill="currentColor">
        <path
          fill-rule="evenodd"
          d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
          clip-rule="evenodd"
        />
      </svg>
    </Button>
    <div v-if="showDropdown" class="absolute top-full right-0 z-50 mt-1 min-w-48 rounded-sm border border-slate-400 bg-white shadow-lg">
      <div v-if="loading" class="px-3 py-2 text-sm text-slate-500">{{ t("common.status.loading") }}</div>
      <template v-else>
        <button
          v-for="cls in classesWithFields"
          :key="cls.id"
          type="button"
          class="block w-full px-3 py-2 text-left text-sm outline-none hover:bg-slate-100 focus:ring-2 focus:ring-primary-500 focus:ring-inset active:bg-slate-200"
          @click="onCreate(cls.id)"
        >
          <!-- TODO: This twice loads same document (here and inside DisplayLabel). Do we care with caching? -->
          <DisplayLabel :doc="cls" />
        </button>
      </template>
    </div>
  </div>
</template>
