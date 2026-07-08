import type { InjectionKey, Ref } from "vue"

import { inject } from "vue"

// DocumentActions is provided by the DocumentGet view to registered document components, so
// downstream sites can render the edit and delete controls inside the page (e.g. next to the
// document's own action buttons) instead of, or in addition to, the navbar buttons. The provided
// handlers run the exact same edit and delete flows the navbar buttons use.
export type DocumentActions = {
  // Whether the caller has permission to edit or delete the document (CAN_EDIT_DOCUMENT and
  // CAN_DELETE_DOCUMENT). Sites can gate the rendered buttons further (for example by role).
  canEdit: Readonly<Ref<boolean>>
  canDelete: Readonly<Ref<boolean>>
  // Progress counters, greater than zero while the respective action runs.
  editBusy: Readonly<Ref<number>>
  deleteBusy: Readonly<Ref<number>>
  // Start editing (begins an edit session and navigates to the edit view) or delete the document.
  edit: () => Promise<void>
  delete: () => Promise<void>
}

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const documentActionsKey: InjectionKey<DocumentActions> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-document-actions") : Symbol()

// useDocumentActions returns the edit and delete actions provided by the DocumentGet view, or null
// outside of it.
export function useDocumentActions(): DocumentActions | null {
  return inject(documentActionsKey, null)
}
