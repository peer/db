import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"

// Well-known core class IDs based on their mnemonics.
//
// Keep this list in sync with internal/core/classes.go and sorted alphabetically.
export const CLASS = (await Identifier.from(Namespace, "CLASS")).toString()
export const DOCUMENT = (await Identifier.from(Namespace, "DOCUMENT")).toString()
export const LANGUAGE = (await Identifier.from(Namespace, "LANGUAGE")).toString()
export const PROPERTY = (await Identifier.from(Namespace, "PROPERTY")).toString()
export const UNIT = (await Identifier.from(Namespace, "UNIT")).toString()
export const VALUE_TYPE = (await Identifier.from(Namespace, "VALUE_TYPE")).toString()
export const VOCABULARY = (await Identifier.from(Namespace, "VOCABULARY")).toString()
