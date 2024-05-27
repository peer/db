package types

import (
	"time"

	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
)

const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamps with is represented in JSON with millisecond
// precision and not (Go default) nanosecond precision..
type Time time.Time

func (t Time) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(RFC3339Milli)+len(`""`))
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, RFC3339Milli)
	b = append(b, '"')
	return b, nil
}

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

type DocumentMetadata struct {
	At Time `json:"at"`
}

type DocumentBeginMetadata struct {
	At      Time                  `json:"at"`
	ID      identifier.Identifier `json:"id"`
	Version store.Version         `json:"version"`
}

type DocumentEndMetadata struct {
	At        Time                   `json:"at"`
	Discarded bool                   `json:"discarded,omitempty"`
	Changeset *identifier.Identifier `json:"changeset,omitempty"`

	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

type DocumentChangeMetadata struct {
	At Time `json:"at"`
}

type NoMetadata struct{}
