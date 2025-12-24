// Package types provides type definitions and utilities for document metadata and time handling.
package types

import (
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
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

// DocumentMetadata contains metadata about a document including its timestamp.
type DocumentMetadata struct {
	At Time `json:"at"`
}

// DocumentBeginMetadata contains metadata captured at the beginning of document edit session.
type DocumentBeginMetadata struct {
	At      Time                  `json:"at"`
	ID      identifier.Identifier `json:"id"`
	Version store.Version         `json:"version"`
}

// DocumentEndMetadata contains metadata captured at the end of document edit session.
type DocumentEndMetadata struct {
	At        Time                   `json:"at"`
	Discarded bool                   `json:"discarded,omitempty"`
	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

// DocumentChangeMetadata contains metadata about document changes.
type DocumentChangeMetadata struct {
	At Time `json:"at"`
}

// NoMetadata represents an empty metadata structure.
type NoMetadata struct{}
