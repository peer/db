import type { DeepReadonly, Ref } from "vue"

import type { D } from "@/document"
import type { FieldsData } from "@/fields"

import { readonly, ref, watchEffect } from "vue"

import { extractFieldsFromClaims, mergeFields } from "@/fields"

// useDocumentFields computes merged field definitions from pre-fetched class documents.
// Takes classDocs and instanceOfClassIds from useParentClasses.
// Returns reactive fieldsData and classTabId.
export function useDocumentFields(
  classDocs: DeepReadonly<Ref<Map<string, D>>>,
  instanceOfClassIds: DeepReadonly<Ref<string[]>>,
): {
  fieldsData: DeepReadonly<Ref<FieldsData | null>>
  classTabId: DeepReadonly<Ref<string>>
} {
  const _fieldsData = ref<FieldsData | null>(null)
  const _classTabId = ref("")
  const fieldsData = process.env.NODE_ENV !== "production" ? readonly(_fieldsData) : _fieldsData
  const classTabId = process.env.NODE_ENV !== "production" ? readonly(_classTabId) : _classTabId

  watchEffect(() => {
    if (classDocs.value.size === 0) {
      _fieldsData.value = null
      _classTabId.value = ""
      return
    }

    const allFields: FieldsData[] = []
    let firstClassTabId = ""

    for (const [classId, classDoc] of classDocs.value) {
      if (!classDoc.claims) {
        continue
      }
      const fields = extractFieldsFromClaims(classDoc.claims)
      if (fields && (fields.fields.length > 0 || fields.sections.length > 0)) {
        allFields.push(fields)
        if (!firstClassTabId && instanceOfClassIds.value.includes(classId)) {
          firstClassTabId = classId
        }
      }
    }

    if (firstClassTabId) {
      _fieldsData.value = allFields.length > 0 ? mergeFields(allFields) : null
      _classTabId.value = firstClassTabId
      return
    }

    _fieldsData.value = null
    _classTabId.value = ""
  })

  return { fieldsData, classTabId }
}
