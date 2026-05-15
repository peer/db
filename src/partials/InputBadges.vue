<script setup lang="ts">
import { ArrowPathSingleCounterclockwiseIcon } from "@sidekickicons/vue/20/solid"
import { useI18n } from "vue-i18n"

defineProps<{
  required?: boolean
  changed?: boolean
}>()

const emit = defineEmits<{
  (e: "revert"): void
}>()

const { t } = useI18n({ useScope: "global" })
</script>

<template>
  <span v-if="required" class="rounded-xs bg-slate-100 px-1.5 py-0.5 text-xs leading-none text-gray-600 shadow-xs">{{ t("common.labels.required") }}</span>
  <!--
    The "changed" badge doubles as a per-field revert button. At rest it looks
    identical to the original static badge; on hover/focus it picks up the
    affordances of a regular button (darker background, focus ring, pointer
    cursor) so the user can tell it is interactive.
  -->
  <button
    v-if="changed"
    type="button"
    :title="t('common.buttons.revert')"
    class="flex flex-row items-center gap-1 rounded-xs bg-primary-300 px-1.5 py-0.5 text-xs leading-none text-gray-100 shadow-xs outline-none hover:cursor-pointer hover:bg-primary-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
    @click="emit('revert')"
  >
    {{ t("common.labels.changed") }}<ArrowPathSingleCounterclockwiseIcon class="size-3" aria-hidden="true" />
  </button>
</template>
