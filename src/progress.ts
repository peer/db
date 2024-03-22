import type { InjectionKey, Ref } from "vue"

export const progressKey = Symbol() as InjectionKey<Ref<number>>
