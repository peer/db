<script setup lang="ts">
import type { QueryValues } from "@/types"

import { PlusIcon } from "@heroicons/vue/20/solid"
import { useI18n } from "vue-i18n"

import ButtonLink from "@/components/ButtonLink.vue"

defineProps<{
  // Query for the SearchShortcut route (the search this shortcut runs).
  query: QueryValues
  label: string
  // When set, a create "+" button is shown to the right, linking to the create view with this query (the
  // resolved CREATE_SHORTCUT). Absent when the shortcut has no create shortcut.
  createQuery?: QueryValues | null
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <div class="flex flex-row items-center gap-1 lg:gap-4">
    <ButtonLink class="grow" :to="{ name: 'SearchShortcut', query }">{{ label }}</ButtonLink>
    <ButtonLink v-if="createQuery" :to="{ name: 'DocumentCreate', query: createQuery }" :title="t('common.buttons.create')" primary>
      <PlusIcon class="size-5" :alt="t('common.buttons.create')" />
    </ButtonLink>
  </div>
</template>
