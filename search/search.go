package search

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	esSearch "github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v9/typedapi/esdsl"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/operator"
	"github.com/elastic/go-elasticsearch/v9/typedapi/types/enums/totalhitsrelation"
	"gitlab.com/tozd/go/errors"
	"gitlab.com/tozd/identifier"
	"gitlab.com/tozd/waf"

	internalSearch "gitlab.com/peerdb/peerdb/internal/search"
	internalStore "gitlab.com/peerdb/peerdb/internal/store"
)

const (
	// MaxResultsCount is the maximum number of search results that can be returned.
	MaxResultsCount = 1000

	// displayBoost is the boost factor for display and naming fields in text search queries.
	// The value is multiplied with the field's relevance score.
	displayBoost = "^0.2"
)

// ViewType represents the type of search view.
type ViewType string

// ViewType values.
const (
	ViewFeed  ViewType = "feed"
	ViewTable ViewType = "table"
)

// ToValue represents a target value in a reference filter.
type ToValue struct {
	ID identifier.Identifier `json:"id"`
}

// HasValue represents a selected property value in a has filter.
type HasValue struct {
	ID identifier.Identifier `json:"id"`
}

// RefFilter contains values for a reference filter.
type RefFilter struct {
	To      []ToValue `json:"to,omitempty"`
	Missing bool      `json:"missing,omitempty"`
}

// ToQuery converts the RefFilter to an ElasticSearch query for the given property.
func (f *RefFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	missingQuery := esdsl.NewBoolQuery().MustNot(
		esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
		).Path("claims.ref"),
	)

	// Missing only.
	if f.Missing && len(f.To) == 0 {
		return missingQuery
	}

	// Build value queries (OR across all To values).
	shoulds := make([]types.QueryVariant, 0, len(f.To)+1)
	for _, to := range f.To {
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.ref.prop", esdsl.NewFieldValue().String(prop.String())),
				esdsl.NewTermQuery("claims.ref.to", esdsl.NewFieldValue().String(to.ID.String())),
			),
		).Path("claims.ref"))
	}

	// Values + missing: OR them together.
	if f.Missing {
		shoulds = append(shoulds, missingQuery)
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// Validate validates the RefFilter.
func (f *RefFilter) Validate() errors.E {
	if len(f.To) == 0 && !f.Missing {
		return errors.New("to or missing has to be set")
	}
	return nil
}

// AmountFilter contains values for an amount filter.
type AmountFilter struct {
	Unit    *identifier.Identifier `json:"unit,omitempty"`
	Gte     *float64               `json:"gte,omitempty"`
	Lte     *float64               `json:"lte,omitempty"`
	Missing bool                   `json:"missing,omitempty"`
}

// ToQuery converts the AmountFilter to an ElasticSearch query for the given property.
func (f *AmountFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if f.Missing {
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
			).Path("claims.amount"),
		)
	}

	r := esdsl.NewNumberRangeQuery("claims.amount.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	must := []types.QueryVariant{
		esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(prop.String())),
		r,
	}
	if f.Unit != nil {
		must = append(must, esdsl.NewTermQuery("claims.amount.unit", esdsl.NewFieldValue().String(f.Unit.String())))
	}
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(must...),
	).Path("claims.amount")
}

// Validate validates the AmountFilter.
func (f *AmountFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Missing {
		return errors.New("both gte and lte or missing has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// TimeFilter contains values for a time filter.
//
// Gte and Lte are in seconds since Unix epoch.
type TimeFilter struct {
	Gte     *float64 `json:"gte,omitempty"`
	Lte     *float64 `json:"lte,omitempty"`
	Missing bool     `json:"missing,omitempty"`
}

// ToQuery converts the TimeFilter to an ElasticSearch query for the given property.
func (f *TimeFilter) ToQuery(prop identifier.Identifier) types.QueryVariant { //nolint:ireturn
	if f.Missing {
		return esdsl.NewBoolQuery().MustNot(
			esdsl.NewNestedQuery(
				esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String())),
			).Path("claims.time"),
		)
	}

	r := esdsl.NewNumberRangeQuery("claims.time.range").Gte(types.Float64(*f.Gte)).Lte(types.Float64(*f.Lte))
	return esdsl.NewNestedQuery(
		esdsl.NewBoolQuery().Must(
			esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(prop.String())),
			r,
		),
	).Path("claims.time")
}

