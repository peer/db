// hash-wasm's type definitions reference Node's Buffer in a union type (one we never use - we pass
// Uint8Array). The app is built for the browser without @types/node, so we declare a minimal global
// Buffer alias to satisfy the type checker without pulling in all of Node's types.
type Buffer = Uint8Array
