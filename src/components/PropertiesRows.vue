<script setup lang="ts">
defineProps({
  properties: {
    type: Object,
    default() {
      return {}
    },
  },
})
</script>

<template>
  <tr v-for="claim in properties.id" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">{{ claim.id }}</td>
  </tr>
  <tr v-for="claim in properties.ref" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <a :href="claim.iri" class="link">{{ claim.iri }}</a>
    </td>
  </tr>
  <tr v-for="claim in properties.text" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <iframe :srcdoc="claim.html?.en" class="w-full"></iframe>
    </td>
  </tr>
  <tr v-for="claim in properties.string" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">{{ claim.string }}</td>
  </tr>
  <tr v-for="claim in properties.label" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">label</td>
    <td class="border-t border-l border-slate-200 px-2 py-1">{{ claim.prop?.name?.en }}</td>
  </tr>
  <tr v-for="claim in properties.amount" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      {{ claim.amount }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template>
    </td>
  </tr>
  <tr v-for="claim in properties.amountRange" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      {{ claim.lower }}-{{ claim.upper }}<template v-if="claim.unit !== '1'"> {{ claim.unit }}</template>
    </td>
  </tr>
  <tr v-for="claim in properties.enum" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      {{ claim.enum.join(", ") }}
    </td>
  </tr>
  <tr v-for="claim in properties.rel" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.to?._id } }" class="link">{{ claim.to?.name?.en }}</router-link>
    </td>
  </tr>
  <tr v-for="claim in properties.file" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <a v-if="claim.preview?.length > 0" :href="claim.url">
        <img :src="claim.preview[0]" />
      </a>
      <a v-else :href="claim.url" class="link">{{ claim.type }}</a>
    </td>
  </tr>
  <tr v-for="claim in properties.none" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1 italic">none</td>
  </tr>
  <tr v-for="claim in properties.unknown" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1 italic">unknown</td>
  </tr>
  <tr v-for="claim in properties.time" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">{{ claim.timestamp }}</td>
  </tr>
  <tr v-for="claim in properties.timeRange" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">{{ claim.lower }}-{{ claim.upper }}</td>
  </tr>
  <tr v-for="claim in properties.is" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">is</td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.to?._id } }" class="link">{{ claim.to?.name?.en }}</router-link>
    </td>
  </tr>
  <tr v-for="claim in properties.list" :key="claim._id">
    <td class="border-t border-r border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.prop?._id } }" class="link">{{ claim.prop?.name?.en }}</router-link>
    </td>
    <td class="border-t border-l border-slate-200 px-2 py-1">
      <router-link :to="{ name: 'DocumentGet', params: { id: claim.el?._id } }" class="link">{{ claim.el?.name?.en }}</router-link>
    </td>
  </tr>
</template>
