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
		ID:       &filterID,
		Base:     base,
		Prop:     []identifier.Identifier{prop},
		Ref:      ref,
		Amount:   amount,
		Time:     timeVal,
		Has:      nil,
		Specials: nil,
	}
}

// makeTestHasFilter builds a valid has Filter with proper Base/ID: props is empty for the
// top-level pooled has facet and holds the parent property in the sub form.
func makeTestHasFilter(props []identifier.Identifier, has *search.HasFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:       &filterID,
		Base:     base,
		Prop:     props,
		Ref:      nil,
		Amount:   nil,
		Time:     nil,
		Has:      has,
		Specials: nil,
	}
}

// makeTestSpecialsFilter builds a valid specials Filter with proper Base/ID for the given
// property path (one element for a top-level path, two for a sub path).
func makeTestSpecialsFilter(props []identifier.Identifier, specials *search.SpecialsFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:       &filterID,
		Base:     base,
		Prop:     props,
		Ref:      nil,
		Amount:   nil,
		Time:     nil,
		Has:      nil,
		Specials: specials,
	}
}

// filtersSession builds a SessionData carrying only the given filters.
func filtersSession(filters ...search.Filter) search.SessionData {
	return search.SessionData{
		Sort:          nil,
		Language:      "",
		View:          "",
		Query:         "",
		Filters:       filters,
		Prefilters:    nil,
		Reverse:       nil,
		ReverseExpand: false,
		IDs:           nil,
	}
}

// prefiltersSession builds a SessionData carrying only the given prefilters.
func prefiltersSession(prefilters ...search.Filter) search.SessionData {
	return search.SessionData{
		Sort:          nil,
		Language:      "",
		View:          "",
		Query:         "",
		Filters:       nil,
		Prefilters:    prefilters,
		Reverse:       nil,
		ReverseExpand: false,
		IDs:           nil,
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
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "DirectSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: []search.ToValue{{ID: value}}, To: nil}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "NeitherSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: nil, To: nil}, nil, nil),
			WantErr: "to or direct has to be set",
		},
		{
			Name:    "BothSet",
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: []search.ToValue{{ID: value}}, To: []search.ToValue{{ID: value}}}, nil, nil),
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
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: false}, nil),
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Exists: false}, nil),
			WantErr: "gte and lte, or exists has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: nil, Exists: false}, nil),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: &lte, Exists: false}, nil),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "ExistsOnly",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Exists: true}, nil),
			WantErr: "",
		},
		{
			Name:    "BothAndExists",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: true}, nil),
			WantErr: "gte/lte and exists cannot be both set",
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
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: false}),
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Exists: false}),
			WantErr: "gte and lte, or exists has to be set",
		},
		{
			Name:    "GteOnly",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: nil, Exists: false}),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "LteOnly",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: &lte, Exists: false}),
			WantErr: "both gte and lte must be set together",
		},
		{
			Name:    "ExistsOnly",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Exists: true}),
			WantErr: "",
		},
		{
			Name:    "BothAndExists",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: true}),
			WantErr: "gte/lte and exists cannot be both set",
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

