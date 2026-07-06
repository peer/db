import type { DeepReadonly, InjectionKey, Ref } from "vue"

import { inject } from "vue"

// DocumentNavigation is provided by the DocumentGet view to registered document components, so
// downstream sites can render their own navigation between the search session's results (e.g.
// previous/next links inside the page instead of the navbar buttons).
export type DocumentNavigation = {
  // The search session id when the document is viewed as part of a search session, null otherwise.
  searchSessionId: Readonly<Ref<string | null>>
  // The neighboring document ids within the search session's results.
  prevNext: Readonly<Ref<DeepReadonly<{ previous: string | null; next: string | null }>>>
}

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const documentNavigationKey: InjectionKey<DocumentNavigation> =
  process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-document-navigation") : Symbol()

// useDocumentNavigation returns the navigation info provided by the DocumentGet view, or null
// outside of it.
export function useDocumentNavigation(): DocumentNavigation | null {
  return inject(documentNavigationKey, null)
}
