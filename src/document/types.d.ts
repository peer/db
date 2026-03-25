// Confidence represents the confidence level of a claim.
//
// Its range is [-1, 1] where negative value represents a
// confidence in a negation of the claim.
export type Confidence = number

// TimePrecision represents the precision level of a timestamp.
// TODO: Add "ms", "us", "ns" support.
export type TimePrecision = "G" | "100M" | "10M" | "M" | "100k" | "10k" | "k" | "100y" | "10y" | "y" | "m" | "d" | "h" | "min" | "s"

// Amount represents a numeric amount.
//
// It generally operates with an additional piece of information
// which is not part of the amount itself:
//
//   - precision: the rounding precision of the amount
//
// It is represented as a string to preserve the original format
// as provided by the user. The format is a decimal number with
// an optional sign and an optional decimal part separated by
// a dot or comma.
export type Amount = string

// Timestamp represents a point in time.
//
// It generally operates with two additional pieces of information
// which are not part of the timestamp itself:
//
//   - [TimePrecision]: precision of the timestamp
//   - [time.Location]: location (timezone) of the timestamp
//
// It is represented as a string to preserve the original format
// as provided by the user. The format is RFC 3339 compatible with
// the following changes:
//
//   - year component can have more than 4 digits and can have a negative sign
//   - supports milliseconds, microseconds and nanoseconds with exactly 3, 6, or
//     9 decimal fraction digits, respectively
//   - day component can be zero for timestamps used with month precision,
//     but month component cannot be zero
//   - timestamp can contain just the part of the format when used with precision
//     which does not require other parts, parts are in order: a) year, b) month + day,
//     c) hours + minutes, d) seconds, e) milliseconds, f) microseconds, and g) nanoseconds
//   - instead of T delimiter, a space is used
//   - location (timezone) must not be present
export type Timestamp = string

// Reference represents a reference to another document.
export type Reference = {
  id: string
}
