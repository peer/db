import { assert, describe, test } from "vitest"

import { getFieldInputComponents, registerFieldInputComponent } from "@/registry/field-input"

describe("field input registry", () => {
  test("registers PeerDB's own inputs under their component names", () => {
    const components = getFieldInputComponents().value
    // The registration globs the input directory, so every input file is in, named after its file.
    for (const name of ["InputAmount", "InputFile", "InputHTML", "InputIdentifier", "InputIdentity", "InputLink", "InputRef", "InputString", "InputTime"]) {
      assert.isTrue(components.has(name), `${name} is not registered`)
    }
    // Only components are registered, not the modules the inputs are built from.
    for (const name of [...components.keys()]) {
      assert.match(name, /^Input[A-Za-z]+$/)
    }
  })

  test("registering a component makes it available under its name", () => {
    const component = { name: "TestInput", template: "<div />" }
    registerFieldInputComponent("TestInput", component)
    assert.equal(getFieldInputComponents().value.get("TestInput"), component)
  })
})