func TestFilterValidSpecials(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	subProp := identifier.From("subProp")

	tests := []struct {
		Name    string
		Filter  search.Filter
		WantErr string
	}{
		{
			Name:    "MissingOnly",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			WantErr: "",
		},
		{
			Name:    "NoneOnly",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: true, Unknown: false, HasProperty: false}),
			WantErr: "",
		},
		{
			Name:    "UnknownOnly",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: false, Unknown: true, HasProperty: false}),
			WantErr: "",
		},
		{
			Name:    "HasPropertyOnly",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: false, Unknown: false, HasProperty: true}),
			WantErr: "",
		},
		{
			Name:    "AllSet",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: true, Unknown: true, HasProperty: true}),
			WantErr: "",
		},
		{
			Name:    "NothingSet",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: false, Unknown: false, HasProperty: false}),
			WantErr: "at least one of missing, none, unknown, or hasProperty has to be set",
		},
		{
			Name:    "SubPath",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop, subProp}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			WantErr: "",
		},
		{
			Name:    "NoProp",
			Filter:  makeTestSpecialsFilter(nil, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			WantErr: "prop must have one or two elements",
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
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
			WantErr: "",
		},
		{
			Name:    "AmountFilter",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: false}, nil),
			WantErr: "",
		},
		{
			Name:    "TimeFilter",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: &gteTime, Lte: &lteTime, Exists: false}),
			WantErr: "",
		},
		{
			Name:    "SpecialsFilter",
			Filter:  makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			WantErr: "",
		},
		{
			Name:    "NoClause",
			Filter:  makeTestFilter(prop, nil, nil, nil),
			WantErr: "exactly one of ref, amount, time, has, or specials must be set",
		},
		{
			Name: "MultipleClausesRefAndAmount",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
				f.Amount = &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: false}
				return f
			}(),
			WantErr: "exactly one of ref, amount, time, has, or specials must be set",
		},
		{
			Name: "MultipleClausesRefAndSpecials",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
				f.Specials = &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}
				return f
			}(),
			WantErr: "exactly one of ref, amount, time, has, or specials must be set",
		},
		{
			Name:    "InvalidRefFilter",
			Filter:  makeTestFilter(prop, &search.RefFilter{Direct: nil, To: nil}, nil, nil),
			WantErr: "to or direct has to be set",
		},
		{
			Name:    "InvalidAmountFilter",
			Filter:  makeTestFilter(prop, nil, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Exists: false}, nil),
			WantErr: "gte and lte, or exists has to be set",
		},
		{
			Name:    "InvalidTimeFilter",
			Filter:  makeTestFilter(prop, nil, nil, &search.TimeFilter{Gte: nil, Lte: nil, Exists: false}),
			WantErr: "gte and lte, or exists has to be set",
		},
		{
			Name: "InvalidID",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
				badID := identifier.New()
				f.ID = &badID
				return f
			}(),
			WantErr: "invalid filter ID",
		},
		{
			Name: "EmptyProp",
			Filter: func() search.Filter {
				f := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
				f.Prop = nil
				return f
			}(),
			WantErr: "prop must have one or two elements",
		},
		{
			Name:    "HasFilter",
			Filter:  makeTestHasFilter(nil, &search.HasFilter{Props: []search.HasValue{{ID: value}}}),
			WantErr: "",
		},
		{
			Name:    "InvalidHasFilter",
			Filter:  makeTestHasFilter(nil, &search.HasFilter{Props: nil}),
			WantErr: "props has to be set",
		},
		{
			Name:    "HasFilterSubHas",
			Filter:  makeTestHasFilter([]identifier.Identifier{prop}, &search.HasFilter{Props: []search.HasValue{{ID: value}}}),
			WantErr: "",
		},
		{
			Name:    "HasFilterTooManyProps",
			Filter:  makeTestHasFilter([]identifier.Identifier{prop, value}, &search.HasFilter{Props: []search.HasValue{{ID: value}}}),
			WantErr: "prop must have zero or one elements for has filter",
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

// Filter payloads no longer compile to queries on their own; all query compilation goes through
// SessionData.ToQuery (the group compiler). The per-filter-type tests below snapshot the compiled
// session query for a session holding just that one filter, so each filter type's clause shape
// stays covered. Correlation between sub filters and their parent property's selections is covered
// by TestSessionToQueryCorrelation and the TestSub*FilterQuery tests.

func TestRefFilterQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name   string
		Filter *search.RefFilter
	}{
		{
			Name:   "To",
			Filter: &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}},
		},
		{
			Name: "MultipleTo",
			Filter: &search.RefFilter{
				Direct: nil,
				To:     []search.ToValue{{ID: value}, {ID: identifier.From("value2")}},
			},
		},
		{
			Name:   "Direct",
			Filter: &search.RefFilter{Direct: []search.ToValue{{ID: value}}, To: nil},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			data := filtersSession(makeTestFilter(prop, tt.Filter, nil, nil))
			assertQueryGolden(t, data.ToQuery(nil))
		})
	}
}

func TestAmountFilterQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	unit := identifier.From("unit")
	gte := 1.0
	lte := 10.0

	tests := []struct {
		Name   string
		Filter *search.AmountFilter
	}{
		{
			Name:   "GteLteUnit",
			Filter: &search.AmountFilter{Unit: &unit, Gte: &gte, Lte: &lte, Exists: false},
		},
		{
			Name:   "GteLteNoUnit",
			Filter: &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: false},
		},
		{
			Name:   "ExistsUnit",
			Filter: &search.AmountFilter{Unit: &unit, Gte: nil, Lte: nil, Exists: true},
		},
		{
			Name:   "ExistsUnitless",
			Filter: &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Exists: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			data := filtersSession(makeTestFilter(prop, nil, tt.Filter, nil))
			assertQueryGolden(t, data.ToQuery(nil))
		})
	}
}

func TestTimeFilterQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	gte := float64(1000)
	lte := float64(2000)

	tests := []struct {
		Name   string
		Filter *search.TimeFilter
	}{
		{
			Name:   "GteLte",
			Filter: &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: false},
		},
		{
			Name:   "Exists",
			Filter: &search.TimeFilter{Gte: nil, Lte: nil, Exists: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			data := filtersSession(makeTestFilter(prop, nil, nil, tt.Filter))
			assertQueryGolden(t, data.ToQuery(nil))
		})
	}
}

