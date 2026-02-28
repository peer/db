import type { InjectionKey, Ref } from "vue"

import { inject, ref } from "vue"

type Config = {
  fixedNavbar?: boolean
}

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
export const configKey: InjectionKey<Ref<Config>> = process.env.NODE_ENV !== "production" ? Symbol.for("peerdb-config") : Symbol()

// getConfig returns the config (as provided with configKey).
export function getConfig(): Ref<Config> {
  return inject(configKey, ref({}))
}
