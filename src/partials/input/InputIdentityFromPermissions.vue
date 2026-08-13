<!--
InputIdentityFromPermissions names one user of the site by their subject, picked from the users the
document being edited grants access to, instead of typed like InputIdentity takes it. The users come
from the document's own permission claims (see usersWithDocumentPermission): somebody who can read
the document through their roles was never added to it and is not offered, so the list is the people
put on this document.

The role prop narrows the list further to users who also hold that site role, which is what turns it
into a list of, say, the researchers on an application. Their roles come from the user API (see
UserGetAPI in user.go), so a user the site cannot describe is left out of a role-filtered list: there
is no role to confirm.

A user another entry of the field already names is not offered, when the field cannot name the same
user twice (see takenValuesKey): the whole point of the list is that everything on it can be picked.

The value stays in the list even when it is not (or no longer) among those users, so a selection is
never invisible, and the selected option can be clicked again to clear it (see RadioButton).
-->

<script setup lang="ts">
import type { Identity, ValidationError, ValidatorFn } from "@/types"

import { computed, inject, onBeforeUnmount, ref, shallowRef, useId, useTemplateRef, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import RadioButton from "@/components/RadioButton.vue"
import { ACTION_READ } from "@/core"
import { documentClaimsKey, takenValuesKey } from "@/fields"
import IdentityInline from "@/partials/IdentityInline.vue"
import { usersWithDocumentPermission } from "@/permissions"
import { useLock } from "@/progress"
import { useValidation } from "@/validation"

const props = withDefaults(
  defineProps<{
    readonly?: boolean
    required?: boolean
    // Presentational override.
    invalid?: boolean
    // Offer only users holding this site role. Empty offers every user with access.
    role?: string
  }>(),
  {
    readonly: false,
    required: false,
    invalid: false,
    role: "",
  },
)

const model = defineModel<string>({ default: "" })

// completeChange reports that the value is a complete decision on its own: picking somebody from a
// list is not followed by a natural blur, so the enclosing slot commits it right away (see
// FieldsFormRow).
const emit = defineEmits<{ errors: [ValidationError[]]; completeChange: [] }>()

const { t } = useI18n({ useScope: "global" })
const router = useRouter()

const errors = ref<ValidationError[]>([])
watch(errors, (v) => emit("errors", v), { flush: "sync" })

// Data modification and controls; useValidation writes to this lock during validation. An active
// enclosing lock behaves like a soft readonly: the options stay focusable but cannot be changed.
const lock = useLock()
const inactive = computed(() => lock.value > 0 || props.readonly)

const baseId = useId()
const fieldsetRef = useTemplateRef<HTMLElement>("fieldsetRef")

// Local lookup progress, like InputIdentity's: resolving the roles drives this input's own progress
// bar and not the enclosing form's.
const lookupProgress = ref(0)

const abortController = new AbortController()
onBeforeUnmount(() => {
  abortController.abort()
})

// The users the document grants the read action to. Read through the injection so the list follows
// the session: granting somebody access on the permissions tab adds them here without a reload.
const documentClaims = inject(documentClaimsKey, null)
const users = computed<string[]>(() => {
  const claims = documentClaims?.()
  return claims ? usersWithDocumentPermission(claims, ACTION_READ) : []
})

// Roles per user, looked up only while a role is asked for: without one every user with access is
// offered and the labels resolve the users themselves (see IdentityInline). A user missing here
// either has not been looked up yet or could not be, and is not offered while filtering.
const userRoles = shallowRef<Map<string, readonly string[]>>(new Map())
const loading = ref(false)

watch(
  [users, () => props.role],
  async ([subjects, role]) => {
    if (!role) {
      userRoles.value = new Map()
      return
    }
    loading.value = true
    try {
      const loaded = new Map<string, readonly string[]>()
      await Promise.all(
        subjects.map(async (subject) => {
          try {
            const response = await getURL<Identity>(router.apiResolve({ name: "UserGet", params: { id: subject } }).href, null, abortController.signal, lookupProgress)
            if (abortController.signal.aborted || response === null) {
              return
            }
            loaded.set(subject, response.doc.roles)
          } catch (err) {
            if (abortController.signal.aborted) {
              return
            }
            // The user cannot be described, so their role cannot be confirmed and they are left out.
            console.error("InputIdentityFromPermissions.roles", err)
          }
        }),
      )
      if (abortController.signal.aborted) {
        return
      }
      userRoles.value = loaded
    } finally {
      loading.value = false
    }
  },
  { immediate: true },
)

// The users another entry of the field already names, when the field cannot name the same user
// twice. They are not offered here: naming one of them is a duplicate, and the user would only be
// told so on save.
const takenValues = inject(takenValuesKey, null)
const taken = computed<ReadonlySet<string>>(() => {
  const values = new Set(takenValues?.() ?? [])
  // This input's own value is among the field's values and is not somebody else's.
  values.delete(model.value)
  return values
})

// The users the input would offer if the field's other entries named nobody.
const candidates = computed<string[]>(() => (props.role ? users.value.filter((user) => userRoles.value.get(user)?.includes(props.role)) : users.value))

const options = computed<string[]>(() => {
  const offered = candidates.value.filter((user) => !taken.value.has(user))
  // A value naming somebody the document no longer grants access to (or who does not hold the role)
  // is still what the field holds, so it stays selectable and clearable.
  return model.value !== "" && !offered.includes(model.value) ? [model.value, ...offered] : offered
})

// Whether the other entries are the reason there is nobody left to offer, which is a different thing
// to tell the user than a document with nobody on it: here they have named everybody already.
const allTaken = computed<boolean>(() => options.value.length === 0 && candidates.value.length > 0)

// The radio's model. Clicking the selected option clears it, which RadioButton reports as undefined.
const selected = computed<string | undefined>({
  get: () => (model.value === "" ? undefined : model.value),
  set: (v) => {
    model.value = v ?? ""
    emit("completeChange")
    // Eager pass, so picking somebody clears a "Required value." from the previous empty state at
    // once, without flagging anything mid-interaction.
    void runValidation({ eager: true })
  },
})

// Empty is invalid only when the input is required, flagged on the lazy pass (leaving the input) and
// on the form-wide pass, never while the user is still choosing.
// eslint-disable-next-line @typescript-eslint/require-await
const validator: ValidatorFn<string> = async function (value, options) {
  if (!props.required || options.initial || options.eager) {
    return []
  }
  // TODO: Use standard codes.
  return value === "" ? [{ code: "required" }] : []
}

const { runValidation, validatedInput } = useValidation(
  model,
  errors,
  lock,
  () => validator,
  // Focus target: the selected option when there is one, the first option otherwise.
  () => fieldsetRef.value?.querySelector<HTMLElement>("input:checked") ?? fieldsetRef.value?.querySelector<HTMLElement>("input") ?? null,
  () => {
    model.value = ""
    errors.value = []
  },
)

defineExpose(validatedInput)

// Restores the focus the prevented mousedown suppressed, so focus lands inside the fieldset and
// leaving it later runs the lazy validation. The mousedown is prevented because focusing a control
// commits the slot the click is still on its way to (see ClaimRefSelect, which does the same).
function focusOption(user: string): void {
  document.getElementById(`${baseId}-${user}`)?.focus()
}

function onFocusout(event: FocusEvent) {
  // Moving between the options is not leaving the input.
  if (fieldsetRef.value?.contains(event.relatedTarget as Node | null)) {
    return
  }
  void runValidation()
}
</script>

<template>
  <fieldset ref="fieldsetRef" class="pd-inputidentityfrompermissions" :class="{ 'pd-locked': lock > 0 }" @focusout="onFocusout">
    <ul v-if="options.length > 0" class="pd-inputidentityfrompermissions-list grid grid-cols-[max-content_auto] gap-x-1">
      <li v-for="user of options" :key="user" class="pd-inputidentityfrompermissions-item contents" :class="`pd-inputidentityfrompermissions-item-${user}`">
        <RadioButton
          :id="`${baseId}-${user}`"
          v-model="selected"
          class="pd-inputidentityfrompermissions-radio"
          :class="`pd-inputidentityfrompermissions-radio-${user}`"
          :name="baseId"
          :value="user"
          :disabled="inactive"
          :invalid="invalid || errors.length > 0"
          @mousedown.prevent
          @click="focusOption(user)"
        />
        <label
          :for="`${baseId}-${user}`"
          class="pd-inputidentityfrompermissions-label"
          :class="inactive ? 'cursor-not-allowed text-gray-600' : 'cursor-pointer'"
          @mousedown.prevent
          @click="focusOption(user)"
          ><IdentityInline :subject="user"
        /></label>
      </li>
    </ul>
    <i v-else-if="loading" class="pd-inputidentityfrompermissions-loading text-gray-500">{{ t("common.status.loading") }}</i>
    <i v-else-if="allTaken" class="pd-inputidentityfrompermissions-empty-alltaken text-gray-500">{{ t("partials.input.InputIdentityFromPermissions.noMoreUsers") }}</i>
    <i v-else class="pd-inputidentityfrompermissions-empty text-gray-500">{{ t("partials.input.InputIdentityFromPermissions.noUsers") }}</i>
  </fieldset>
</template>
