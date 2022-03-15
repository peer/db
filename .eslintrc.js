/* global module */

module.exports = {
  root: true,
  env: {
    browser: true,
    "vue/setup-compiler-macros": true,
  },
  extends: [
    "eslint:recommended",
    "plugin:vue/vue3-recommended",
    "@vue/typescript/recommended",
    "prettier",
  ],
  rules: {},
  parser: "vue-eslint-parser",
  parserOptions: {
    parser: "@typescript-eslint/parser",
    ecmaVersion: "latest",
    sourceType: "module",
  },
}
