<script setup lang="ts">
import type { DeepReadonly } from "vue"

import { onBeforeUnmount } from "vue"
import { useI18n } from "vue-i18n"

import Button from "@/components/Button.vue"
import { ClaimTypes } from "@/document"
import ClaimValueAmount from "@/partials/claimvalue/ClaimValueAmount.vue"
import ClaimValueAmountInterval from "@/partials/claimvalue/ClaimValueAmountInterval.vue"
import ClaimValueHas from "@/partials/claimvalue/ClaimValueHas.vue"
import ClaimValueHtml from "@/partials/claimvalue/ClaimValueHtml.vue"
import ClaimValueId from "@/partials/claimvalue/ClaimValueId.vue"
import ClaimValueLink from "@/partials/claimvalue/ClaimValueLink.vue"
import ClaimValueNone from "@/partials/claimvalue/ClaimValueNone.vue"
import ClaimValueRef from "@/partials/claimvalue/ClaimValueRef.vue"
import ClaimValueString from "@/partials/claimvalue/ClaimValueString.vue"
import ClaimValueTime from "@/partials/claimvalue/ClaimValueTime.vue"
import ClaimValueTimeInterval from "@/partials/claimvalue/ClaimValueTimeInterval.vue"
import ClaimValueUnknown from "@/partials/claimvalue/ClaimValueUnknown.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

withDefaults(
  defineProps<{
    claims?: DeepReadonly<ClaimTypes>
    level?: number
    editable?: boolean
  }>(),
  {
    claims: () => new ClaimTypes({}),
    level: 0,
    editable: false,
  },
)

const $emit = defineEmits<{
  editClaim: [value: string]
  removeClaim: [value: string]
}>()

const { t } = useI18n({ useScope: "global" })

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

function onEdit(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("editClaim", id)
}

function onRemove(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("removeClaim", id)
}
</script>

<template>
  <template v-for="claim in claims.id" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueId :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.string" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueString :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.html" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueHtml :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.amount" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueAmount :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.amountInterval" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        ><ClaimValueAmountInterval :claim="claim"
      /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.time" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueTime :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.timeInterval" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueTimeInterval :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.link" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueLink :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.ref" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueRef :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.has" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueHas :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.none" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueNone :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
  <template v-for="claim in claims.unknown" :key="claim.id">
    <tr>
      <td
        class="border-r border-slate-200 py-1 pr-2 align-top whitespace-nowrap"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueUnknown :claim="claim" /></td>
      <td v-if="editable" class="border-slate-200 py-1 pl-2 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onEdit(claim.id)">{{ t("common.buttons.edit") }}</Button>
      </td>
      <td v-if="editable" class="border-slate-200 py-1 pl-1 align-top whitespace-nowrap" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <Button type="button" class="w-full px-4 py-1.5" @click.prevent="onRemove(claim.id)">{{ t("common.buttons.remove") }}</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.sub" :level="level + 1" :editable="editable" @edit-claim="onEdit" @remove-claim="onRemove" />
  </template>
</template>
