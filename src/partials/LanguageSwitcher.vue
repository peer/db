<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"

import siteContext from "@/context"

const { locale } = useI18n({ useScope: "global" })

const languages = computed(() => Object.keys(siteContext.languagePriority ?? {}))
const showDropdown = ref(false)

async function selectLanguage(lang: string) {
  locale.value = lang
  showDropdown.value = false
  // Store language preference in a cookie (expires in 1 year).
  await cookieStore.set({ name: "language", value: lang, path: "/", expires: Date.now() + 365 * 24 * 60 * 60 * 1000, sameSite: "lax" })
}

function onClickOutside(event: MouseEvent) {
  const target = event.target as HTMLElement
  if (!target.closest(".pd-language-switcher")) {
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
  <div v-if="languages.length > 1" class="pd-language-switcher relative shrink-0 self-center">
    <button
      type="button"
      class="flex items-center rounded-sm px-2 py-1.5 text-sm leading-tight font-medium text-slate-700 uppercase outline-none hover:bg-slate-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-slate-200"
      @click="showDropdown = !showDropdown"
    >
      {{ locale.toUpperCase() }}
      <svg class="ml-1 size-4" viewBox="0 0 20 20" fill="currentColor">
        <path
          fill-rule="evenodd"
          d="M5.23 7.21a.75.75 0 011.06.02L10 11.168l3.71-3.938a.75.75 0 111.08 1.04l-4.25 4.5a.75.75 0 01-1.08 0l-4.25-4.5a.75.75 0 01.02-1.06z"
          clip-rule="evenodd"
        />
      </svg>
    </button>
    <div v-if="showDropdown" class="absolute top-full right-0 z-50 mt-1 min-w-full rounded-sm border border-slate-400 bg-slate-200 shadow-md">
      <button
        v-for="lang in languages"
        :key="lang"
        type="button"
        class="block w-full px-3 py-1.5 text-left text-sm leading-tight font-medium uppercase outline-none hover:bg-slate-300 focus:ring-2 focus:ring-primary-500 focus:ring-inset active:bg-slate-100"
        :class="{ 'text-primary-600': lang === locale, 'text-slate-700': lang !== locale }"
        @click="selectLanguage(lang)"
      >
        {{ lang.toUpperCase() }}
      </button>
    </div>
  </div>
</template>
