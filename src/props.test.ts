import { assert, test } from "vitest"

import { getCorePropertyID, getWikidataDocumentID } from "@/props"

test.each([
  ["MEDIAWIKI_MEDIA_TYPE", "BfSBAS8qXcBgFkc7TmDuxK"],
  ["HAS_ARTICLE", "MQYs7JmAR3tge25eTPS8XT"],
  ["DESCRIPTION", "E7DXhBtz9UuoSG9V3uYeYF"],
  ["ARTICLE", "FJJLydayUgDuqFsRK2ZtbR"],
  ["LABEL", "5SoFeEFk5aWXUYFC1EZFec"],
  ["TYPE", "CAfaL1ZZs6L4uyFdrJZ2wN"],
])("getCorePropertyID(%s)", (m, u) => {
  assert.equal(getCorePropertyID(m), u)
})

test.each([
  ["P31", "TkGHDJvPRb2bPy7t7LDNU1"],
  ["P279", "UAWhwUnX4wwVQKERrXKg1n"],
  ["P1476", "R1jaB4dw245WMrHCMeEDEi"],
  ["P6216", "Lzs5CV1xwj9ec14h3QQWKM"],
])("getWikidataDocumentID(%s)", (m, u) => {
  assert.equal(getWikidataDocumentID(m), u)
})
