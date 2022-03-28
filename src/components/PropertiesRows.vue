<script setup lang="ts">
import RouterLink from "@/components/RouterLink.vue"

defineProps({
  properties: {
    type: Object,
    default() {
      return {}
    },
  },
  level: {
    type: Number,
    default: 0,
  },
})
</script>

<template>
  <template v-for="claim in properties.id" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.id }}</td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.ref" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="break-all border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <a :href="claim.iri" class="link">{{ claim.iri }}</a>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.text" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <iframe :srcdoc="claim.html?.en" class="w-full"></iframe>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.string" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.string }}
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.label" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        label
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.prop?.name?.en }}
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.amount" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.amount }} <template v-if="claim.unit !== '1'">{{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.amountRange" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.lower }}-{{ claim.upper }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.enum" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.enum.join(", ") }}
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.rel" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.to?._id } }" class="link">{{ claim.to?.name?.en }}</RouterLink>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.file" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <a v-if="claim.preview?.length > 0" :href="claim.url">
          <img :src="claim.preview[0]" />
        </a>
        <a v-else :href="claim.url" class="link">{{ claim.type }}</a>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.none" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">none</td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.unknown" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-t border-l border-slate-200 px-2 py-1 align-top italic">unknown</td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.time" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        {{ claim.timestamp }}
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.timeRange" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">{{ claim.lower }}-{{ claim.upper }}</td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.is" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        is
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.to?._id } }" class="link">{{ claim.to?.name?.en }}</RouterLink>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
  <template v-for="claim in properties.list" :key="claim._id">
    <tr>
      <td
        class="whitespace-nowrap border-r border-slate-200 py-1 pr-2 align-top"
        :class="{ 'border-t': level === 0, 'text-sm': level > 0 }"
        :style="{ 'padding-left': 0.5 + level * 0.75 + 'rem' }"
      >
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</RouterLink>
      </td>
      <td class="border-l border-slate-200 px-2 py-1 align-top" :class="{ 'border-t': level === 0, 'text-sm': level > 0 }">
        <RouterLink :to="{ name: 'DocumentGet', params: { id: claim.el?._id } }" class="link">{{ claim.el?.name?.en }}</RouterLink>
      </td>
    </tr>
    <PropertiesRows :properties="claim.meta" :level="level + 1" />
  </template>
</template>
