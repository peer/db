import vue from "@vitejs/plugin-vue"
import { glob } from "glob"
import path from "path"
import url from "url"
import { defineConfig } from "vite"

const __dirname = path.dirname(url.fileURLToPath(import.meta.url))

const entries = await Promise.all([glob("src/**/*.vue", { cwd: __dirname }), glob("src/**/*.ts", { cwd: __dirname })]).then(([vueFiles, tsFiles]) =>
  [...vueFiles, ...tsFiles]
    .filter((file) => !file.includes(".test.") && !file.endsWith(".d.ts"))
    .reduce(
      (acc, file) => {
        // Keep .vue extension in name, only strip .ts.
        const name = file.replace(/^src\//, "").replace(/\.ts$/, "")
        acc[name] = path.resolve(__dirname, file)
        return acc
      },
      {} as Record<string, string>,
    ),
)

export default defineConfig({
  define: {
    __VUE_OPTIONS_API__: false,
  },
  plugins: [vue()],
  resolve: {
    alias: {
      "@": "/src",
    },
  },
  build: {
    lib: {
      entry: entries,
      formats: ["es"],
    },
    rollupOptions: {
      onwarn(warning, warn) {
        // Suppress "empty chunk" warning for intentionally empty index file.
        if (warning.code === "EMPTY_BUNDLE" && warning.message.includes("index")) {
          return
        }
        warn(warning)
      },
      external: (id) => {
        const externals = [
          "@all1ndev/vue-local-scope",
          "@headlessui/vue",
          "@heroicons/vue",
          "@sidekickicons/vue",
          "@tozd/identifier",
          "esm-seedrandom",
          "lodash-es",
          "nouislider",
          "structured-field-values",
          "uuid",
          "vue",
          "vue-router",
          "vue-i18n",
        ]
        return externals.some((ext) => id === ext || id.startsWith(ext + "/"))
      },
      output: {
        preserveModules: true,
        preserveModulesRoot: "src",
      },
      preserveEntrySignatures: "strict",
    },
    outDir: "lib",
    emptyOutDir: true,
    sourcemap: true,
    target: ["esnext"],
  },
  esbuild: {
    legalComments: "none",
  },
})
