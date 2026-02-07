#!/usr/bin/env node

/**
 * Script to automatically generate .d.ts TypeScript proxy definitions for
 * .vue components, so that one can import components without .vue extension.
 */

import fs from "fs"
import { glob } from "glob"
import path from "path"

const LIB_DIR = "lib"

const files = glob.sync(`${LIB_DIR}/**/*.vue.d.ts`)

for (const file of files) {
  const dir = path.dirname(file)
  const base = path.basename(file, ".vue.d.ts")
  const proxyFile = path.join(dir, base + ".d.ts")

  const content = `export * from "./${base}.vue"\nexport { default } from "./${base}.vue"\n`

  fs.writeFileSync(proxyFile, content)
  console.log("Generated proxy:", proxyFile)
}

console.log(`✅ Generated ${files.length} .d.ts proxies for .vue files.`)
