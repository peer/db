<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { FieldData, FieldsData, FieldsFormSaveChange } from "@/fields"

import { XMarkIcon } from "@heroicons/vue/20/solid"
import { Identifier } from "@tozd/identifier"
import { computed, inject, onBeforeUnmount, onMounted, reactive, watch } from "vue"
import { useI18n } from "vue-i18n"

import InputText from "@/components/InputText.vue"
import { ClaimTypes } from "@/document"
import { AddClaimChange, RemoveClaimChange, SetClaimChange } from "@/document/patch"
import {
  fieldKey,
  getExistingClaimValues,
  getNextChangeNumberKey,
  isIntervalField,
  makePatchForField,
  registerForFlushKey,
  saveChangeKey,
  unregisterForFlushKey,
} from "@/fields"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = withDefaults(
  defineProps<{
    fieldsData: FieldsData
    claims: DeepReadonly<ClaimTypes>
    base: DeepReadonly<string[]>
    session: string
    progress?: number
    parentClaimId?: string
  }>(),
  {
    progress: 0,
    parentClaimId: undefined,
  },
)

const invalid = defineModel<boolean>("invalid", { default: false })

const { t } = useI18n({ useScope: "global" })

// Injected services from DocumentEdit.
let fallbackNextChangeNumber = 1
const getNextChangeNumber = inject(getNextChangeNumberKey, () => fallbackNextChangeNumber++)
const saveChange = inject(saveChangeKey, () => Promise.resolve())
const registerForFlush = inject(registerForFlushKey, () => {})
const unregisterForFlush = inject(unregisterForFlushKey, () => {})

// EntryState represents a single claim value being edited.
interface EntryState {
  field: FieldData
  value: string
  valueTo: string
  dirty: boolean
  dirtyTo: boolean
  persisted: boolean
}

// All entries keyed by claim ID (real for existing, pre-computed for new).
const entries = reactive(new Map<string, EntryState>())

// Empty slot local values, keyed by a local counter string per field key.
// These are purely local — no claim ID, no entry in the entries map.
const emptySlots = reactive(new Map<string, { fieldKey: string; field: FieldData; value: string; valueTo: string }>())
let emptySlotCounter = 0

// Track child FieldsForm invalid states, keyed by parent claim ID.
const childInvalid = reactive<Record<string, boolean>>({})

function sortedByOrder<T extends { orderInList: number }>(items: T[]): T[] {
  return [...items].sort((a, b) => a.orderInList - b.orderInList)
}

function entriesForField(field: FieldData): [string, EntryState][] {
  return [...entries].filter(([_, e]) => e.field.propertyId === field.propertyId)
}

function emptySlotsForField(fk: string): [string, { value: string; valueTo: string }][] {
  return [...emptySlots].filter(([_, s]) => s.fieldKey === fk).map(([id, s]) => [id, { value: s.value, valueTo: s.valueTo }])
}

function syncFromDoc() {
  if (!props.claims) {
    // No claims: remove all persisted entries.
    for (const [id, entry] of entries) {
      if (entry.persisted && !entry.dirty && !entry.dirtyTo) {
        entries.delete(id)
      }
    }
    return
  }

  const allFieldsList = allFields()
  const seenClaimIds = new Set<string>()

  for (const field of allFieldsList) {
    const existing = getExistingClaimValues(props.claims, field)
    for (const { claimId, value, valueTo } of existing) {
      seenClaimIds.add(claimId)
      const entry = entries.get(claimId)
      if (entry) {
        // Existing entry: update if not dirty.
        if (!entry.dirty) {
          entry.value = value
        }
        if (!entry.dirtyTo) {
          entry.valueTo = valueTo
        }
        entry.persisted = true
      } else {
        // New claim from doc: create entry.
        entries.set(claimId, {
          field,
          value,
          valueTo,
          dirty: false,
          dirtyTo: false,
          persisted: true,
        })
      }
    }

    // Ensure at least one empty slot for fields with no entries and no existing empty slots.
    const fk = fieldKey(field)
    if (existing.length === 0 && entriesForField(field).length === 0 && emptySlotsForField(fk).length === 0) {
      addEmptySlot(field)
    }
  }

  // Remove persisted entries that no longer exist in doc (deleted elsewhere).
  for (const [id, entry] of entries) {
    if (entry.persisted && !seenClaimIds.has(id) && !entry.dirty && !entry.dirtyTo) {
      entries.delete(id)
    }
  }
}

