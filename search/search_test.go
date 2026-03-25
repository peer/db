package search_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gitlab.com/tozd/go/x"
	"gitlab.com/tozd/identifier"

	"gitlab.com/peerdb/peerdb/internal/testutils"
	"gitlab.com/peerdb/peerdb/search"
)

func TestRefFilterValid(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name    string
		Filter  search.RefFilter
		WantErr string
	}{
		{
			Name:    "ValueSet",
			Filter:  search.RefFilter{Prop: prop, Value: &value, None: false},
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  search.RefFilter{Prop: prop, Value: nil, None: true},
			WantErr: "",
		},
		{
			Name:    "NeitherSet",
			Filter:  search.RefFilter{Prop: prop, Value: nil, None: false},
			WantErr: "value or none has to be set",
		},
		{
			Name:    "BothSet",
			Filter:  search.RefFilter{Prop: prop, Value: &value, None: true},
			WantErr: "value and none cannot be both set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Valid()
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestAmountFilterValid(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := 1.0
	lte := 10.0

	tests := []struct {
		Name    string
		Filter  search.AmountFilter
		WantErr string
	}{
		{
			Name:    "BothGteLteSet",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false},
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: nil, Lte: nil, None: true},
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: nil, Lte: nil, None: false},
			WantErr: "both gte and lte or none has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: nil, None: false},
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: nil, Lte: &lte, None: false},
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "BothAndNone",
			Filter:  search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: true},
			WantErr: "gte/lte and none cannot be both set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Valid()
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestTimeFilterValid(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := float64(1000)
	lte := float64(2000)

	tests := []struct {
		Name    string
		Filter  search.TimeFilter
		WantErr string
	}{
		{
			Name:    "BothGteLteSet",
			Filter:  search.TimeFilter{Prop: prop, Gte: &gte, Lte: &lte, None: false},
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  search.TimeFilter{Prop: prop, Gte: nil, Lte: nil, None: true},
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  search.TimeFilter{Prop: prop, Gte: nil, Lte: nil, None: false},
			WantErr: "both gte and lte or none has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  search.TimeFilter{Prop: prop, Gte: &gte, Lte: nil, None: false},
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  search.TimeFilter{Prop: prop, Gte: nil, Lte: &lte, None: false},
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "BothAndNone",
			Filter:  search.TimeFilter{Prop: prop, Gte: &gte, Lte: &lte, None: true},
			WantErr: "gte/lte and none cannot be both set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Valid()
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestFiltersValid(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")
	gte := 1.0
	lte := 10.0
	gteTime := float64(1000)
	lteTime := float64(2000)

	tests := []struct {
		Name    string
		Filters search.Filters
		WantErr string
	}{
		{
			Name: "RefFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
			},
			WantErr: "",
		},
		{
			Name: "AmountFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil,
				Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false}, Time: nil,
			},
			WantErr: "",
		},
		{
			Name: "TimeFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
				Time: &search.TimeFilter{Prop: prop, Gte: &gteTime, Lte: &lteTime, None: false},
			},
			WantErr: "",
		},
		{
			Name: "AndFilters",
			Filters: search.Filters{
				And: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
					},
					{
						And: nil, Or: nil, Not: nil, Ref: nil,
						Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false},
						Time:   nil,
					},
				},
				Or: nil, Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "",
		},
		{
			Name: "OrFilters",
			Filters: search.Filters{
				And: nil,
				Or: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
					},
					{
						And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
						Time: &search.TimeFilter{Prop: prop, Gte: &gteTime, Lte: &lteTime, None: false},
					},
				},
				Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "",
		},
		{
			Name: "NotFilter",
			Filters: search.Filters{
				And: nil, Or: nil,
				Not: &search.Filters{
					And: nil, Or: nil, Not: nil,
					Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
				},
				Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "",
		},
		{
			Name:    "NoClause",
			Filters: search.Filters{And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil, Time: nil},
			WantErr: "no clause is set",
		},
		{
			Name: "MultipleClausesRelAndAmount",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref:    &search.RefFilter{Prop: prop, Value: &value, None: false},
				Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false},
				Time:   nil,
			},
			WantErr: "only one clause can be set",
		},
		{
			Name: "MultipleClausesAndAndOr",
			Filters: search.Filters{
				And: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
					},
				},
				Or: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
					},
				},
				Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "only one clause can be set",
		},
		{
			Name: "InvalidNestedAndFilter",
			Filters: search.Filters{
				And: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: nil, None: false}, Amount: nil, Time: nil,
					},
				},
				Or: nil, Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "value or none has to be set",
		},
		{
			Name: "InvalidNestedOrFilter",
			Filters: search.Filters{
				And: nil,
				Or: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: nil, None: false}, Amount: nil, Time: nil,
					},
				},
				Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "value or none has to be set",
		},
		{
			Name: "InvalidNotFilter",
			Filters: search.Filters{
				And: nil, Or: nil,
				Not: &search.Filters{
					And: nil, Or: nil, Not: nil,
					Ref: &search.RefFilter{Prop: prop, Value: nil, None: false}, Amount: nil, Time: nil,
				},
				Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "value or none has to be set",
		},
		{
			Name: "InvalidRefFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref: &search.RefFilter{Prop: prop, Value: nil, None: false}, Amount: nil, Time: nil,
			},
			WantErr: "value or none has to be set",
		},
		{
			Name: "InvalidAmountFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil,
				Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: nil, Lte: nil, None: false}, Time: nil,
			},
			WantErr: "both gte and lte or none has to be set",
		},
		{
			Name: "InvalidTimeFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
				Time: &search.TimeFilter{Prop: prop, Gte: nil, Lte: nil, None: false},
			},
			WantErr: "both gte and lte or none has to be set",
		},
		{
			Name: "NotAndRefFilter",
			Filters: search.Filters{
				And: nil, Or: nil,
				Not: &search.Filters{
					And: nil, Or: nil, Not: nil,
					Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
				},
				Ref:    &search.RefFilter{Prop: prop, Value: &value, None: false},
				Amount: nil, Time: nil,
			},
			WantErr: "only one clause can be set",
		},
		{
			Name: "AndAndNotFilter",
			Filters: search.Filters{
				And: []search.Filters{
					{
						And: nil, Or: nil, Not: nil,
						Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
					},
				},
				Or: nil,
				Not: &search.Filters{
					And: nil, Or: nil, Not: nil,
					Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
				},
				Ref: nil, Amount: nil, Time: nil,
			},
			WantErr: "only one clause can be set",
		},
		{
			Name: "TimeAndRefFilter",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref:    &search.RefFilter{Prop: prop, Value: &value, None: false},
				Amount: nil,
				Time:   &search.TimeFilter{Prop: prop, Gte: &gteTime, Lte: &lteTime, None: false},
			},
			WantErr: "only one clause can be set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filters.Valid()
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestFiltersToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")
	unit := identifier.From("unit")
	gte := 1.0
	lte := 10.0
	gteTime := float64(1000)
	lteTime := float64(2000)

	refFilter := &search.RefFilter{Prop: prop, Value: &value, None: false}
	refNoneFilter := &search.RefFilter{Prop: prop, Value: nil, None: true}

	tests := []struct {
		Name    string
		Filters search.Filters
		Want    string
	}{
		{
			Name:    "RelValue",
			Filters: search.Filters{And: nil, Or: nil, Not: nil, Ref: refFilter, Amount: nil, Time: nil},
			//nolint:lll
			Want: `{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}}`,
		},
		{
			Name:    "RelNone",
			Filters: search.Filters{And: nil, Or: nil, Not: nil, Ref: refNoneFilter, Amount: nil, Time: nil},

			Want: `{"bool":{"must_not":[{"nested":{"path":"claims.ref","query":{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
		{
			Name: "AmountGteLteUnit",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil,
				Amount: &search.AmountFilter{Prop: prop, Unit: &unit, Gte: &gte, Lte: &lte, None: false},
				Time:   nil,
			},
			//nolint:lll
			Want: `{"nested":{"path":"claims.amount","query":{"bool":{"must":[{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.amount.range":{"gte":1,"lte":10}}},{"term":{"claims.amount.unit":{"value":"7xgMSp3wauK811A8Fwk3rY"}}}]}}}}`,
		},
		{
			Name: "AmountNone",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil,
				Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: nil, Lte: nil, None: true},
				Time:   nil,
			},

			Want: `{"bool":{"must_not":[{"nested":{"path":"claims.amount","query":{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
		{
			Name: "AmountGteLteNoUnit",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil,
				Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false},
				Time:   nil,
			},
			//nolint:lll
			Want: `{"nested":{"path":"claims.amount","query":{"bool":{"must":[{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.amount.range":{"gte":1,"lte":10}}}]}}}}`,
		},
		{
			Name: "TimeGteLte",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
				Time: &search.TimeFilter{Prop: prop, Gte: &gteTime, Lte: &lteTime, None: false},
			},
			//nolint:lll
			Want: `{"nested":{"path":"claims.time","query":{"bool":{"must":[{"term":{"claims.time.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.time.range":{"gte":1000,"lte":2000}}}]}}}}`,
		},
		{
			Name: "TimeNone",
			Filters: search.Filters{
				And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
				Time: &search.TimeFilter{Prop: prop, Gte: nil, Lte: nil, None: true},
			},

			Want: `{"bool":{"must_not":[{"nested":{"path":"claims.time","query":{"term":{"claims.time.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
		{
			Name: "And",
			Filters: search.Filters{
				And: []search.Filters{
					{And: nil, Or: nil, Not: nil, Ref: refFilter, Amount: nil, Time: nil},
					{
						And: nil, Or: nil, Not: nil, Ref: nil,
						Amount: &search.AmountFilter{Prop: prop, Unit: nil, Gte: &gte, Lte: &lte, None: false},
						Time:   nil,
					},
				},
				Or: nil, Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			//nolint:lll
			Want: `{"bool":{"must":[{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}},{"nested":{"path":"claims.amount","query":{"bool":{"must":[{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.amount.range":{"gte":1,"lte":10}}}]}}}}]}}`,
		},
		{
			Name: "Or",
			Filters: search.Filters{
				And: nil,
				Or: []search.Filters{
					{And: nil, Or: nil, Not: nil, Ref: refFilter, Amount: nil, Time: nil},
					{
						And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil,
						Time: &search.TimeFilter{Prop: prop, Gte: &gteTime, Lte: &lteTime, None: false},
					},
				},
				Not: nil, Ref: nil, Amount: nil, Time: nil,
			},
			//nolint:lll
			Want: `{"bool":{"minimum_should_match":1,"should":[{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}},{"nested":{"path":"claims.time","query":{"bool":{"must":[{"term":{"claims.time.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.time.range":{"gte":1000,"lte":2000}}}]}}}}]}}`,
		},
		{
			Name: "Not",
			Filters: search.Filters{
				And: nil, Or: nil,
				Not: &search.Filters{And: nil, Or: nil, Not: nil, Ref: refFilter, Amount: nil, Time: nil},
				Ref: nil, Amount: nil, Time: nil,
			},
			//nolint:lll
			Want: `{"bool":{"must_not":[{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			q := tt.Filters.ToQuery()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, q))
		})
	}
}

func TestFiltersToQueryPanicsOnInvalid(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		f := search.Filters{And: nil, Or: nil, Not: nil, Ref: nil, Amount: nil, Time: nil}
		f.ToQuery()
	})
}

func TestSessionValidate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("NewSessionGetsID", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{ID: nil, Version: 0, View: search.ViewFeed, Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.NoError(t, err)
		assert.NotNil(t, s.ID)
		assert.Equal(t, 0, s.Version)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("NewSessionWithIDFails", func(t *testing.T) {
		t.Parallel()
		id := identifier.New()
		s := &search.Session{ID: &id, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.Error(t, err)
		assert.EqualError(t, err, "ID provided for new document")
	})

	t.Run("ExistingSessionVersionIncremented", func(t *testing.T) {
		t.Parallel()
		id := identifier.New()
		existing := &search.Session{ID: &id, Version: 3, View: "", Query: "old", Filters: nil}
		s := &search.Session{ID: &id, Version: 0, View: "", Query: "updated", Filters: nil}
		err := s.Validate(ctx, existing)
		require.NoError(t, err)
		assert.Equal(t, 4, s.Version)
	})

	t.Run("ExistingMissingPayloadID", func(t *testing.T) {
		t.Parallel()
		id := identifier.New()
		existing := &search.Session{ID: &id, Version: 0, View: "", Query: "", Filters: nil}
		s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, existing)
		require.Error(t, err)
		assert.EqualError(t, err, "ID missing for existing document")
	})

	t.Run("ExistingMissingExistingID", func(t *testing.T) {
		t.Parallel()
		id := identifier.New()
		existing := &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil}
		s := &search.Session{ID: &id, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, existing)
		require.Error(t, err)
		assert.EqualError(t, err, "ID missing for existing document")
	})

	t.Run("IDMismatch", func(t *testing.T) {
		t.Parallel()
		id1 := identifier.New()
		id2 := identifier.New()
		existing := &search.Session{ID: &id1, Version: 0, View: "", Query: "", Filters: nil}
		s := &search.Session{ID: &id2, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, existing)
		require.Error(t, err)
		assert.EqualError(t, err, "payload ID does not match existing ID")
	})

	t.Run("DefaultView", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("TableView", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{ID: nil, Version: 0, View: search.ViewTable, Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.NoError(t, err)
		assert.Equal(t, search.ViewTable, s.View)
	})

	t.Run("InvalidView", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{ID: nil, Version: 0, View: "grid", Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid view")
	})

	t.Run("InvalidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		s := &search.Session{
			ID: nil, Version: 0, View: "", Query: "test",
			Filters: &search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref: &search.RefFilter{Prop: prop, Value: nil, None: false}, Amount: nil, Time: nil,
			},
		}
		err := s.Validate(ctx, nil)
		require.Error(t, err)
		assert.EqualError(t, err, "value or none has to be set")
	})

	t.Run("ValidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		value := identifier.From("value")
		s := &search.Session{
			ID: nil, Version: 0, View: "", Query: "test",
			Filters: &search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
			},
		}
		err := s.Validate(ctx, nil)
		require.NoError(t, err)
	})

	t.Run("NilFilters", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
		err := s.Validate(ctx, nil)
		require.NoError(t, err)
	})
}

func TestSessionRef(t *testing.T) {
	t.Parallel()

	id := identifier.From("prop")
	s := &search.Session{ID: &id, Version: 5, View: "", Query: "", Filters: nil}
	ref := s.Ref()
	assert.Equal(t, id, ref.ID)
	assert.Equal(t, 5, ref.Version)
}

func TestSessionToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name    string
		Session *search.Session
		Want    string
	}{
		{
			Name:    "QueryOnly",
			Session: &search.Session{ID: nil, Version: 0, View: "", Query: "hello", Filters: nil},
			//nolint:lll
			Want: `{"bool":{"must":[{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}}]}}]}}`,
		},
		{
			Name:    "Empty",
			Session: &search.Session{ID: nil, Version: 0, View: "", Query: "", Filters: nil},
			Want:    `{"bool":{}}`,
		},
		{
			Name: "QueryAndFilter",
			Session: &search.Session{
				ID: nil, Version: 0, View: "", Query: "hello",
				Filters: &search.Filters{
					And: nil, Or: nil, Not: nil,
					Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
				},
			},
			//nolint:lll
			Want: `{"bool":{"must":[{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}}]}},{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			q := tt.Session.ToQuery()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, q))
		})
	}
}

func TestCreateSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s := &search.Session{ID: nil, Version: 0, View: "", Query: "test search", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)
	assert.NotNil(t, s.ID)
	assert.Equal(t, 0, s.Version)

	// Verify the session was stored.
	retrieved, errE := search.GetSession(ctx, *s.ID)
	require.NoError(t, errE)
	assert.Equal(t, s.Query, retrieved.Query)
}

func TestCreateSessionValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := identifier.New()

	s := &search.Session{ID: &id, Version: 0, View: "", Query: "test", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestUpdateSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// First create a session.
	s := &search.Session{ID: nil, Version: 0, View: "", Query: "original", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)

	id := *s.ID

	// Update it.
	updated := &search.Session{ID: &id, Version: 0, View: search.ViewTable, Query: "updated", Filters: nil}
	errE = search.UpdateSession(ctx, updated)
	require.NoError(t, errE)
	assert.Equal(t, 1, updated.Version)

	// Verify update.
	retrieved, errE := search.GetSession(ctx, id)
	require.NoError(t, errE)
	assert.Equal(t, "updated", retrieved.Query)
	assert.Equal(t, search.ViewTable, retrieved.View)
}

func TestUpdateSessionMissingID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
	errE := search.UpdateSession(ctx, s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "ID is missing: validation failed")
}

func TestUpdateSessionNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	id := identifier.New()

	s := &search.Session{ID: &id, Version: 0, View: "", Query: "test", Filters: nil}
	errE := search.UpdateSession(ctx, s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestUpdateSessionValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s := &search.Session{ID: nil, Version: 0, View: "", Query: "original", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)
	id := *s.ID

	updated := &search.Session{ID: &id, Version: 0, View: "invalid", Query: "updated", Filters: nil}
	errE = search.UpdateSession(ctx, updated)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestGetSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)

	retrieved, errE := search.GetSession(ctx, *s.ID)
	require.NoError(t, errE)
	assert.Equal(t, "test", retrieved.Query)

	notFoundID := identifier.New()
	_, errE = search.GetSession(ctx, notFoundID)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestGetSessionFromID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	s := &search.Session{ID: nil, Version: 0, View: "", Query: "test", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)

	retrieved, errE := search.GetSessionFromID(ctx, s.ID.String())
	require.NoError(t, errE)
	assert.Equal(t, "test", retrieved.Query)

	// Invalid ID string.
	_, errE = search.GetSessionFromID(ctx, "invalid-id")
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")

	// Valid ID format but not found.
	notFoundID := identifier.New()
	_, errE = search.GetSessionFromID(ctx, notFoundID.String())
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestCreateAndUpdateSessionRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	prop := identifier.From("prop")
	value := identifier.From("value")

	s := &search.Session{ID: nil, Version: 0, View: search.ViewFeed, Query: "initial", Filters: nil}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE)
	id := *s.ID

	s2 := &search.Session{
		ID: &id, Version: 0, View: search.ViewTable, Query: "updated",
		Filters: &search.Filters{
			And: nil, Or: nil, Not: nil,
			Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
		},
	}
	errE = search.UpdateSession(ctx, s2)
	require.NoError(t, errE)
	assert.Equal(t, 1, s2.Version)

	s3 := &search.Session{ID: &id, Version: 0, View: "", Query: "updated again", Filters: nil}
	errE = search.UpdateSession(ctx, s3)
	require.NoError(t, errE)
	assert.Equal(t, 2, s3.Version)

	final, errE := search.GetSession(ctx, id)
	require.NoError(t, errE)
	assert.Equal(t, "updated again", final.Query)
	assert.Equal(t, 2, final.Version)
}

func TestViewTypeConstants(t *testing.T) {
	t.Parallel()

	assert.Equal(t, search.ViewFeed, search.ViewType("feed"))
	assert.Equal(t, search.ViewTable, search.ViewType("table"))
}

func TestMaxResultsCount(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 1000, search.MaxResultsCount)
}

func TestErrorVariables(t *testing.T) {
	t.Parallel()

	assert.EqualError(t, search.ErrNotFound, "not found")
	assert.EqualError(t, search.ErrValidationFailed, "validation failed")
}

func TestJSONSerialization(t *testing.T) {
	t.Parallel()

	t.Run("FilterResult", func(t *testing.T) {
		t.Parallel()
		fr := search.FilterResult{ID: "test-id", Count: 42, Type: "ref", Unit: ""}
		data, errE := x.MarshalWithoutEscapeHTML(fr)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.FilterResult
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, fr, decoded)

		fr.Unit = "kg"
		data, errE = x.MarshalWithoutEscapeHTML(fr)
		require.NoError(t, errE, "% -+#.1v", errE)
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, fr, decoded)
	})

	t.Run("RefFilterResult", func(t *testing.T) {
		t.Parallel()
		rfr := search.RefFilterResult{ID: "test-id", Count: 10}
		data, errE := x.MarshalWithoutEscapeHTML(rfr)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.RefFilterResult
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, rfr, decoded)
	})

	t.Run("HistogramResult", func(t *testing.T) {
		t.Parallel()
		hr := search.HistogramResult{From: 1.5, Count: 20}
		data, errE := x.MarshalWithoutEscapeHTML(hr)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.HistogramResult
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, hr, decoded)
	})

	t.Run("Result", func(t *testing.T) {
		t.Parallel()
		r := search.Result{ID: "doc-123"}
		data, errE := x.MarshalWithoutEscapeHTML(r)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.Result
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, r, decoded)
	})

	t.Run("Session", func(t *testing.T) {
		t.Parallel()
		id := identifier.From("prop")
		prop := identifier.From("prop")
		value := identifier.From("value")
		s := search.Session{
			ID: &id, Version: 3, View: search.ViewTable, Query: "test query",
			Filters: &search.Filters{
				And: nil, Or: nil, Not: nil,
				Ref: &search.RefFilter{Prop: prop, Value: &value, None: false}, Amount: nil, Time: nil,
			},
		}
		data, errE := x.MarshalWithoutEscapeHTML(s)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.Session
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, s.Query, decoded.Query)
		assert.Equal(t, *s.ID, *decoded.ID)
	})

	t.Run("SessionRef", func(t *testing.T) {
		t.Parallel()
		id := identifier.From("prop")
		ref := search.SessionRef{ID: id, Version: 7}
		data, errE := x.MarshalWithoutEscapeHTML(ref)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.SessionRef
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, ref, decoded)
	})
}
