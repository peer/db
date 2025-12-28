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
</script>

<template>
  <div class="flex items-center gap-1 rounded-sm bg-slate-200 px-1 py-1">
    <button
      v-for="option in options"
      :key="option.name"
      :disabled="(option.progress || 0) > 0 || option.disabled"
      class="rounded-sm px-2 py-0.5"
      :class="{
        'bg-white shadow-xs disabled:bg-slate-100': model === option.value,
        'disabled: enabled:hover:bg-slate-100': model !== option.value,
        'disabled:text-slate-300': (option.progress || 0) > 0 || option.disabled,
      }"
      @click.prevent="model = option.value"
    >
      <!-- You can use a named slot to control contents of a particular option button (based on option's name). -->
      <slot :option="option" :selected="model === option.value" :name="option.name">
        <!-- Or you can use a default slot to control contents of all option buttons (which do not have a named slot set). -->
        <slot :option="option" :selected="model === option.value">
          <component :is="option.icon.component" v-if="option.icon" :alt="option.icon.alt" class="h-7 w-7" />
        </slot>
      </slot>
    </button>
  </div>
</template>
