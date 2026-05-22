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

// makeTestFilter builds a valid Filter with proper Base/ID for testing.
func makeTestFilter(prop identifier.Identifier, ref *search.RefFilter, amount *search.AmountFilter, timeVal *search.TimeFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:     &filterID,
		Base:   base,
		Prop:   []identifier.Identifier{prop},
		Ref:    ref,
		Amount: amount,
		Time:   timeVal,
		Has:    nil,
	}
}

func TestFilterValidRef(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name    string
		Filter  search.Filter
		WantErr string
	}{
		{
			Name:    "ToSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: nil, Missing: true}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "NeitherSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: nil, Missing: false}, nil, nil),
			WantErr: "to or missing has to be set",
		},
		{
			Name:    "BothSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: true}, nil, nil),
			WantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Validate(false)
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestFilterValidAmount(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := 1.0
	lte := 10.0

	tests := []struct {
		Name    string
		Filter  search.Filter
		WantErr string
	}{
		{
			Name:    "BothGteLteSet",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Missing: false}, nil),
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: true}, nil),
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: false}, nil),
			WantErr: "both gte and lte or missing has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: nil, Missing: false}, nil),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: &lte, Missing: false}, nil),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "BothAndMissing",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Missing: true}, nil),
			WantErr: "gte/lte and missing cannot be both set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Validate(false)
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestFilterValidTime(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := float64(1000)
	lte := float64(2000)

	tests := []struct {
		Name    string
		Filter  search.Filter
		WantErr string
	}{
		{
			Name:    "BothGteLteSet",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false}),
			WantErr: "",
		},
		{
			Name:    "NoneSet",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Missing: true}),
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Missing: false}),
			WantErr: "both gte and lte or missing has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: nil, Missing: false}),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: &lte, Missing: false}),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "BothAndMissing",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: true}),
			WantErr: "gte/lte and missing cannot be both set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Validate(false)
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

func TestFilterValid(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")
	gte := 1.0
	lte := 10.0
	gteTime := float64(1000)
	lteTime := float64(2000)

	tests := []struct {
		Name    string
		Filter  search.Filter
		WantErr string
	}{
		{
			Name:    "RefFilter",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "AmountFilter",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Missing: false}, nil),
			WantErr: "",
		},
		{
			Name:    "TimeFilter",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gteTime, Lte: &lteTime, Missing: false}),
			WantErr: "",
		},
		{
			Name: "NoClause",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, nil, nil, nil)
				// Set a dummy Ref so makeTestFilter produces a valid base/id, then clear it.
				f.Ref = nil
				f.Amount = nil
				f.Time = nil
				return f
			}(),
			WantErr: "exactly one of ref, amount, time, or has must be set",
		},
		{
			Name: "MultipleClausesRefAndAmount",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
				f.Amount = &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Missing: false}
				return f
			}(),
			WantErr: "exactly one of ref, amount, time, or has must be set",
		},
		{
			Name: "MultipleClausesRefAndTime",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
				f.Time = &search.TimeFilter{Gte: &gteTime, Lte: &lteTime, Missing: false}
				return f
			}(),
			WantErr: "exactly one of ref, amount, time, or has must be set",
		},
		{
			Name:    "InvalidRefFilter",
			Filter:  makeTestFilter(prop, &search.RefFilter{To: nil, Missing: false}, nil, nil),
			WantErr: "to or missing has to be set",
		},
		{
			Name:    "InvalidAmountFilter",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: false}, nil),
			WantErr: "both gte and lte or missing has to be set",
		},
		{
			Name:    "InvalidTimeFilter",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Missing: false}),
			WantErr: "both gte and lte or missing has to be set",
		},
		{
			Name: "InvalidID",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
				badID := identifier.New()
				f.ID = &badID
				return f
			}(),
			WantErr: "invalid filter ID",
		},
		{
			Name: "EmptyProp",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
				f.Prop = nil
				return f
			}(),
			WantErr: "prop must have one or two elements for ref filter",
		},
		{
			Name: "HasFilter",
			Filter: func() search.Filter {
				base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
				filterID := identifier.From(base...)
				return search.Filter{
					ID:     &filterID,
					Base:   base,
					Prop:   nil,
					Ref:    nil,
					Amount: nil,
					Time:   nil,
					Has:    &search.HasFilter{Props: []search.HasValue{{ID: value}}},
				}
			}(),
			WantErr: "",
		},
		{
			Name: "InvalidHasFilter",
			Filter: func() search.Filter {
				base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
				filterID := identifier.From(base...)
				return search.Filter{
					ID:     &filterID,
					Base:   base,
					Prop:   nil,
					Ref:    nil,
					Amount: nil,
					Time:   nil,
					Has:    &search.HasFilter{Props: nil},
				}
			}(),
			WantErr: "props has to be set",
		},
		{
			Name: "HasFilterWithPropNotEmpty",
			Filter: func() search.Filter {
				base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
				filterID := identifier.From(base...)
				return search.Filter{
					ID:     &filterID,
					Base:   base,
					Prop:   []identifier.Identifier{prop},
					Ref:    nil,
					Amount: nil,
					Time:   nil,
					Has:    &search.HasFilter{Props: []search.HasValue{{ID: value}}},
				}
			}(),
			WantErr: "prop must be empty for has filter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			err := tt.Filter.Validate(false)
			if tt.WantErr == "" {
				require.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.WantErr)
			}
		})
	}
}

