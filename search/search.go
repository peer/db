package search

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/elastic/go-elasticsearch/v9/typedapi/core/search"
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
)

// ViewType represents the type of search view.
type ViewType string

// ViewType values.
const (
	ViewFeed  ViewType = "feed"
	ViewTable ViewType = "table"
)

// RelFilter represents a filter for relation claims.
type RelFilter struct {
	Prop  identifier.Identifier  `json:"prop"`
	Value *identifier.Identifier `json:"value,omitempty"`
	None  bool                   `json:"none,omitempty"`
}

// Valid validates the RelFilter to ensure it has a valid configuration.
func (f RelFilter) Valid() errors.E {
	if f.Value == nil && !f.None {
		return errors.New("value or none has to be set")
	}
	if f.Value != nil && f.None {
		return errors.New("value and none cannot be both set")
	}
	return nil
}

// AmountFilter represents a filter for amount claims.
type AmountFilter struct {
	Prop identifier.Identifier  `json:"prop"`
	Unit *identifier.Identifier `json:"unit,omitempty"`
	Gte  *float64               `json:"gte,omitempty"`
	Lte  *float64               `json:"lte,omitempty"`
	None bool                   `json:"none,omitempty"`
}

// Valid validates the AmountFilter to ensure it has a valid configuration.
// Both gte and lte must be set together, or none must be true.
func (f AmountFilter) Valid() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("both gte and lte or none has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.None {
		return errors.New("gte/lte and none cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// TimeFilter represents a filter for time claims.
type TimeFilter struct {
	Prop identifier.Identifier `json:"prop"`
	Gte  *int64                `json:"gte,omitempty"`
	Lte  *int64                `json:"lte,omitempty"`
	None bool                  `json:"none,omitempty"`
}

// Valid validates the TimeFilter to ensure it has a valid configuration.
// Both gte and lte must be set together, or none must be true.
func (f TimeFilter) Valid() errors.E {
	if f.Gte == nil && f.Lte == nil && !f.None {
		return errors.New("both gte and lte or none has to be set")
	}
	if (f.Gte != nil || f.Lte != nil) && f.None {
		return errors.New("gte/lte and none cannot be both set")
	}
	if (f.Gte == nil) != (f.Lte == nil) {
		return errors.New("both gte and lte must be set together")
	}
	return nil
}

// Filters represents a collection of search filters.
type Filters struct {
	And    []Filters     `json:"and,omitempty"`
	Or     []Filters     `json:"or,omitempty"`
	Not    *Filters      `json:"not,omitempty"`
	Rel    *RelFilter    `json:"rel,omitempty"`
	Amount *AmountFilter `json:"amount,omitempty"`
	Time   *TimeFilter   `json:"time,omitempty"`
}

// Valid validates the Filters to ensure it has a valid configuration.
func (f Filters) Valid() errors.E {
	nonEmpty := 0
	if len(f.And) > 0 {
		nonEmpty++
		for _, c := range f.And {
			err := c.Valid()
			if err != nil {
				return err
			}
		}
	}
	if len(f.Or) > 0 {
		nonEmpty++
		for _, c := range f.Or {
			err := c.Valid()
			if err != nil {
				return err
			}
		}
	}
	if f.Not != nil {
		nonEmpty++
		err := f.Not.Valid()
		if err != nil {
			return err
		}
	}
	if f.Rel != nil {
		nonEmpty++
		err := f.Rel.Valid()
		if err != nil {
			return err
		}
	}
	if f.Amount != nil {
		nonEmpty++
		err := f.Amount.Valid()
		if err != nil {
			return err
		}
	}
	if f.Time != nil {
		nonEmpty++
		err := f.Time.Valid()
		if err != nil {
			return err
		}
	}
	if nonEmpty > 1 {
		return errors.New("only one clause can be set")
	} else if nonEmpty == 0 {
		return errors.New("no clause is set")
	}
	return nil
}

// ToQuery converts the Filters to an ElasticSearch query.
func (f Filters) ToQuery() types.QueryVariant { //nolint:ireturn
	if len(f.And) > 0 {
		musts := make([]types.QueryVariant, 0, len(f.And))
		for _, filter := range f.And {
			musts = append(musts, filter.ToQuery())
		}
		return esdsl.NewBoolQuery().Must(musts...)
	}
	if len(f.Or) > 0 {
		shoulds := make([]types.QueryVariant, 0, len(f.Or))
		for _, filter := range f.Or {
			shoulds = append(shoulds, filter.ToQuery())
		}
		return esdsl.NewBoolQuery().Should(shoulds...).MinimumShouldMatch(esdsl.NewMinimumShouldMatch().Int(1))
	}
	if f.Not != nil {
		boolQuery := esdsl.NewBoolQuery()
		boolQuery.MustNot(f.Not.ToQuery())
		return boolQuery
	}
	if f.Rel != nil {
		if f.Rel.None {
			return esdsl.NewBoolQuery().MustNot(
				esdsl.NewNestedQuery(
					esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(f.Rel.Prop.String())),
				).Path("claims.rel"),
			)
		}
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.rel.prop", esdsl.NewFieldValue().String(f.Rel.Prop.String())),
				esdsl.NewTermQuery("claims.rel.to", esdsl.NewFieldValue().String(f.Rel.Value.String())),
			),
		).Path("claims.rel")
	}
	if f.Amount != nil {
		if f.Amount.None {
			return esdsl.NewBoolQuery().MustNot(
				esdsl.NewNestedQuery(
					esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(f.Amount.Prop.String())),
				).Path("claims.amount"),
			)
		}
		r := esdsl.NewNumberRangeQuery("claims.amount.range").Gte(types.Float64(*f.Amount.Gte)).Lte(types.Float64(*f.Amount.Lte))
		must := []types.QueryVariant{
			esdsl.NewTermQuery("claims.amount.prop", esdsl.NewFieldValue().String(f.Amount.Prop.String())),
			r,
		}
		if f.Amount.Unit != nil {
			must = append(must, esdsl.NewTermQuery("claims.amount.unit", esdsl.NewFieldValue().String(f.Amount.Unit.String())))
		}
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(must...),
		).Path("claims.amount")
	}
	if f.Time != nil {
		if f.Time.None {
			return esdsl.NewBoolQuery().MustNot(
				esdsl.NewNestedQuery(
					esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(f.Time.Prop.String())),
				).Path("claims.time"),
			)
		}
		r := esdsl.NewNumberRangeQuery("claims.time.range").Gte(types.Float64(*f.Time.Gte)).Lte(types.Float64(*f.Time.Lte))
		return esdsl.NewNestedQuery(
			esdsl.NewBoolQuery().Must(
				esdsl.NewTermQuery("claims.time.prop", esdsl.NewFieldValue().String(f.Time.Prop.String())),
				r,
			),
		).Path("claims.time")
	}
	panic(errors.New("invalid filters"))
}

