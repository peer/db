import colors from "tailwindcss/colors"
import forms from "@tailwindcss/forms"
import typography from "@tailwindcss/typography"
import headlessui from "@headlessui/tailwindcss"

/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{vue,js,ts,jsx,tsx}"],
  theme: {
    extend: {
      colors: {
        primary: colors.indigo,
        secondary: colors.yellow,
        error: colors.red,
        warning: colors.yellow,
        success: colors.green,
      },
    },
  },
  plugins: [forms, typography, headlessui],
}
