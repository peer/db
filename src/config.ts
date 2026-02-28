import type { InjectionKey, Ref } from "vue"

import { inject, ref } from "vue"

type Config = {
  fixedNavbar?: boolean
}

export const configKey: InjectionKey<Ref<Config>> = Symbol()

// getConfig returns the config (as provided with configKey).
export function getConfig(): Ref<Config> {
  return inject(configKey, ref({}))
}
