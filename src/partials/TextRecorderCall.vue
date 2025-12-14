<script setup lang="ts">
import type { TextRecorderCall } from '@/types';
import { DeepReadonly } from 'vue';

import TextRecorderMessage from "@/partials/TextRecorderMessage.vue"

defineProps<{
  call: DeepReadonly<TextRecorderCall>
}>()
</script>

<template>
  <TextRecorderMessage v-for="(message, i) in call.messages" :key="i" :message="message"></TextRecorderMessage>
  <div>{{ call.duration }}</div>
</template>