// Validate validates the TimeFilter.
func (f *TimeFilter) Validate() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.Missing {
		return errors.New("both gte and lte or missing has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.Missing {
		return errors.New("gte/lte and missing cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// HasFilter contains values for a has filter.
//
// The has filter is a global filter where values are the distinct has claim properties.
// Unlike other filters, it does not filter within a specific property.
// Only has claims without ref sub-claims are considered (pure "has" claims).
type HasFilter struct {
	Props []HasValue `json:"props,omitempty"`
}

// ToQuery converts the HasFilter to an ElasticSearch query.
func (f *HasFilter) ToQuery() types.QueryVariant { //nolint:ireturn
	// Build value queries (OR across all selected props).
	// Only simple has claims (without sub-claims) are indexed in claims.has,
	// so we just match by claims.has.prop. Has claims with sub-claims are stored
	// in claims.sub instead and naturally excluded from this query.
	shoulds := make([]types.QueryVariant, 0, len(f.Props))
	for _, p := range f.Props {
		shoulds = append(shoulds, esdsl.NewNestedQuery(
			esdsl.NewTermQuery("claims.has.prop", esdsl.NewFieldValue().String(p.ID.String())),
		).Path("claims.has"))
	}

	if len(shoulds) == 1 {
		return shoulds[0]
	}
	return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
}

// Validate validates the HasFilter.
func (f *HasFilter) Validate() errors.E {
	if len(f.Props) == 0 {
		return errors.New("props has to be set")
	}
	return nil
}

// Filter represents a single active search filter.
//
// Exactly one of Ref, Amount, Time, or Has must be set.
type Filter struct {
	ID     *identifier.Identifier  `json:"id,omitempty"`
	Base   []string                `json:"base,omitempty"`
	Prop   []identifier.Identifier `json:"prop"`
	Ref    *RefFilter              `json:"ref,omitempty"`
	Amount *AmountFilter           `json:"amount,omitempty"`
	Time   *TimeFilter             `json:"time,omitempty"`
	Has    *HasFilter              `json:"has,omitempty"`
}

// Validate validates the Filter to ensure it has a valid configuration.
func (f Filter) Validate(withoutSession bool) errors.E {
	if !withoutSession {
		if len(f.Base) < 2 { //nolint:mnd
			errE := errors.New("base must have at least two elements")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}

		expectedID := identifier.From(f.Base...)
		if f.ID == nil || *f.ID != expectedID {
			errE := errors.New("invalid filter ID")
			errors.Details(errE)["got"] = f.ID.String()
			errors.Details(errE)["expected"] = expectedID.String()
			return errE
		}
	} else {
		if len(f.Base) > 0 {
			errE := errors.New("base must be empty")
			errors.Details(errE)["length"] = len(f.Base)
			return errE
		}
		if f.ID != nil {
			errE := errors.New("id must be empty")
			errors.Details(errE)["id"] = f.ID.String()
			return errE
		}
	}

	nonEmpty := 0
	if f.Ref != nil {
		nonEmpty++
	}
	if f.Amount != nil {
		nonEmpty++
	}
	if f.Time != nil {
		nonEmpty++
	}
	if f.Has != nil {
		nonEmpty++
	}
	if nonEmpty != 1 {
		return errors.New("exactly one of ref, amount, time, or has must be set")
	}

	// Has filter does not use Prop (it is a global filter).
	// Ref filter supports 1 prop (top-level) or 2 props (sub-ref: parentProp + prop).
	// Amount/Time filters use exactly 1 prop.
	switch {
	case f.Has != nil:
		if len(f.Prop) != 0 {
			errE := errors.New("prop must be empty for has filter")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	case f.Ref != nil:
		if len(f.Prop) != 1 && len(f.Prop) != 2 {
			errE := errors.New("prop must have one or two elements for ref filter")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	default:
		if len(f.Prop) != 1 {
			errE := errors.New("prop must have exactly one element")
			errors.Details(errE)["length"] = len(f.Prop)
			return errE
		}
	}

	if f.Ref != nil {
		return f.Ref.Validate()
	}
	if f.Amount != nil {
		return f.Amount.Validate()
	}
	if f.Time != nil {
		return f.Time.Validate()
	}
	return f.Has.Validate()
}

// ToQuery converts the Filter to an ElasticSearch query.
func (f Filter) ToQuery() types.QueryVariant { //nolint:ireturn
	if f.Has != nil {
		return f.Has.ToQuery()
	}
	if f.Ref != nil {
		if len(f.Prop) == 2 { //nolint:mnd
			return f.Ref.ToSubRefQuery(f.Prop[0], f.Prop[1])
		}
		return f.Ref.ToQuery(f.Prop[0])
	}
	prop := f.Prop[0]
	if f.Amount != nil {
		return f.Amount.ToQuery(prop)
	}
	if f.Time != nil {
		return f.Time.ToQuery(prop)
	}
	panic(errors.New("invalid filter"))
}

// GetFilterByID finds a filter by ID in the session's filters.
func (s *Session) GetFilterByID(id identifier.Identifier) (*Filter, errors.E) {
	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == id {
			return &s.Filters[i], nil
		}
	}
	return nil, errors.WithDetails(ErrNotFound, "filter", id)
}

// SessionData represents the data of the search session.
type SessionData struct {
	View    ViewType `json:"view,omitempty"`
	Query   string   `json:"query,omitempty"`
	Filters []Filter `json:"filters,omitempty"`
}

// Validate validates the session data .
func (s *SessionData) Validate(withoutSession bool) errors.E {
	seenFilters := map[identifier.Identifier]bool{}
	for i, f := range s.Filters {
		errE := f.Validate(withoutSession)
		if errE != nil {
			errors.Details(errE)["filter"] = i
			return errE
		}
		if !withoutSession {
			// We checked that f.ID is not nil in f.Validate().
			if seenFilters[*f.ID] {
				errE := errors.New("duplicate filter ID")
				errors.Details(errE)["id"] = f.ID.String()
				errors.Details(errE)["filter"] = i
				return errE
			}
			seenFilters[*f.ID] = true
		}
	}

	if !withoutSession {
		if s.View == "" {
			s.View = ViewFeed
		}
		if s.View != ViewFeed && s.View != ViewTable {
			errE := errors.New("invalid view")
			errors.Details(errE)["view"] = s.View
			return errE
		}
	}

	return nil
}

// ToQuery converts the Session to an ElasticSearch query.
//
// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *SessionData) ToQuery() types.QueryVariant { //nolint:ireturn
	musts := make([]types.QueryVariant, 0, len(s.Filters)+1)

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.Or))
	}

	for i := range s.Filters {
		musts = append(musts, s.Filters[i].ToQuery())
	}

	return esdsl.NewBoolQuery().Must(musts...)
}

