<script setup lang="ts" generic="T">
import { SelectButtonOption } from "@/types"

defineProps<{
  modelValue: T
  // Options cannot have an option with name "default" because it is
  // reserved for the default slot.
  options: SelectButtonOption<T>[]
}>()

const $emit = defineEmits<{
  "update:modelValue": [value: T]
}>()

// We do not define slots using defineSlots because slots are dynamic based on the options.
</script>

<template>
  <div class="flex gap-1 items-center bg-slate-200 py-1 px-2 rounded">
    <button
      v-for="option in options"
      :key="option.name"
      :disabled="(option.progress || 0) > 0 || option.disabled"
      class="py-0.5 px-2 rounded"
      :class="{
        'bg-white shadow-sm disabled:bg-slate-100': modelValue === option.value,
        'enabled:hover:bg-slate-100 disabled:': modelValue !== option.value,
        'disabled:text-slate-300': (option.progress || 0) > 0 || option.disabled,
      }"
      @click.prevent="$emit('update:modelValue', option.value)"
    >
      <slot :option="option" :selected="modelValue === option.value" :name="option.name">
        <slot :option="option" :selected="modelValue === option.value">
          <component :is="option.icon.component" v-if="option.icon" :alt="option.icon.alt" class="w-7 h-7" />
        </slot>
      </slot>
    </button>
  </div>
</template>
