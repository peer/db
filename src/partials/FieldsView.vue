<script setup lang="ts">
import type { DeepReadonly } from "vue"

import type { Claim, ClaimTypeName } from "@/document"
import type { FieldData, FieldsData, SectionData } from "@/fields"

import { computed } from "vue"
import { useI18n } from "vue-i18n"

import { IN_LANGUAGE } from "@/core"
import { ClaimTypes, getClaimsOfTypeWithConfidence, selectClaimsByLanguage } from "@/document"
import { fieldKey, getClaimsForField, valueTypeToClaimType } from "@/fields"
import ClaimValue from "@/partials/ClaimValue.vue"
import DocumentRefInline from "@/partials/DocumentRefInline.vue"

const props = withDefaults(
  defineProps<{
    fieldsData: DeepReadonly<FieldsData>
    claims: DeepReadonly<ClaimTypes>
    sections?: boolean
  }>(),
  {
    sections: false,
  },
)

const { locale } = useI18n({ useScope: "global" })

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

// Check if any claims for a field have IN_LANGUAGE sub-claims in the actual data.
function hasLanguageClaims(field: FieldData): boolean {
  const claims = getClaimsForField(normalizedClaims.value, field)
  return claims.some((claim) => claim.sub && getClaimsOfTypeWithConfidence(claim.sub, "ref", IN_LANGUAGE).length > 0)
}

// Get claims for a field, returning the claim type name and array of claims.
// If claims have IN_LANGUAGE sub-claims, only claims matching the current
// locale (with fallbacks) are returned.
function claimsForField(field: FieldData): { claimType: ClaimTypeName; claims: DeepReadonly<Claim>[] } {
  const claimType = valueTypeToClaimType(field.valueType)
  if (hasLanguageClaims(field)) {
    const claims = selectClaimsByLanguage(normalizedClaims.value, claimType, field.propertyId, locale.value, (c) => c.length > 0)
    return { claimType, claims: claims ?? [] }
  }
  const claims = getClaimsForField(normalizedClaims.value, field)
  return { claimType, claims }
}

// Check if a field has any claim values.
function hasValues(field: FieldData): boolean {
  return claimsForField(field).claims.length > 0
}

// Get sub-claims for a specific claim (for recursive sub-fields).
function getSubClaims(claimId: string): ClaimTypes {
  const claim = normalizedClaims.value.GetByID(claimId)
  return new ClaimTypes(claim?.sub ?? {})
}

// Check if any top-level field has values.
const hasAnyFieldValues = computed(() => props.fieldsData.fields.some(hasValues))

// Check if any section field has values.
const hasAnySectionValues = computed(() => props.fieldsData.sections.some((s) => s.fields.some(hasValues)))

// Check if there's anything to display.
const hasContent = computed(() => hasAnyFieldValues.value || (props.sections && hasAnySectionValues.value))
</script>

<template>
  <table v-if="hasContent" class="w-full table-auto border-collapse">
    <tbody>
      <!-- Top-level fields first (sorted by orderInList). -->
      <template v-for="field in sortedByOrder(fieldsData.fields)" :key="fieldKey(field)">
        <template v-if="hasValues(field)">
          <template v-for="(claim, cIndex) in claimsForField(field).claims" :key="claim.GetID()">
            <tr>
              <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                <DocumentRefInline :id="field.propertyId" :link="false" />
              </td>
              <td v-else></td>
              <td class="px-2 py-1 text-gray-700">
                <ClaimValue :claim="claim" :type="claimsForField(field).claimType" />
              </td>
            </tr>
            <!-- Sub-fields for this claim value (recursive). -->
            <tr v-if="field.subFields.length > 0 && claim.sub">
              <td></td>
              <td class="px-2 py-0">
                <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" />
              </td>
            </tr>
          </template>
        </template>
      </template>

      <!-- Sections (sorted by orderInList), only if sections prop is true. -->
      <template v-if="sections">
        <template v-for="section in sortedByOrder(fieldsData.sections)" :key="'section-' + section.id">
          <template v-if="section.fields.some(hasValues)">
            <tr>
              <th colspan="2" class="border-b border-slate-200 px-2 pt-4 pb-1 text-left text-lg font-semibold">{{ (section as SectionData).id }}</th>
            </tr>
            <template v-for="field in sortedByOrder(section.fields)" :key="fieldKey(field)">
              <template v-if="hasValues(field)">
                <template v-for="(claim, cIndex) in claimsForField(field).claims" :key="claim.GetID()">
                  <tr>
                    <td v-if="cIndex === 0" class="w-1/5 px-2 py-1 align-top font-medium text-gray-700">
                      <DocumentRefInline :id="field.propertyId" :link="false" />
                    </td>
                    <td v-else></td>
                    <td class="px-2 py-1 text-gray-700">
                      <ClaimValue :claim="claim" :type="claimsForField(field).claimType" />
                    </td>
                  </tr>
                  <!-- Sub-fields for this claim value (recursive). -->
                  <tr v-if="field.subFields.length > 0 && claim.sub">
                    <td></td>
                    <td class="px-2 py-0">
                      <FieldsView :fields-data="{ sections: [], fields: field.subFields }" :claims="getSubClaims(claim.GetID())" />
                    </td>
                  </tr>
                </template>
              </template>
            </template>
          </template>
        </template>
      </template>
    </tbody>
  </table>
</template>
