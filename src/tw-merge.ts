// Based on: https://github.com/yakir-reznik/tailwind-merge-vue-directive

import type { App, DirectiveBinding } from "vue"

import { twMerge } from "tailwind-merge"

function computeClasses(el: HTMLElement, binding: DirectiveBinding) {
  const existingClasses = el.classList.value
  const inheritedClasses = binding.instance?.$attrs?.class as string | undefined

  // No need to run tw-merge if there are no classes.
  if (!existingClasses || !inheritedClasses) return

  // This works because all fallthrough classes are added at the end of the string.
  el.classList.value = twMerge(existingClasses, inheritedClasses)
}

export const directive = {
  beforeMount: computeClasses,
  updated: computeClasses,
}

export default {
  install: (app: App) => {
    app.directive("twMerge", directive)
  },
}
