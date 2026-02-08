import VueI18n from "@intlify/unplugin-vue-i18n/vite"
import tailwindcss from "@tailwindcss/vite"
import vue from "@vitejs/plugin-vue"
import path from "path"
import license from "rollup-plugin-license"
import url from "url"
import { defineConfig } from "vitest/config"

const __dirname = path.dirname(url.fileURLToPath(import.meta.url))

// https://vitejs.dev/config/
// https://vitest.dev/config/
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
    license({
      sourcemap: true,
      thirdParty: {
        includeSelf: true,
        allow: {
          test: "(Apache-2.0 OR MIT OR BSD-2-Clause OR BSD-3-Clause OR ISC)",
          failOnUnlicensed: true,
          failOnViolation: true,
        },
        output: {
          file: path.join(__dirname, "dist", "NOTICE.txt"),
        },
      },
    }),
    tailwindcss(),
  ],
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
      "@": "/src",
    },
  },
  build: {
    sourcemap: true,
    target: ["esnext"],
    // We have dist.go file in dist directory.
    // We empty it ourselves in Makefile.
    emptyOutDir: false,
  },
  test: {
    coverage: {
      include: ["src/**/*.{ts,vue}"],
      exclude: ["**/*.d.ts"],
      provider: "v8",
      reporter: ["text", "cobertura", "html"],
    },
  },
  esbuild: {
    legalComments: "none",
  },
})
