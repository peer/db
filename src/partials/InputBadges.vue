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

    It is rendered unconditionally (just visibility:hidden when the field is
    not changed) so it reserves layout width in the parent label row. This
    keeps the label - and any sibling layout that follows from it, like an
    input directly below in a column flex - at a stable minimum width that
    always accommodates the badge, instead of growing/shrinking as the user
    edits. aria-hidden + tabindex + disabled keep the hidden state inert.
  -->
  <button
    type="button"
    :title="t('common.buttons.revert')"
    :tabindex="changed ? 0 : -1"
    :aria-hidden="!changed || undefined"
    :disabled="!changed"
    class="flex flex-row items-center gap-1 rounded-xs bg-primary-300 px-1.5 py-0.5 text-xs leading-none text-gray-100 shadow-xs outline-none hover:cursor-pointer hover:bg-primary-400 focus:ring-2 focus:ring-primary-500 focus:ring-offset-1 active:bg-primary-500"
    :class="{ invisible: !changed }"
    @click="emit('revert')"
  >
    {{ t("common.labels.changed") }}<ArrowPathSingleCounterclockwiseIcon class="size-3" aria-hidden="true" />
  </button>
</template>