func TestHasFilterQuery(t *testing.T) {
	t.Parallel()

	value := identifier.From("value")

	tests := []struct {
		Name   string
		Filter *search.HasFilter
	}{
		{
			Name:   "SingleProp",
			Filter: &search.HasFilter{Props: []search.HasValue{{ID: value}}},
		},
		{
			Name: "MultipleProps",
			Filter: &search.HasFilter{
				Props: []search.HasValue{{ID: value}, {ID: identifier.From("value2")}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			data := filtersSession(makeTestHasFilter(nil, tt.Filter))
			assertQueryGolden(t, data.ToQuery(nil))
		})
	}
}

// TestSpecialsFilterQuery snapshots the compiled clauses of top-level specials selections: none,
// unknown, and hasProperty compile into claimType conditions on the property's rel records, while
// missing compiles into the universe rule (no rel, amount, or time record for the property; text-only
// claims do not block). A valued sibling filter on the same path OR's with the specials in one clause.
func TestSpecialsFilterQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name     string
		Specials *search.SpecialsFilter
	}{
		{
			Name:     "Missing",
			Specials: &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false},
		},
		{
			Name:     "None",
			Specials: &search.SpecialsFilter{Missing: false, None: true, Unknown: false, HasProperty: false},
		},
		{
			Name:     "Unknown",
			Specials: &search.SpecialsFilter{Missing: false, None: false, Unknown: true, HasProperty: false},
		},
		{
			Name:     "HasProperty",
			Specials: &search.SpecialsFilter{Missing: false, None: false, Unknown: false, HasProperty: true},
		},
		{
			Name:     "All",
			Specials: &search.SpecialsFilter{Missing: true, None: true, Unknown: true, HasProperty: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			data := filtersSession(makeTestSpecialsFilter([]identifier.Identifier{prop}, tt.Specials))
			assertQueryGolden(t, data.ToQuery(nil))
		})
	}

	t.Run("WithRefSibling", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
			makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
		)
		assertQueryGolden(t, data.ToQuery(nil))
	})
}

// TestSubSpecialsFilterQuery snapshots the compiled clauses of sub-path specials selections. Sub
// missing is universe-scoped: standalone it requires a parent claim for the parent property to
// exist while no qualifying parent claim carries a facetable sub-claim for the sub property, so a
// document without any parent claim does not match. Inside a multi-member group it takes the
// same-claim form instead (the shared parent claim lacks the sub property). Top-level selections
// on the parent property constrain which parent claims qualify.
func TestSubSpecialsFilterQuery(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	subProp := identifier.From("subProp")
	otherProp := identifier.From("otherProp")
	l1 := identifier.From("l1")
	a := identifier.From("a")

	missing := &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}

	t.Run("MissingStandalone", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(makeTestSpecialsFilter([]identifier.Identifier{parentProp, subProp}, missing))
		assertQueryGolden(t, data.ToQuery(nil))
	})

	t.Run("MissingWithParentConstraint", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil),
			makeTestSpecialsFilter([]identifier.Identifier{parentProp, subProp}, missing),
		)
		assertQueryGolden(t, data.ToQuery(nil))
	})

	t.Run("MissingInGroup", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubRefFilter(parentProp, otherProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
			makeTestSpecialsFilter([]identifier.Identifier{parentProp, subProp}, missing),
		)
		assertQueryGolden(t, data.ToQuery(nil))
	})

	t.Run("NoneStandalone", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(makeTestSpecialsFilter(
			[]identifier.Identifier{parentProp, subProp},
			&search.SpecialsFilter{Missing: false, None: true, Unknown: false, HasProperty: false},
		))
		assertQueryGolden(t, data.ToQuery(nil))
	})
}

// TestSessionToQueryInvalidFilterMatchesNone ensures the compiler maps a Filter with no payload
// set (a state Validate is supposed to catch) to a clause matching no documents instead of
// panicking.
func TestSessionToQueryInvalidFilterMatchesNone(t *testing.T) {
	t.Parallel()

	f := makeTestFilter(identifier.From("prop"), nil, nil, nil)
	data := filtersSession(f)
	j := testutils.QueryJSON(t, data.ToQuery(nil))
	assert.Contains(t, j, `"must_not":[{"match_all":{}}]`)
}

func TestSessionValidate(t *testing.T) {
	t.Parallel()

	t.Run("ValidSession", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{
				Sort:          nil,
				Language:      "",
				View:          search.ViewFeed,
				Query:         "test",
				Filters:       nil,
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("BaseTooShort", func(t *testing.T) {
		t.Parallel()
		s := &search.Session{
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
			ID:          identifier.From("short"),
			Base:        []string{"short"},
			Version:     0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "base must have at least two elements")
	})

	t.Run("InvalidSessionID", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		wrongID := identifier.New()
		s := &search.Session{
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
			ID:          wrongID,
			Base:        base,
			Version:     0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid session ID")
	})

	t.Run("DefaultView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, s.View)
	})

	t.Run("TableView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{
				Sort:          nil,
				Language:      "",
				View:          search.ViewTable,
				Query:         "test",
				Filters:       nil,
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.NoError(t, err)
		assert.Equal(t, search.ViewTable, s.View)
	})

	t.Run("InvalidView", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{Sort: nil, Language: "", View: "grid", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.Error(t, err)
		assert.EqualError(t, err, "invalid view")
	})

	t.Run("InvalidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		// Filter with invalid ref (neither to nor direct set).
		s := &search.Session{
			SessionData: search.SessionData{
				Sort:     nil,
				Language: "",
				View:     "", Query: "test",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{Direct: nil, To: nil}, nil, nil),
				},
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.Error(t, err)
		assert.EqualError(t, err, "to or direct has to be set")
	})

	t.Run("ValidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		value := identifier.From("value")
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{
				Sort:     nil,
				Language: "",
				View:     "", Query: "test",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
				},
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.NoError(t, err)
	})

	t.Run("NilFilters", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := &search.Session{
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
			ID:          identifier.From(base...),
			Base:        base,
			Version:     0,
		}
		err := s.Validate(siteContext(t.Context()))
		require.NoError(t, err)
	})
}

