import { assert, test } from "vitest"

import { timestampToSeconds, secondsToTimestamp } from "@/utils"

test.each([
  ["2006-12-04T12:34:45Z", 1165235685n],
  ["0206-12-04T12:34:45Z", -55637321115n],
  ["0001-12-04T12:34:45Z", -62106434715n],
  ["20006-12-04T12:34:45Z", 569190371685n],
  ["0000-12-04T12:34:45Z", -62137970715n],
  ["-0001-12-04T12:34:45Z", -62169593115n],
  ["-0206-12-04T12:34:45Z", -68638706715n],
  ["-2006-12-04T12:34:45Z", -125441263515n],
  ["-20006-12-04T12:34:45Z", -693466399515n],
  ["-239999999-01-01T00:00:00Z", -7573730615596800n],
])("timestamp(%s)", (t, u) => {
  const s = timestampToSeconds(t)
  assert.equal(s, u)
  assert.equal(secondsToTimestamp(s), t)
})
