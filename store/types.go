package store

import (
	"time"

	"github.com/mohae/deepcopy"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
)

// RFC3339Milli is the time format string for RFC3339 with millisecond precision.
const RFC3339Milli = "2006-01-02T15:04:05.000Z07:00"

// Time is a timestamp which is represented in JSON with millisecond
// precision and not (Go default) nanosecond precision.
//
//nolint:recvcheck
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

// DeepCopy implements the github.com/mohae/deepcopy Interface so deepcopy.Copy copies a Time by value.
// Without it deepcopy reflects into the struct and, because Time is a named type over time.Time rather than
// the exact time.Time it special-cases, skips time.Time's unexported fields and zeroes the timestamp. A Time
// is a plain value, so returning it is a complete copy.
func (t Time) DeepCopy() any {
	return t
}

var _ deepcopy.Interface = Time{}

// DocumentMetadata contains metadata about a document including its timestamp.
type DocumentMetadata struct {
	At Time `json:"at"`

	// Users is the deduplicated, sorted-by-ID union of users who contributed
	// to this version: the user who began the edit session plus every user who
	// appended a change. The user who ended the session (committer) is NOT
	// included here; that user goes to CommitMetadata.User instead.
	Users []User `json:"users,omitempty"`
}

// CommitMetadata contains metadata about a commit.
type CommitMetadata struct {
	Base []string `json:"base,omitempty"`

	// User is the user who invoked the End that produced this commit.
	// nil when the commit was made by an unauthenticated caller.
	User *User `json:"user,omitempty"`
}

// ChangesetID implements store.ChangesetID interface.
func (c *CommitMetadata) ChangesetID() identifier.Identifier {
	if len(c.Base) == 0 {
		panic(errors.New("base is empty"))
	}
	return identifier.From(c.Base...)
}

// NoMetadata represents an empty metadata structure.
type NoMetadata struct{}
