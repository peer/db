<script setup lang="ts">
import { Bars3Icon } from "@heroicons/vue/20/solid"
import { onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"

// NavBarMenu keeps the navbar on a single row on narrow viewports by folding its slotted
// actions into a dropdown menu. Above the breakpoint the component renders the slot directly:
// its root is the slot itself, a fragment with no wrapping element, so the slotted actions stay
// direct children of the navbar and any navbar layout that targets them with a "> child" selector
// keeps working. Below the breakpoint they move into the menu panel.

const { t } = useI18n({ useScope: "global" })

const open = ref(false)

// The collapse breakpoint matches Tailwind's md (48rem): below it the actions fold into the menu.
const mediaQuery = window.matchMedia("(width < 48rem)")
const collapsed = ref(mediaQuery.matches)
function onMediaChange(event: MediaQueryListEvent): void {
  collapsed.value = event.matches
  if (!event.matches) {
    open.value = false
  }
}
mediaQuery.addEventListener("change", onMediaChange)

function onClickOutside(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (!target.closest(".pd-navbar-menu")) {
    open.value = false
  }
}

onMounted(() => {
  document.addEventListener("click", onClickOutside)
})

onBeforeUnmount(() => {
  mediaQuery.removeEventListener("change", onMediaChange)
  document.removeEventListener("click", onClickOutside)
})
</script>

<template>
  <slot v-if="!collapsed" />
  <div v-else class="pd-navbar-menu relative shrink-0">
    <button
      type="button"
      :aria-label="t('common.buttons.menu')"
      :aria-expanded="open"
      class="pd-navbar-menu-button flex items-center rounded-sm p-1.5 text-gray-700 outline-none hover:bg-slate-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-slate-200"
      @click="open = !open"
    >
      <Bars3Icon class="size-6" />
    </button>
    <div
      v-if="open"
      class="pd-navbar-menu-panel absolute top-full right-0 z-50 mt-1 flex flex-col items-stretch gap-1 rounded-sm border border-slate-400 bg-slate-200 p-2 shadow-md"
    >
      <slot />
    </div>
  </div>
</template>
