import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"

// Well-known permission action IDs, the values of the PERMISSION_ACTIONS vocabulary used by role
// grants and document-level permission claims.
//
// Keep this list in sync with auth/permissions.go and core/vocabularies.go.
export const ACTION_CREATE = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_CREATE")).toString()
export const ACTION_READ = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_READ")).toString()
export const ACTION_UPDATE = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_UPDATE")).toString()
export const ACTION_DELETE = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_DELETE")).toString()
export const ACTION_READ_HISTORIC = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_READ_HISTORIC")).toString()
export const ACTION_UPDATE_PERMISSIONS = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_UPDATE_PERMISSIONS")).toString()
export const ACTION_READ_BULK = (await Identifier.from(Namespace, "PERMISSION_ACTIONS", "ACTION_READ_BULK")).toString()
