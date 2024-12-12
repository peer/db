import { v5 as uuidv5 } from "uuid"
import { Identifier } from "@tozd/identifier"

const nameSpaceCoreProperties = "34cd10b4-5731-46b8-a6dd-45444680ca62"
const nameSpaceWikidata = "8f8ba777-bcce-4e45-8dd4-a328e6722c82"

function getID(namespace: string, ...args: string[]): string {
  let res = namespace
  for (const arg of args) {
    res = uuidv5(arg, res)
  }
  return Identifier.fromUUID(res).toString()
}

export function getCorePropertyID(mnemonic: string): string {
  return getID(nameSpaceCoreProperties, mnemonic)
}

export function getWikidataDocumentID(id: string): string {
  return getID(nameSpaceWikidata, id)
}

export const DESCRIPTION = getCorePropertyID("DESCRIPTION")
export const ORIGINAL_CATALOG_DESCRIPTION = getWikidataDocumentID("P10358")
export const TITLE = getWikidataDocumentID("P1476")
export const LABEL = getCorePropertyID("LABEL")
export const TYPE = getCorePropertyID("TYPE")
export const INSTANCE_OF = getWikidataDocumentID("P31")
export const SUBCLASS_OF = getWikidataDocumentID("P279")
export const MEDIAWIKI_MEDIA_TYPE = getCorePropertyID("MEDIAWIKI_MEDIA_TYPE")
export const MEDIA_TYPE = getCorePropertyID("MEDIA_TYPE")
export const COPYRIGHT_STATUS = getWikidataDocumentID("P6216")
export const PREVIEW_URL = getCorePropertyID("PREVIEW_URL")
export const LIST = getCorePropertyID("LIST")
export const ORDER = getCorePropertyID("ORDER")
export const ARTICLE = getCorePropertyID("ARTICLE")
export const FILE_URL = getCorePropertyID("FILE_URL")
export const DEPARTMENT = getCorePropertyID("DEPARTMENT")
export const CLASSIFICATION = getCorePropertyID("CLASSIFICATION")
export const MEDIUM = getCorePropertyID("MEDIUM")
export const NATIONALITY = getCorePropertyID("NATIONALITY")
export const GENDER = getCorePropertyID("GENDER")
export const NAME = getCorePropertyID("NAME")
export const CATEGORY = getCorePropertyID("CATEGORY")
export const INGREDIENTS = getCorePropertyID("INGREDIENTS")
