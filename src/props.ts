import { v5 as uuidv5, parse as uuidParse } from "uuid"
import bs58 from "bs58"

const idLength = 22
const nameSpaceStandardProperties = "34cd10b4-5731-46b8-a6dd-45444680ca62"
const nameSpaceWikidata = "8f8ba777-bcce-4e45-8dd4-a328e6722c82"

function identifierFromUUID(uuid: string): string {
  const res = bs58.encode(uuidParse(uuid) as Uint8Array)
  if (res.length < idLength) {
    return "1".repeat(idLength - res.length) + res
  }
  return res
}

function getID(namespace: string, ...args: string[]): string {
  let res = namespace
  for (const arg of args) {
    res = uuidv5(arg, res)
  }
  return identifierFromUUID(res)
}

export function getStandardPropertyID(mnemonic: string): string {
  return getID(nameSpaceStandardProperties, mnemonic)
}

export function getWikidataDocumentID(id: string): string {
  return getID(nameSpaceWikidata, id)
}

export const DESCRIPTION = getStandardPropertyID("DESCRIPTION")
export const ORIGINAL_CATALOG_DESCRIPTION = getWikidataDocumentID("P10358")
export const TITLE = getWikidataDocumentID("P1476")
export const LABEL = getStandardPropertyID("LABEL")
export const IS = getStandardPropertyID("IS")
export const INSTANCE_OF = getWikidataDocumentID("P31")
export const SUBCLASS_OF = getWikidataDocumentID("P279")
export const MEDIAWIKI_MEDIA_TYPE = getStandardPropertyID("MEDIAWIKI_MEDIA_TYPE")
export const MEDIA_TYPE = getStandardPropertyID("MEDIA_TYPE")
export const COPYRIGHT_STATUS = getWikidataDocumentID("P6216")
export const PREVIEW_URL = getStandardPropertyID("PREVIEW_URL")
export const LIST = getStandardPropertyID("LIST")
export const ORDER = getStandardPropertyID("ORDER")
export const ARTICLE = getStandardPropertyID("ARTICLE")
export const FILE_URL = getStandardPropertyID("FILE_URL")
