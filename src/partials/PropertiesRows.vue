<script setup lang="ts">
import type { DeepReadonly } from "vue"

import { onBeforeUnmount } from "vue"

import Button from "@/components/Button.vue"
import { ClaimTypes } from "@/document"
import ClaimValueId from "@/partials/claimvalue/ClaimValueId.vue"
import ClaimValueRef from "@/partials/claimvalue/ClaimValueRef.vue"
import ClaimValueText from "@/partials/claimvalue/ClaimValueText.vue"
import ClaimValueString from "@/partials/claimvalue/ClaimValueString.vue"
import ClaimValueAmount from "@/partials/claimvalue/ClaimValueAmount.vue"
import ClaimValueAmountRange from "@/partials/claimvalue/ClaimValueAmountRange.vue"
import ClaimValueRel from "@/partials/claimvalue/ClaimValueRel.vue"
import ClaimValueFile from "@/partials/claimvalue/ClaimValueFile.vue"
import ClaimValueNone from "@/partials/claimvalue/ClaimValueNone.vue"
import ClaimValueUnknown from "@/partials/claimvalue/ClaimValueUnknown.vue"
import ClaimValueTime from "@/partials/claimvalue/ClaimValueTime.vue"
import ClaimValueTimeRange from "@/partials/claimvalue/ClaimValueTimeRange.vue"
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

const abortController = new AbortController()

onBeforeUnmount(() => {
  abortController.abort()
})

async function onEdit(id: string) {
  if (abortController.signal.aborted) {
    return
  }

  $emit("editClaim", id)
}

async function onRemove(id: string) {
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
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueId :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.ref" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueRef :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.text" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueText :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.string" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueString :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.amount" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueAmount :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.amountRange" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueAmountRange :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.rel" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueRel :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.file" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueFile :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.none" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueNone :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.unknown" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueUnknown :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.time" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueTime :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
  <template v-for="claim in claims.timeRange" :key="claim.id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
        ><DocumentRefInline :id="claim.prop.id"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><ClaimValueTimeRange :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
</template>
