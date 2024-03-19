<script setup lang="ts">
import { ref } from "vue"
import InputText from "@/components/InputText.vue"
import Button from "@/components/Button.vue"
import Footer from "@/components/Footer.vue"
import { postSearch } from "@/search"
import siteContext from "@/context"
import { useRouter } from "@/utils"

const router = useRouter()
const form = ref()
const progress = ref(0)

async function onSubmit() {
  try {
    await postSearch(router, form.value, progress)
  } catch (err) {
    // TODO: Show notification with error.
    console.error("onSubmit", err)
  }
}
</script>

<template>
  <form ref="form" :disabled="progress > 0" class="flex flex-grow flex-col" @submit.prevent="onSubmit">
    <div class="flex flex-grow basis-0 flex-col-reverse">
      <h1 class="mb-10 p-4 text-center text-5xl font-bold">{{ siteContext.title }}</h1>
    </div>
    <div class="flex justify-center">
      <InputText :progress="progress" name="q" class="mx-4 w-full max-w-2xl sm:w-4/5 md:w-2/3 lg:w-1/2" />
    </div>
    <div class="flex-grow basis-0 pt-4 text-center">
      <Button :progress="progress" type="submit" class="mx-4">Search</Button>
    </div>
  </form>
  <Teleport to="footer">
    <Footer />
  </Teleport>
</template>
