<script setup lang="ts">
import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"

import { isOIDCConfigured, isSignedIn, signIn, signOut } from "@/auth"
import Button from "@/components/Button.vue"
import { useBusy } from "@/progress"

const { t } = useI18n({ useScope: "global" })

const busy = useBusy()

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

async function onSignIn() {
  if (abortController.signal.aborted) {
    return
  }
  busy.value += 1
  try {
    await signIn(busy)
  } finally {
    // We never decrement when the redirect actually fires (signIn redirects
    // away), but if it threw early we need to release the lock so the button
    // becomes usable again.
    if (!abortController.signal.aborted) {
      busy.value -= 1
    }
  }
}

function onSignOut() {
  if (abortController.signal.aborted) {
    return
  }
  signOut(busy)
}
</script>

<template>
  <template v-if="isOIDCConfigured()">
    <Button v-if="isSignedIn()" id="navbar-button-signout" primary type="button" :progress="busy" @click.prevent="onSignOut">
      {{ t("common.buttons.signOut") }}
    </Button>
    <Button v-else id="navbar-button-signin" primary type="button" :progress="busy" @click.prevent="onSignIn">
      {{ t("common.buttons.signIn") }}
    </Button>
  </template>
</template>