// ToQueryExcluding converts the SessionData to an ElasticSearch query, excluding
// the filter with the given ID. This is used when fetching filter data so that
// the current filter's own restrictions do not affect its available values.
func (s *SessionData) ToQueryExcluding(excludeFilterID identifier.Identifier) types.QueryVariant { //nolint:ireturn
	musts := make([]types.QueryVariant, 0, len(s.Filters)+1)

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.Or))
	}

	for i := range s.Filters {
		if s.Filters[i].ID != nil && *s.Filters[i].ID == excludeFilterID {
			continue
		}
		musts = append(musts, s.Filters[i].ToQuery())
	}

	return esdsl.NewBoolQuery().Must(musts...)
}

// Session represents a search session.
//
// A search session includes WHAT is being searched for and HOW are
// results shown/visualized, but not WHERE the user is looking at.
type Session struct {
	SessionData

	ID      identifier.Identifier `json:"id"`
	Base    []string              `json:"base"`
	Version int                   `json:"version"`
}

// Validate validates the Session struct.
func (s *Session) Validate() errors.E {
	if len(s.Base) < 2 { //nolint:mnd
		errE := errors.New("base must have at least two elements")
		errors.Details(errE)["length"] = len(s.Base)
		return errE
	}

	expectedID := identifier.From(s.Base...)
	if s.ID != expectedID {
		errE := errors.New("invalid session ID")
		errors.Details(errE)["got"] = s.ID.String()
		errors.Details(errE)["expected"] = expectedID.String()
		return errE
	}

	return s.SessionData.Validate(false)
}

