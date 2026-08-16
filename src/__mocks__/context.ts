import type { SiteContext, UserInfo } from "@/types"

import { Identifier } from "@tozd/identifier"

import { Namespace } from "@/core/namespace"
import { ALTERNATIVE_NAME, NAME, NAMING, SHORT_NAME, TITLE } from "@/core/properties"

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
  namingProperties: [NAME, TITLE, SHORT_NAME, ALTERNATIVE_NAME, NAMING],
  features: {},
}

// Mirror the named exports of the real @/context so modules that read
// initialRoles / initialUserInfo (eg. @/auth) work under the test mock.
// Tests can override these via vi.doMock if they need a signed-in state.
export const initialRoles: string[] = []
export const initialUserInfo: UserInfo | null = null

export default siteContext