// The per-filter-type ToQuery shapes are unit-tested directly against the
// builder methods on each filter type. Session-level dispatch (and the
// cross-filter wiring on top of these shapes) is covered by
// TestSessionToQuery, TestRefFilterToSubRefQuery, and
// TestSessionToQueryCrossFilter.

func TestRefFilterToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name   string
		Filter *search.RefFilter
		Want   string
	}{
		{
			Name:   "To",
			Filter: &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false},
			//nolint:lll
			Want: `{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}}`,
		},
		{
			Name:   "MissingOnly",
			Filter: &search.RefFilter{To: nil, Missing: true},
			Want:   `{"bool":{"must_not":[{"nested":{"path":"claims.ref","query":{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
		{
			Name: "MultipleTo",
			Filter: &search.RefFilter{
				To:      []search.ToValue{{ID: value}, {ID: identifier.From("value2")}},
				Missing: false,
			},
			//nolint:lll
			Want: `{"bool":{"minimum_should_match":1,"should":[{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}},{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"1eNbijZLjE6RCP9J3v6yz1"}}}]}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, tt.Filter.ToQuery(prop)))
		})
	}
}

func TestAmountFilterToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	unit := identifier.From("unit")
	gte := 1.0
	lte := 10.0

	tests := []struct {
		Name   string
		Filter *search.AmountFilter
		Want   string
	}{
		{
			Name:   "GteLteUnit",
			Filter: &search.AmountFilter{Unit: &unit, Gte: &gte, Lte: &lte, Missing: false},
			//nolint:lll
			Want: `{"nested":{"path":"claims.amount","query":{"bool":{"must":[{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.amount.range":{"gte":1,"lte":10}}},{"term":{"claims.amount.unit":{"value":"7xgMSp3wauK811A8Fwk3rY"}}}]}}}}`,
		},
		{
			Name:   "MissingOnly",
			Filter: &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Missing: true},
			Want:   `{"bool":{"must_not":[{"nested":{"path":"claims.amount","query":{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
		{
			Name:   "GteLteNoUnit",
			Filter: &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Missing: false},
			//nolint:lll
			Want: `{"nested":{"path":"claims.amount","query":{"bool":{"must":[{"term":{"claims.amount.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.amount.range":{"gte":1,"lte":10}}}]}}}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, tt.Filter.ToQuery(prop)))
		})
	}
}

func TestTimeFilterToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := float64(1000)
	lte := float64(2000)

	tests := []struct {
		Name   string
		Filter *search.TimeFilter
		Want   string
	}{
		{
			Name:   "GteLte",
			Filter: &search.TimeFilter{Gte: &gte, Lte: &lte, Missing: false},
			//nolint:lll
			Want: `{"nested":{"path":"claims.time","query":{"bool":{"must":[{"term":{"claims.time.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"range":{"claims.time.range":{"gte":1000,"lte":2000}}}]}}}}`,
		},
		{
			Name:   "MissingOnly",
			Filter: &search.TimeFilter{Gte: nil, Lte: nil, Missing: true},
			Want:   `{"bool":{"must_not":[{"nested":{"path":"claims.time","query":{"term":{"claims.time.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, tt.Filter.ToQuery(prop)))
		})
	}
}

func TestHasFilterToQuery(t *testing.T) {
	t.Parallel()

	value := identifier.From("value")

	tests := []struct {
		Name   string
		Filter *search.HasFilter
		Want   string
	}{
		{
			Name:   "SingleProp",
			Filter: &search.HasFilter{Props: []search.HasValue{{ID: value}}},
			Want:   `{"nested":{"path":"claims.has","query":{"term":{"claims.has.prop":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}}}`,
		},
		{
			Name: "MultipleProps",
			Filter: &search.HasFilter{
				Props: []search.HasValue{{ID: value}, {ID: identifier.From("value2")}},
			},
			//nolint:lll
			Want: `{"bool":{"minimum_should_match":1,"should":[{"nested":{"path":"claims.has","query":{"term":{"claims.has.prop":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}}},{"nested":{"path":"claims.has","query":{"term":{"claims.has.prop":{"value":"1eNbijZLjE6RCP9J3v6yz1"}}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, tt.Filter.ToQuery()))
		})
	}
}

// TestSessionToQueryPanicsOnInvalidFilter ensures the session-level dispatch
// panics on an unreachable Filter shape (a state Validate is supposed to
// catch).
func TestSessionToQueryPanicsOnInvalidFilter(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		f := makeTestFilter(identifier.From("prop"), nil, nil, nil)
		f.Ref = nil
		f.Amount = nil
		f.Time = nil
		data := search.SessionData{View: "", Query: "", Filters: []search.Filter{f}, Reverse: nil}
		_ = data.ToQuery()
	})
}

func TestSessionValidate(t *testing.T) {
	t.Parallel()

	t.Run("ValidSession", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{View: search.ViewFeed, Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("BaseTooShort", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{
			SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From("short"),
			Base:        []string{"short"},
			Version:     0,
		}
		err := s.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "base must have at least two elements")
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		wrongID := identifier.New()
		s := &search.Session{
			SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
			ID:          wrongID,
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid session ID")
	})

	t.Run("DefaultView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("TableView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{View: search.ViewTable, Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.NoError(t, err)
		assert.Equal(t, search.ViewTable, s.View)
	})

	t.Run("InvalidView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{View: "grid", Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "invalid view")
	})

	t.Run("InvalidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		// Filter with invalid ref (neither to nor none set).
		s := &search.Session{
			SessionData: search.SessionData{
				View: "", Query: "test",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{To: nil, Missing: false}, nil, nil),
				},
				Reverse: nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate()
		require.Error(t, err)
		assert.EqualError(t, err, "to or missing has to be set")
	})

	t.Run("ValidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		value := identifier.From("value")
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{
				View: "", Query: "test",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
				},
				Reverse: nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate()
		require.NoError(t, err)
	})

	t.Run("NilFilters", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate()
		require.NoError(t, err)
	})
}

