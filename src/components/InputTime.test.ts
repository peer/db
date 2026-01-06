import { assert, test } from "vitest"

import { normalizeForParsing } from "./InputTime.vue"

test.each([
  ["2022-01-01T00:00:00", "2022-01-01 00:00:00"],
])("getCorePropertyID(%s)", (input, output) => {
  // TODO: Enable once eslint parser for extra files is used.
  //       See: https://github.com/ota-meshi/typescript-eslint-parser-for-extra-files/issues/162
  // eslint-disable-next-line @typescript-eslint/no-unsafe-call
  assert.equal(normalizeForParsing(input), output)
})
