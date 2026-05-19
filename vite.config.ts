import VueI18n from "@intlify/unplugin-vue-i18n/vite"
import tailwindcss from "@tailwindcss/vite"
import vue from "@vitejs/plugin-vue"
import path from "path"
import license from "rollup-plugin-license"
import istanbul from "vite-plugin-istanbul"
import { configDefaults, defineConfig } from "vitest/config"

// https://vite.dev/config/
// https://vitest.dev/config/
export default defineConfig({
  define: {
    __VUE_OPTIONS_API__: false,
  },
  plugins: [
    vue(),
    VueI18n({
      include: [path.resolve(import.meta.dirname, "src/locales/**")],
      runtimeOnly: true,
      compositionOnly: true,
      dropMessageCompiler: true,
      fullInstall: true,
      forceStringify: true,
    }),
    istanbul({
      include: "src/**/*",
      exclude: ["node_modules", "tests/"],
      extension: [".js", ".ts", ".vue"],
      // Only instrument for E2E coverage when VITE_COVERAGE is set to "true".
      requireEnv: true,
      forceBuildInstrument: true,
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
          file: path.join(import.meta.dirname, "dist", "NOTICE.txt"),
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
    // Disable minification for E2E tests to improve debugging.
    minify: !process.env.VITE_E2E_TESTS,
  },
  test: {
    exclude: [...configDefaults.exclude, "**/tests/**"],
    setupFiles: ["src/test-setup.ts"],
    coverage: {
      include: ["src/**/*.{ts,vue}"],
      exclude: ["**/tests/**", "**/*.d.ts"],
      provider: "istanbul", // Required for nyc compatibility.
      reporter: ["json", "html", "text"], // JSON required for nyc compatibility.
      reportsDirectory: "./coverage/vitest",
    },
  },
  esbuild: {
    legalComments: "none",
  },
})
