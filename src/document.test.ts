import { assert, test } from "vitest"

import { Identifier } from "@tozd/identifier"
import { Changes, PeerDBDocument } from "./document"

test("patch json", () => {
  const id1 = "LpcGdCUThc22mhuBwQJQ5Z"
  const id2 = "AyNNP5CVsSx3w9b75erF1m"
  const prop1 = "XkbTJqwFCFkfoxMBXow4HU"
  const prop2 = "3EL2nZdWVbw85XG1zTH2o5"

  const changes = new Changes(
    {
      type: "add",
      patch: {
        type: "amount",
        prop: prop1,
        amount: 42.1,
        unit: "°C",
      },
    },
    {
      type: "add",
      under: id1,
      patch: {
        type: "id",
        prop: prop2,
        value: "foobar",
      },
    },
  )

  const out = JSON.stringify(changes)
  assert.equal(
    out,
    `[{"type":"add","patch":{"type":"amount","prop":"XkbTJqwFCFkfoxMBXow4HU","amount":42.1,"unit":"°C"}},{"type":"add","under":"LpcGdCUThc22mhuBwQJQ5Z","patch":{"type":"id","prop":"3EL2nZdWVbw85XG1zTH2o5","value":"foobar"}}]`,
  )

  const changes2 = new Changes(...JSON.parse(out))
  assert.deepEqual(changes, changes2)

  const id = Identifier.new().toString()
  const doc = new PeerDBDocument({
    id: id,
    score: 1.0,
  })
  const base = "TqtRsbk7rTKviW3TJapTim"
  changes.Apply(doc, base)
  assert.deepEqual(
    new PeerDBDocument({
      id: id,
      score: 1.0,
      claims: {
        amount: [
          {
            id: id1,
            confidence: 1.0,
            meta: {
              id: [
                {
                  id: id2,
                  confidence: 1.0,
                  prop: {
                    id: prop2,
                    score: 1.0,
                  },
                  value: "foobar",
                },
              ],
            },
            prop: {
              id: prop1,
              score: 1.0,
            },
            amount: 42.1,
            unit: "°C",
          },
        ],
      },
    }),
    doc,
  )
})