watch(
  () => props.claims,
  () => {
    syncFromDoc()
  },
  { deep: true, immediate: true },
)

watch(
  () => props.fieldsData,
  () => {
    syncFromDoc()
  },
)

function allFields(): FieldData[] {
  const result: FieldData[] = []
  for (const section of props.fieldsData.sections) {
    result.push(...section.fields)
  }
  result.push(...props.fieldsData.fields)
  return result
}

const ownInvalid = computed(() => {
  for (const field of allFields()) {
    if (field.minCardinality > 0) {
      const fieldEntries = entriesForField(field)
      const nonEmpty = fieldEntries.filter(([_, e]) => e.value.trim() !== "").length
      if (nonEmpty < field.minCardinality) {
        return true
      }
    }
  }
  return false
})

const computedInvalid = computed(() => ownInvalid.value || Object.values(childInvalid).some((v) => v))

// Sync invalid model synchronously so it's always up-to-date when parent reads it after flush().
watch(
  computedInvalid,
  (v) => {
    invalid.value = v
  },
  { immediate: true, flush: "sync" },
)

function onEntryInput(claimId: string, value: string, isTo?: boolean) {
  const entry = entries.get(claimId)
  if (!entry) {
    return
  }
  if (isTo) {
    entry.valueTo = value
    entry.dirtyTo = true
  } else {
    entry.value = value
    entry.dirty = true
  }
}

function onEmptySlotInput(slotId: string, value: string, isTo?: boolean) {
  const slot = emptySlots.get(slotId)
  if (!slot) {
    return
  }
  if (isTo) {
    slot.valueTo = value
  } else {
    slot.value = value
  }
}

async function onEntryBlur(claimId: string) {
  const entry = entries.get(claimId)
  if (!entry || (!entry.dirty && !entry.dirtyTo)) {
    return
  }

  const value = entry.value.trim()
  const valueTo = entry.valueTo.trim()
  const interval = isIntervalField(entry.field)
  const isEmpty = interval ? value === "" && valueTo === "" : value === ""

  if (isEmpty && entry.persisted) {
    // Remove persisted claim.
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: claimId }), num)
    entries.delete(claimId)
    // Child entries are managed by nested FieldsForm instances.
    // They will clean up when their claims prop becomes null/empty via syncFromDoc.
    return
  }

  if (isEmpty) {
    // Empty non-persisted entry: just clean up.
    entries.delete(claimId)
    return
  }

  const patch = makePatchForField(entry.field, value, interval ? valueTo : undefined)

  if (entry.persisted) {
    // Check if actually changed.
    if (props.claims) {
      const existing = getExistingClaimValues(props.claims, entry.field)
      const existingValue = existing.find((e) => e.claimId === claimId)
      if (existingValue && existingValue.value === value && existingValue.valueTo === valueTo) {
        entry.dirty = false
        entry.dirtyTo = false
        return
      }
    }
    const num = getNextChangeNumber()
    await saveChange(new SetClaimChange({ id: claimId, patch }), num)
  } else {
    // Non-persisted entries with dirty values are handled by onEmptySlotBlur which
    // computes the ID and submits the AddClaimChange atomically. If we reach here,
    // it means an entry was created from an empty slot (via onEmptySlotBlur) but
    // then edited again before the server confirmed it. Treat as a no-op since
    // the original add is still in flight.
    entry.dirty = false
    entry.dirtyTo = false
    return
  }

  entry.dirty = false
  entry.dirtyTo = false
}

