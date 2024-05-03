package store

import (
	"encoding/json"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"
	"gitlab.com/tozd/go/x"
)

var DummyData = []byte(`{}`) //nolint:gochecknoglobals

type TestData struct {
	Data  int
	Patch bool
}

func (t *TestData) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t TestData) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *TestData) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t TestData) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

type TestMetadata struct {
	Metadata string
}

func (t *TestMetadata) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t TestMetadata) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *TestMetadata) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t TestMetadata) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

type TestPatch struct {
	Patch bool
}

func (t *TestPatch) ScanBytes(v []byte) error {
	return x.Unmarshal(v, t)
}

func (t TestPatch) BytesValue() ([]byte, error) {
	return x.MarshalWithoutEscapeHTML(t)
}

func (t *TestPatch) ScanText(v pgtype.Text) error {
	b := x.String2ByteSlice(v.String)
	return t.ScanBytes(b)
}

func (t TestPatch) TextValue() (pgtype.Text, error) {
	b, err := t.BytesValue()
	return pgtype.Text{
		String: x.ByteSlice2String(b),
		Valid:  err == nil,
	}, err
}

func ToRawMessagePtr(data string) *json.RawMessage {
	j := json.RawMessage(data)
	return &j
}

type LockableSlice[T any] struct {
	data []T
	mu   sync.Mutex
}

func (l *LockableSlice[T]) Append(v T) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.data = append(l.data, v)
}

func (l *LockableSlice[T]) Prune() []T {
	l.mu.Lock()
	defer l.mu.Unlock()
	c := slices.Clone(l.data)
	l.data = nil
	return c
}
