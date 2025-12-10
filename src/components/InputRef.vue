<script setup lang="ts">
import { Combobox, ComboboxInput, ComboboxOption, ComboboxOptions } from "@headlessui/vue"
import { computed, ref } from "vue"

withDefaults(defineProps<{ id: string }>(), {})

const people = [
  { id: 1, name: "Durward Reynolds", unavailable: false },
  { id: 2, name: "Kenton Towne", unavailable: false },
  { id: 3, name: "Therese Wunsch", unavailable: false },
  { id: 4, name: "Benedict Kessler", unavailable: true },
  { id: 5, name: "Katelyn Rohan", unavailable: false },
]
const selectedPerson = ref(people[0])
const query = ref("")

const filteredPeople = computed(() =>
  query.value === ""
    ? people
    : people.filter((person) => {
        return person.name.toLowerCase().includes(query.value.toLowerCase())
      }),
)
</script>

<template>
  <Combobox v-model="selectedPerson" as="div" class="w-full">
    <div class="relative">
      <ComboboxInput
        class="w-full cursor-pointer p-2 bg-white text-left rounded border-0 shadow ring-2 ring-neutral-300 focus:ring-2"
        :display-value="(person) => person.name"
        @input="query = $event.target.value"
      />

      <ComboboxOptions
        v-if="filteredPeople.length > 0"
        class="absolute max-h-40 overflow-scroll mt-2 w-full bg-white rounded border-0 shadow ring-2 ring-neutral-300 z-10"
      >
        <ComboboxOption v-for="person in filteredPeople" v-slot="{ active }" :key="person.id" :disabled="person.unavailable" :value="person" as="template">
          <li class="cursor-pointer p-2" :class="active ? 'bg-neutral-100' : ''">
            {{ person.name }}
          </li>
        </ComboboxOption>
      </ComboboxOptions>
    </div>
  </Combobox>
</template>
