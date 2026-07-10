<script setup lang="ts">
import { PlusIcon } from "@heroicons/vue/20/solid"
import { useI18n } from "vue-i18n"

import { CAN_EDIT_DOCUMENT, hasPermission } from "@/auth"
import ButtonLink from "@/components/ButtonLink.vue"

// home keeps the full text label at every width instead of collapsing to the plus icon below sm. The home
// navbar carries only a few trailing buttons, so it has room for the label even on narrow viewports, unlike
// the main navbar which must also fit the search box.
withDefaults(
  defineProps<{
    home?: boolean
  }>(),
  {
    home: false,
  },
)

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <ButtonLink v-if="hasPermission(CAN_EDIT_DOCUMENT)" :to="{ name: 'DocumentCreate' }" primary class="pd-navbar-create">
    <template v-if="home">{{ t("common.buttons.create") }}</template>
    <template v-else>
      <PlusIcon class="size-5 sm:hidden" :alt="t('common.buttons.create')" />
      <span class="hidden sm:inline">{{ t("common.buttons.create") }}</span>
    </template>
  </ButtonLink>
</template>
