// Filesystem-safe filename helpers. Mirrors gitlab.com/tozd/go/x SafeFilename so frontend
// and backend produce equivalent results.

// Reserved device names on Windows.
const RESERVED_NAMES: ReadonlySet<string> = new Set([
  "CON",
  "PRN",
  "AUX",
  "NUL",
  "COM1",
  "COM2",
  "COM3",
  "COM4",
  "COM5",
  "COM6",
  "COM7",
  "COM8",
  "COM9",
  "LPT1",
  "LPT2",
  "LPT3",
  "LPT4",
  "LPT5",
  "LPT6",
  "LPT7",
  "LPT8",
  "LPT9",
])

// Characters disallowed in filenames on common filesystems plus C0 control characters.
// eslint-disable-next-line no-control-regex
const INVALID_CHARS = /[<>:"/\\|?*\x00-\x1F]/g

// Trailing dots and spaces are not allowed on Windows.
const TRAILING_DOTS_OR_SPACES = /[. ]+$/

// Returns a safe filename for the given name.
export function safeFilename(name: string): string {
  // Trim leading and trailing whitespace.
  name = name.trim()

  // Windows does not allow trailing spaces or dots.
  name = name.replace(TRAILING_DOTS_OR_SPACES, "")

  // Replace invalid characters with underscore.
  name = name.replace(INVALID_CHARS, "_")

  // Prevent empty filename.
  if (name === "") {
    return "_"
  }

  // Check reserved device names (case-insensitive, without extension).
  let base: string
  let ext: string
  const dotIndex = name.indexOf(".")
  if (dotIndex !== -1) {
    base = name.substring(0, dotIndex).toUpperCase()
    ext = name.substring(dotIndex)
  } else {
    base = name.toUpperCase()
    ext = ""
  }

  if (RESERVED_NAMES.has(base)) {
    name = "_" + base + ext
  }

  return name
}
