<script setup lang="ts">
import { ref } from "vue"

const progress = ref(false)

function onSubmit() {
  progress.value = true
}
</script>

<template>
  <form :readonly="progress" class="flex flex-grow flex-col" @submit.prevent="onSubmit">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">Wikipedia Search</h1>
    </div>
    <div class="flex justify-center">
      <input
        :readonly="progress"
        type="text"
        class="mx-4 w-full max-w-2xl rounded border-neutral-300 bg-white shadow-sm hover:border-neutral-400 focus:border-primary-500 sm:w-4/5 md:w-2/3 lg:w-1/2"
      />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <button
        :readonly="progress"
        type="submit"
        class="mx-4 my-px rounded bg-primary-600 px-6 py-2.5 font-medium uppercase leading-tight text-white shadow-sm outline-none hover:bg-primary-700 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
      >
        Search
      </button>
    </div>
  </form>
  <Teleport to="footer">
    <div class="flex justify-between px-4 py-4 leading-none">
      <ul class="flex gap-x-4">
        <!-- <li><router-link :to="{ name: 'HomeGet' }" class="link">About</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Help</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Privacy</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">Terms</router-link></li>
        <li><router-link :to="{ name: 'HomeGet' }" class="link">API</router-link></li> -->
      </ul>
      <ul class="flex gap-x-4">
        <li class="text-neutral-500" title="build ">Powered by <a href="https://gitlab.com/peerdb/search" class="link">PeerDB Search</a></li>
      </ul>
    </div>
  </Teleport>
</template>
