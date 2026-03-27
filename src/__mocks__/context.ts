import type { SiteContext } from "@/types"

import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"

const siteContext: SiteContext = {
  domain: "test.example.com",
  title: "Test Site",
  languagePriority: {
    en: ["sl"],
    sl: ["en"],
  },
  languageCodes: {
    [(await Identifier.from(Namespace, "LANGUAGE", "en-GB")).toString()]: "en",
    [(await Identifier.from(Namespace, "LANGUAGE", "sl-SI")).toString()]: "sl",
  },
  features: {},
}

export default siteContext
