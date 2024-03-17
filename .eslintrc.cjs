/* global module */

module.exports = {
  root: true,
  env: {
    browser: true,
    es2020: true,
    "vue/setup-compiler-macros": true,
  },
  extends: ["eslint:recommended", "plugin:vue/vue3-recommended", "@vue/typescript/recommended", "prettier"],
  rules: {
    "vue/multi-word-component-names": ["off"],
    "vue/no-v-html": ["off"],
    "vue/no-v-text-v-html-on-component": [
      "error",
      {
        "allow": ["RouterLink"],
      },
    ],
    "@typescript-eslint/no-unused-vars": [
      "error",
      {
        args: "none",
      },
    ],
  },
  parser: "vue-eslint-parser",
  parserOptions: {
    parser: "@typescript-eslint/parser",
    ecmaVersion: "latest",
    sourceType: "module",
  },
}
