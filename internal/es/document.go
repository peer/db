package es

import (
	"time"

	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/store"
)

type DocumentMetadata struct {
	At time.Time `json:"at"`
}

type DocumentBeginMetadata struct {
	At      time.Time             `json:"at"`
	ID      identifier.Identifier `json:"id"`
	Version store.Version         `json:"version"`
}

type DocumentEndMetadata struct {
	At        time.Time              `json:"at"`
	Discarded bool                   `json:"discarded,omitempty"`
	Changeset *identifier.Identifier `json:"changeset,omitempty"`
	// Processing time in milliseconds.
	Time int64 `json:"time,omitempty"`
}

type DocumentChangeMetadata struct {
	At time.Time `json:"at"`
}
