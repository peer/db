<!--
One line naming the user with the given subject (see IdentityLabel), who is looked up through the user
API for it (see UserGetAPI in user.go). While the lookup runs a placeholder line stands in for the name,
and a user the site cannot look up is named by their subject: it is what identifies them either way.

Use IdentityLabel directly where the identity has been loaded already, so it is not fetched twice.
-->

<script setup lang="ts">
import type { Identity } from "@/types"

import WithDocument from "@/components/WithDocument.vue"
import IdentityLabel from "@/partials/IdentityLabel.vue"
import { loadingWidth } from "@/utils"

const props = defineProps<{
  subject: string
}>()

// We want all fallthrough attributes to be passed to the element naming the user.
defineOptions({
  inheritAttrs: false,
})

const WithDocumentIdentity = WithDocument<Identity>
</script>

<template>
  <WithDocumentIdentity :id="subject" name="UserGet">
    <template #default="{ doc, url }">
      <IdentityLabel :identity="doc" class="pd-identityinline" :data-url="url" v-bind="$attrs" />
    </template>
    <template #loading="{ url }">
      <div
        class="pd-identityinline pd-identityinline-loading inline-block h-2 rounded-sm bg-slate-200 motion-safe:animate-pulse"
        :data-url="url"
        :class="[loadingWidth(props.subject)]"
        aria-hidden="true"
      />
    </template>
    <template #error="{ url }">
      <span class="pd-identityinline pd-identityinline-error" :data-url="url" v-bind="$attrs">{{ props.subject }}</span>
    </template>
  </WithDocumentIdentity>
</template>
