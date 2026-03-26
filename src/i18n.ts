import { createI18n } from "vue-i18n"

import siteContext from "@/context"
import en from "@/locales/en.json"
import sl from "@/locales/sl.json"

const messages = {
  en,
  sl,
}

const enabledLanguages = Object.keys(siteContext.languagePriority ?? {})

async function getInitialLocale(): Promise<string> {
  // Check cookie first.
  const cookie = await cookieStore.get("language")
  if (cookie && cookie.value) {
    if (enabledLanguages.includes(cookie.value)) {
      return cookie.value
    }
  }

  // Then check browser languages.
  for (const browserLang of navigator.languages) {
    // Try exact match first, then language prefix (e.g., "en-US" -> "en").
    if (enabledLanguages.includes(browserLang)) {
      return browserLang
    }
    const prefix = browserLang.split("-")[0]
    if (enabledLanguages.includes(prefix)) {
      return prefix
    }
  }

  // Use site's default language. It should already be validated by the backend that
  // it is enabled, but we check it here just in case.
  if (siteContext.defaultLanguage && enabledLanguages.includes(siteContext.defaultLanguage)) {
    return siteContext.defaultLanguage
  }

  // Backend requires that defaultLanguage is set if more than one language is enabled.
  // So here we should return the first and only language, if there is any set.
  for (const lang of enabledLanguages) {
    return lang
  }

  // Default.
  return "en"
}

export const i18n = createI18n({
  legacy: false,
  locale: await getInitialLocale(),
  fallbackLocale: siteContext.languagePriority,
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
