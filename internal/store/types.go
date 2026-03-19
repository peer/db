package store

import (
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/document"
)

// RFC3339Milli is the time format string for RFC3339 with millisecond precision.
const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamp which is represented in JSON with millisecond
// precision and not (Go default) nanosecond precision.
type Time time.Time

// MarshalJSON marshals Time to JSON with millisecond precision.
func (t Time) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(RFC3339Milli)+len(`""`))
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, RFC3339Milli)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON unmarshals Time from JSON with millisecond precision.
func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("Time.UnmarshalJSON: input is not a JSON string")
	}
	data = data[len(`"`) : len(data)-len(`"`)]
	tt, err := time.Parse(RFC3339Milli, string(data))
	*t = Time(tt)
	return errors.WithStack(err)
}

// InverseRelation contains data about a relation claim from another document.
// When document A has a relation claim with property X pointing to document B,
// then document B's metadata will contain an InverseRelation entry.
type InverseRelation struct {
	// Claim is the ID of the relation claim in the source document (A).
	Claim identifier.Identifier `json:"claim"`
	// Document is the ID of the source document (A) that has the forward relation claim.
	Document identifier.Identifier `json:"document"`
	// Prop is the property ID of the forward relation claim in the source document (X).
	Prop identifier.Identifier `json:"prop"`
	// Confidence is the confidence of the forward relation claim.
	Confidence document.Confidence `json:"confidence"`
}

// NoMetadata represents an empty metadata structure.
type NoMetadata struct{}
