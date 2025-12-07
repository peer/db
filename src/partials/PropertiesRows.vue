<script setup lang="ts">
import type { DeepReadonly } from "vue"
import { PeerDBDocument } from "@/document"

import Button from "@/components/Button.vue"
import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingWidth } from "@/utils"
import { ClaimTypes } from "@/document"
import ClaimValue from "@/partials/ClaimValue.vue"

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

const WithPeerDBDocument = WithDocument<PeerDBDocument>

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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="id" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="break-all border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="ref" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <!-- eslint-disable vue/no-v-html -->
      <td class="prose prose-slate max-w-none border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="text" />
      </td>
      <!-- eslint-enable vue/no-v-html -->
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="string" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="amount" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="amountRange" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="rel" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="file" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">
        <ClaimValue :claim="claim" type="none" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">
        <ClaimValue :claim="claim" type="unknown" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="time" />
      </td>
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
      >
        <WithPeerDBDocument :id="claim.prop.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.prop.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.prop.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <ClaimValue :claim="claim" type="timeRange" />
      </td>
      <td v-if="editable" class="flex flex-row gap-1 ml-2" :class="{ 'text-sm': level > 0 }">
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onEdit(claim.id)">Edit</Button>
        <Button type="button" class="!px-3.5 !py-1" @click.prevent="onRemove(claim.id)">Remove</Button>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" :editable="editable" />
  </template>
</template>
