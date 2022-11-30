<script setup lang="ts">
import type { DeepReadonly } from "vue"
import type { ClaimTypes } from "@/types"

import RouterLink from "@/components/RouterLink.vue"
import WithDocument from "@/components/WithDocument.vue"
import { getName, loadingLength } from "@/utils"

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
</script>

<template>
  <template v-for="(claim, i) in claims.id" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`id/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.id }}</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.ref" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`ref/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="break-all border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <a :href="claim.iri" class="link">{{ claim.iri }}</a>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.text" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`text/${level}`, i)]"></div></template>
        </WithDocument>
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
  <template v-for="(claim, i) in claims.string" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`string/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.string }}
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.amount" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`amount/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.amount }} <template v-if="claim.unit !== '1'">{{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.amountRange" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`amountRange/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.lower }}-{{ claim.upper }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.rel" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`rel/prop/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <WithDocument :id="claim.to._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.to._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`rel/to/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.file" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`file/${level}`, i)]"></div></template>
        </WithDocument>
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
  <template v-for="(claim, i) in claims.none" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`none/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">none</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.unknown" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`unknown/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">unknown</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.time" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`time/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.timestamp }}
      </td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
  <template v-for="(claim, i) in claims.timeRange" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <WithDocument :id="claim.prop._id">
          <template #default="{ doc }">
            <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop._id } }" class="link" v-html="getName(doc.claims) || '<i>no name</i>'"></RouterLink>
          </template>
          <template #loading><div class="inline-block h-2 animate-pulse rounded bg-slate-200" :class="[loadingLength(`timeRange/${level}`, i)]"></div></template>
        </WithDocument>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.lower }}-{{ claim.upper }}</td>
    </tr>
    <PropertiesRows :claims="claim.meta" :level="level + 1" />
  </template>
</template>
