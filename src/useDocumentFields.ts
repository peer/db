import type { DeepReadonly, Ref } from "vue"

import type { D } from "@/document"
import type { FieldsData } from "@/fields"

import { computed, ref, watch } from "vue"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import { INSTANCE_OF, SUBCLASS_OF } from "@/core"
import { D as DocClass, getClaimsOfTypeWithConfidence } from "@/document"
import { extractFieldsFromClaims, mergeFields } from "@/fields"

// useDocumentFields resolves the merged field definitions for a document by walking its
// INSTANCE_OF classes and their SUBCLASS_OF parents. Returns reactive fieldsData and classTabId.
export function useDocumentFields(
  doc: Ref<DeepReadonly<D> | null | undefined>,
  locale: Ref<string>,
  abortSignal: AbortSignal,
): {
  fieldsData: Ref<FieldsData | null>
  classTabId: Ref<string>
  initialized: Ref<boolean>
} {
  const router = useRouter()
  const fieldsData = ref<FieldsData | null>(null)
  const classTabId = ref("")
  const initialized = ref(false)

  async function fetchClassDocument(classId: string): Promise<DocClass | null> {
    try {
      const { doc: rawDoc } = await getURL<object>(router.apiResolve({ name: "DocumentGet", params: { id: classId } }).href, null, abortSignal, null)
      if (abortSignal.aborted) {
        return null
      }
      return new DocClass(rawDoc)
    } catch {
      return null
    }
  }

  async function collectAllClassIds(classIds: string[]): Promise<string[]> {
    const visited = new Set<string>()
    const result: string[] = []

    async function walk(id: string) {
      if (visited.has(id)) {
        return
      }
      visited.add(id)
      result.push(id)

      const classDoc = await fetchClassDocument(id)
      if (!classDoc?.claims || abortSignal.aborted) {
        return
      }

      const subclassOfClaims = getClaimsOfTypeWithConfidence(classDoc.claims, "ref", SUBCLASS_OF)
      for (const claim of subclassOfClaims) {
        await walk(claim.to.id)
        if (abortSignal.aborted) {
          return
        }
      }
    }

    for (const id of classIds) {
      await walk(id)
      if (abortSignal.aborted) {
        return []
      }
    }

    return result
  }

  const instanceOfClassIds = computed(() => {
    if (!doc.value?.claims) {
      return []
    }
    return getClaimsOfTypeWithConfidence(doc.value.claims, "ref", INSTANCE_OF).map((c) => c.to.id)
  })

  watch(
    instanceOfClassIds,
    async (classIds) => {
      if (classIds.length === 0) {
        classTabId.value = ""
        fieldsData.value = null
        initialized.value = true
        return
      }

      const allClassIds = await collectAllClassIds(classIds)
      if (abortSignal.aborted) {
        return
      }

      const allFields: FieldsData[] = []
      const classTabIds: string[] = []

      for (const classId of allClassIds) {
        const classDoc = await fetchClassDocument(classId)
        if (abortSignal.aborted) {
          return
        }
        if (!classDoc?.claims) {
          continue
        }

        const fields = extractFieldsFromClaims(classDoc.claims, locale.value)
        if (fields && (fields.fields.length > 0 || fields.sections.length > 0)) {
          allFields.push(fields)
          if (classIds.includes(classId)) {
            classTabIds.push(classId)
          }
        }
      }

      initialized.value = true
      if (classTabIds.length > 0) {
        classTabId.value = classTabIds[0]
        fieldsData.value = allFields.length > 0 ? mergeFields(allFields) : null
      } else {
        classTabId.value = ""
        fieldsData.value = null
      }
    },
    { immediate: true },
  )

  return { fieldsData, classTabId, initialized }
}
