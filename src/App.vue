<script setup lang="ts">
import { authEpoch } from "@/auth"
</script>

<template>
  <!--
    Keyed on authEpoch so that an in-app identity change (sign-out) remounts the
    whole component tree, forcing every component to refetch under the new roles.
  -->
  <router-view :key="authEpoch"></router-view>
</template>
