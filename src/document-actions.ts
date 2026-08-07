import type { InjectionKey, Ref } from "vue"

import { inject } from "vue"

// DocumentActions is provided by the DocumentGet view to registered document components, so
// downstream sites can render the edit control (and a permission-gated delete control) inside the
// page (e.g. next to the document's own action buttons) instead of, or in addition to, the built-in
// side-column buttons. The provided edit handler runs the exact same edit flow the built-in button
// uses. Deletion is performed on the dedicated DocumentDelete confirmation page, so sites link their
// delete control to the DocumentDelete route rather than calling a handler here.
export type DocumentActions = {
  // Whether the caller has permission to update, delete, or manage permissions of the document (the
  // ACTION_UPDATE, ACTION_DELETE, and ACTION_UPDATE_PERMISSIONS actions checked against the document, so
  // document-level permission claims count). Managing permissions takes the update action as well,
  // because it is done by changing the document's claims in an edit session.
  canUpdate: Readonly<Ref<boolean>>
  canDelete: Readonly<Ref<boolean>>
  canUpdatePermissions: Readonly<Ref<boolean>>
  // Progress counter, greater than zero while an edit session is starting.
  editBusy: Readonly<Ref<number>>
  // Start editing (begins an edit session and navigates to the edit view). The edit view opens on the tab
  // with the given slug ("permissions" for its permissions tab), or on its default tab when none is given.
  edit: (tab?: string) => Promise<void>
}

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const documentActionsKey: InjectionKey<DocumentActions> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-document-actions") : Symbol()

// useDocumentActions returns the edit action and whether the caller holds the update, delete, and
// permissions actions on the document, as provided by the DocumentGet view, or null outside of it.
export function useDocumentActions(): DocumentActions | null {
  return inject(documentActionsKey, null)
}
