<script setup lang="ts">
import type { DocumentSessionResponse, DocumentSessionsResponse } from "@/types"
import type { DeepReadonly } from "vue"

import { onBeforeUnmount, ref, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import { isSignedIn } from "@/auth"
import ButtonLink from "@/components/ButtonLink.vue"
import { INSTANCE_OF } from "@/core"
import { getClaimsOfTypeWithConfidence } from "@/document"
import DisplayLabel from "@/partials/DisplayLabel.vue"
import Footer from "@/partials/Footer.vue"
import IdentityInline from "@/partials/IdentityInline.vue"
import NavBar from "@/partials/NavBar.vue"
import SearchResultTags from "@/partials/SearchResultTags.vue"
import TimeDisplay from "@/partials/TimeDisplay.vue"
import { useBusy } from "@/progress"
import { timeStringFromFloat64 } from "@/utils"

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const busy = useBusy()

// The sessions the caller can open, newest first, as the backend lists them (see
// DocumentSessionsGetAPI in document.go). Null until they have been loaded.
const sessions = ref<DocumentSessionsResponse | null>(null)

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

async function loadSessions() {
  busy.value += 1
  try {
    const response = await getURL<DocumentSessionsResponse>(router.apiResolve({ name: "DocumentSessions" }).href, null, abortController.signal, busy)
    if (abortController.signal.aborted || response === null) {
      return
    }
    sessions.value = response.doc
  } catch (err) {
    if (abortController.signal.aborted) {
      return
    }
    // TODO: Show notification with error.
    console.error("DocumentSessions.loadSessions", err)
  } finally {
    busy.value -= 1
  }
}

// A caller who is not signed in takes part in no session, so there is nothing to ask the backend for
// until they are signed in.
watch(
  () => isSignedIn(),
  (signedIn) => {
    if (signedIn) {
      // eslint-disable-next-line @typescript-eslint/no-floating-promises
      loadSessions()
    } else {
      sessions.value = null
    }
  },
  { immediate: true },
)

// The timestamps are RFC 3339 strings, which TimeDisplay renders as a Time claim at second precision.
function timeString(at: string): string {
  return timeStringFromFloat64(new Date(at).getTime() / 1000, "s")
}

function sessionLink(session: DeepReadonly<DocumentSessionResponse>) {
  return { name: "DocumentEdit", params: { id: session.doc.id, session: session.session } }
}

// What the session does comes first (creating a document or editing one), followed by the classes the
// document is an instance of, so that the session is told apart by what it works on. They are rendered
// the same as the tags of a search result.
function tags(session: DeepReadonly<DocumentSessionResponse>): { id?: string; label?: string }[] {
  return [
    { label: session.create ? t("views.DocumentSessions.create") : t("views.DocumentSessions.edit") },
    ...getClaimsOfTypeWithConfidence(session.doc.claims, "ref", INSTANCE_OF).map((claim) => ({ id: claim.to.id })),
  ]
}
</script>

<template>
  <Teleport to="header">
    <NavBar />
  </Teleport>
  <div class="pd-documentsessions mt-[var(--pd-navbar-offset)] flex w-full flex-col p-1 sm:p-4 xl:px-16">
    <div class="flex flex-col rounded-sm border border-gray-200 bg-white p-4 shadow-sm">
      <template v-if="isSignedIn()">
        <h1 id="documentsessions-title" class="text-3xl font-bold drop-shadow-xs">{{ t("views.DocumentSessions.title") }}</h1>
        <div v-if="sessions === null" id="documentsessions-loading" class="mt-4">{{ t("common.status.loading") }}</div>
        <div v-else-if="sessions.length === 0" id="documentsessions-empty" class="mt-4 text-gray-700">{{ t("views.DocumentSessions.noSessions") }}</div>
        <ul v-else class="pd-documentsessions-list mt-4 flex flex-col gap-y-3">
          <li v-for="session of sessions" :key="session.session" class="pd-documentsessions-item rounded border border-slate-300 p-3">
            <ButtonLink :to="sessionLink(session)" class="pd-documentsessions-button-open pd-print-hidden float-end mb-1 ml-4 px-4">{{
              t("views.DocumentSessions.open")
            }}</ButtonLink>
            <!-- The document is named by the state the session has it in, so a document being created is named by what has been put into it. -->
            <h2 class="pd-documentsessions-title-document mb-2 min-w-0 text-xl leading-none font-bold">
              <RouterLink :to="sessionLink(session)" class="pd-documentsessions-link-document link"><DisplayLabel :doc="session.doc" /></RouterLink>
            </h2>
            <SearchResultTags :tags="tags(session)" class="pd-documentsessions-tags mb-2" />
            <!-- A session begun or changed while nobody was signed in has no user to name. -->
            <i18n-t keypath="views.DocumentSessions.started" scope="global" tag="div" class="pd-documentsessions-text-started text-gray-700">
              <template #time><TimeDisplay :timestamp="timeString(session.at)" precision="s" /></template>
              <template #user>
                <IdentityInline v-if="session.by" :subject="session.by.id" />
                <template v-else>{{ t("views.DocumentGet.history.anonymous") }}</template>
              </template>
            </i18n-t>
            <i18n-t keypath="views.DocumentSessions.lastChange" scope="global" tag="div" class="pd-documentsessions-text-lastchange text-gray-700">
              <template #time><TimeDisplay :timestamp="timeString(session.lastChangeAt)" precision="s" /></template>
              <template #user>
                <IdentityInline v-if="session.lastChangeBy" :subject="session.lastChangeBy.id" />
                <template v-else>{{ t("views.DocumentGet.history.anonymous") }}</template>
              </template>
            </i18n-t>
          </li>
        </ul>
      </template>
      <div v-else id="documentsessions-text-notsignedin" class="my-1 text-center sm:my-4">{{ t("views.DocumentSessions.notSignedIn") }}</div>
    </div>
  </div>
  <Teleport to="footer">
    <Footer class="border-t border-slate-50 bg-slate-200 shadow-sm" />
  </Teleport>
</template>
