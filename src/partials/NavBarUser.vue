<!--
What the navbar menu shows about the signed-in user themselves: the roles they hold, the subject they
are known by (with a button which copies it, because it is what one hands to somebody granting access),
and the way to their editing sessions. Roles are shown as the site knows them, untranslated.
-->

<script setup lang="ts">
import { ClipboardDocumentCheckIcon, ClipboardDocumentIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, ref } from "vue"
import { useI18n } from "vue-i18n"

import { currentIdentityId, currentRoles } from "@/auth"

const { t } = useI18n({ useScope: "global" })

// The copy button reports back by turning into a check for a moment, so a copy which changed nothing
// visible on the page is still confirmed.
const copied = ref(false)
let copiedTimeout: ReturnType<typeof setTimeout> | null = null

function clearCopiedTimeout() {
  if (copiedTimeout !== null) {
    clearTimeout(copiedTimeout)
    copiedTimeout = null
  }
}

function onCopy() {
  navigator.clipboard.writeText(currentIdentityId.value).then(
    () => {
      copied.value = true
      clearCopiedTimeout()
      copiedTimeout = setTimeout(() => {
        copied.value = false
        copiedTimeout = null
      }, 2000)
    },
    (err: unknown) => {
      console.error("NavBarUser.onCopy", err)
    },
  )
}

onBeforeUnmount(() => {
  clearCopiedTimeout()
})
</script>

<template>
  <ul v-if="currentRoles.length" class="pd-navbaruser pd-navbaruser-roles flex flex-row flex-wrap items-baseline gap-1 px-2 py-1 text-sm">
    <li v-for="role of currentRoles" :key="role" class="rounded-xs bg-slate-100 px-1.5 py-0.5 leading-none text-gray-600 shadow-xs">{{ role }}</li>
  </ul>
  <div class="pd-navbaruser pd-navbaruser-id flex flex-row items-center gap-x-1 px-2 py-1">
    <span class="min-w-0 truncate font-mono text-xs text-gray-700" :title="currentIdentityId">{{ currentIdentityId }}</span>
    <button
      type="button"
      :title="copied ? t('partials.NavBarUser.copied') : t('partials.NavBarUser.copyId')"
      class="shrink-0 rounded-sm p-0.5 text-slate-500 outline-none hover:bg-slate-300 hover:text-slate-700 focus:ring-2 focus:ring-primary-500 active:bg-slate-100"
      @click="onCopy"
    >
      <ClipboardDocumentCheckIcon v-if="copied" class="size-4" :alt="t('partials.NavBarUser.copied')" />
      <ClipboardDocumentIcon v-else class="size-4" :alt="t('partials.NavBarUser.copyId')" />
    </button>
  </div>
  <RouterLink
    :to="{ name: 'DocumentSessions' }"
    class="pd-navbaruser pd-navbaruser-sessions rounded-sm px-2 py-1.5 text-sm leading-tight font-medium text-gray-700 outline-none hover:bg-slate-300 focus:ring-2 focus:ring-primary-500 active:bg-slate-100"
    >{{ t("views.DocumentSessions.title") }}</RouterLink
  >
</template>
