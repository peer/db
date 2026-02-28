// Use it imported from search.ts. This is here because we use it in types.d.ts as well.

// During development, Vite can optimize dependencies and can duplicate imports and thus symbols.
// So we use Symbol.for to make sure that symbols are deduplicated. Also symbol name is useful for debugging.
// @ts-expect-error -- See: https://github.com/microsoft/TypeScript/issues/63203
export const NONE: unique symbol = import.meta.env.DEV ? Symbol.for("peerdb-none") : Symbol()
