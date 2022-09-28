import { getStandardPropertyID, getWikidataDocumentID } from "@/utils"

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
