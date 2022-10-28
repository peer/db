/// <reference types="vitest" />
import { resolve } from "path"
import { defineConfig } from "vite"
import vue from "@vitejs/plugin-vue"

// https://vitejs.dev/config/
// https://vitest.dev/config/
export default defineConfig({
  plugins: [vue()],
  server: {
    strictPort: true,
    hmr: {
      // We use a different port for HMR so that it goes
      // through our Go development proxy.
      clientPort: 8080,
    },
  },
  resolve: {
    alias: {
      "@": resolve(__dirname, "src"),
    },
  },
  build: {
    target: ["es2022"],
  },
  test: {
    coverage: {
      reporter: ["text", "cobertura", "html"],
      all: true,
    },
  },
})