// Session represents a search session.
//
// A search session includes WHAT is being searched for and HOW are
// results shown/visualized, but not WHERE the user is looking at.
type Session struct {
	ID      *identifier.Identifier `json:"id"`
	Version int                    `json:"version"`
	View    ViewType               `json:"view"`
	Query   string                 `json:"query"`
	Filters *Filters               `json:"filters,omitempty"`
}

// Validate validates the Session struct.
func (s *Session) Validate(_ context.Context, existing *Session) errors.E {
	if existing == nil {
		if s.ID != nil {
			errE := errors.New("ID provided for new document")
			errors.Details(errE)["id"] = *s.ID
			return errE
		}
		// TODO: Compute ID using identifier.From and store what was used to compute it.
		id := identifier.New()
		s.ID = &id
	} else if s.ID == nil {
		// This should not really happen because we fetch existing based on i.ID.
		return errors.New("ID missing for existing document")
	} else if existing.ID == nil {
		// This should not really happen because we always store documents with ID.
		return errors.New("ID missing for existing document")
	} else if *s.ID != *existing.ID {
		// This should not really happen because we fetch existing based on i.ID.
		errE := errors.New("payload ID does not match existing ID")
		errors.Details(errE)["payload"] = *s.ID
		errors.Details(errE)["existing"] = *existing.ID
		return errE
	}

	if existing == nil {
		// We set the version to zero for new sessions.
		s.Version = 0
	} else {
		// We increase the version by one.
		// TODO: This is not race safe, needs improvement once we have storage that supports transactions.
		s.Version = existing.Version + 1
	}

	if s.Filters != nil {
		errE := s.Filters.Valid()
		if errE != nil {
			return errE
		}
	}

	if s.View == "" {
		s.View = ViewFeed
	}
	if s.View != ViewFeed && s.View != ViewTable {
		errE := errors.New("invalid view")
		errors.Details(errE)["view"] = s.View
		return errE
	}

	return nil
}

