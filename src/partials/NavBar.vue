<script setup lang="ts">
import ProgressBar from "@/components/ProgressBar.vue"
import siteContext from "@/context"
import { useNavbar } from "@/navbar"
import LanguageSwitcher from "@/partials/LanguageSwitcher.vue"
import { injectMainProgress } from "@/progress"
import { getNavbarComponents } from "@/registry/navbar"

const { attrs: navbarAttrs } = useNavbar()

const navbarComponents = getNavbarComponents()
const mainProgress = injectMainProgress()
</script>

<template>
  <ProgressBar :progress="mainProgress" class="pd-navbar-progress fixed inset-x-0 top-0 z-40 will-change-transform" />
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the navbar horizontally shifts.
  -->
  <div class="pd-navbar-wrapper sticky left-0 z-30 w-0">
    <!-- useNavbar uses a template ref named "navbar". -->
    <div
      id="navbar"
      ref="navbar"
      class="pd-navbar w-container left-0 flex min-h-12 grow gap-x-1 border-b border-slate-400 bg-slate-300 p-1 shadow-md will-change-transform sm:gap-x-4 sm:p-4"
      v-bind="navbarAttrs"
    >
      <RouterLink :to="{ name: 'Home' }" class="group shrink-0 rounded-sm outline-none hover:bg-slate-400 active:bg-slate-200">
        <img
          v-if="siteContext.logo"
          :src="siteContext.logo"
          :alt="siteContext.title"
          :title="siteContext.title"
          class="pd-navbar-logo h-10 group-focus:ring-2 group-focus:ring-primary-500 group-focus:ring-offset-1"
        />
        <h1 v-else class="pd-navbar-logo text-4xl font-bold group-focus:ring-2 group-focus:ring-primary-500 group-focus:ring-offset-1">{{ siteContext.title }}</h1>
      </RouterLink>
      <slot name="start" />
      <component :is="c" v-for="(c, i) in navbarComponents" :key="i" />
      <slot name="end" />
      <LanguageSwitcher />
    </div>
  </div>
</template>
