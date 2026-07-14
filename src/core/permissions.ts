import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"

// Well-known permission action IDs, the values of the PERMISSION_ACTIONS vocabulary used by
// document-level permission claims.
//
// Keep this list in sync with core/vocabularies.go.
export const ACTION_READ = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_READ")).toString()
export const ACTION_EDIT = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_EDIT")).toString()
export const ACTION_PERMISSIONS = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_PERMISSIONS")).toString()
