// Time functions based on Go's time.Time from version 1.23.

const secondsPerMinute = 60n
const secondsPerHour = 60n * secondsPerMinute
const secondsPerDay = 24n * secondsPerHour
const daysPer400Years = 365n * 400n + 97n
const daysPer100Years = 365n * 100n + 24n
const daysPer4Years = 365n * 4n + 1n
const internalYear = 1n
const absoluteZeroYear = -292277022399n
const absoluteToInternal = BigInt(Number(absoluteZeroYear - internalYear) * 365.2425) * secondsPerDay
const internalToAbsolute = -absoluteToInternal
const unixToInternal = (1969n * 365n + 1969n / 4n - 1969n / 100n + 1969n / 400n) * secondsPerDay
const internalToUnix = -unixToInternal

const daysBefore = [
  0n,
  31n,
  31n + 28n,
  31n + 28n + 31n,
  31n + 28n + 31n + 30n,
  31n + 28n + 31n + 30n + 31n,
  31n + 28n + 31n + 30n + 31n + 30n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n + 31n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n + 31n + 30n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n + 31n + 30n + 31n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n + 31n + 30n + 31n + 30n,
  31n + 28n + 31n + 30n + 31n + 30n + 31n + 31n + 30n + 31n + 30n + 31n,
]

function norm(hi: number, lo: number, base: number): [number, number] {
  if (lo < 0) {
    const n = Math.floor((-lo - 1) / base) + 1
    hi -= n
    lo += n * base
  }
  if (lo >= base) {
    const n = Math.floor(lo / base)
    hi += n
    lo -= n * base
  }
  return [hi, lo]
}

function daysSinceEpoch(year: number): bigint {
  let y = BigInt(year) - absoluteZeroYear

  // Add in days from 400-year cycles.
  let n = y / 400n
  y -= 400n * n
  let d = daysPer400Years * n

  // Add in 100-year cycles.
  n = y / 100n
  y -= 100n * n
  d += daysPer100Years * n

  // Add in 4-year cycles.
  n = y / 4n
  y -= 4n * n
  d += daysPer4Years * n

  // Add in non-leap years.
  n = y
  d += 365n * n

  return d
}

export function isLeap(year: number): boolean {
  return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0)
}

export function daysIn(month: number, year: number): number {
  // February.
	if (month == 2 && isLeap(year)) {
		return 29
	}
	return Number(daysBefore[month] - daysBefore[month-1])
}

function toAbs(unixSeconds: bigint): bigint {
  return unixSeconds + (unixToInternal + internalToAbsolute)
}

export function toDate(unixSeconds: bigint): [number, number, number] {
  // Split into time and day.
  let d = toAbs(unixSeconds) / secondsPerDay

  // Account for 400 year cycles.
  let n = d / daysPer400Years
  let y = 400n * n
  d -= daysPer400Years * n

  // Cut off 100-year cycles.
  // The last cycle has one extra leap year, so on the last day
  // of that year, day / daysPer100Years will be 4 instead of 3.
  // Cut it back down to 3 by subtracting n>>2.
  n = d / daysPer100Years
  n -= n >> 2n
  y += 100n * n
  d -= daysPer100Years * n

  // Cut off 4-year cycles.
  // The last cycle has a missing leap year, which does not
  // affect the computation.
  n = d / daysPer4Years
  y += 4n * n
  d -= daysPer4Years * n

  // Cut off years within a 4-year cycle.
  // The last year is a leap year, so on the last day of that year,
  // day / 365 will be 4 instead of 3. Cut it back down to 3
  // by subtracting n>>2.
  n = d / 365n
  n -= n >> 2n
  y += n
  d -= 365n * n

  const year = Number(y + absoluteZeroYear)

  let day = Number(d)
  if (isLeap(year)) {
    // Leap year
    if (day > 31 + 29 - 1) {
      // After leap day; pretend it wasn't there.
      day--
    } else if (day === 31 + 29 - 1) {
      // Leap day.
      return [year, 2, 29]
    }
  }

  // Estimate month on assumption that every month has 31 days.
  // The estimate may be too low by at most one month, so adjust.
  let month = Math.floor(day / 31)
  const end = Number(daysBefore[month + 1])
  let begin
  if (day >= end) {
    month++
    begin = end
  } else {
    begin = Number(daysBefore[month])
  }

  month++ // because January is 1
  day = day - begin + 1
  return [year, month, day]
}

export function hour(unixSeconds: bigint): number {
  return Number((toAbs(unixSeconds) % secondsPerDay) / secondsPerHour)
}

export function minute(unixSeconds: bigint): number {
  return Number((toAbs(unixSeconds) % secondsPerHour) / secondsPerMinute)
}

export function second(unixSeconds: bigint): number {
  return Number(toAbs(unixSeconds) % secondsPerMinute)
}

export function fromDate(year: number, month: number, day: number, hour: number, min: number, sec: number): bigint {
  // Normalize month, overflowing into year.
  const m = month - 1
  const [year2, m2] = norm(year, m, 12)
  month = m2 + 1
  year = year2

  // Normalize sec, min, hour, overflowing into day.
  const [min2, sec2] = norm(min, sec, 60)
  min = min2
  sec = sec2
  const [hour2, min3] = norm(hour, min, 60)
  hour = hour2
  min = min3
  const [day2, hour3] = norm(day, hour, 24)
  day = day2
  hour = hour3

  // Compute days since the absolute epoch.
  let d = daysSinceEpoch(year)

  // Add in days before this month.
  d += daysBefore[month - 1]
  if (isLeap(year) && month >= 3) {
    d++ // February 29
  }

  // Add in days before today.
  d += BigInt(day) - 1n

  // Add in time elapsed today.
  let abs = d * secondsPerDay
  abs += BigInt(hour) * secondsPerHour + BigInt(min) * secondsPerMinute + BigInt(sec)

  const unix = abs + (absoluteToInternal + internalToUnix)

  return unix
}