func TestSessionDataValidate(t *testing.T) {
	t.Parallel()

	t.Run("DefaultView", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil}
		err := data.Validate(siteContext(t.Context()), false)
		require.NoError(t, err)
		assert.Equal(t, search.ViewFeed, data.View)
	})

	t.Run("InvalidView", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{Sort: nil, Language: "", View: "grid", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil}
		err := data.Validate(siteContext(t.Context()), false)
		require.Error(t, err)
		assert.EqualError(t, err, "invalid view")
	})

	t.Run("ValidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		value := identifier.From("value")
		data := filtersSession(makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil))
		data.Query = "test"
		err := data.Validate(siteContext(t.Context()), false)
		require.NoError(t, err)
	})

	t.Run("InvalidFilters", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		data := filtersSession(makeTestFilter(prop, &search.RefFilter{Direct: nil, To: nil}, nil, nil))
		data.Query = "test"
		err := data.Validate(siteContext(t.Context()), false)
		require.Error(t, err)
		assert.EqualError(t, err, "to or direct has to be set")
	})

	// At most one specials filter may exist per property path within one filter set.
	t.Run("DuplicateSpecialsForPath", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		data := filtersSession(
			makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: true, Unknown: false, HasProperty: false}),
		)
		err := data.Validate(siteContext(t.Context()), false)
		require.Error(t, err)
		assert.EqualError(t, err, "duplicate specials filter for path")
	})

	// The per-path specials uniqueness is per filter set: Filters and Prefilters may each carry
	// their own specials selection for the same path.
	t.Run("SpecialsPerSet", func(t *testing.T) {
		t.Parallel()
		prop := identifier.From("prop")
		data := filtersSession(
			makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
		)
		data.Prefilters = []search.Filter{
			makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: false, None: true, Unknown: false, HasProperty: false}),
		}
		err := data.Validate(siteContext(t.Context()), false)
		require.NoError(t, err)
	})

	t.Run("ReverseExpandWithoutReverse", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: true, IDs: nil}
		err := data.Validate(siteContext(t.Context()), false)
		require.Error(t, err)
		assert.EqualError(t, err, "reverseExpand is set without reverse")
	})

	t.Run("ReverseExpandWithReverse", func(t *testing.T) {
		t.Parallel()
		reverse := identifier.From("target")
		data := search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: &reverse, ReverseExpand: true, IDs: nil}
		err := data.Validate(siteContext(t.Context()), false)
		require.NoError(t, err)
	})
}

func TestSessionToQuery(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	value := identifier.From("value")

	tests := []struct {
		Name        string
		SessionData search.SessionData
	}{
		{
			Name:        "QueryOnly",
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "hello", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		},
		{
			Name:        "Empty",
			SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		},
		{
			Name: "QueryAndFilter",
			SessionData: search.SessionData{
				Sort:     nil,
				Language: "",
				View:     "", Query: "hello",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
				},
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			q := tt.SessionData.ToQuery(nil)
			assertQueryGolden(t, q)
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
			Sort:     nil,
			Language: "",
			View:     "",
			Query:    "", Filters: nil,
			Prefilters:    nil,
			Reverse:       &reverseID,
			ReverseExpand: false,
			IDs:           nil,
		}
		q := data.ToQuery(nil)
		assertQueryGolden(t, q)
	})

	t.Run("ReverseAndFilter", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil))
		data.Reverse = &reverseID
		q := data.ToQuery(nil)
		j := testutils.QueryJSON(t, q)
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+reverseID.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.prop":{"value":"`+prop.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+value.String()+`"}`)
	})

	t.Run("ReverseInToQueryExcluding", func(t *testing.T) {
		t.Parallel()
		filter := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
		data := filtersSession(filter)
		data.Reverse = &reverseID
		q := data.ToQueryExcluding([]identifier.Identifier{*filter.ID}, nil)
		j := testutils.QueryJSON(t, q)
		// Reverse scope is applied even when filter is excluded.
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+reverseID.String()+`"}`)
		// Excluded filter's value is not in the query.
		assert.NotContains(t, j, `"claims.rel.to":{"value":"`+value.String()+`"}`)
	})

	t.Run("NoReverse", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{Sort: nil, Language: "", View: "", Query: "", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil}
		q := data.ToQuery(nil)
		assertQueryGolden(t, q)
	})
}