func documentTextSearchQuery(searchQuery string, defaultOperator operator.Operator) types.QueryVariant { //nolint:ireturn
	if searchQuery == "" {
		return esdsl.NewBoolQuery()
	}

	shoulds := []types.QueryVariant{
		esdsl.NewTermQuery("id", esdsl.NewFieldValue().String(searchQuery)),
	}
	for _, f := range []field{
		{"claims.id", "value"},
		{"claims.link", "iri"},
	} {
		// TODO: Can we use simple query for keyword fields? Which analyzer is used?
		q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(f.Prefix + "." + f.Field).DefaultOperator(defaultOperator)
		shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(f.Prefix))
	}
	// Search string and HTML claims across all supported languages.
	// Languages are sorted for deterministic query generation.
	langs := slices.Sorted(maps.Keys(internalSearch.SupportedLanguages))
	for _, f := range []field{
		{"claims.string", "string"},
		{"claims.html", "html"},
	} {
		for _, lang := range langs {
			q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(f.Prefix + "." + f.Field + "." + lang).DefaultOperator(defaultOperator)
			shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(f.Prefix))
		}
	}
	// Search display and naming fields across all claim types with reduced boost.
	for _, claimType := range []string{"amount", "has", "html", "id", "link", "none", "ref", "string", "time", "unknown"} {
		prefix := "claims." + claimType
		var fields []string
		// propDisplay and propNaming exist on all claim types.
		for _, fieldName := range []string{"propDisplay", "propNaming"} {
			for _, lang := range langs {
				fields = append(fields, prefix+"."+fieldName+"."+lang+displayBoost)
			}
		}
		// Type-specific display/naming fields.
		switch claimType {
		case "ref":
			for _, fieldName := range []string{"toDisplay", "toNaming"} {
				for _, lang := range langs {
					fields = append(fields, prefix+"."+fieldName+"."+lang+displayBoost)
				}
			}
		case "amount", "time":
			fields = append(fields, prefix+".fromDisplay"+displayBoost, prefix+".toDisplay"+displayBoost)
		}
		q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(fields...).DefaultOperator(defaultOperator)
		shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(prefix))
	}
	// Display and naming fields in denormalized sub-claims.
	{
		prefix := "claims.sub"
		var fields []string
		for _, fieldName := range []string{"propDisplay", "propNaming", "toDisplay", "toNaming"} {
			for _, lang := range langs {
				fields = append(fields, prefix+"."+fieldName+"."+lang+displayBoost)
			}
		}
		q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(fields...).DefaultOperator(defaultOperator)
		shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(prefix))
	}

	return esdsl.NewBoolQuery().Should(shoulds...)
}

// TODO: Use a database instead.
var searches = sync.Map{} //nolint:gochecknoglobals

// field describes a nested field for ElasticSearch to search on.
type field struct {
	Prefix string
	Field  string
}

// TODO: Return (and log) and error on invalid search requests (e.g., filters).

// CreateSession creates a new search session.
func CreateSession(_ context.Context, session *Session) errors.E {
	errE := session.Validate()
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(session.ID, session)

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit store twice?

	return nil
}

// UpdateSession updates an existing search session.
func UpdateSession(_ context.Context, session *Session) errors.E {
	errE := session.Validate()
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(session.ID, session)

	return nil
}

// GetSessionFromID resolves an existing search session if possible.
func GetSessionFromID(ctx context.Context, value string) (*Session, errors.E) {
	id, errE := identifier.MaybeString(value)
	if errE != nil {
		return nil, errors.WrapWith(errE, ErrNotFound)
	}

	return GetSession(ctx, id)
}

// GetSession resolves an existing search session if possible.
func GetSession(_ context.Context, id identifier.Identifier) (*Session, errors.E) {
	session, ok := searches.Load(id)
	if !ok {
		return nil, errors.WithDetails(ErrNotFound, "id", id)
	}
	// We make a shallow copy so that it is closer to how would a real database retrieve it.
	s := *session.(*Session) //nolint:forcetypeassert,errcheck
	return &s, nil
}

// Result represents a search result document.
type Result struct {
	ID string `json:"id"`
}

// ResultsGet retrieves search results for a given search session.
func ResultsGet(
	ctx context.Context, getSearchService func() (*esSearch.Search, int64, int64), searchData *SessionData,
) ([]Result, map[string]any, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchData.ToQuery()

	searchService, _, _ := getSearchService()

	searchService = searchService.From(0).Size(MaxResultsCount).Query(query)

	m := metrics.Duration(internalStore.MetricElasticSearch).Start()
	res, err := searchService.Do(ctx)
	m.Stop()
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	metrics.Duration(internalStore.MetricElasticSearchInternal).Duration = time.Duration(res.Took) * time.Millisecond

	results := make([]Result, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		results = append(results, Result{ID: *hit.Id_})
	}

	// Total is a string or a number.
	var total any
	if res.Hits.Total.Relation == totalhitsrelation.Gte {
		total = fmt.Sprintf("%d+", res.Hits.Total.Value)
	} else {
		total = res.Hits.Total.Value
	}

	return results, map[string]any{
		"total": total,
	}, nil
}