func TestSessionDataValidate(t *testing.T) {
	t.Parallel()

	t.Run("DefaultView", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil}
		err := data.Validate(false)
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, data.View)
	})

	t.Run("InvalidView", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{View: "grid", Query: "test", Filters: nil, Reverse: nil}
		err := data.Validate(false)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid view")
	})

	t.Run("ValidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		value := identifier.From("value")
		data := search.SessionData{
			View: "", Query: "test",
			Filters: []search.Filter{
				makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
			},
			Reverse: nil,
		}
		err := data.Validate(false)
		require.NoError(t, err)
	})

	t.Run("InvalidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		data := search.SessionData{
			View: "", Query: "test",
			Filters: []search.Filter{
				makeTestFilter(prop, &search.RefFilter{To: nil, Missing: false}, nil, nil),
			},
			Reverse: nil,
		}
		err := data.Validate(false)
		require.Error(t, err)
		assert.EqualError(t, err, "to or missing has to be set")
	})
}

func TestSessionToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name        string
		SessionData search.SessionData
		Want        string
	}{
		{
			Name:        "QueryOnly",
			SessionData: search.SessionData{View: "", Query: "hello", Filters: nil, Reverse: nil},
			//nolint:lll
			Want: `{"bool":{"must":[{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}},{"nested":{"path":"claims.amount","query":{"simple_query_string":{"default_operator":"or","fields":["claims.amount.propDisplay.en^0.2","claims.amount.propDisplay.pt^0.2","claims.amount.propDisplay.sl^0.2","claims.amount.propDisplay.und^0.2","claims.amount.propNaming.en^0.2","claims.amount.propNaming.pt^0.2","claims.amount.propNaming.sl^0.2","claims.amount.propNaming.und^0.2","claims.amount.fromDisplay^0.2","claims.amount.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.has","query":{"simple_query_string":{"default_operator":"or","fields":["claims.has.propDisplay.en^0.2","claims.has.propDisplay.pt^0.2","claims.has.propDisplay.sl^0.2","claims.has.propDisplay.und^0.2","claims.has.propNaming.en^0.2","claims.has.propNaming.pt^0.2","claims.has.propNaming.sl^0.2","claims.has.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.propDisplay.en^0.2","claims.html.propDisplay.pt^0.2","claims.html.propDisplay.sl^0.2","claims.html.propDisplay.und^0.2","claims.html.propNaming.en^0.2","claims.html.propNaming.pt^0.2","claims.html.propNaming.sl^0.2","claims.html.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.propDisplay.en^0.2","claims.id.propDisplay.pt^0.2","claims.id.propDisplay.sl^0.2","claims.id.propDisplay.und^0.2","claims.id.propNaming.en^0.2","claims.id.propNaming.pt^0.2","claims.id.propNaming.sl^0.2","claims.id.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.propDisplay.en^0.2","claims.link.propDisplay.pt^0.2","claims.link.propDisplay.sl^0.2","claims.link.propDisplay.und^0.2","claims.link.propNaming.en^0.2","claims.link.propNaming.pt^0.2","claims.link.propNaming.sl^0.2","claims.link.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.none","query":{"simple_query_string":{"default_operator":"or","fields":["claims.none.propDisplay.en^0.2","claims.none.propDisplay.pt^0.2","claims.none.propDisplay.sl^0.2","claims.none.propDisplay.und^0.2","claims.none.propNaming.en^0.2","claims.none.propNaming.pt^0.2","claims.none.propNaming.sl^0.2","claims.none.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.propDisplay.en^0.2","claims.ref.propDisplay.pt^0.2","claims.ref.propDisplay.sl^0.2","claims.ref.propDisplay.und^0.2","claims.ref.propNaming.en^0.2","claims.ref.propNaming.pt^0.2","claims.ref.propNaming.sl^0.2","claims.ref.propNaming.und^0.2","claims.ref.toDisplay.en^0.2","claims.ref.toDisplay.pt^0.2","claims.ref.toDisplay.sl^0.2","claims.ref.toDisplay.und^0.2","claims.ref.toNaming.en^0.2","claims.ref.toNaming.pt^0.2","claims.ref.toNaming.sl^0.2","claims.ref.toNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.propDisplay.en^0.2","claims.string.propDisplay.pt^0.2","claims.string.propDisplay.sl^0.2","claims.string.propDisplay.und^0.2","claims.string.propNaming.en^0.2","claims.string.propNaming.pt^0.2","claims.string.propNaming.sl^0.2","claims.string.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.time","query":{"simple_query_string":{"default_operator":"or","fields":["claims.time.propDisplay.en^0.2","claims.time.propDisplay.pt^0.2","claims.time.propDisplay.sl^0.2","claims.time.propDisplay.und^0.2","claims.time.propNaming.en^0.2","claims.time.propNaming.pt^0.2","claims.time.propNaming.sl^0.2","claims.time.propNaming.und^0.2","claims.time.fromDisplay^0.2","claims.time.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.unknown","query":{"simple_query_string":{"default_operator":"or","fields":["claims.unknown.propDisplay.en^0.2","claims.unknown.propDisplay.pt^0.2","claims.unknown.propDisplay.sl^0.2","claims.unknown.propDisplay.und^0.2","claims.unknown.propNaming.en^0.2","claims.unknown.propNaming.pt^0.2","claims.unknown.propNaming.sl^0.2","claims.unknown.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.sub","query":{"simple_query_string":{"default_operator":"or","fields":["claims.sub.propDisplay.en^0.2","claims.sub.propDisplay.pt^0.2","claims.sub.propDisplay.sl^0.2","claims.sub.propDisplay.und^0.2","claims.sub.propNaming.en^0.2","claims.sub.propNaming.pt^0.2","claims.sub.propNaming.sl^0.2","claims.sub.propNaming.und^0.2","claims.sub.toDisplay.en^0.2","claims.sub.toDisplay.pt^0.2","claims.sub.toDisplay.sl^0.2","claims.sub.toDisplay.und^0.2","claims.sub.toNaming.en^0.2","claims.sub.toNaming.pt^0.2","claims.sub.toNaming.sl^0.2","claims.sub.toNaming.und^0.2"],"query":"hello"}}}}]}}]}}`,
		},
		{
			Name:        "Empty",
			SessionData: search.SessionData{View: "", Query: "", Filters: nil, Reverse: nil},
			Want:        `{"bool":{}}`,
		},
		{
			Name: "QueryAndFilter",
			SessionData: search.SessionData{
				View: "", Query: "hello",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
				},
				Reverse: nil,
			},
			//nolint:lll
			Want: `{"bool":{"must":[{"bool":{"should":[{"term":{"id":{"value":"hello"}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.value"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.iri"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.en"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.pt"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.sl"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.string.und"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.en"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.pt"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.sl"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.html.und"],"query":"hello"}}}},{"nested":{"path":"claims.amount","query":{"simple_query_string":{"default_operator":"or","fields":["claims.amount.propDisplay.en^0.2","claims.amount.propDisplay.pt^0.2","claims.amount.propDisplay.sl^0.2","claims.amount.propDisplay.und^0.2","claims.amount.propNaming.en^0.2","claims.amount.propNaming.pt^0.2","claims.amount.propNaming.sl^0.2","claims.amount.propNaming.und^0.2","claims.amount.fromDisplay^0.2","claims.amount.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.has","query":{"simple_query_string":{"default_operator":"or","fields":["claims.has.propDisplay.en^0.2","claims.has.propDisplay.pt^0.2","claims.has.propDisplay.sl^0.2","claims.has.propDisplay.und^0.2","claims.has.propNaming.en^0.2","claims.has.propNaming.pt^0.2","claims.has.propNaming.sl^0.2","claims.has.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.html","query":{"simple_query_string":{"default_operator":"or","fields":["claims.html.propDisplay.en^0.2","claims.html.propDisplay.pt^0.2","claims.html.propDisplay.sl^0.2","claims.html.propDisplay.und^0.2","claims.html.propNaming.en^0.2","claims.html.propNaming.pt^0.2","claims.html.propNaming.sl^0.2","claims.html.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.id","query":{"simple_query_string":{"default_operator":"or","fields":["claims.id.propDisplay.en^0.2","claims.id.propDisplay.pt^0.2","claims.id.propDisplay.sl^0.2","claims.id.propDisplay.und^0.2","claims.id.propNaming.en^0.2","claims.id.propNaming.pt^0.2","claims.id.propNaming.sl^0.2","claims.id.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.link","query":{"simple_query_string":{"default_operator":"or","fields":["claims.link.propDisplay.en^0.2","claims.link.propDisplay.pt^0.2","claims.link.propDisplay.sl^0.2","claims.link.propDisplay.und^0.2","claims.link.propNaming.en^0.2","claims.link.propNaming.pt^0.2","claims.link.propNaming.sl^0.2","claims.link.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.none","query":{"simple_query_string":{"default_operator":"or","fields":["claims.none.propDisplay.en^0.2","claims.none.propDisplay.pt^0.2","claims.none.propDisplay.sl^0.2","claims.none.propDisplay.und^0.2","claims.none.propNaming.en^0.2","claims.none.propNaming.pt^0.2","claims.none.propNaming.sl^0.2","claims.none.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.ref","query":{"simple_query_string":{"default_operator":"or","fields":["claims.ref.propDisplay.en^0.2","claims.ref.propDisplay.pt^0.2","claims.ref.propDisplay.sl^0.2","claims.ref.propDisplay.und^0.2","claims.ref.propNaming.en^0.2","claims.ref.propNaming.pt^0.2","claims.ref.propNaming.sl^0.2","claims.ref.propNaming.und^0.2","claims.ref.toDisplay.en^0.2","claims.ref.toDisplay.pt^0.2","claims.ref.toDisplay.sl^0.2","claims.ref.toDisplay.und^0.2","claims.ref.toNaming.en^0.2","claims.ref.toNaming.pt^0.2","claims.ref.toNaming.sl^0.2","claims.ref.toNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.string","query":{"simple_query_string":{"default_operator":"or","fields":["claims.string.propDisplay.en^0.2","claims.string.propDisplay.pt^0.2","claims.string.propDisplay.sl^0.2","claims.string.propDisplay.und^0.2","claims.string.propNaming.en^0.2","claims.string.propNaming.pt^0.2","claims.string.propNaming.sl^0.2","claims.string.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.time","query":{"simple_query_string":{"default_operator":"or","fields":["claims.time.propDisplay.en^0.2","claims.time.propDisplay.pt^0.2","claims.time.propDisplay.sl^0.2","claims.time.propDisplay.und^0.2","claims.time.propNaming.en^0.2","claims.time.propNaming.pt^0.2","claims.time.propNaming.sl^0.2","claims.time.propNaming.und^0.2","claims.time.fromDisplay^0.2","claims.time.toDisplay^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.unknown","query":{"simple_query_string":{"default_operator":"or","fields":["claims.unknown.propDisplay.en^0.2","claims.unknown.propDisplay.pt^0.2","claims.unknown.propDisplay.sl^0.2","claims.unknown.propDisplay.und^0.2","claims.unknown.propNaming.en^0.2","claims.unknown.propNaming.pt^0.2","claims.unknown.propNaming.sl^0.2","claims.unknown.propNaming.und^0.2"],"query":"hello"}}}},{"nested":{"path":"claims.sub","query":{"simple_query_string":{"default_operator":"or","fields":["claims.sub.propDisplay.en^0.2","claims.sub.propDisplay.pt^0.2","claims.sub.propDisplay.sl^0.2","claims.sub.propDisplay.und^0.2","claims.sub.propNaming.en^0.2","claims.sub.propNaming.pt^0.2","claims.sub.propNaming.sl^0.2","claims.sub.propNaming.und^0.2","claims.sub.toDisplay.en^0.2","claims.sub.toDisplay.pt^0.2","claims.sub.toDisplay.sl^0.2","claims.sub.toDisplay.und^0.2","claims.sub.toNaming.en^0.2","claims.sub.toNaming.pt^0.2","claims.sub.toNaming.sl^0.2","claims.sub.toNaming.und^0.2"],"query":"hello"}}}}]}},{"nested":{"path":"claims.ref","query":{"bool":{"must":[{"term":{"claims.ref.prop":{"value":"Vg7NV61DJJ5HS2nheTZrQE"}}},{"term":{"claims.ref.to":{"value":"SM5iogb5kamoWQ2S65rzHz"}}}]}}}}]}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			q := tt.SessionData.ToQuery()
			assert.Equal(t, tt.Want, testutils.QueryJSON(t, q))
		})
	}
}

func TestSessionToQueryReverse(t *testing.T) {
	t.Parallel()

	reverseID := identifier.From("reverseTarget")
	prop := identifier.From("prop")
	value := identifier.From("value")

	t.Run("ReverseOnly", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View: "", Query: "", Filters: nil,
			Reverse: &reverseID,
		}
		q := data.ToQuery()
		want := `{"bool":{"must":[{"bool":{"minimum_should_match":1,"should":[` +
			`{"nested":{"path":"claims.ref","query":{"term":{"claims.ref.to":{"value":"` + reverseID.String() + `"}}}}},` +
			`{"nested":{"path":"claims.sub","query":{"term":{"claims.sub.to":{"value":"` + reverseID.String() + `"}}}}}` +
			`]}}]}}`
		assert.Equal(t, want, testutils.QueryJSON(t, q))
	})

	t.Run("ReverseAndFilter", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View: "", Query: "",
			Filters: []search.Filter{
				makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
			},
			Reverse: &reverseID,
		}
		q := data.ToQuery()
		j := testutils.QueryJSON(t, q)
		assert.Contains(t, j, `"claims.ref.to":{"value":"`+reverseID.String()+`"}`)
		assert.Contains(t, j, `"claims.ref.prop":{"value":"`+prop.String()+`"}`)
		assert.Contains(t, j, `"claims.ref.to":{"value":"`+value.String()+`"}`)
	})

	t.Run("ReverseInToQueryExcluding", func(t *testing.T) {
		t.Parallel()
		filter := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
		data := search.SessionData{
			View: "", Query: "",
			Filters: []search.Filter{filter},
			Reverse: &reverseID,
		}
		q := data.ToQueryExcluding(*filter.ID)
		j := testutils.QueryJSON(t, q)
		// Reverse scope is applied even when filter is excluded.
		assert.Contains(t, j, `"claims.ref.to":{"value":"`+reverseID.String()+`"}`)
		// Excluded filter's value is not in the query.
		assert.NotContains(t, j, `"claims.ref.to":{"value":"`+value.String()+`"}`)
	})

	t.Run("NoReverse", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{View: "", Query: "", Filters: nil, Reverse: nil}
		q := data.ToQuery()
		assert.JSONEq(t, `{"bool":{}}`, testutils.QueryJSON(t, q))
	})
}

func TestSessionDataValidateReverse(t *testing.T) {
	t.Parallel()

	reverseID := identifier.From("reverseTarget")

	t.Run("ReverseSet", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{View: "", Query: "test", Filters: nil, Reverse: &reverseID}
		err := data.Validate(false)
		require.NoError(t, err)
	})

	t.Run("ReverseRoundTrip", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := search.Session{
			SessionData: search.SessionData{View: search.ViewFeed, Query: "", Filters: nil, Reverse: &reverseID},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		data, errE := x.MarshalWithoutEscapeHTML(s)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.Session
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		require.NotNil(t, decoded.Reverse)
		assert.Equal(t, reverseID, *decoded.Reverse)
	})
}

func TestCreateSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "test search", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.NotEqual(t, identifier.Identifier{}, s.ID)
	assert.Equal(t, 0, s.Version)

	// Verify the session was stored.
	retrieved, errE := search.GetSession(ctx, s.ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, s.Query, retrieved.Query)
}

func TestCreateSessionValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Base with only one element triggers validation error.
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
		ID:          identifier.From("bad"),
		Base:        []string{"bad"},
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestUpdateSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// First create a session.
	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "original", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)

	id := s.ID

	// Update it.
	updated := &search.Session{
		SessionData: search.SessionData{View: search.ViewTable, Query: "updated", Filters: nil, Reverse: nil},
		ID:          id,
		Base:        base,
		Version:     1,
	}
	errE = search.UpdateSession(ctx, updated)
	require.NoError(t, errE, "% -+#.1v", errE)

	// Verify update.
	retrieved, errE := search.GetSession(ctx, id)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "updated", retrieved.Query)
	assert.Equal(t, search.ViewTable, retrieved.View)
}

func TestUpdateSessionMissingBase(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Session with no base at all fails validation.
	s := &search.Session{ //nolint:exhaustruct
		SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
		Version:     0,
	}
	errE := search.UpdateSession(ctx, s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestUpdateSessionValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "original", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	id := s.ID

	updated := &search.Session{
		SessionData: search.SessionData{View: "invalid", Query: "updated", Filters: nil, Reverse: nil},
		ID:          id,
		Base:        base,
		Version:     1,
	}
	errE = search.UpdateSession(ctx, updated)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestGetSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)

	retrieved, errE := search.GetSession(ctx, s.ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, "test", retrieved.Query)

	notFoundID := identifier.New()
	_, errE = search.GetSession(ctx, notFoundID)
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestGetSessionFromID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: "", Query: "test", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)

	retrieved, errE := search.GetSessionFromID(ctx, s.ID.String())
	require.NoError(t, errE, "% -+#.1v", errE)
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

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{View: search.ViewFeed, Query: "initial", Filters: nil, Reverse: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(ctx, s)
	require.NoError(t, errE, "% -+#.1v", errE)
	id := s.ID

	s2 := &search.Session{
		SessionData: search.SessionData{
			View: search.ViewTable, Query: "updated",
			Filters: []search.Filter{
				makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
			},
			Reverse: nil,
		},
		ID:      id,
		Base:    base,
		Version: 1,
	}
	errE = search.UpdateSession(ctx, s2)
	require.NoError(t, errE, "% -+#.1v", errE)

	s3 := &search.Session{
		SessionData: search.SessionData{View: "", Query: "updated again", Filters: nil, Reverse: nil},
		ID:          id,
		Base:        base,
		Version:     2,
	}
	errE = search.UpdateSession(ctx, s3)
	require.NoError(t, errE, "% -+#.1v", errE)

	final, errE := search.GetSession(ctx, id)
	require.NoError(t, errE, "% -+#.1v", errE)
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

func TestGetFilterByID(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	f1 := makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil)
	f2 := makeTestFilter(prop, &search.RefFilter{To: nil, Missing: true}, nil, nil)
	session := &search.Session{ //nolint:exhaustruct
		SessionData: search.SessionData{
			View:    "",
			Query:   "",
			Filters: []search.Filter{f1, f2},
			Reverse: nil,
		},
	}

	// Found.
	found, errE := session.GetFilterByID(*f1.ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, f1.ID, found.ID)

	found, errE = session.GetFilterByID(*f2.ID)
	require.NoError(t, errE, "% -+#.1v", errE)
	assert.Equal(t, f2.ID, found.ID)

	// Not found.
	_, errE = session.GetFilterByID(identifier.New())
	require.Error(t, errE)
	assert.EqualError(t, errE, "not found")
}

func TestJSONSerialization(t *testing.T) {
	t.Parallel()

	t.Run("FilterResult", func(t *testing.T) {
		t.Parallel()
		fr := search.FilterResult{Props: []string{"test-id"}, Type: "ref", Unit: "", FilterID: "", Count: 42}
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
		rfr := search.RefFilterResult{ID: "test-id", Count: 10, Paths: nil}
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
		prop := identifier.From("prop")
		value := identifier.From("value")
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		id := identifier.From(base...)
		s := search.Session{
			SessionData: search.SessionData{
				View: search.ViewTable, Query: "test query",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{To: []search.ToValue{{ID: value}}, Missing: false}, nil, nil),
				},
				Reverse: nil,
			},
			ID: id, Base: base, Version: 3,
		}
		data, errE := x.MarshalWithoutEscapeHTML(s)
		require.NoError(t, errE, "% -+#.1v", errE)
		var decoded search.Session
		errE = x.UnmarshalWithoutUnknownFields(data, &decoded)
		require.NoError(t, errE, "% -+#.1v", errE)
		assert.Equal(t, s.Query, decoded.Query)
		assert.Equal(t, s.ID, decoded.ID)
	})
}

// makeTestSubRefFilter builds a valid two-prop sub-ref Filter with proper
// Base/ID for testing.
func makeTestSubRefFilter(parentProp, prop identifier.Identifier, ref *search.RefFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:     &filterID,
		Base:   base,
		Prop:   []identifier.Identifier{parentProp, prop},
		Ref:    ref,
		Amount: nil,
		Time:   nil,
		Has:    nil,
	}
}

// TestRefFilterToSubRefQuery exercises ToSubRefQuery directly, including the
// new parentToRestrictions argument that constrains the sub-claim match to
// matching parent values inside the same nested record.
func TestRefFilterToSubRefQuery(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	prop := identifier.From("prop")
	a := identifier.From("a")
	l1 := identifier.From("l1")
	l2 := identifier.From("l2")

	tests := []struct {
		Name         string
		Filter       *search.RefFilter
		Restrictions []identifier.Identifier
		WantContains []string
		WantAbsent   []string
	}{
		{
			Name:         "ToWithoutRestrictions",
			Filter:       &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false},
			Restrictions: nil,
			WantContains: []string{
				`"claims.sub.parentProp":{"value":"` + parentProp.String() + `"}`,
				`"claims.sub.prop":{"value":"` + prop.String() + `"}`,
				`"claims.sub.to":{"value":"` + a.String() + `"}`,
			},
			WantAbsent: []string{
				`"claims.sub.parentTo"`,
			},
		},
		{
			Name:         "ToWithSingleRestriction",
			Filter:       &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false},
			Restrictions: []identifier.Identifier{l1},
			WantContains: []string{
				`"claims.sub.parentProp":{"value":"` + parentProp.String() + `"}`,
				`"claims.sub.prop":{"value":"` + prop.String() + `"}`,
				`"claims.sub.to":{"value":"` + a.String() + `"}`,
				`"claims.sub.parentTo":{"value":"` + l1.String() + `"}`,
			},
			WantAbsent: nil,
		},
		{
			Name:         "ToWithMultipleRestrictions",
			Filter:       &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false},
			Restrictions: []identifier.Identifier{l1, l2},
			WantContains: []string{
				`"claims.sub.parentTo":{"value":"` + l1.String() + `"}`,
				`"claims.sub.parentTo":{"value":"` + l2.String() + `"}`,
				`"minimum_should_match":1`,
			},
			WantAbsent: nil,
		},
		{
			Name:         "MissingOnlyWithRestriction",
			Filter:       &search.RefFilter{To: nil, Missing: true},
			Restrictions: []identifier.Identifier{l1},
			WantContains: []string{
				`"must_not"`,
				`"claims.sub.parentTo":{"value":"` + l1.String() + `"}`,
			},
			WantAbsent: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			q := tt.Filter.ToSubRefQuery(parentProp, prop, tt.Restrictions)
			j := testutils.QueryJSON(t, q)
			for _, s := range tt.WantContains {
				assert.Contains(t, j, s, "rendered JSON should contain %q", s)
			}
			for _, s := range tt.WantAbsent {
				assert.NotContains(t, j, s, "rendered JSON should NOT contain %q", s)
			}
		})
	}
}

// TestSessionToQueryCrossFilter verifies that SessionData.ToQuery composes a
// sub-ref filter together with a sibling parent-level ref filter on the same
// parentProp: the sub-claim match must include the parent-level To values as
// a parentTo restriction so that "parent=X AND parent>child=Y" matches only
// documents where Y is nested under X.
func TestSessionToQueryCrossFilter(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	otherProp := identifier.From("otherProp")
	subProp := identifier.From("subProp")
	l1 := identifier.From("l1")
	l2 := identifier.From("l2")
	a := identifier.From("a")
	x := identifier.From("x")

	t.Run("SubRefAlone_NoRestriction", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View:  "",
			Query: "",
			Filters: []search.Filter{
				makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false}),
			},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQuery())
		assert.Contains(t, j, `"claims.sub.to":{"value":"`+a.String()+`"}`)
		assert.NotContains(t, j, `"claims.sub.parentTo"`)
	})

	t.Run("SubRefWithSiblingParentRef_RestrictedToParentTo", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View:  "",
			Query: "",
			Filters: []search.Filter{
				makeTestFilter(parentProp, &search.RefFilter{To: []search.ToValue{{ID: l1}}, Missing: false}, nil, nil),
				makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false}),
			},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQuery())
		assert.Contains(t, j, `"claims.ref.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.sub.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.sub.parentTo":{"value":"`+l1.String()+`"}`)
	})

	t.Run("SubRefWithSiblingParentRef_MultipleParentTo", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View:  "",
			Query: "",
			Filters: []search.Filter{
				makeTestFilter(parentProp, &search.RefFilter{To: []search.ToValue{{ID: l1}, {ID: l2}}, Missing: false}, nil, nil),
				makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false}),
			},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQuery())
		assert.Contains(t, j, `"claims.sub.parentTo":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.sub.parentTo":{"value":"`+l2.String()+`"}`)
	})

	t.Run("SubRefWithSiblingOnDifferentProp_NoRestriction", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{
			View:  "",
			Query: "",
			Filters: []search.Filter{
				makeTestFilter(otherProp, &search.RefFilter{To: []search.ToValue{{ID: x}}, Missing: false}, nil, nil),
				makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false}),
			},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQuery())
		assert.NotContains(t, j, `"claims.sub.parentTo"`)
	})

	t.Run("SubRefWithSiblingMissingParentRef_NoRestriction", func(t *testing.T) {
		t.Parallel()
		// Sibling parent ref filter has Missing=true and no To values, so
		// there is nothing to restrict by.
		data := search.SessionData{
			View:  "",
			Query: "",
			Filters: []search.Filter{
				makeTestFilter(parentProp, &search.RefFilter{To: nil, Missing: true}, nil, nil),
				makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false}),
			},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQuery())
		assert.NotContains(t, j, `"claims.sub.parentTo"`)
	})

	t.Run("ToQueryExcludingParentRef_NoRestriction", func(t *testing.T) {
		t.Parallel()
		parentRef := makeTestFilter(parentProp, &search.RefFilter{To: []search.ToValue{{ID: l1}}, Missing: false}, nil, nil)
		subRef := makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false})
		data := search.SessionData{
			View:    "",
			Query:   "",
			Filters: []search.Filter{parentRef, subRef},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQueryExcluding(*parentRef.ID))
		assert.NotContains(t, j, `"claims.ref.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.sub.to":{"value":"`+a.String()+`"}`)
		assert.NotContains(t, j, `"claims.sub.parentTo"`)
	})

	t.Run("ToQueryExcludingSubRef_ParentStillPresent", func(t *testing.T) {
		t.Parallel()
		parentRef := makeTestFilter(parentProp, &search.RefFilter{To: []search.ToValue{{ID: l1}}, Missing: false}, nil, nil)
		subRef := makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{To: []search.ToValue{{ID: a}}, Missing: false})
		data := search.SessionData{
			View:    "",
			Query:   "",
			Filters: []search.Filter{parentRef, subRef},
			Reverse: nil,
		}
		j := testutils.QueryJSON(t, data.ToQueryExcluding(*subRef.ID))
		assert.Contains(t, j, `"claims.ref.to":{"value":"`+l1.String()+`"}`)
		assert.NotContains(t, j, `"claims.sub.to"`)
	})
}