// SessionRef represents a reference to a search session.
type SessionRef struct {
	ID      identifier.Identifier `json:"id"`
	Version int                   `json:"version"`
}

// Ref returns a SessionRef reference to this Session.
func (s *Session) Ref() SessionRef {
	return SessionRef{ID: *s.ID, Version: s.Version}
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
		{"claims.ref", "iri"},
	} {
		// TODO: Can we use simple query for keyword fields? Which analyzer is used?
		q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(f.Prefix + "." + f.Field).DefaultOperator(defaultOperator)
		shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(f.Prefix))
	}
	// Search string and HTML claims across all supported languages.
	// Languages are sorted for deterministic query generation.
	for _, f := range []field{
		{"claims.string", "string"},
		{"claims.html", "html"},
	} {
		for _, lang := range slices.Sorted(maps.Keys(internalSearch.SupportedLanguages)) {
			q := esdsl.NewSimpleQueryStringQuery(searchQuery).Fields(f.Prefix + "." + f.Field + "." + lang).DefaultOperator(defaultOperator)
			shoulds = append(shoulds, esdsl.NewNestedQuery(q).Path(f.Prefix))
		}
	}

	return esdsl.NewBoolQuery().Should(shoulds...)
}

// ToQuery converts the Session to an ElasticSearch query.
//
// TODO: Determine which operator should be the default?
// TODO: Make sure right analyzers are used for all fields.
// TODO: Limit allowed syntax for simple queries (disable fuzzy matching).
func (s *Session) ToQuery() types.QueryVariant { //nolint:ireturn
	var musts []types.QueryVariant

	if s.Query != "" {
		musts = append(musts, documentTextSearchQuery(s.Query, operator.Or))
	}

	if s.Filters != nil {
		musts = append(musts, s.Filters.ToQuery())
	}

	return esdsl.NewBoolQuery().Must(musts...)
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
func CreateSession(ctx context.Context, session *Session) errors.E {
	errE := session.Validate(ctx, nil)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(*session.ID, session)

	// TODO: Should we already do the query, to warm up ES cache?
	//       Maybe we should cache response ourselves so that we do not hit store twice?

	return nil
}

// UpdateSession updates an existing search session.
func UpdateSession(ctx context.Context, session *Session) errors.E {
	if session.ID == nil {
		return errors.WithMessage(ErrValidationFailed, "ID is missing")
	}

	// TODO: This is not race safe, needs improvement once we have storage that supports transactions.
	existingSession, errE := GetSession(ctx, *session.ID)
	if errE != nil {
		return errE
	}

	errE = session.Validate(ctx, existingSession)
	if errE != nil {
		return errors.WrapWith(errE, ErrValidationFailed)
	}

	searches.Store(*session.ID, session)

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
	return session.(*Session), nil //nolint:forcetypeassert,errcheck
}

// Result represents a search result document.
type Result struct {
	ID string `json:"id"`
}

// ResultsGet retrieves search results for a given search session.
func ResultsGet(
	ctx context.Context, getSearchService func() (*search.Search, int64, int64), searchSession *Session,
) ([]Result, map[string]interface{}, errors.E) {
	metrics, _ := waf.GetMetrics(ctx)

	query := searchSession.ToQuery()

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
	var total interface{}
	if res.Hits.Total.Relation == totalhitsrelation.Gte {
		total = fmt.Sprintf("%d+", res.Hits.Total.Value)
	} else {
		total = res.Hits.Total.Value
	}

	return results, map[string]interface{}{
		"total": total,
	}, nil
}
