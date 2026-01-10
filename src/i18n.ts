import { createI18n } from "vue-i18n"

import en from "@/locales/en.json"
import sl from "@/locales/sl.json"

const messages = {
  en,
  sl,
}

export const i18n = createI18n({
  legacy: false,
  locale: "en",
  fallbackLocale: "en",
  globalInjection: false,
  escapeParameter: true,
  messages,
  pluralRules: {
    sl: (choice: number, choicesLength: number) => {
      if (choicesLength === 1) {
        return 0
      }
      if (choicesLength === 2) {
        return choice === 1 ? 0 : 1
      }
      if (choice % 100 === 1) {
        return 0
      }
      if (choice % 100 === 2) {
        return 1
      }
      if (choice % 100 === 3 || choice % 100 === 4) {
        return 2
      }
      return 3
    },
  },
})

export default i18n
