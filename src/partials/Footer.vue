<script setup lang="ts">
// We want all fallthrough attributes to be passed to the footer element and not the container.
defineOptions({
  inheritAttrs: false,
})
</script>

<template>
  <!--
    TODO: No idea why w-0 (and w-fit) work here, but w-full does not.
          One would assume that w-full is needed to make the container div as wide as the
          body inside which then the footer horizontally shifts.
  -->
  <div class="sticky left-0 z-30 w-0">
    <div v-bind="$attrs" class="w-container flex justify-between gap-x-2 p-2 leading-none sm:gap-x-4 sm:p-4">
      <ul class="flex gap-x-2 sm:gap-x-4">
        <!-- <li><RouterLink :to="{ name: 'Home' }" class="link">About</RouterLink></li>
        <li><RouterLink :to="{ name: 'Home' }" class="link">Help</RouterLink></li>
        <li><RouterLink :to="{ name: 'Home' }" class="link">Privacy</RouterLink></li>
        <li><RouterLink :to="{ name: 'Home' }" class="link">Terms</RouterLink></li>
        <li><RouterLink :to="{ name: 'Home' }" class="link">API</RouterLink></li> -->
      </ul>
      <ul class="flex gap-x-2 sm:gap-x-4">
        <li class="text-neutral-500">Powered by <a href="https://gitlab.com/peerdb/peerdb" class="link">PeerDB</a></li>
      </ul>
    </div>
  </div>
</template>
