<script setup lang="ts">
import { ChevronDownIcon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"

import { enabledLanguages } from "@/i18n"

const { t, locale } = useI18n({ useScope: "global" })

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
  <div v-if="enabledLanguages.length > 1" class="pd-language-switcher relative shrink-0">
    <button
      type="button"
      :aria-label="t('partials.LanguageSwitcher.selectLanguage')"
      class="flex items-center rounded-sm px-2 py-1.5 text-sm leading-tight font-medium text-gray-700 uppercase outline-none hover:bg-slate-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-slate-200"
      @click="showDropdown = !showDropdown"
    >
      {{ locale.toUpperCase() }}
      <ChevronDownIcon class="ml-1 h-4 w-4" />
    </button>
    <div v-if="showDropdown" class="absolute top-full right-0 z-50 mt-1 min-w-full rounded-sm border border-slate-400 bg-slate-200 shadow-md">
      <button
        v-for="lang in enabledLanguages"
        :key="lang"
        type="button"
        class="block w-full px-3 py-1.5 text-left text-sm leading-tight font-medium uppercase outline-none hover:bg-slate-300 focus:ring-2 focus:ring-primary-500 focus:ring-inset active:bg-slate-100"
        :class="lang === locale ? 'text-primary-600' : 'text-gray-700'"
        @click="selectLanguage(lang)"
      >
        {{ lang.toUpperCase() }}
      </button>
    </div>
  </div>
</template>
