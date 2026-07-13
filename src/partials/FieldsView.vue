<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim } from "@/document"
import type { FieldData, FieldsData, SectionData } from "@/fields"

import { computed, inject, onBeforeUnmount, ref, watch } from "vue"
import { useI18n } from "vue-i18n"
import { useRouter } from "vue-router"

import Button from "@/components/Button.vue"
import { IN_LANGUAGE } from "@/core"
import { ClaimTypes, claimTypeName, getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { fieldKey, fieldShownInView, getClaimsForField, getSectionName, valueTypeToClaimType } from "@/fields"
import { classifyLink, LINK_CLASS_FILE } from "@/internal-links"
import ClaimValue from "@/partials/ClaimValue.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"
import { searchHiddenClaimsKey, searchLoadAllClaimsKey, SKIP_TO_END } from "@/utils"

const props = withDefaults(
  defineProps<{
    fieldsData: DeepReadonly<FieldsData>
    claims: DeepReadonly<ClaimTypes>
    sections?: boolean
    // When limited is true, fields whose property repeats show only the first few claim values, with a
    // "Show all" button to reveal the rest. Used in compact contexts like search result cards.
    limited?: boolean
    // True when this instance renders the sub-fields of a claim (the recursive calls set it). It
    // changes where this instance places its OWN sub-field tables: a top-level instance puts them
    // in the value column (sub-fields sit under the field's value, slightly indented), while a
    // nested instance spans them across both columns (deeper sub-fields sit under the sub-field's
    // label, slightly indented, with their values landing slightly right of its value).
    nested?: boolean
  }>(),
  {
    sections: false,
    limited: false,
    nested: false,
  },
)

const { t, locale } = useI18n({ useScope: "global" })

// Number of repeating claim values shown for a field before the "Show all" button when limited is true.
const LIMITED_CLAIMS = 3

// Set by the search print view's "Load all" button (see searchLoadAllClaimsKey). When true, every repeating
// claim value is shown regardless of the limit, so a printout is not cut off at LIMITED_CLAIMS.
const loadAllClaims = inject(searchLoadAllClaimsKey, null)

// Field keys (see fieldKey) for fields whose repeating claim values have been expanded via "Show all".
const expandedFields = ref(new Set<string>())

function expandField(field: FieldData): void {
  expandedFields.value = new Set(expandedFields.value).add(fieldKey(field))
}

// Ensure claims is a proper ClaimTypes instance (props may receive raw JSON from WithDocument).
const normalizedClaims = computed(() => {
  if (!props.claims) {
    return new ClaimTypes({})
  }
  if (props.claims instanceof ClaimTypes) {
    return props.claims
  }
  return new ClaimTypes(props.claims as unknown as Record<string, object[]>)
})

function sortedByOrder<T extends { orderInList: number }>(items: readonly T[]): T[] {
  return [...items].sort((a, b) => a.orderInList - b.orderInList)
}

const router = useRouter()

// Routes link claims between sibling LINK and FILE fields sharing a property
// (see getClaimsForField).
function isFileLink(iri: string): boolean {
  return classifyLink(iri, router).includes(LINK_CLASS_FILE)
}

// Check if any claims for a field have IN_LANGUAGE sub-claims in the actual data.
function hasLanguageClaims(field: FieldData): boolean {
  const claims = getClaimsForField(normalizedClaims.value, field, isFileLink)
  return claims.some((claim) => claim.sub && getClaimsOfTypeWithConfidence(claim.sub, "ref", IN_LANGUAGE).length > 0)
}

// Get the claims to display for a field. If claims have IN_LANGUAGE sub-claims, only claims
// matching the current locale (with fallbacks) are returned. Each claim is rendered by its own
// type (see claimTypeName in the template), so a field whose claims span more than one type (a
// value field whose default lets it also hold none/unknown claims) renders each correctly.
function claimsForField(field: FieldData): DeepReadonly<Claim>[] {
  if (hasLanguageClaims(field)) {
    const claimType = valueTypeToClaimType(field.valueType)
    const claims = selectClaimsByLanguage(normalizedClaims.value, claimType, field.propertyId, locale.value, (c) => c.length > 0)
    return claims ?? []
  }
  return getClaimsForField(normalizedClaims.value, field, isFileLink)
}

// Check if a field has any claim values.
function hasValues(field: FieldData): boolean {
  return claimsForField(field).length > 0
}

// Whether the field renders here: it must have claim values and not be an
// edit-only field (context "edit", see fieldShownInView).
function shown(field: FieldData): boolean {
  return fieldShownInView(field) && hasValues(field)
}

// The claim values to render for a field. When limited and the field is not expanded, repeating values
// are capped at LIMITED_CLAIMS so the remaining ones stay hidden behind the "Show all" button. If capping
// would leave only SKIP_TO_END or fewer values hidden, all values are shown instead of the button. The print
// view's "Load all" button (loadAllClaims) lifts the cap entirely so a printout shows every value.
function displayedClaimsForField(field: FieldData): DeepReadonly<Claim>[] {
  const claims = claimsForField(field)
  if (props.limited && !loadAllClaims?.value && !expandedFields.value.has(fieldKey(field)) && claims.length > LIMITED_CLAIMS + SKIP_TO_END) {
    return claims.slice(0, LIMITED_CLAIMS)
  }
  return claims
}

// Count of claim values currently hidden for a field (zero unless limited and the field is collapsed).
function hiddenClaimsCount(field: FieldData): number {
  return claimsForField(field).length - displayedClaimsForField(field).length
}

// Report to the search print view whether any field this instance renders still has repeating claim values hidden,
// so its "Load all" button can appear even when every result already fits. Sub-fields render their own FieldsView
// and report separately, so only this instance's own fields count here.
const reportHiddenClaims = inject(searchHiddenClaimsKey, null)

const hasHiddenClaims = computed(() => {
  const fields = [...props.fieldsData.fields]
  if (props.sections) {
    for (const section of props.fieldsData.sections) {
      fields.push(...section.fields)
    }
  }
  return fields.some((field) => shown(field) && hiddenClaimsCount(field) > 0)
})

// Emit each transition as a +1/-1 delta, guarded by reported so this instance is only ever counted once, and on
// unmount release a contribution that is still standing, so the print view's total stays balanced. Syncing the
// watcher keeps reported in lockstep with hasHiddenClaims so the count is accurate the moment a card loads.
let reported = 0
watch(
  hasHiddenClaims,
  (now) => {
    if (now && reported === 0) {
      reportHiddenClaims?.(1)
      reported = 1
    } else if (!now && reported === 1) {
      reportHiddenClaims?.(-1)
      reported = 0
    }
  },
  { immediate: true, flush: "sync" },
)

onBeforeUnmount(() => {
  if (reported > 0) {
    reportHiddenClaims?.(-reported)
  }
})

// Get sub-claims for a specific claim (for recursive sub-fields).
function getSubClaims(claimId: string): ClaimTypes {
  const claim = normalizedClaims.value.GetByID(claimId)
  return new ClaimTypes(claim?.sub ?? {})
}

// Check if any top-level field is shown.
const hasAnyFieldValues = computed(() => props.fieldsData.fields.some(shown))

// Check if any section field is shown.
const hasAnySectionValues = computed(() => props.fieldsData.sections.some((s) => s.fields.some(shown)))

// Check if there's anything to display.
const hasContent = computed(() => hasAnyFieldValues.value || (props.sections && hasAnySectionValues.value))
</script>

<template>
  <!--
    The table is laid out as a CSS grid rather than with table layout, so it reflows on narrow viewports: the
    tbody is the grid and the tr elements are display: contents, so their th/td children are the grid items.
    Below sm it is a single column (the property label on its own line, then the value below it); from sm up it
    is a two-column 20%/1fr grid (label beside value). This lets a search result card shrink to narrow viewports
    instead of being held open by the value column's minimum width. The empty label cells for repeated values and
    the sub-field indent cell are dropped below sm so a stacked value does not sit under a blank line.
  -->
  <table v-if="hasContent" class="flex w-full flex-col">
    <tbody class="grid grid-cols-1 sm:grid-cols-[20%_1fr] sm:gap-x-3">
      <!-- Top-level fields first (sorted by orderInList). -->
      <template v-for="field in sortedByOrder(fieldsData.fields)" :key="fieldKey(field)">
        <template v-if="shown(field)">
          <template v-for="(claim, cIndex) in displayedClaimsForField(field)" :key="claim.GetID()">
            <!--
              A value-less HAS claim renders no value. At the top level its sub-fields table sits in the value cell of the
              label row, so the first sub-field aligns to the right of the label (where a value would be) from sm up, and
              indents one step (pl-2) below sm so it sits under the label. In a nested instance the value column is left
              empty and the sub-fields stair-step in the sub-row below (the value branch), so a deep HAS chain does not march.
            -->
            <tr v-if="claimTypeName(claim) === 'has' && !nested && field.subFields.length > 0 && claim.sub" class="contents">
              <td v-if="cIndex === 0" class="px-2 py-1 align-top font-medium text-gray-700">
                <DocumentRefInline :id="field.propertyId" :link="false" />
              </td>
              <td v-else class="hidden sm:block"></td>
              <td class="py-0 pr-0 pl-2 align-top sm:pl-0">
                <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
              </td>
            </tr>
            <template v-else>
              <tr class="contents">
                <td v-if="cIndex === 0" class="px-2 py-1 align-top font-medium text-gray-700">
                  <DocumentRefInline :id="field.propertyId" :link="false" />
                </td>
                <td v-else class="hidden sm:block"></td>
                <!--
                  A value-less HAS claim (nested, or without sub-fields) shows nothing in the value column: an empty
                  cell keeps the two-column grid aligned from sm up, and is hidden below sm so it adds no empty line.
                -->
                <td v-if="claimTypeName(claim) === 'has'" class="hidden sm:block"></td>
                <td v-else class="px-2 pt-0 pb-1 align-top text-gray-700 sm:pt-1">
                  <ClaimValue :claim="claim" :type="claimTypeName(claim)" />
                </td>
              </tr>
              <!--
                Sub-fields render indented below the field. For a value in a top-level instance the sub-table sits in the
                value column (under the value). A nested value, and any nested value-less HAS field, spans both columns
                (sm:col-span-2) and indents by this cell's px-2 under the label, so deeper sub-fields stair-step down per level.
              -->
              <tr v-if="field.subFields.length > 0 && claim.sub" class="contents">
                <td v-if="!nested" class="hidden sm:block"></td>
                <td class="px-2 py-0 align-top" :class="{ 'sm:col-span-2': nested }">
                  <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                </td>
              </tr>
            </template>
          </template>
          <tr v-if="hiddenClaimsCount(field) > 0" class="contents">
            <td class="hidden sm:block"></td>
            <td class="px-2 py-1">
              <Button type="button" class="px-2.5 py-1" @click.prevent="expandField(field)">{{ t("common.buttons.showAll") }}</Button>
            </td>
          </tr>
        </template>
      </template>

      <!-- Sections (sorted by orderInList), only if sections prop is true. -->
      <template v-if="sections">
        <template v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id">
          <template v-if="section.fields.some(shown)">
            <tr class="contents">
              <!--
                The heading role (replacing the cell's columnheader role, which this
                section separator is not anyway) lets assistive technology jump
                between sections. It spans both columns from sm up.
              -->
              <th role="heading" aria-level="2" class="border-b border-slate-200 px-2 pt-4 pb-1 text-left text-lg font-semibold sm:col-span-2">
                {{ getSectionName(section as SectionData, locale) }}
              </th>
            </tr>
            <template v-for="field in sortedByOrder(section.fields)" :key="fieldKey(field)">
              <template v-if="shown(field)">
                <template v-for="(claim, cIndex) in displayedClaimsForField(field)" :key="claim.GetID()">
                  <!-- A top-level value-less HAS claim's sub-fields sit in the label row's value cell (first sub-field to the right), see above. -->
                  <tr v-if="claimTypeName(claim) === 'has' && !nested && field.subFields.length > 0 && claim.sub" class="contents">
                    <td v-if="cIndex === 0" class="px-2 py-1 align-top font-medium text-gray-700">
                      <DocumentRefInline :id="field.propertyId" :link="false" />
                    </td>
                    <td v-else class="hidden sm:block"></td>
                    <td class="py-0 pr-0 pl-2 align-top sm:pl-0">
                      <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                    </td>
                  </tr>
                  <template v-else>
                    <tr class="contents">
                      <td v-if="cIndex === 0" class="px-2 py-1 align-top font-medium text-gray-700">
                        <DocumentRefInline :id="field.propertyId" :link="false" />
                      </td>
                      <td v-else class="hidden sm:block"></td>
                      <!-- A value-less HAS claim (nested, or without sub-fields) shows nothing in the value column, see above. -->
                      <td v-if="claimTypeName(claim) === 'has'" class="hidden sm:block"></td>
                      <td v-else class="px-2 pt-0 pb-1 align-top text-gray-700 sm:pt-1">
                        <ClaimValue :claim="claim" :type="claimTypeName(claim)" />
                      </td>
                    </tr>
                    <!-- Sub-fields for this claim value (recursive), see above. -->
                    <tr v-if="field.subFields.length > 0 && claim.sub" class="contents">
                      <td v-if="!nested" class="hidden sm:block"></td>
                      <td class="px-2 py-0 align-top" :class="{ 'sm:col-span-2': nested }">
                        <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" :limited="limited" nested />
                      </td>
                    </tr>
                  </template>
                </template>
                <tr v-if="hiddenClaimsCount(field) > 0" class="contents">
                  <td class="hidden sm:block"></td>
                  <td class="px-2 py-1">
                    <Button type="button" class="px-2.5 py-1" @click.prevent="expandField(field)">{{ t("common.buttons.showAll") }}</Button>
                  </td>
                </tr>
              </template>
            </template>
          </template>
        </template>
      </template>
    </tbody>
  </table>
</template>
