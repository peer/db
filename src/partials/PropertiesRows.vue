<script setup lang="ts">
import type { DeepReadonly } from "vue"
import type { ClaimTypes, PeerDBDocument } from "@/types"

import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingWidth } from "@/utils"

withDefaults(
  defineProps<{
    claims?: DeepReadonly<ClaimTypes>
    level?: number
  }>(),
  {
    claims: () => ({}),
    level: 0,
  },
)

const WithPeerDBDocument = WithDocument<PeerDBDocument>
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
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.id }}</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        <a :href="claim.iri" class="link">{{ claim.iri }}</a>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
      <td
        class="prose prose-slate max-w-none border-l border-slate-200 px-2 py-1 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        v-html="claim.html?.en"
      ></td>
      <!-- eslint-enable vue/no-v-html -->
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        {{ claim.string }}
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        {{ claim.amount }} <template v-if="claim.unit !== '1'">{{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        {{ claim.lower }}-{{ claim.upper }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        <WithPeerDBDocument :id="claim.to.id" name="DocumentGet">
          <template #default="{ doc, url }">
            <RouterLink
              :to="{ name: 'DocumentGet', params: { id: claim.to.id } }"
              :data-url="url"
              class="link"
              v-html="getName(doc.claims) || '<i>no name</i>'"
            ></RouterLink>
          </template>
          <template #loading="{ url }">
            <div class="inline-block h-2 animate-pulse rounded bg-slate-200" :data-url="url" :class="[loadingWidth(claim.to.id)]"></div>
          </template>
        </WithPeerDBDocument>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        <a v-if="claim.preview?.[0]" :href="claim.url">
          <img :src="claim.preview[0]" />
        </a>
        <a v-else :href="claim.url" class="link">{{ claim.type }}</a>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">none</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">unknown</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
        {{ claim.timestamp }}
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
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
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.lower }}-{{ claim.upper }}</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
</template>
