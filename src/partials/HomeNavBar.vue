<script setup lang="ts">
import { currentIdentityId, currentUsername, isSignedIn } from "@/auth"
import ProgressBar from "@/components/ProgressBar.vue"
import CreateButton from "@/partials/CreateButton.vue"
import LanguageSwitcher from "@/partials/LanguageSwitcher.vue"
import NavBarMenu from "@/partials/NavBarMenu.vue"
import NavBarUser from "@/partials/NavBarUser.vue"
import SignInButton from "@/partials/SignInButton.vue"
import { getParentProgress } from "@/progress"
import { getNavbarComponents } from "@/registry/navbar"

const navbarComponents = getNavbarComponents()
const parentProgress = getParentProgress()
</script>

<template>
  <ProgressBar :progress="parentProgress" class="pd-navbar-progress fixed inset-x-0 top-0 z-40 will-change-transform" />
  <div class="pd-navbar-wrapper">
    <div id="navbar" class="pd-navbar w-container flex min-h-[var(--pd-navbar-height)] grow items-center justify-end gap-x-1 p-1 sm:gap-x-4 sm:p-4">
      <component :is="c" v-for="(c, i) in navbarComponents" :key="i" home />
      <CreateButton home />
      <!--
        A signed-in user has the same menu of their own as on every other page (see NavBar). The home
        navbar carries little else, so a caller who is not signed in keeps the language switcher and
        the sign-in button inline at every width.
      -->
      <NavBarMenu v-if="isSignedIn()" :label="currentUsername || currentIdentityId">
        <NavBarUser />
        <LanguageSwitcher />
        <SignInButton />
      </NavBarMenu>
      <template v-else>
        <LanguageSwitcher />
        <SignInButton />
      </template>
    </div>
  </div>
</template>