func TestSessionToQueryPrefilters(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prefilterProp")
	value := identifier.From("prefilterValue")

	t.Run("PrefilterGoesIntoFilterClause", func(t *testing.T) {
		t.Parallel()
		prefilter := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
		data := prefiltersSession(prefilter)
		assertQueryGolden(t, data.ToQuery(nil))
	})

	t.Run("FilterScoresButPrefilterDoesNot", func(t *testing.T) {
		t.Parallel()
		ref := &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}

		// As a regular filter the ref query goes into the scoring must clause.
		asFilter := filtersSession(makeTestFilter(prop, ref, nil, nil))
		jFilter := testutils.QueryJSON(t, asFilter.ToQuery(nil))
		assert.Contains(t, jFilter, `"must":[{"nested"`)
		assert.NotContains(t, jFilter, `"filter":`)

		// The same filter as a prefilter goes into the non-scoring filter clause instead.
		asPrefilter := prefiltersSession(makeTestFilter(prop, ref, nil, nil))
		jPrefilter := testutils.QueryJSON(t, asPrefilter.ToQuery(nil))
		assert.Contains(t, jPrefilter, `"filter":[{"nested"`)
		assert.NotContains(t, jPrefilter, `"must":[{"nested"`)
	})
}

// TestSessionToQueryNoCrossSetCorrelation verifies that Filters and Prefilters compile as
// independent sets: a top-level selection on a sub filter's parent property correlates with the
// sub filter only within the same set, never across sets. An unconstrained sub group compiles one
// nested arm per parent collection, so the presence of non-rel parent arms proves the other set's
// rel selection was not injected as a parent constraint.
func TestSessionToQueryNoCrossSetCorrelation(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	childProp := identifier.From("childProp")
	l1 := identifier.From("L1")
	a := identifier.From("A")

	makeSubRef := func() search.Filter {
		return makeTestSubRefFilter(parentProp, childProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}})
	}
	makeTopRef := func() search.Filter {
		return makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil)
	}

	t.Run("PrefilterDoesNotConstrainFilter", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(makeSubRef())
		data.Prefilters = []search.Filter{makeTopRef()}
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		// The prefilter's own clause is present.
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		// The sub filter's group stays unconstrained: parent collections beyond rel participate.
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
	})

	t.Run("FilterDoesNotConstrainPrefilter", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(makeTopRef())
		data.Prefilters = []search.Filter{makeSubRef()}
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
	})
}

