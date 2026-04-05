import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"

// Well-known value type IDs based on their mnemonics.
//
// Keep this list in sync with core/vocabularies.go.
export const VT_STRING = (await Identifier.from(Namespace, "VALUE_TYPE", "STRING")).toString()
export const VT_HTML = (await Identifier.from(Namespace, "VALUE_TYPE", "HTML")).toString()
export const VT_IDENTIFIER = (await Identifier.from(Namespace, "VALUE_TYPE", "IDENTIFIER")).toString()
export const VT_AMOUNT = (await Identifier.from(Namespace, "VALUE_TYPE", "AMOUNT")).toString()
export const VT_AMOUNT_INTERVAL = (await Identifier.from(Namespace, "VALUE_TYPE", "AMOUNT_INTERVAL")).toString()
export const VT_TIME = (await Identifier.from(Namespace, "VALUE_TYPE", "TIME")).toString()
export const VT_TIME_INTERVAL = (await Identifier.from(Namespace, "VALUE_TYPE", "TIME_INTERVAL")).toString()
export const VT_LINK = (await Identifier.from(Namespace, "VALUE_TYPE", "LINK")).toString()
export const VT_FILE = (await Identifier.from(Namespace, "VALUE_TYPE", "FILE")).toString()
export const VT_REFERENCE = (await Identifier.from(Namespace, "VALUE_TYPE", "REFERENCE")).toString()
export const VT_HAS = (await Identifier.from(Namespace, "VALUE_TYPE", "HAS")).toString()
export const VT_NONE = (await Identifier.from(Namespace, "VALUE_TYPE", "NONE")).toString()
export const VT_UNKNOWN = (await Identifier.from(Namespace, "VALUE_TYPE", "UNKNOWN")).toString()
