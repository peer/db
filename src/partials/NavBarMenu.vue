<script setup lang="ts">
import { Bars3Icon, ChevronDownIcon } from "@heroicons/vue/20/solid"
import { computed, onBeforeUnmount, onMounted, ref } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

// NavBarMenu keeps the navbar on a single row on narrow viewports by folding its slotted
// actions into a dropdown menu. Above the breakpoint the component renders the slot directly:
// its root is the slot itself, a fragment with no wrapping element, so the slotted actions stay
// direct children of the navbar and any navbar layout that targets them with a "> child" selector
// keeps working. Below the breakpoint they move into the menu panel.

const props = withDefaults(
  defineProps<{
    // The label of the menu button, which folds the actions into the menu at every width and not only
    // below the collapse breakpoint. There is no room for a label below it, so the button is the menu
    // icon there and the label becomes the first entry of the panel.
    label?: string
  }>(),
  {
    label: undefined,
  },
)

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const open = ref(false)

// The collapse breakpoint matches Tailwind's md (48rem): below it the actions fold into the menu.
const mediaQuery = window.matchMedia("(width < 48rem)")
const collapsed = ref(mediaQuery.matches)
function onMediaChange(event: MediaQueryListEvent): void {
  collapsed.value = event.matches
  if (!event.matches && props.label === undefined) {
    open.value = false
  }
}
mediaQuery.addEventListener("change", onMediaChange)

const asMenu = computed(() => collapsed.value || props.label !== undefined)

function onClickOutside(event: MouseEvent): void {
  const target = event.target as HTMLElement
  if (!target.closest(".pd-navbar-menu")) {
    open.value = false
  }
}

// A menu entry which navigates is clicked inside the panel, so the panel would stay open behind the
// page it opened. The hook runs on every navigation, including one to the page which is already open
// (vue-router reports that as a duplicated navigation, but runs the hook all the same), so an entry
// leading where the caller already is closes the menu, too.
const stopAfterEach = router.afterEach(() => {
  open.value = false
})

onMounted(() => {
  document.addEventListener("click", onClickOutside)
})

onBeforeUnmount(() => {
  mediaQuery.removeEventListener("change", onMediaChange)
  document.removeEventListener("click", onClickOutside)
  stopAfterEach()
})
</script>

<template>
  <slot v-if="!asMenu" />
  <div v-else class="pd-navbar-menu relative shrink-0">
    <button
      type="button"
      :aria-label="collapsed ? t('common.buttons.menu') : undefined"
      :aria-expanded="open"
      class="pd-navbar-menu-button flex items-center rounded-sm text-gray-700 outline-none hover:bg-slate-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-slate-200"
      :class="collapsed ? 'p-1.5' : 'px-2 py-1.5 text-sm leading-tight font-medium'"
      @click="open = !open"
    >
      <Bars3Icon v-if="collapsed" class="size-6" />
      <template v-else>
        <span class="max-w-48 truncate">{{ label }}</span>
        <ChevronDownIcon class="ml-1 size-4 shrink-0" />
      </template>
    </button>
    <div
      v-if="open"
      class="pd-navbar-menu-panel absolute top-full right-0 z-50 mt-1 flex flex-col items-stretch gap-1 rounded-sm border border-slate-400 bg-slate-200 p-2 shadow-md"
    >
      <!-- The label names the menu where the button cannot, so the panel opens with whose menu it is. -->
      <div v-if="collapsed && label !== undefined" class="pd-navbar-menu-label truncate px-2 py-1 text-sm leading-tight font-medium text-gray-700">
        {{ label }}
      </div>
      <slot />
    </div>
  </div>
</template>
