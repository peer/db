<script setup lang="ts" generic="T">
import { FunctionalComponent, HTMLAttributes, VNodeProps } from "vue"

export type SelectButtonOptionsIconProp = {
  component: FunctionalComponent<HTMLAttributes & VNodeProps>
  alt: string
}

export type SelectButtonOptionsProp<T> = {
  icon?: SelectButtonOptionsIconProp
  name?: string
  disabled?: boolean
  value: T
}

export type SelectButtonProps<T> = {
  modelValue: T | null
  options: SelectButtonOptionsProp<T>[]
}

const props = defineProps<SelectButtonProps<T>>()
const $emit = defineEmits<{
  "update:modelValue": [value: T]
}>()

defineSlots<{
  option(props: { option: SelectButtonOptionsProp<T> }): unknown
}>()
</script>

<template>
  <div class="flex gap-1 items-center bg-slate-200 py-1 px-2 rounded">
    <button
      v-for="option in props.options"
      :key="option.value as PropertyKey"
      :disabled="!!option.disabled"
      class="py-0.5 px-2 rounded"
      :class="{
        'bg-white shadow-sm disabled:bg-slate-100': props.modelValue === option.value,
        'enabled:hover:bg-slate-100 disabled:': props.modelValue !== option.value,
        'disabled:text-slate-300': option.disabled,
      }"
      @click.prevent="$emit('update:modelValue', option.value)"
    >
      <slot :option="option" name="option">
        <component :is="option.icon?.component" v-if="option.icon" v-bind="{ alt: option.icon.alt }" class="w-6 h-6" />
        <template v-else-if="option.name">{{ option.name }}</template>
      </slot>
    </button>
  </div>
</template>
