<script setup lang="ts">
import ProgressBar from "@/components/ProgressBar.vue"
import CreateDropdown from "@/partials/CreateDropdown.vue"
import LanguageSwitcher from "@/partials/LanguageSwitcher.vue"
import SignInButton from "@/partials/SignInButton.vue"
import { getParentProgress } from "@/progress"
import { getNavbarComponents } from "@/registry/navbar"

const navbarComponents = getNavbarComponents()
const parentProgress = getParentProgress()
</script>

<template>
  <ProgressBar :progress="parentProgress" class="pd-navbar-progress fixed inset-x-0 top-0 z-40 will-change-transform" />
  <div class="pd-navbar-wrapper">
    <div id="navbar" class="pd-navbar w-container flex min-h-12 grow items-center justify-end gap-x-1 p-1 sm:min-h-18 sm:gap-x-4 sm:p-4">
      <component :is="c" v-for="(c, i) in navbarComponents" :key="i" home />
      <CreateDropdown />
      <LanguageSwitcher />
      <SignInButton />
    </div>
  </div>
</template>
