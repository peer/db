<script setup lang="ts">
import { PlusIcon } from "@heroicons/vue/20/solid"
import { useI18n } from "vue-i18n"

import { CAN_EDIT_DOCUMENT, hasPermission } from "@/auth"
import ButtonLink from "@/components/ButtonLink.vue"

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <ButtonLink v-if="hasPermission(CAN_EDIT_DOCUMENT)" :to="{ name: 'DocumentCreate' }" primary class="pd-navbar-create">
    <PlusIcon class="size-5 sm:hidden" :alt="t('common.buttons.create')" />
    <span class="hidden sm:inline">{{ t("common.buttons.create") }}</span>
  </ButtonLink>
</template>
