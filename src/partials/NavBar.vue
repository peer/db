<script setup lang="ts">
import { GlobeAltIcon } from "@heroicons/vue/24/outline"
import ProgressBar from "@/components/ProgressBar.vue"
import { useNavbar } from "@/navbar"
import { injectMainProgress } from "@/progress"

const { ref: navbar, attrs: navbarAttrs } = useNavbar()

const mainProgress = injectMainProgress()
</script>

<template>
  <ProgressBar :progress="mainProgress" class="fixed inset-x-0 top-0 z-40 will-change-transform" />
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the navbar horizontally shifts.
  -->
  <div class="sticky left-0 w-0 z-30">
    <div
      ref="navbar"
      class="flex w-container min-h-12 flex-grow gap-x-1 left-0 border-b border-slate-400 bg-slate-300 p-1 shadow-md will-change-transform sm:gap-x-4 sm:p-4 sm:pl-0"
      v-bind="navbarAttrs"
    >
      <RouterLink
        :to="{ name: 'Home' }"
        class="p-1.5 sm:p-0 group -my-1 -ml-1 sm:ml-0 sm:-my-4 border-r border-slate-400 outline-none hover:bg-slate-400 active:bg-slate-200"
      >
        <GlobeAltIcon class="m-1 sm:m-4 sm:h-10 sm:w-10 h-7 w-7 rounded group-focus:ring-2 group-focus:ring-primary-500" />
      </RouterLink>
      <slot />
    </div>
  </div>
</template>
