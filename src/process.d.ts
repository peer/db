// We do not want to use import.meta.env because it gets replaced at the library build time.
// We want that user of PeerDB (as the library) to decide if is development or production.
// Bundler like Vite then processes process.env.NODE_ENV even when building for the frontend
// so we just want declare its type here (instead of pulling in full @types/node types).
declare let process: {
  env: {
    NODE_ENV: string
  }
}
