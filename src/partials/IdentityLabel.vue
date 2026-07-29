<!--
One line naming a user: the username the site knows them by, or the subject they are known by when it
knows no username (see auth.Identity in auth/auth.go). It is how a user is named wherever they are
listed, so that a listing reads as names and falls back to the subject only when there is nothing else.

The identity has to be loaded already. IdentityInline is the same for a user given by their subject
alone: it loads them and renders this.
-->

<script setup lang="ts">
import type { Identity } from "@/types"
import type { DeepReadonly } from "vue"

defineProps<{
  identity: DeepReadonly<Identity>
}>()
</script>

<template>
  <span class="pd-identitylabel">{{ identity.username || identity.subject }}</span>
</template>
