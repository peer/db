<!--
We do not use :read-only or :disabled pseudo classes to style the component because
we want component to retain how it visually looks even if DOM element's read-only or
disabled attributes are set, unless they are set through component's props.
This is used during transitions/animations to disable the component by directly setting
its DOM attributes without flickering how the component looks.
-->

<script setup lang="ts" generic="T">
import type { SelectButtonOption } from "@/types"

import { useSlots } from "vue"

const props = defineProps<{
  // Options cannot have an option with name "default" because it is
  // reserved for the default slot.
  options: SelectButtonOption<T>[]
}>()

const model = defineModel<T>({ required: true })

// We do not define slots using defineSlots because slots are dynamic based on the options.
const allOptions = new Set(props.options.map((option) => option.name))

// But we check at runtime if there are any extra slots used which do not have a corresponding option.
for (const slot in useSlots()) {
  if (slot === "default") {
    continue
  }
  if (!allOptions.has(slot)) {
    throw new Error(`slot '${slot}' used, but there is no corresponding option`)
  }
}

function isDisabled(option: SelectButtonOption<T>) {
  return (option.progress ?? 0) > 0 || option.disabled
}
</script>

<template>
  <div class="pd-selectbutton flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
    <button
      v-for="option in options"
      :key="option.name"
      :disabled="isDisabled(option)"
      class="h-full rounded-sm px-2 py-0.5"
      :class="{
        'cursor-not-allowed text-slate-500': isDisabled(option),
        'bg-white shadow-xs': model === option.value && !isDisabled(option),
        'bg-slate-100 shadow-xs': model === option.value && isDisabled(option),
        'hover:bg-slate-100': model !== option.value && !isDisabled(option),
      }"
      @click.prevent="model = option.value"
    >
      <!-- You can use a named slot to control contents of a particular option button (based on option's name). -->
      <slot :option="option" :selected="model === option.value" :name="option.name">
        <!-- Or you can use a default slot to control contents of all option buttons (which do not have a named slot set). -->
        <slot :option="option" :selected="model === option.value">
          <component :is="option.icon.component" v-if="option.icon" :alt="option.icon.alt" class="size-6" />
        </slot>
      </slot>
    </button>
  </div>
</template>