async function onEmptySlotBlur(slotId: string, field: FieldData) {
  const slot = emptySlots.get(slotId)
  if (!slot) {
    return
  }

  const value = slot.value.trim()
  const valueTo = slot.valueTo.trim()
  const interval = isIntervalField(field)
  const isEmpty = interval ? value === "" && valueTo === "" : value === ""

  if (isEmpty) {
    // Still empty: do nothing.
    return
  }

  // Compute claim ID and submit.
  const num = getNextChangeNumber()
  const changeBase = [...props.base, "SESSION", props.session, String(num)]
  const claimId = (await Identifier.from(...changeBase)).toString()

  const patch = makePatchForField(field, value, interval ? valueTo : undefined)

  const addChange = new AddClaimChange({
    id: claimId,
    base: changeBase,
    patch,
  })
  if (props.parentClaimId) {
    addChange.under = props.parentClaimId
  }

  // Move from empty slot to entries map.
  emptySlots.delete(slotId)
  entries.set(claimId, {
    field,
    value,
    valueTo,
    dirty: false,
    dirtyTo: false,
    persisted: false, // Will become persisted when doc syncs.
  })

  await saveChange(addChange, num)
}

function addEmptySlot(field: FieldData) {
  const id = `empty-${emptySlotCounter++}`
  emptySlots.set(id, { fieldKey: fieldKey(field), field, value: "", valueTo: "" })
}

async function removeEntry(claimId: string) {
  const entry = entries.get(claimId)
  if (!entry) {
    return
  }

  if (entry.persisted) {
    const num = getNextChangeNumber()
    await saveChange(new RemoveClaimChange({ id: claimId }), num)
  }

  entries.delete(claimId)
  // Child entries are managed by nested FieldsForm instances.
  // They clean up via syncFromDoc when their claims prop becomes null/empty.
  // Clean up child invalid state.
  delete childInvalid[claimId]
}

function removeEmptySlot(slotId: string) {
  emptySlots.delete(slotId)
}

function isRequired(field: FieldData): boolean {
  return field.minCardinality > 0
}

function isRepeatable(field: FieldData): boolean {
  return field.maxCardinality > 1
}

function canAddValue(field: FieldData): boolean {
  const count = entriesForField(field).length + emptySlotsForField(fieldKey(field)).length
  return count < field.maxCardinality
}

function canRemoveEntry(field: FieldData): boolean {
  const count = entriesForField(field).length
  return count > field.minCardinality
}

function isEmptySlotInvalid(field: FieldData, slotValue: string): boolean {
  if (!isRequired(field)) {
    return false
  }
  // A required field's empty slot is invalid if the value is empty and the field doesn't have enough non-empty entries.
  const fieldEntries = entriesForField(field)
  const nonEmpty = fieldEntries.filter(([_, e]) => e.value.trim() !== "").length
  return nonEmpty < field.minCardinality && slotValue.trim() === ""
}

function isEntryInvalid(entry: EntryState): boolean {
  if (!isRequired(entry.field)) {
    return false
  }
  const fieldEntries = entriesForField(entry.field)
  const nonEmpty = fieldEntries.filter(([_, e]) => e.value.trim() !== "").length
  return nonEmpty < entry.field.minCardinality && entry.value.trim() === ""
}

function getSubClaims(claimId: string): DeepReadonly<ClaimTypes> {
  if (!props.claims) {
    return new ClaimTypes({})
  }
  // Find the claim in our claims and return its sub.
  const claim = props.claims.GetByID(claimId)
  return new ClaimTypes(claim?.sub ?? {})
}

