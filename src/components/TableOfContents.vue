<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue"

const props = defineProps<{
  // List of sections in document order. Each section must have a corresponding DOM element with the given id.
  sections: { id: string; label: string }[]
}>()

// Index of the current section (the last section whose top has scrolled past the viewport top).
// -1 means no section has been scrolled past yet.
const currentIndex = ref(-1)

function update() {
  // First section is always current if sections exist.
  // Other sections become current when their top scrolls past the top of the viewport.
  let found = props.sections.length > 0 ? 0 : -1
  for (let i = 1; i < props.sections.length; i++) {
    const el = document.getElementById(props.sections[i].id)
    if (el) {
      const rect = el.getBoundingClientRect()
      if (rect.top <= 0) {
        found = i
      }
    }
  }
  currentIndex.value = found
}

// Sections that have been scrolled past or are current (shown at the top of the TOC).
const passedSections = computed(() => {
  if (currentIndex.value < 0) return []
  return props.sections.slice(0, currentIndex.value + 1)
})

// Sections that haven't been reached yet (shown at the bottom of the TOC).
const upcomingSections = computed(() => {
  return props.sections.slice(currentIndex.value + 1)
})

function scrollTo(id: string) {
  const el = document.getElementById(id)
  if (el) {
    el.scrollIntoView({ behavior: "smooth", block: "start" })
  }
}

// Re-evaluate when sections change (e.g., new results loaded).
watch(
  () => props.sections,
  () => update(),
)

onMounted(() => {
  window.addEventListener("scroll", update, { passive: true })
  window.addEventListener("resize", update, { passive: true })
  // Initial position check.
  update()
})

onBeforeUnmount(() => {
  window.removeEventListener("scroll", update)
  window.removeEventListener("resize", update)
})
</script>

<template>
  <nav class="sticky top-16 flex h-[calc(100vh-5rem)] flex-col justify-between py-2">
    <!-- Passed/current sections (top group). -->
    <div class="flex flex-col gap-y-1">
      <button
        v-for="section in passedSections"
        :key="section.id"
        class="shrink-0 cursor-pointer truncate rounded-sm px-2 py-1 text-left text-sm hover:bg-neutral-100"
        :class="section.id === sections[currentIndex]?.id ? 'font-bold' : ''"
        @click="scrollTo(section.id)"
      >
        {{ section.label }}
      </button>
    </div>
    <!-- Upcoming sections (bottom group). -->
    <div class="flex flex-col gap-y-1">
      <button
        v-for="section in upcomingSections"
        :key="section.id"
        class="shrink-0 cursor-pointer truncate rounded-sm px-2 py-1 text-left text-sm text-neutral-400 hover:bg-neutral-100 hover:text-neutral-600"
        @click="scrollTo(section.id)"
      >
        {{ section.label }}
      </button>
    </div>
  </nav>
</template>
