<script setup lang="ts">
import type { DeepReadonly } from "vue"

import Button from "@/components/Button.vue"
import { ClaimTypes } from "@/document"
import Id from "@/partials/claimvalue/Id.vue"
import Ref from "@/partials/claimvalue/Ref.vue"
import Text from "@/partials/claimvalue/Text.vue"
import String from "@/partials/claimvalue/String.vue"
import Amount from "@/partials/claimvalue/Amount.vue"
import AmountRange from "@/partials/claimvalue/AmountRange.vue"
import Rel from "@/partials/claimvalue/Rel.vue"
import File from "@/partials/claimvalue/File.vue"
import None from "@/partials/claimvalue/None.vue"
import Unknown from "@/partials/claimvalue/Unknown.vue"
import Time from "@/partials/claimvalue/Time.vue"
import TimeRange from "@/partials/claimvalue/TimeRange.vue"
import ClaimProp from "@/partials/ClaimProp.vue"

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

async function onEdit(id: string) {
  $emit("editClaim", id)
}

async function onRemove(id: string) {
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><Id :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="break-all border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><Ref :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="prose prose-slate max-w-none border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        ><Text :claim="claim"
      /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><String :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><Amount :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><AmountRange :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><Rel :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><File :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic"><None :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic"><Unknown :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><Time :claim="claim" /></td>
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
        ><ClaimProp :claim="claim"
      /></td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"><TimeRange :claim="claim" /></td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
</template>