async function flush(): Promise<FieldsFormSaveChange[]> {
  const changes: FieldsFormSaveChange[] = []

  // Flush dirty entries.
  for (const [claimId, entry] of entries) {
    if (!entry.dirty && !entry.dirtyTo) {
      continue
    }

    const value = entry.value.trim()
    const valueTo = entry.valueTo.trim()
    const interval = isIntervalField(entry.field)
    const isEmpty = interval ? value === "" && valueTo === "" : value === ""

    if (isEmpty && entry.persisted) {
      const num = getNextChangeNumber()
      changes.push({ change: new RemoveClaimChange({ id: claimId }), changeNumber: num })
      entries.delete(claimId)
    } else if (!isEmpty && entry.persisted) {
      const existing = props.claims ? getExistingClaimValues(props.claims, entry.field) : []
      const existingValue = existing.find((e) => e.claimId === claimId)
      if (!existingValue || existingValue.value !== value || existingValue.valueTo !== valueTo) {
        const patch = makePatchForField(entry.field, value, interval ? valueTo : undefined)
        const num = getNextChangeNumber()
        changes.push({ change: new SetClaimChange({ id: claimId, patch }), changeNumber: num })
      }
    }

    entry.dirty = false
    entry.dirtyTo = false
  }

  // Flush non-empty empty slots.
  for (const [slotId, slot] of [...emptySlots]) {
    const value = slot.value.trim()
    const valueTo = slot.valueTo.trim()
    const interval = isIntervalField(slot.field)
    const isEmpty = interval ? value === "" && valueTo === "" : value === ""

    if (isEmpty) {
      continue
    }

    const num = getNextChangeNumber()
    const changeBase = [...props.base, "SESSION", props.session, String(num)]
    const claimId = (await Identifier.from(...changeBase)).toString()
    const patch = makePatchForField(slot.field, value, interval ? valueTo : undefined)

    const addChange = new AddClaimChange({ id: claimId, base: changeBase, patch })
    if (props.parentClaimId) {
      addChange.under = props.parentClaimId
    }

    changes.push({ change: addChange, changeNumber: num })

    // Move to entries.
    emptySlots.delete(slotId)
    entries.set(claimId, {
      field: slot.field,
      value,
      valueTo,
      dirty: false,
      dirtyTo: false,
      persisted: false,
    })
  }

  return changes
}

// Register for flush.
onMounted(() => {
  registerForFlush(flush)
})
onBeforeUnmount(() => {
  unregisterForFlush(flush)
})
</script>

