<!--
A navbar search shortcut button. Outside a search session (e.g. on Home) it is a normal link to the
SearchShortcut route. Inside the SearchGet view it toggles the current session's prefilters in place:
it shows active when its prefilter is the session's current prefilter, clears it when clicked while
active, and switches the session's prefilters to its payload (replacing any other) when clicked while
inactive. While the session is updating it is disabled. Modified clicks (ctrl/cmd/shift, middle) keep
the normal link behavior so they still load a fresh search session in a new tab/window.
-->

<script setup lang="ts">
import type { QueryValues } from "@/types"

import { computed, inject } from "vue"

import ButtonLink from "@/components/ButtonLink.vue"
import { useLocked } from "@/progress"
import { prefiltersMatch, queryToPrefilterPayloads, searchShortcutControllerKey } from "@/search"

const props = withDefaults(
  defineProps<{
    query: QueryValues
    primary?: boolean
  }>(),
  {
    primary: false,
  },
)

// The SearchGet view provides the controller; it is null on other views, where the shortcut just
// navigates to the SearchShortcut route as a normal link.
const controller = inject(searchShortcutControllerKey, null)
const locked = useLocked()

const payloads = computed(() => queryToPrefilterPayloads(props.query))
const active = computed(() => controller != null && prefiltersMatch(controller.prefilters.value, payloads.value))
// Disable only while SearchGet (the sole controller provider) is busy, so the link stays clickable on
// other views even if that view happens to be busy.
const disabled = computed(() => controller != null && locked.value)

function onClickCapture(event: MouseEvent) {
  if (!controller) {
    // No session to toggle: let ButtonLink navigate to the SearchShortcut route.
    return
  }
  // Leave modified clicks (open in new tab/window) and middle/aux clicks to the normal link behavior,
  // mirroring vue-router's guardEvent, so for example ctrl/cmd-click still loads a fresh search session.
  if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.altKey || event.ctrlKey || event.shiftKey) {
    return
  }
  event.preventDefault()
  if (disabled.value) {
    return
  }
  // Clicking the active shortcut clears the prefilters; clicking an inactive one switches the session's
  // prefilters to this shortcut's payload (replacing any other), so at most one shortcut is active.
  void controller.applyPrefilters(active.value ? null : payloads.value)
}
</script>

<template>
  <ButtonLink :to="{ name: 'SearchShortcut', query }" :active="active" :disabled="disabled" :primary="primary" @click.capture="onClickCapture">
    <slot />
  </ButtonLink>
</template>
