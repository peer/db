// TODO: Improve by using size prefixes for some units (e.g., KB).
//       Both for large and small numbers (e.g., micro gram).
export function formatValue(value: number, unit: string): string {
  let res = parseFloat(value.toPrecision(5)).toString()
  if (unit !== "1") {
    res += unit
  }
  return res
}