func TestSessionDataValidateReverse(t *testing.T) {
	t.Parallel()

	reverseID := identifier.From("reverseTarget")

	t.Run("ReverseSet", func(t *testing.T) {
		t.Parallel()
		data := search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: &reverseID, ReverseExpand: false, IDs: nil}
		err := data.Validate(siteContext(t.Context()), false)
		require.NoError(t, err)
	})

	t.Run("ReverseRoundTrip", func(t *testing.T) {
		t.Parallel()
		base := []string{"test.example.com", "SEARCH", identifier.New().String()}
		s := search.Session{
			SessionData: search.SessionData{
				Sort:          nil,
				Language:      "",
				View:          search.ViewFeed,
				Query:         "",
				Filters:       nil,
				Prefilters:    nil,
				Reverse:       &reverseID,
				ReverseExpand: false,
				IDs:           nil,
			},
			ID:      identifier.From(base...),
			Base:    base,
			Version: 0,
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
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test search", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
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
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From("bad"),
		Base:        []string{"bad"},
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestUpdateSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// First create a session.
	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "original", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
	require.NoError(t, errE, "% -+#.1v", errE)

	id := s.ID

	// Update it.
	updated := &search.Session{
		SessionData: search.SessionData{
			Sort:          nil,
			Language:      "",
			View:          search.ViewTable,
			Query:         "updated",
			Filters:       nil,
			Prefilters:    nil,
			Reverse:       nil,
			ReverseExpand: false,
			IDs:           nil,
		},
		ID:      id,
		Base:    base,
		Version: 1,
	}
	errE = search.UpdateSession(siteContext(ctx), updated)
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
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		Version:     0,
	}
	errE := search.UpdateSession(siteContext(ctx), s)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestUpdateSessionValidationError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "original", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
	require.NoError(t, errE, "% -+#.1v", errE)
	id := s.ID

	updated := &search.Session{
		SessionData: search.SessionData{
			Sort:          nil,
			Language:      "",
			View:          "invalid",
			Query:         "updated",
			Filters:       nil,
			Prefilters:    nil,
			Reverse:       nil,
			ReverseExpand: false,
			IDs:           nil,
		},
		ID:      id,
		Base:    base,
		Version: 1,
	}
	errE = search.UpdateSession(siteContext(ctx), updated)
	require.Error(t, errE)
	assert.EqualError(t, errE, "validation failed")
}

func TestGetSession(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	base := []string{"test.example.com", "SEARCH", identifier.New().String()}
	s := &search.Session{
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
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
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "test", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          identifier.From(base...),
		Base:        base,
		Version:     0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
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
		SessionData: search.SessionData{
			Sort:          nil,
			Language:      "",
			View:          search.ViewFeed,
			Query:         "initial",
			Filters:       nil,
			Prefilters:    nil,
			Reverse:       nil,
			ReverseExpand: false,
			IDs:           nil,
		},
		ID:      identifier.From(base...),
		Base:    base,
		Version: 0,
	}
	errE := search.CreateSession(siteContext(ctx), s)
	require.NoError(t, errE, "% -+#.1v", errE)
	id := s.ID

	s2 := &search.Session{
		SessionData: search.SessionData{
			Sort:     nil,
			Language: "",
			View:     search.ViewTable, Query: "updated",
			Filters: []search.Filter{
				makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
			},
			Prefilters:    nil,
			Reverse:       nil,
			ReverseExpand: false,
			IDs:           nil,
		},
		ID:      id,
		Base:    base,
		Version: 1,
	}
	errE = search.UpdateSession(siteContext(ctx), s2)
	require.NoError(t, errE, "% -+#.1v", errE)

	s3 := &search.Session{
		SessionData: search.SessionData{Sort: nil, Language: "", View: "", Query: "updated again", Filters: nil, Prefilters: nil, Reverse: nil, ReverseExpand: false, IDs: nil},
		ID:          id,
		Base:        base,
		Version:     2,
	}
	errE = search.UpdateSession(siteContext(ctx), s3)
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

	f1 := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
	f2 := makeTestFilter(prop, &search.RefFilter{Direct: []search.ToValue{{ID: value}}, To: nil}, nil, nil)
	session := &search.Session{ //nolint:exhaustruct
		SessionData: filtersSession(f1, f2),
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

// TestSpecialsFor covers the session's per-path specials lookup: it returns the path's specials
// selection from Filters (never Prefilters) and skips excluded filter IDs.
func TestSpecialsFor(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	other := identifier.From("other")

	specials := &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}
	specialsFilter := makeTestSpecialsFilter([]identifier.Identifier{prop}, specials)
	data := filtersSession(specialsFilter)

	assert.Equal(t, specials, data.SpecialsFor([]identifier.Identifier{prop}))
	assert.Nil(t, data.SpecialsFor([]identifier.Identifier{other}))
	assert.Nil(t, data.SpecialsFor([]identifier.Identifier{prop}, *specialsFilter.ID))

	prefilterOnly := prefiltersSession(makeTestSpecialsFilter([]identifier.Identifier{other}, specials))
	assert.Nil(t, prefilterOnly.SpecialsFor([]identifier.Identifier{other}))
}

// TestFacetExcludeIDs covers the exclusion set for rendering a facet: the facet's own valued
// filter plus the path's specials filter, so the facet's own selections do not narrow its
// available values.
func TestFacetExcludeIDs(t *testing.T) {
	t.Parallel()

	prop := identifier.From("prop")
	other := identifier.From("other")
	value := identifier.From("value")

	valued := makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil)
	specials := makeTestSpecialsFilter([]identifier.Identifier{prop}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false})
	otherSpecials := makeTestSpecialsFilter([]identifier.Identifier{other}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false})
	data := filtersSession(valued, specials, otherSpecials)

	assert.Equal(t, []identifier.Identifier{*valued.ID, *specials.ID}, data.FacetExcludeIDs([]identifier.Identifier{prop}, valued.ID))
	assert.Equal(t, []identifier.Identifier{*specials.ID}, data.FacetExcludeIDs([]identifier.Identifier{prop}, nil))
	assert.Equal(t, []identifier.Identifier{*otherSpecials.ID}, data.FacetExcludeIDs([]identifier.Identifier{other}, nil))
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
		rfr := search.RefFilterResult{ID: "test-id", Count: 10, ChildCount: 5, Paths: nil}
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
		r := search.Result{Count: nil, Col: 0, Group: nil, ID: "doc-123"}
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
				Sort:     nil,
				Language: "",
				View:     search.ViewTable, Query: "test query",
				Filters: []search.Filter{
					makeTestFilter(prop, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: value}}}, nil, nil),
				},
				Prefilters:    nil,
				Reverse:       nil,
				ReverseExpand: false,
				IDs:           nil,
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
		ID:       &filterID,
		Base:     base,
		Prop:     []identifier.Identifier{parentProp, prop},
		Ref:      ref,
		Amount:   nil,
		Time:     nil,
		Has:      nil,
		Specials: nil,
	}
}

// makeTestSubAmountFilter and makeTestSubTimeFilter build valid two-prop sub-claim filters of the
// amount and time types with proper Base/ID.

func makeTestSubAmountFilter(parentProp, prop identifier.Identifier, amount *search.AmountFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:       &filterID,
		Base:     base,
		Prop:     []identifier.Identifier{parentProp, prop},
		Ref:      nil,
		Amount:   amount,
		Time:     nil,
		Has:      nil,
		Specials: nil,
	}
}

func makeTestSubTimeFilter(parentProp, prop identifier.Identifier, t *search.TimeFilter) search.Filter {
	base := []string{"test.example.com", "SEARCH", "testsession", "FILTER", identifier.New().String()}
	filterID := identifier.From(base...)
	return search.Filter{
		ID:       &filterID,
		Base:     base,
		Prop:     []identifier.Identifier{parentProp, prop},
		Ref:      nil,
		Amount:   nil,
		Time:     t,
		Has:      nil,
		Specials: nil,
	}
}

