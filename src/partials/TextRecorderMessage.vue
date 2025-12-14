<script setup lang="ts">
import type { TextRecorderMessage } from '@/types';
import { DeepReadonly } from 'vue';

defineProps<{
  message: DeepReadonly<TextRecorderMessage>
}>()
</script>

<template>
  <div v-if="message.role !== 'system'" class="bg-white p-2 rounded border">
    <div class="whitespace-pre-line"><span class="font-bold">{{ message.role }}:</span> <span v-if="message.toolUseName">{{ message.toolUseName }}(</span>{{ message.content }}<span v-if="message.toolUseName">)</span></div>
  </div>
</template>