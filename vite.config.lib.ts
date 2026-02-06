import vue from "@vitejs/plugin-vue"
import { glob } from "glob"
import path from "path"
import url from "url"
import { defineConfig } from "vite"

const __dirname = path.dirname(url.fileURLToPath(import.meta.url))

// Read peer dependencies from package.json to use as externals.
const packageJson = await import("./package.json", { with: { type: "json" } })
const peerDependencies = Object.keys(packageJson.default.peerDependencies || {})

const entries = await Promise.all([glob("src/**/*.vue", { cwd: __dirname }), glob("src/**/*.ts", { cwd: __dirname })]).then(([vueFiles, tsFiles]) =>
  [...vueFiles, ...tsFiles]
    .filter((file) => !file.includes(".test.") && !file.endsWith(".d.ts") && !file.endsWith(".css"))
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
    // The library does not need LICENSE.txt and robots.txt bundled.
    copyPublicDir: false,
    rollupOptions: {
      onwarn(warning, warn) {
        // Suppress "empty chunk" warning for intentionally empty index file.
        if (warning.code === "EMPTY_BUNDLE" && warning.message.includes("index")) {
          return
        }
        warn(warning)
      },
      external: (id) => {
        // Externalize CSS files - consumers have to use their own TailwindCSS setup.
        if (id.endsWith(".css")) {
          return true
        }
        // Externalize all peer dependencies.
        return peerDependencies.some((peer) => id === peer || id.startsWith(peer + "/"))
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
