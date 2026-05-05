// Amount parsing and validation, mirroring document/amount.go.

const amountRegex = /^(-?\d+)(?:[.,](\d+))?$/

// amountFloat64 parses an Amount string and returns its float64
// representation, validating the format and (when precision != 0) the
// rounding-to-precision invariant. Passing 0 for precision skips precision
// checks (just validates format).
//
// Mirrors Amount.Float64 in document/amount.go.
export function amountFloat64(amount: string, precision: number): number {
  const match = amountRegex.exec(amount)
  if (!match) {
    throw new Error("unable to parse amount")
  }

  let numStr = match[1]
  if (match[2]) {
    numStr += "." + match[2]
  }

  const value = parseFloat(numStr)
  if (!Number.isFinite(value)) {
    throw new Error("amount must be a finite number")
  }

  if (precision !== 0) {
    if (!Number.isFinite(precision) || precision <= 0) {
      throw new Error("precision must be a finite positive number")
    }

    let expectedDecimals = 0
    if (precision < 1) {
      expectedDecimals = Math.ceil(-Math.log10(precision))
    }
    const actualDecimals = match[2] ? match[2].length : 0
    if (actualDecimals !== expectedDecimals) {
      throw new Error("number of decimal digits does not match precision")
    }

    const rounded = Math.round(value / precision) * precision
    const v = value.toFixed(expectedDecimals)
    const r = rounded.toFixed(expectedDecimals)
    if (v !== r) {
      throw new Error("amount is not rounded to precision")
    }

    return rounded
  }

  return value
}

// validateAmount checks if the amount is valid for the given precision.
// Passing 0 for precision skips precision checks and just checks the format.
export function validateAmount(amount: string, precision: number): void {
  amountFloat64(amount, precision)
}

// amountWindowStart returns the lower edge that this bound contributes to
// a half-open indexed range. When the bound is closed (default,
// isOpen=false) this is the start of the precision window; when open
// (isOpen=true) the precision window is excluded and the edge advances to
// the end of the window.
export function amountWindowStart(amount: string, precision: number, isOpen: boolean): number {
  const value = amountFloat64(amount, precision)
  if (isOpen) {
    return value + precision / 2
  }
  return value - precision / 2
}

// amountWindowEnd returns the upper edge that this bound contributes to a
// half-open indexed range. When the bound is closed (default,
// isOpen=false) this is the end of the precision window; when open
// (isOpen=true) the precision window is excluded and the edge retreats to
// the start of the window.
export function amountWindowEnd(amount: string, precision: number, isOpen: boolean): number {
  const value = amountFloat64(amount, precision)
  if (isOpen) {
    return value - precision / 2
  }
  return value + precision / 2
}