// TestSessionToQueryCorrelation verifies the correlation semantics of sub filters within one
// filter set: a sub filter compiles into one nested query per parent collection, and a top-level
// selection on the same parent property becomes a parent constraint inside that nested query, so
// all conditions must hold on the SAME parent claim. With rel constraints only the rel parent
// collection participates; without constraints every parent collection does.
func TestSessionToQueryCorrelation(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	otherProp := identifier.From("otherProp")
	subProp := identifier.From("subProp")
	l1 := identifier.From("l1")
	l2 := identifier.From("l2")
	a := identifier.From("a")
	x := identifier.From("x")

	t.Run("SubRefAlone_Unconstrained", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		// Each parent collection hosts one nested arm binding the parent claim's property.
		assert.Contains(t, j, `"claims.rel.prop":{"value":"`+parentProp.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.prop":{"value":"`+subProp.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.time.sub.rel.to":{"value":"`+a.String()+`"}`)
		// No parent constraint exists.
		assert.NotContains(t, j, `"claims.rel.to":{"value":`)
	})

	t.Run("WithSiblingParentRef_CorrelatedOnSameParentClaim", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil),
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		// The rel constraint excludes the other parent collections from the group.
		assert.NotContains(t, j, `"claims.amount.sub.rel.to"`)
		assert.NotContains(t, j, `"claims.time.sub.rel.to"`)
	})

	t.Run("WithSiblingParentRef_MultipleTo", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}, {ID: l2}}}, nil, nil),
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l2.String()+`"}`)
		assert.Contains(t, j, `"minimum_should_match":1`)
	})

	t.Run("WithSiblingParentDirect_LeafConstraint", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: []search.ToValue{{ID: l1}}, To: nil}, nil, nil),
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.isLeaf":{"value":true}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.NotContains(t, j, `"claims.amount.sub.rel.to"`)
	})

	t.Run("WithSiblingOnDifferentProp_Unconstrained", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(otherProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: x}}}, nil, nil),
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		// The selection on another property contributes no parent constraint, so all parent
		// collections still participate in the group.
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
	})

	t.Run("WithSiblingParentMissingSpecials_Unconstrained", func(t *testing.T) {
		t.Parallel()
		// A missing specials selection on the parent property contributes no parent constraint
		// arm (its own clause already excludes documents with any parent claim).
		data := filtersSession(
			makeTestSpecialsFilter([]identifier.Identifier{parentProp}, &search.SpecialsFilter{Missing: true, None: false, Unknown: false, HasProperty: false}),
			makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
	})

	t.Run("ToQueryExcludingParentRef_Unconstrained", func(t *testing.T) {
		t.Parallel()
		parentRef := makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil)
		subRef := makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}})
		data := filtersSession(parentRef, subRef)
		j := testutils.QueryJSON(t, data.ToQueryExcluding([]identifier.Identifier{*parentRef.ID}, nil))
		assert.NotContains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.to":{"value":"`+a.String()+`"}`)
		assert.Contains(t, j, `"claims.amount.sub.rel.to":{"value":"`+a.String()+`"}`)
	})

	t.Run("ToQueryExcludingSubRef_ParentStillPresent", func(t *testing.T) {
		t.Parallel()
		parentRef := makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil)
		subRef := makeTestSubRefFilter(parentProp, subProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: a}}})
		data := filtersSession(parentRef, subRef)
		j := testutils.QueryJSON(t, data.ToQueryExcluding([]identifier.Identifier{*subRef.ID}, nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.NotContains(t, j, `.sub.rel.to`)
	})
}

// TestSubAmountFilterQuery verifies the compiled shape of a sub amount filter: a nested query on
// claims.<parent>.sub.amount inside each participating parent collection's nested query, with the
// range window, the unit condition, and the sub property term all bound to the same sub record.
func TestSubAmountFilterQuery(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	prop := identifier.From("prop")
	unit := identifier.From("unit")
	l1 := identifier.From("l1")
	gte := 1.0
	lte := 10.0

	t.Run("GteLteUnit_Unconstrained", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubAmountFilter(parentProp, prop, &search.AmountFilter{Unit: &unit, Gte: &gte, Lte: &lte, Exists: false}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.prop":{"value":"`+parentProp.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.amount.prop":{"value":"`+prop.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.amount.unit":{"value":"`+unit.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.amount.range":{"gte":1,"lte":10}`)
		// Without parent constraints every parent collection participates.
		assert.Contains(t, j, `"claims.amount.sub.amount.range":{"gte":1,"lte":10}`)
	})

	t.Run("ExistsUnit", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubAmountFilter(parentProp, prop, &search.AmountFilter{Unit: &unit, Gte: nil, Lte: nil, Exists: true}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.sub.amount.prop":{"value":"`+prop.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.amount.unit":{"value":"`+unit.String()+`"}`)
		assert.NotContains(t, j, `"claims.rel.sub.amount.range"`)
	})

	t.Run("ExistsUnitless", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubAmountFilter(parentProp, prop, &search.AmountFilter{Unit: nil, Gte: nil, Lte: nil, Exists: true}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		// The unitless facet matches records with no unit field.
		assert.Contains(t, j, `"exists":{"field":"claims.rel.sub.amount.unit"}`)
		assert.Contains(t, j, `"must_not"`)
	})

	t.Run("WithParentConstraint", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil),
			makeTestSubAmountFilter(parentProp, prop, &search.AmountFilter{Unit: nil, Gte: &gte, Lte: &lte, Exists: false}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.amount.range":{"gte":1,"lte":10}`)
		assert.NotContains(t, j, `"claims.amount.sub.amount.range"`)
	})
}

