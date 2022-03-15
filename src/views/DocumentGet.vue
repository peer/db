<script setup lang="ts">
defineProps({
  id: {
    type: String,
    required: true,
  },
})
</script>

<template>
  <p>document {{ id }}</p>
</template>
