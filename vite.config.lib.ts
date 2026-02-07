import VueI18n from "@intlify/unplugin-vue-i18n/vite"
import vue from "@vitejs/plugin-vue"
import { glob } from "glob"
import path from "path"
import url from "url"
import { defineConfig } from "vite"
import { viteStaticCopy } from "vite-plugin-static-copy"

const __dirname = path.dirname(url.fileURLToPath(import.meta.url))

// Read peer dependencies from package.json to use as externals.
const packageJson = await import("./package.json", { with: { type: "json" } })
const peerDependencies = Object.keys(packageJson.default.peerDependencies || {})

const entries = await Promise.all([glob("src/**/*.vue", { cwd: __dirname }), glob("src/**/*.ts", { cwd: __dirname })]).then(([vueFiles, tsFiles]) =>
  [...vueFiles, ...tsFiles]
    .filter((file) => !file.includes(".test.") && !file.endsWith(".d.ts") && !file.endsWith(".css"))
    .reduce(
      (acc, file) => {
        // Strip .ts and .vue extensions from the filename.
        const name = file.replace(/^src\//, "").replace(/\.(vue|ts)$/, "")
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
  plugins: [
    vue(),
    VueI18n({
      include: [path.resolve(__dirname, "src/locales/**")],
      runtimeOnly: true,
      compositionOnly: true,
      dropMessageCompiler: true,
      fullInstall: true,
      forceStringify: true,
    }),
    viteStaticCopy({
      targets: [
        {
          src: "src/theme.css",
          dest: ".",
        },
        {
          src: "src/**/*.d.ts",
          dest: ".",
        },
      ],
    }),
  ],
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
