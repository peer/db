<!--
The permission actions to ask for or to grant, as a checkbox per action with the hint describing what
it allows under it. The hint is part of the action's label, so clicking it toggles the action as well.
Choosing an action chooses what it requires with it, and removing one removes everything which requires
it (see permissionActionsWith), so the chosen actions always make sense together.

It is one input of the enclosing form: at least one action has to be chosen, which is checked when
focus leaves the checkboxes and when the form validates them all, like any other required input.
-->

<script setup lang="ts">
import type { ValidatedInput, ValidationError } from "@/types"

import { computed, nextTick, ref, useId, useTemplateRef } from "vue"
import { useI18n } from "vue-i18n"

import CheckBox from "@/components/CheckBox.vue"
import { permissionActions, permissionActionsWith, permissionActionsWithout } from "@/permissions"
import { pickErrorMessage, useRegisterForValidation } from "@/validation"

const props = defineProps<{
  // Id of the element naming the checkboxes, so they are a named group.
  labelledby?: string
  // The actions to offer, by their document IDs. Absent means all of them. An action left out is not
  // chosen even when another action requires it: the caller decides what the choices mean (an access
  // request leaves out what the user already holds and closes the chosen set itself before sending).
  available?: string[]
}>()

const model = defineModel<string[]>({ required: true })

const { t } = useI18n({ useScope: "global" })

const actions = computed(() => permissionActions.filter((action) => isAvailable(action.id)))

function isAvailable(action: string): boolean {
  return props.available === undefined || props.available.includes(action)
}

const rootRef = useTemplateRef<HTMLElement>("rootRef")

const baseId = useId()
const errorId = useId()
// A checkbox is named by its action's label alone and described by its hint, which its own label
// contains as well: without them the name would be the label and the hint run together.
const labelId = (action: string) => `${baseId}-label-${action}`
const hintId = (action: string) => `${baseId}-hint-${action}`

const errors = ref<ValidationError[]>([])
const errorMessage = computed(() => pickErrorMessage(errors.value, t))

// eslint-disable-next-line @typescript-eslint/require-await
async function validate(): Promise<void> {
  errors.value = model.value.length === 0 ? [{ code: "required" }] : []
}

const checkpointed = ref<string[]>([...model.value])

function sameActions(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((action) => b.includes(action))
}

const validatedInput: ValidatedInput = {
  validate,
  reset: () => {
    model.value = []
    errors.value = []
  },
  revert: () => {
    model.value = [...checkpointed.value]
    errors.value = []
  },
  inputEl: () => rootRef.value?.querySelector<HTMLInputElement>("input") ?? null,
  mainEl: () => rootRef.value,
  isDirty: computed(() => !sameActions(model.value, checkpointed.value)),
  isEmpty: computed(() => model.value.length === 0),
  errors,
  checkpoint: () => {
    checkpointed.value = [...model.value]
  },
}

const { onInteraction } = useRegisterForValidation(validatedInput)

defineExpose(validatedInput)

// The required error clears on the first interaction and comes back, when nothing is chosen after all,
// once focus leaves the checkboxes, so a form which has not been touched yet does not light up.
function onToggle(action: string, checked: boolean): void {
  const chosen = checked ? permissionActionsWith(model.value, action) : permissionActionsWithout(model.value, action)
  model.value = [...chosen].filter((chosenAction) => isAvailable(chosenAction))
  errors.value = []
  onInteraction()
}

// The nextTick is needed because focusout fires while document.activeElement is still in transition.
async function onFocusout(): Promise<void> {
  await nextTick()
  if (rootRef.value?.contains(document.activeElement)) {
    return
  }
  await validate()
}
</script>

<template>
  <div
    ref="rootRef"
    role="group"
    :aria-labelledby="labelledby"
    :aria-describedby="errorMessage ? errorId : undefined"
    class="pd-permissionactions flex flex-col gap-y-2"
    @focusout="onFocusout"
  >
    <!-- The grid's second column aligns the hint under the label's text. -->
    <label v-for="option of actions" :key="option.id" class="grid cursor-pointer grid-cols-[auto_1fr] items-center gap-x-2 leading-none">
      <CheckBox
        :id="`${baseId}-${option.id}`"
        :model-value="model.includes(option.id)"
        :invalid="errors.length > 0"
        :aria-labelledby="labelId(option.id)"
        :aria-describedby="hintId(option.id)"
        @update:model-value="onToggle(option.id, $event as boolean)"
      />
      <span :id="labelId(option.id)">{{ option.label(t) }}</span>
      <span :id="hintId(option.id)" class="col-start-2 mt-1 text-sm text-neutral-500 italic">{{ option.hint(t) }}</span>
    </label>
    <p v-if="errorMessage" :id="errorId" class="text-sm text-error-600">{{ errorMessage }}</p>
  </div>
</template>
