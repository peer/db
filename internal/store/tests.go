package store

import (
	"encoding/json"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"gitlab.com/tozd/go/x"
)

// DummyData is a placeholder JSON data for testing purposes.
var DummyData = []byte(`{}`) //nolint:gochecknoglobals

// TestData represents test data with an integer value and patch flag.
//
//nolint:recvcheck
type TestData struct {
	Data  int
	Patch bool
}

// ScanBytes deserializes byte data into TestData.
func (t *TestData) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

// BytesValue serializes TestData to bytes.
func (t TestData) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

// ScanText deserializes text data into TestData.
func (t *TestData) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

// TextValue serializes TestData to text.
func (t TestData) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

// TestMetadata represents test metadata with a string value.
//
//nolint:recvcheck
type TestMetadata struct {
	Metadata string
}

// ScanBytes deserializes byte data into TestMetadata.
func (t *TestMetadata) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

// BytesValue serializes TestMetadata to bytes.
func (t TestMetadata) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

// ScanText deserializes text data into TestMetadata.
func (t *TestMetadata) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

// TextValue serializes TestMetadata to text.
func (t TestMetadata) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

// TestPatch represents a test patch with patch data.
//
//nolint:recvcheck
type TestPatch struct {
	Patch bool
}

// ScanBytes deserializes byte data into TestPatch.
func (t *TestPatch) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

// BytesValue serializes TestPatch to bytes.
func (t TestPatch) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

// ScanText deserializes text data into TestPatch.
func (t *TestPatch) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

// TextValue serializes TestPatch to text.
func (t TestPatch) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

// ToRawMessagePtr converts a string to a JSON RawMessage pointer.
func ToRawMessagePtr(data string) *json.RawMessage {
	j := json.RawMessage(data)
	return &j
}

// LockableSlice is a thread-safe slice with mutex protection.
type LockableSlice[T any] struct {
	data []T
	mu   sync.Mutex
}

// Append adds a value to the slice in a thread-safe manner.
func (l *LockableSlice[T]) Append(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, v)
}

// Prune returns and clears all values from the slice in a thread-safe manner.
func (l *LockableSlice[T]) Prune() []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := slices.Clone(l.data)
	l.data = nil
	return c
}
