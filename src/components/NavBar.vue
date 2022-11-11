<script setup lang="ts">
import { GlobeAltIcon } from "@heroicons/vue/24/outline"
import ProgressBar from "@/components/ProgressBar.vue"
import RouterLink from "@/components/RouterLink.vue"
import { useNavbar } from "@/navbar"

defineProps<{
  progress: number
}>()

const { ref: navbar, attrs: navbarAttrs } = useNavbar()
</script>

<template>
  <ProgressBar :progress="progress" class="fixed inset-x-0 top-0 z-40 will-change-transform" />
  <div
    ref="navbar"
    class="z-30 flex w-full flex-grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow-md will-change-transform sm:gap-x-4 sm:p-4 sm:pl-0"
    v-bind="navbarAttrs"
  >
    <RouterLink :to="{ name: 'HomeGet' }" class="group -my-4 hidden border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200 sm:block">
      <GlobeAltIcon class="m-4 h-10 w-10 rounded group-focus:ring-2 group-focus:ring-primary-500" />
    </RouterLink>
    <slot />
  </div>
</template>