<template>
  <table class="w-full table-auto border-collapse">
    <tbody>
      <!-- Top-level fields first (sorted by orderInList). -->
      <template v-for="field in sortedByOrder(fieldsData.fields)" :key="fieldKey(field)">
        <!-- Existing entries. -->
        <template v-for="([claimId, entry], eIndex) in entriesForField(field)" :key="claimId">
          <tr>
            <td v-if="eIndex === 0" class="w-1/5 px-2 py-1 align-top text-sm font-medium text-slate-700">
              <DocumentRefInline :id="field.propertyId" :link="false" />
              <span v-if="isRequired(field)" class="ml-0.5 text-error-600">*</span>
            </td>
            <td v-else></td>
            <td class="px-2 py-1">
              <div class="flex items-center gap-x-1">
                <div v-if="isIntervalField(field)" class="flex min-w-0 flex-auto grow gap-x-1">
                  <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.from") }}</span>
                  <InputText
                    :model-value="entry.value"
                    :invalid="isEntryInvalid(entry)"
                    :progress="progress"
                    class="min-w-0 flex-1"
                    @update:model-value="(v: string) => onEntryInput(claimId, v)"
                    @blur="onEntryBlur(claimId)"
                  />
                  <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.to") }}</span>
                  <InputText
                    :model-value="entry.valueTo"
                    :progress="progress"
                    class="min-w-0 flex-1"
                    @update:model-value="(v: string) => onEntryInput(claimId, v, true)"
                    @blur="onEntryBlur(claimId)"
                  />
                </div>
                <InputText
                  v-else
                  :model-value="entry.value"
                  :invalid="isEntryInvalid(entry)"
                  :progress="progress"
                  class="min-w-0 flex-auto grow"
                  @update:model-value="(v: string) => onEntryInput(claimId, v)"
                  @blur="onEntryBlur(claimId)"
                />
                <button
                  v-if="isRepeatable(field) && canRemoveEntry(field)"
                  type="button"
                  class="shrink-0 rounded p-1 text-slate-400 hover:text-error-600 focus:ring-2 focus:ring-primary-500 focus:outline-none"
                  @click="removeEntry(claimId)"
                >
                  <XMarkIcon class="size-4" />
                </button>
              </div>
            </td>
          </tr>
          <!-- Sub-fields for this entry (recursive). -->
          <tr v-if="field.subFields.length > 0" :key="claimId + '-sub'">
            <td class="px-2 py-1">
              <FieldsForm
                v-model:invalid="childInvalid[claimId]"
                :fields-data="{ sections: [], fields: field.subFields }"
                :claims="getSubClaims(claimId)"
                :base="base"
                :session="session"
                :progress="progress"
                :parent-claim-id="claimId"
              />
            </td>
          </tr>
        </template>
        <!-- Empty slots. -->
        <template v-for="[slotId, slotVal] in emptySlotsForField(fieldKey(field))" :key="slotId">
          <tr>
            <td
              v-if="entriesForField(field).length === 0 && emptySlotsForField(fieldKey(field))[0]?.[0] === slotId"
              class="w-1/5 px-2 py-1 align-top text-sm font-medium text-slate-700"
            >
              <DocumentRefInline :id="field.propertyId" :link="false" />
              <span v-if="isRequired(field)" class="ml-0.5 text-error-600">*</span>
            </td>
            <td v-else></td>
            <td class="px-2 py-1">
              <div class="flex items-center gap-x-1">
                <div v-if="isIntervalField(field)" class="flex min-w-0 flex-auto grow gap-x-1">
                  <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.from") }}</span>
                  <InputText
                    :model-value="slotVal.value"
                    :invalid="isEmptySlotInvalid(field, slotVal.value)"
                    :progress="progress"
                    class="min-w-0 flex-1"
                    @update:model-value="(v: string) => onEmptySlotInput(slotId, v)"
                    @blur="onEmptySlotBlur(slotId, field)"
                  />
                  <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.to") }}</span>
                  <InputText
                    :model-value="slotVal.valueTo"
                    :progress="progress"
                    class="min-w-0 flex-1"
                    @update:model-value="(v: string) => onEmptySlotInput(slotId, v, true)"
                    @blur="onEmptySlotBlur(slotId, field)"
                  />
                </div>
                <InputText
                  v-else
                  :model-value="slotVal.value"
                  :invalid="isEmptySlotInvalid(field, slotVal.value)"
                  :progress="progress"
                  class="min-w-0 flex-auto grow"
                  @update:model-value="(v: string) => onEmptySlotInput(slotId, v)"
                  @blur="onEmptySlotBlur(slotId, field)"
                />
                <button
                  type="button"
                  class="shrink-0 rounded p-1 text-slate-400 hover:text-error-600 focus:ring-2 focus:ring-primary-500 focus:outline-none"
                  @click="removeEmptySlot(slotId)"
                >
                  <XMarkIcon class="size-4" />
                </button>
              </div>
            </td>
          </tr>
        </template>
        <tr v-if="canAddValue(field)">
          <td></td>
          <td class="px-2 py-1">
            <button
              type="button"
              class="text-sm text-primary-600 hover:text-primary-800 focus:ring-2 focus:ring-primary-500 focus:outline-none"
              @click="addEmptySlot(field)"
            >
              {{ t("partials.FieldsForm.addAnother") }}
            </button>
          </td>
        </tr>
      </template>

      <!-- Sections (sorted by orderInList). -->
      <template v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id">
        <tr>
          <th colspan="2" class="border-b border-slate-200 px-2 pt-4 pb-1 text-left text-lg font-semibold">{{ section.id }}</th>
        </tr>
        <template v-for="field in sortedByOrder(section.fields)" :key="fieldKey(field)">
          <!-- Existing entries. -->
          <template v-for="([claimId, entry], eIndex) in entriesForField(field)" :key="claimId">
            <tr>
              <td v-if="eIndex === 0" class="w-1/5 px-2 py-1 align-top text-sm font-medium text-slate-700">
                <DocumentRefInline :id="field.propertyId" :link="false" />
                <span v-if="isRequired(field)" class="ml-0.5 text-error-600">*</span>
              </td>
              <td class="px-2 py-1">
                <div class="flex items-center gap-x-1">
                  <div v-if="isIntervalField(field)" class="flex min-w-0 flex-auto grow gap-x-1">
                    <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.from") }}</span>
                    <InputText
                      :model-value="entry.value"
                      :invalid="isEntryInvalid(entry)"
                      :progress="progress"
                      class="min-w-0 flex-1"
                      @update:model-value="(v: string) => onEntryInput(claimId, v)"
                      @blur="onEntryBlur(claimId)"
                    />
                    <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.to") }}</span>
                    <InputText
                      :model-value="entry.valueTo"
                      :progress="progress"
                      class="min-w-0 flex-1"
                      @update:model-value="(v: string) => onEntryInput(claimId, v, true)"
                      @blur="onEntryBlur(claimId)"
                    />
                  </div>
                  <InputText
                    v-else
                    :model-value="entry.value"
                    :invalid="isEntryInvalid(entry)"
                    :progress="progress"
                    class="min-w-0 flex-auto grow"
                    @update:model-value="(v: string) => onEntryInput(claimId, v)"
                    @blur="onEntryBlur(claimId)"
                  />
                  <button
                    v-if="isRepeatable(field) && canRemoveEntry(field)"
                    type="button"
                    class="shrink-0 rounded p-1 text-slate-400 hover:text-error-600 focus:ring-2 focus:ring-primary-500 focus:outline-none"
                    @click="removeEntry(claimId)"
                  >
                    <XMarkIcon class="size-4" />
                  </button>
                </div>
              </td>
            </tr>
            <!-- Sub-fields for this entry (recursive). -->
            <tr v-if="field.subFields.length > 0" :key="claimId + '-sub'">
              <td class="px-2 py-1">
                <FieldsForm
                  v-model:invalid="childInvalid[claimId]"
                  :fields-data="{ sections: [], fields: field.subFields }"
                  :claims="getSubClaims(claimId)"
                  :base="base"
                  :session="session"
                  :progress="progress"
                  :parent-claim-id="claimId"
                />
              </td>
            </tr>
          </template>
          <!-- Empty slots. -->
          <template v-for="[slotId, slotVal] in emptySlotsForField(fieldKey(field))" :key="slotId">
            <tr>
              <td
                v-if="entriesForField(field).length === 0 && emptySlotsForField(fieldKey(field))[0]?.[0] === slotId"
                class="w-1/5 px-2 py-1 align-top text-sm font-medium text-slate-700"
              >
                <DocumentRefInline :id="field.propertyId" :link="false" />
                <span v-if="isRequired(field)" class="ml-0.5 text-error-600">*</span>
              </td>
              <td class="px-2 py-1">
                <div class="flex items-center gap-x-1">
                  <div v-if="isIntervalField(field)" class="flex min-w-0 flex-auto grow gap-x-1">
                    <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.from") }}</span>
                    <InputText
                      :model-value="slotVal.value"
                      :progress="progress"
                      class="min-w-0 flex-1"
                      @update:model-value="(v: string) => onEmptySlotInput(slotId, v)"
                      @blur="onEmptySlotBlur(slotId, field)"
                    />
                    <span class="self-center text-xs text-slate-500">{{ t("partials.FieldsForm.to") }}</span>
                    <InputText
                      :model-value="slotVal.valueTo"
                      :progress="progress"
                      class="min-w-0 flex-1"
                      @update:model-value="(v: string) => onEmptySlotInput(slotId, v, true)"
                      @blur="onEmptySlotBlur(slotId, field)"
                    />
                  </div>
                  <InputText
                    v-else
                    :model-value="slotVal.value"
                    :progress="progress"
                    class="min-w-0 flex-auto grow"
                    @update:model-value="(v: string) => onEmptySlotInput(slotId, v)"
                    @blur="onEmptySlotBlur(slotId, field)"
                  />
                  <button
                    type="button"
                    class="shrink-0 rounded p-1 text-slate-400 hover:text-error-600 focus:ring-2 focus:ring-primary-500 focus:outline-none"
                    @click="removeEmptySlot(slotId)"
                  >
                    <XMarkIcon class="size-4" />
                  </button>
                </div>
              </td>
            </tr>
          </template>
          <tr v-if="canAddValue(field)">
            <td></td>
            <td class="px-2 py-1">
              <button
                type="button"
                class="text-sm text-primary-600 hover:text-primary-800 focus:ring-2 focus:ring-primary-500 focus:outline-none"
                @click="addEmptySlot(field)"
              >
                {{ t("partials.FieldsForm.addAnother") }}
              </button>
            </td>
          </tr>
        </template>
      </template>
    </tbody>
  </table>
</template>
