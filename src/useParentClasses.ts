import type { DeepReadonly, Ref } from "vue"

import { computed, onBeforeUnmount, readonly, ref, watch } from "vue"
import { useRouter } from "vue-router"

import { getURL } from "@/api"
import { INSTANCE_OF, SUBCLASS_OF } from "@/core"
import { D, getClaimsOfTypeWithConfidence } from "@/document"

// useParentClasses resolves all parent class documents for a document by walking its
// INSTANCE_OF classes and their SUBCLASS_OF parents. Returns reactive classDocs map
// (preserving walk order), the direct instanceOfClassIds, and an initialized flag.
export function useParentClasses(
  doc: Ref<DeepReadonly<D> | null | undefined>,
  el: Ref<Element | null>,
  progress: Ref<number>,
): {
  classDocs: DeepReadonly<Ref<Map<string, D>>>
  instanceOfClassIds: DeepReadonly<Ref<string[]>>
  initialized: DeepReadonly<Ref<boolean>>
} {
  // Uses a separate abort controller tied to component lifecycle (not route changes),
  // because useParentClasses watches doc reactively and handles route changes via doc becoming null.
  const abortController = new AbortController()
  onBeforeUnmount(() => abortController.abort())

  const router = useRouter()
  const _classDocs = ref<Map<string, D>>(new Map())
  const _initialized = ref(false)
  const classDocs = process.env.NODE_ENV !== "production" ? readonly(_classDocs) : _classDocs
  const initialized = process.env.NODE_ENV !== "production" ? readonly(_initialized) : _initialized

  async function fetchClassDocument(classId: string): Promise<D | null> {
    try {
      const { doc: rawDoc } = await getURL<object>(
        router.apiResolve({
          name: "DocumentGet",
          params: {
            id: classId,
          },
        }).href,
        el,
        abortController.signal,
        progress,
      )
      if (abortController.signal.aborted) {
        return null
      }
      return new D(rawDoc)
    } catch (err) {
      // TODO: Do something better?
      console.error("useParentClasses.fetchClassDocument", classId, err)
      return null
    }
  }

  const _instanceOfClassIds = computed(() => {
    if (!doc.value?.claims) {
      return []
    }
    return getClaimsOfTypeWithConfidence(doc.value.claims, "ref", INSTANCE_OF).map((c) => c.to.id)
  })
  const instanceOfClassIds = process.env.NODE_ENV !== "production" ? readonly(_instanceOfClassIds) : _instanceOfClassIds

  watch(
    _instanceOfClassIds,
    async (classIds) => {
      if (classIds.length === 0) {
        _classDocs.value = new Map()
        // TODO: Do something better here?
        _initialized.value = !!doc.value?.claims
        return
      }

      const docs = new Map<string, D>()
      const visited = new Set<string>()

      async function walk(id: string) {
        if (visited.has(id)) {
          return
        }
        visited.add(id)

        const classDoc = await fetchClassDocument(id)
        if (abortController.signal.aborted) {
          return
        }
        if (!classDoc) {
          return
        }

        docs.set(id, classDoc)

        if (!classDoc.claims) {
          return
        }

        const subclassOfClaims = getClaimsOfTypeWithConfidence(classDoc.claims, "ref", SUBCLASS_OF)
        for (const claim of subclassOfClaims) {
          await walk(claim.to.id)
          if (abortController.signal.aborted) {
            return
          }
        }
      }

      for (const id of classIds) {
        await walk(id)
        if (abortController.signal.aborted) {
          return
        }
      }

      _classDocs.value = docs
      _initialized.value = true
    },
    {
      immediate: true,
    },
  )

  return { classDocs, instanceOfClassIds, initialized }
}