// TestSubTimeFilterQuery verifies the compiled shape of a sub time filter, mirroring
// TestSubAmountFilterQuery for the time collection.
func TestSubTimeFilterQuery(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	prop := identifier.From("prop")
	l1 := identifier.From("l1")
	gte := float64(1000)
	lte := float64(2000)

	t.Run("GteLte_Unconstrained", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubTimeFilter(parentProp, prop, &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: false}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.sub.time.prop":{"value":"`+prop.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.time.range":{"gte":1000,"lte":2000}`)
		assert.Contains(t, j, `"claims.time.sub.time.range":{"gte":1000,"lte":2000}`)
	})

	t.Run("Exists", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestSubTimeFilter(parentProp, prop, &search.TimeFilter{Gte: nil, Lte: nil, Exists: true}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.sub.time.prop":{"value":"`+prop.String()+`"}`)
		assert.NotContains(t, j, `"claims.rel.sub.time.range"`)
	})

	t.Run("WithParentConstraint", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil),
			makeTestSubTimeFilter(parentProp, prop, &search.TimeFilter{Gte: &gte, Lte: &lte, Exists: false}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.time.range":{"gte":1000,"lte":2000}`)
		assert.NotContains(t, j, `"claims.time.sub.time.range"`)
	})
}

// TestSubHasFilterQuery verifies the compiled shape of a pooled sub-has filter: has sub-claims are
// rel records with the has claimType, so the filter compiles into a claimType condition plus a terms
// condition over the selected properties, nested under each participating parent collection.
func TestSubHasFilterQuery(t *testing.T) {
	t.Parallel()

	parentProp := identifier.From("parentProp")
	value := identifier.From("value")
	value2 := identifier.From("value2")
	l1 := identifier.From("l1")

	t.Run("SingleProp_Unconstrained", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestHasFilter([]identifier.Identifier{parentProp}, &search.HasFilter{Props: []search.HasValue{{ID: value}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.prop":{"value":"`+parentProp.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.claimType":{"value":"has"}`)
		assert.Contains(t, j, `"terms":{"claims.rel.sub.rel.prop":["`+value.String()+`"]}`)
		// Without parent constraints every parent collection participates.
		assert.Contains(t, j, `"claims.amount.sub.rel.claimType":{"value":"has"}`)
	})

	t.Run("MultipleProps", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestHasFilter([]identifier.Identifier{parentProp}, &search.HasFilter{Props: []search.HasValue{{ID: value}, {ID: value2}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"terms":{"claims.rel.sub.rel.prop":["`+value.String()+`","`+value2.String()+`"]}`)
	})

	t.Run("WithParentConstraint", func(t *testing.T) {
		t.Parallel()
		data := filtersSession(
			makeTestFilter(parentProp, &search.RefFilter{Direct: nil, To: []search.ToValue{{ID: l1}}}, nil, nil),
			makeTestHasFilter([]identifier.Identifier{parentProp}, &search.HasFilter{Props: []search.HasValue{{ID: value}}}),
		)
		j := testutils.QueryJSON(t, data.ToQuery(nil))
		assert.Contains(t, j, `"claims.rel.to":{"value":"`+l1.String()+`"}`)
		assert.Contains(t, j, `"claims.rel.sub.rel.claimType":{"value":"has"}`)
		assert.NotContains(t, j, `"claims.amount.sub.rel.claimType"`)
	})
}

func TestDistinctValuesTotal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		Name        string
		BucketCount int
		Cardinality int64
		Want        int64
	}{
		// Not truncated (fewer than MaxResultsCount buckets): the bucket count is exact and the
		// approximate cardinality is ignored, even when it over or under counts. This is the case
		// that previously made the frontend show "N values not shown" with everything visible.
		{Name: "ExactCardinalityMatches", BucketCount: 5, Cardinality: 5, Want: 5},
		{Name: "CardinalityOvercounts", BucketCount: 5, Cardinality: 7, Want: 5},
		{Name: "CardinalityUndercounts", BucketCount: 5, Cardinality: 3, Want: 5},
		{Name: "Empty", BucketCount: 0, Cardinality: 0, Want: 0},
		{Name: "JustBelowCap", BucketCount: search.MaxResultsCount - 1, Cardinality: search.MaxResultsCount + 100, Want: search.MaxResultsCount - 1},
		// Saturated (exactly MaxResultsCount buckets): the aggregation may have omitted values, so
		// the cardinality estimate is used to report how many exist beyond the cap, guarded by the
		// bucket count so it never reports fewer than what we already hold.
		{Name: "SaturatedCardinalityHigher", BucketCount: search.MaxResultsCount, Cardinality: search.MaxResultsCount + 250, Want: search.MaxResultsCount + 250},
		{Name: "SaturatedCardinalityLower", BucketCount: search.MaxResultsCount, Cardinality: search.MaxResultsCount - 10, Want: search.MaxResultsCount},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.Want, search.TestingDistinctValuesTotal(tt.BucketCount, tt.Cardinality))
		})
	}
}
